# Epic 29: Agent Hardening — kubectl Write Blocking, Hardened Mode, and Redaction Improvements

## Purpose

The mendabot agent is architecturally read-only: it investigates cluster state and proposes
fixes via GitOps pull requests. It never applies changes directly to the cluster. However,
this contract is currently enforced only at the RBAC layer and via prompt instructions —
neither of which the agent can override, but neither of which is visible at the tool-call
layer where the LLM actually operates.

This epic hardens the agent at three independent layers:

1. **kubectl write-blocking (always-on):** The `kubectl` wrapper is extended to detect and
   block all write subcommands (`apply`, `create`, `delete`, `edit`, `patch`, `replace`,
   `scale`, `drain`, etc.) before the real binary is ever invoked. This enforces the design
   contract at the tool layer, making it impossible for an adversarial or confused LLM to
   mutate cluster state via `kubectl` regardless of RBAC.

2. **kubectl hardened mode (opt-in via `agent.hardenKubectl`):** A new Helm flag adds a
   second tier of blocking — `kubectl get/describe secret(s)`, `kubectl get all`, `kubectl
   exec`, and `kubectl port-forward` — for operators who want to eliminate any path by
   which the agent could read or exfiltrate Kubernetes Secret values or expose internal
   services. The hardened flag is enforced via an immutable sentinel file (read-only volume
   mount, `chmod 444`), mirroring the three-layer dry-run enforcement introduced in epic20.

3. **Redaction improvements:** `internal/domain/redact.go` gains five new built-in patterns
   covering credential formats not currently caught (`age` private keys, OpenAI/Anthropic
   `sk-*` keys, AWS access key IDs, JWTs, non-Bearer Authorization headers), a new `Redactor`
   struct that supports user-defined custom patterns at runtime, and propagation of custom
   patterns through to the `redact` binary inside agent Jobs via `EXTRA_REDACT_PATTERNS`.

## Status: Complete

## Dependencies

- epic25-tool-output-redaction complete (`docker/scripts/redact-wrappers/kubectl` exists;
  `internal/domain/redact.go` `RedactSecrets` function established)
- epic20-dry-run-mode complete (three-layer sentinel pattern; `mechanic-cfg` emptyDir volume;
  `dry-run-gate` init container — all extended by this epic)
- epic12-security-review complete (`docs/SECURITY/THREAT_MODEL.md` established — updated
  by STORY_05)

## Blocks

Nothing downstream depends on this epic.

## Success Criteria

- [ ] `kubectl` wrapper blocks all write subcommands unconditionally; blocked calls exit 1
      with a clear `[KUBECTL]` prefixed error message to stderr
- [ ] `kubectl` wrapper hardened mode blocks `get/describe secret(s)`, `get all`, `exec`,
      `port-forward` when the `harden-kubectl` sentinel is present
- [ ] Hardened mode sentinel (`/mechanic-cfg/harden-kubectl`) is `chmod 444` and mounted
      read-only — cannot be unset from within the agent container
- [ ] Three-layer detection (sentinel file → `/proc/1/environ` → env var) matches the
      pattern established in epic20
- [ ] `agent.hardenKubectl: false` is the default; existing deployments are unaffected
- [ ] `internal/domain/redact.go` adds five new built-in patterns: `age` private key,
      `sk-*` (OpenAI/Anthropic), AWS `AKIA*`, JWT two-segment, non-Bearer Authorization header
- [ ] `domain.New(extraPatterns []string)` returns a `*Redactor` that applies built-in
      patterns plus any user-supplied extras
- [ ] `cmd/redact/main.go` reads `EXTRA_REDACT_PATTERNS` (comma-separated) and initialises
      a `Redactor` with extras
- [ ] `agent.extraRedactPatterns` Helm value propagates through watcher env var →
      `EXTRA_REDACT_PATTERNS` in agent Job env → `redact` binary at runtime
- [ ] All new fields appear under `agent:` in `values.yaml` (not `watcher:`)
- [ ] `agent.kyvernoPolicy.enabled: false` is the default; no Kyverno resource is emitted
      unless explicitly opted into
- [ ] When `agent.kyvernoPolicy.enabled: true`, a valid Kyverno `ClusterPolicy` is emitted
      containing all seven hardening rules (secret read denial, exec/portforward denial,
      write denial, image allowlist, pod security profile, curl audit)
- [ ] `RemediationJob` and `RemediationJob/status` are excluded from the write-deny rule
      so the agent can still patch its own job status
- [ ] Agent Job containers have `readOnlyRootFilesystem: true`, `runAsNonRoot: true`,
      `allowPrivilegeEscalation: false`, and `capabilities.drop: ["ALL"]` set in the
      jobbuilder-produced Job spec
- [ ] A dedicated `/tmp` emptyDir volume is added to agent Jobs so the kubectl wrapper's
      `mktemp` continues to work with a read-only root filesystem
- [ ] `agent.kyvernoPolicy.allowedImagePrefix` defaults to `"ghcr.io/lenaxia/mechanic-agent"`;
      when empty, the image allowlist rule is skipped
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] Worklog entry created

## Design

### Threat model context

The agent's RBAC (`clusterrole-agent.yaml`) grants only `get/list/watch` on an explicit
resource allowlist — `secrets` is not included. In `agentRBACScope: cluster` (the default),
RBAC is the authoritative server-side control. The kubectl wrapper write-blocking is
defense-in-depth at the tool layer.

In `agentRBACScope: namespace`, the agent SA gains `resources: ["*"]` `get/list/watch`
in watched namespaces (including Secrets). The hardened mode flag is the primary
mitigation for operators using namespace scope who want to prevent Secret exfiltration.

`curl` with the mounted SA bearer token can bypass the kubectl wrapper entirely — the API
server's RBAC enforcement is the control for that path. The hardened mode does not attempt
to block `curl` (too fragile; IP-address bypass). This residual risk is documented in
`THREAT_MODEL.md` AV-03 / AR-07.

### kubectl wrapper two-tier architecture

```
kubectl <subcommand> [args...]
  │
  ├── [redact availability check]         ← fail-safe: abort if redact missing
  │
  ├── [Tier 1: always-on write block]     ← applies regardless of harden flag
  │     apply, create, delete, edit,
  │     patch, replace, rollout restart,
  │     rollout undo, scale, set,
  │     label, annotate, taint,
  │     drain, cordon, uncordon
  │     → exit 1, "[KUBECTL] ... blocked"
  │
  ├── [harden-kubectl sentinel detection] ← 3-layer: file → /proc/1/environ → env
  │
  ├── [Tier 2: hardened-mode block]       ← only when _harden_kubectl=true
  │     get secret(s)[/name]
  │     get all
  │     describe secret(s)[/name]
  │     exec
  │     port-forward
  │     → exit 1, "[KUBECTL-HARDENED] ... blocked"
  │
  ├── kubectl.real "$@" > tmpfile 2>&1    ← real binary runs only if not blocked
  │
  └── redact < tmpfile                    ← output redaction (existing, unchanged)
```

### Redactor struct design

`internal/domain/redact.go` is refactored from a package-level function to a `Redactor`
struct. The existing `RedactSecrets(text string) string` function is preserved as a
package-level shim backed by a default zero-extras `Redactor`, so all existing call sites
in `internal/provider/native/*.go` require no changes.

```go
// Redactor holds compiled built-in + custom patterns.
type Redactor struct {
    patterns []redactPattern
}

// New returns a Redactor with the built-in patterns plus any extras.
// extraPatterns are validated as legal Go regexes at construction time;
// invalid patterns return an error.
func New(extraPatterns []string) (*Redactor, error)

// Redact applies all patterns to text and returns the sanitised result.
func (r *Redactor) Redact(text string) string

// RedactSecrets is a package-level convenience wrapper around a default Redactor
// (no extra patterns). All existing call sites continue to work unchanged.
func RedactSecrets(text string) string
```

### Custom pattern propagation

```
values.yaml
  agent.extraRedactPatterns: ["CORP-[0-9]{8}", "INT-[A-Z]+-[0-9]+"]
        │
        ▼
deployment-watcher.yaml
  env: EXTRA_REDACT_PATTERNS=CORP-[0-9]{8},INT-[A-Z]+-[0-9]+
        │
        ▼ (watcher reads at startup)
config.go ExtraRedactPatterns []string
        │
        ▼ (injected into agent Job env by jobbuilder)
  agent Job container env:
  EXTRA_REDACT_PATTERNS=CORP-[0-9]{8},INT-[A-Z]+-[0-9]+
        │
        ▼ (read at startup by redact binary)
cmd/redact/main.go  →  domain.New(extras)  →  r.Redact(stdin)
```

Custom patterns are also applied by the watcher's own redaction of `Finding.Errors` and
`Finding.Details` — the watcher initialises a `domain.Redactor` at startup from
`cfg.ExtraRedactPatterns` and uses it in the provider layer.

### Sentinel immutability (hardened mode)

The `dry-run-gate` init container (epic20) is extended to also write the `harden-kubectl`
sentinel when `cfg.HardenAgentKubectl` is true. The `mechanic-cfg` emptyDir volume is
created whenever either `DryRun` or `HardenAgentKubectl` is set. The init container
command is constructed dynamically:

```sh
# DryRun only:
echo -n 'true' > /mechanic-cfg/dry-run && chmod 444 /mechanic-cfg/dry-run

# HardenAgentKubectl only:
echo -n 'true' > /mechanic-cfg/harden-kubectl && chmod 444 /mechanic-cfg/harden-kubectl

# Both:
echo -n 'true' > /mechanic-cfg/dry-run && chmod 444 /mechanic-cfg/dry-run && \
echo -n 'true' > /mechanic-cfg/harden-kubectl && chmod 444 /mechanic-cfg/harden-kubectl
```

The main container mounts `mechanic-cfg` as `ReadOnly: true` — the agent process cannot
modify or remove the sentinel file after the init container has written it.

## Configuration

```bash
# --- agent.hardenKubectl (values.yaml) ---
# When true: blocks kubectl get/describe secret(s), get all, exec, port-forward
# in all agent Jobs. Enforced via read-only sentinel file — cannot be bypassed
# from within the agent container. Default: false.
HARDEN_AGENT_KUBECTL=true   # env var injected into watcher by Helm when flag is set

# --- agent.extraRedactPatterns (values.yaml) ---
# Comma-separated list of Go regex patterns applied by both the watcher's finding
# redaction and the agent redact binary. Invalid patterns cause watcher startup failure.
# Default: "" (empty — no extra patterns).
EXTRA_REDACT_PATTERNS=CORP-[0-9]{8},INT-[A-Z]+-[0-9]+
```

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| kubectl wrapper — Tier 1 write blocking (always-on) | [STORY_00_kubectl_write_blocking.md](STORY_00_kubectl_write_blocking.md) | Not Started | Critical | 2h |
| kubectl wrapper — Tier 2 hardened mode + sentinel + Helm flag | [STORY_01_kubectl_hardened_mode.md](STORY_01_kubectl_hardened_mode.md) | Not Started | High | 3h |
| redact.go — five new built-in patterns | [STORY_02_redact_patterns.md](STORY_02_redact_patterns.md) | Not Started | High | 2h |
| Redactor struct + custom pattern support + redact binary propagation | [STORY_03_redactor_custom_patterns.md](STORY_03_redactor_custom_patterns.md) | Not Started | High | 3h |
| Config, jobbuilder, and deploy wiring | [STORY_04_config_and_deploy.md](STORY_04_config_and_deploy.md) | Not Started | Critical | 2h |
| Threat model update | [STORY_05_threat_model_update.md](STORY_05_threat_model_update.md) | Not Started | Medium | 1h |
| Kyverno policy — pod security + access restriction + image allowlist (opt-in) | [STORY_06_kyverno_policy.md](STORY_06_kyverno_policy.md) | Not Started | Medium | 3h |

## Story execution order

STORY_00 can be built and tested independently — it touches only the kubectl wrapper
shell script. STORY_02 is similarly independent (pure Go, no other changes needed).
STORY_04 must precede STORY_01 (the `HardenAgentKubectl` config field and jobbuilder
changes are prerequisites for the Helm wiring). STORY_03 depends on STORY_02 (extends
the redact.go changes). STORY_05 closes the epic.

```
STORY_00 (kubectl write blocking)
STORY_02 (new redact patterns)
    └──> STORY_03 (Redactor struct + custom patterns)
STORY_04 (config + jobbuilder + deploy wiring)
    └──> STORY_01 (kubectl hardened mode)
                └──> STORY_05 (threat model update)
                         └──> STORY_06 (Kyverno policy — independent; references threat model)
```

STORY_06 is also independently executable after STORY_04 (the jobbuilder security
context changes in STORY_06 are parallel to, not dependent on, STORY_01). It is placed
after STORY_05 in the diagram because STORY_05 establishes the AV-03 / AR-07
documentation that STORY_06 extends.

## Technical Overview

### New files

| File | Purpose |
|------|---------|
| `charts/mechanic/templates/kyverno-policy-agent.yaml` | Optional Kyverno ClusterPolicy with seven rules: secret read denial, exec/portforward denial, write denial, image allowlist, pod security profile (readOnlyRootFilesystem, runAsNonRoot, no privilege escalation, drop ALL caps), and a curl-bypass audit rule |

### Modified files

| File | Change |
|------|--------|
| `docker/scripts/redact-wrappers/kubectl` | Add Tier 1 write-block + Tier 2 hardened-mode block + sentinel detection |
| `internal/domain/redact.go` | Add `Redactor` struct + five new built-in patterns |
| `internal/domain/redact_test.go` | Test cases for new patterns + custom pattern support |
| `cmd/redact/main.go` | Read `EXTRA_REDACT_PATTERNS`; use `domain.New(extras)` instead of `RedactSecrets` |
| `cmd/redact/main_test.go` | Test custom pattern propagation via env var |
| `internal/config/config.go` | Add `HardenAgentKubectl bool` and `ExtraRedactPatterns []string` |
| `internal/config/config_test.go` | Config parsing tests for new fields |
| `internal/jobbuilder/job.go` | Extend dry-run-gate init container; inject `HARDEN_KUBECTL` and `EXTRA_REDACT_PATTERNS` env vars; add pod `SecurityContext` (readOnlyRootFilesystem, runAsNonRoot, allowPrivilegeEscalation=false, drop ALL); add `/tmp` emptyDir volume |
| `internal/jobbuilder/job_test.go` | Test Job spec for new sentinel, env vars, SecurityContext fields, and `/tmp` volume mount |
| `charts/mendabot/values.yaml` | Add `agent.hardenKubectl`, `agent.extraRedactPatterns`, `agent.kyvernoPolicy.enabled`, and `agent.kyvernoPolicy.allowedImagePrefix` |
| `charts/mendabot/templates/deployment-watcher.yaml` | Add `HARDEN_AGENT_KUBECTL` and `EXTRA_REDACT_PATTERNS` env var injection |
| `docs/SECURITY/THREAT_MODEL.md` | Update AV-03 mitigations; update AR-01; document new kubectl Tier 1/Tier 2 controls |

## Definition of Done

- [ ] All unit tests pass: `go test -timeout 30s -race ./...`
- [ ] `go build ./...` succeeds
- [ ] `shellcheck docker/scripts/redact-wrappers/kubectl` passes with no errors
- [ ] `kubectl apply` inside a running agent container exits 1 with `[KUBECTL]` message
      (manual verification or wrapper-test.sh extension)
- [ ] `kubectl get secret` inside a hardened agent container exits 1 with
      `[KUBECTL-HARDENED]` message
- [ ] `kubectl get pods` inside a hardened agent container succeeds (not over-blocked)
- [ ] `EXTRA_REDACT_PATTERNS=sk-[A-Za-z0-9]{20,}` causes `sk-abc123...` to be redacted
      in both watcher findings and agent tool output
- [ ] `agent.hardenKubectl: false` (default) — existing deployments unaffected
- [ ] `agent.kyvernoPolicy.enabled: false` (default) — no Kyverno resource emitted
- [ ] `helm template` with `agent.kyvernoPolicy.enabled: true` emits a valid
      `ClusterPolicy` with all seven rules and no Helm rendering errors
- [ ] `helm template` with `agent.kyvernoPolicy.allowedImagePrefix: ""` emits a policy
      with no `restrict-agent-image` rule
- [ ] `helm lint charts/mechanic/` passes with no errors
- [ ] Agent Job spec has `readOnlyRootFilesystem: true`, `runAsNonRoot: true`,
      `allowPrivilegeEscalation: false`, and `capabilities.drop: ["ALL"]` on the main
      container (verified by jobbuilder unit tests)
- [ ] Agent Job spec includes a `/tmp` emptyDir volume mount (verified by jobbuilder
      unit tests)
- [ ] Worklog entry created in `docs/WORKLOGS/`
