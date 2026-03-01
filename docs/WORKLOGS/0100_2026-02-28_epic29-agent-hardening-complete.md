# Worklog: Epic 29 — Agent Hardening Complete

**Date:** 2026-02-28
**Session:** Full epic29 implementation across all 7 stories (orchestrator session)
**Status:** Complete

---

## Objective

Implement all 7 stories of epic29-agent-hardening:
- STORY_00: kubectl wrapper Tier 1 write blocking (always-on)
- STORY_01: kubectl wrapper Tier 2 hardened mode + sentinel
- STORY_02: five new built-in redaction patterns
- STORY_03: Redactor struct + custom pattern support + redact binary propagation
- STORY_04: config, jobbuilder, and deploy wiring
- STORY_05: threat model update
- STORY_06: Kyverno policy (pod security + access restriction + image allowlist)

---

## Work Completed

### 1. STORY_00 — kubectl Tier 1 Write Blocking
- Added Tier 1 always-on write-subcommand block to `docker/scripts/redact-wrappers/kubectl`
- Blocked: apply, create, delete, edit, patch, replace, scale, set, label, annotate, taint, drain, cordon, uncordon, rollout restart, rollout undo
- Exit 1 with `[KUBECTL]` prefix message to stderr; redact pipeline intact for reads
- Extended `docker/scripts/wrapper-test.sh` with 22 test cases (16 blocked + 6 pass-through)

### 2. STORY_02 — Five New Redaction Patterns
- Added 5 patterns to `internal/domain/redact.go` before the base64 catch-all:
  - age private key (`AGE-SECRET-KEY-1[A-Z0-9]{40,}` → `[REDACTED-AGE-KEY]`)
  - sk-* API keys (OpenAI/Anthropic) → `[REDACTED-SK-KEY]`
  - AWS AKIA access key ID → `[REDACTED-AWS-KEY]`
  - JWT two-segment (ey....ey....) → `[REDACTED-JWT]`
  - Non-Bearer Authorization header (Token, Basic, Digest, etc.)
- Test cases added to `internal/domain/redact_test.go`

### 3. STORY_03 — Redactor Struct + Custom Patterns
- Refactored `internal/domain/redact.go`: introduced `Redactor` struct with `New(extraPatterns []string) (*Redactor, error)` and `(*Redactor).Redact(text) string`
- `RedactSecrets` shim preserved — all existing call sites unchanged
- Updated `cmd/redact/main.go` to read `EXTRA_REDACT_PATTERNS` env var (comma-separated); invalid patterns logged + skipped (never crash agent Job)
- Tests added to `internal/domain/redact_test.go` (TestNew) and `cmd/redact/main_test.go`

### 4. STORY_04 — Config, Jobbuilder, Deploy Wiring
- `internal/config/config.go`: added `HardenAgentKubectl bool` and `ExtraRedactPatterns []string` with full parsing and early validation via `domain.New`
- `internal/jobbuilder/job.go`: added same fields to `Config`; extracted `buildGateCommand()` helper; `mechanic-cfg` volume and `dry-run-gate` init container now triggered by `DryRun || HardenAgentKubectl`; injects `HARDEN_KUBECTL` and `EXTRA_REDACT_PATTERNS` into main container env
- `charts/mechanic/values.yaml`: `agent.hardenKubectl: false` and `agent.extraRedactPatterns: []` added
- `charts/mechanic/templates/deployment-watcher.yaml`: conditional `HARDEN_AGENT_KUBECTL` and `EXTRA_REDACT_PATTERNS` env vars
- All 6 native providers (`internal/provider/native/`): accept `*domain.Redactor` + use `redactor.Redact()` instead of `domain.RedactSecrets()`
- `cmd/watcher/main.go`: initialises `domain.Redactor` from config at startup

### 5. STORY_01 — kubectl Tier 2 Hardened Mode
- Added three-layer sentinel detection to `docker/scripts/redact-wrappers/kubectl`: sentinel file (`/mechanic-cfg/harden-kubectl`) → `/proc/1/environ` → `$HARDEN_KUBECTL` env var
- Tier 2 blocks (when sentinel active): exec, port-forward, get/describe secret(s), get all, comma-separated multi-resource lists containing secret
- Extended `wrapper-test.sh` with 19 hardened-mode test cases

### 6. STORY_05 — Threat Model Update
- `docs/SECURITY/THREAT_MODEL.md` bumped to v1.6
- AV-03: added Tier 1 / Tier 2 kubectl blocking + Kyverno server-side enforcement bullets
- AR-01: updated with cluster-scope vs. namespace-scope detail and dual-control discussion
- AV-02: added 5 new patterns + `agent.extraRedactPatterns` custom pattern support
- AR-02: age blind spot closed; remaining gaps documented
- New kubectl write-blocking section added after "Tools deliberately NOT wrapped"

### 7. STORY_06 — Kyverno Policy + Pod Security
- `charts/mechanic/templates/kyverno-policy-agent.yaml`: new ClusterPolicy with 10 rules
  - deny-agent-secret-read, deny-agent-pod-exec, deny-agent-pod-portforward (Category A — Enforce)
  - deny-agent-writes excluding RemediationJob/status (Category B — Enforce)
  - restrict-agent-image (only when allowedImagePrefix set, opt-in, Enforce)
  - enforce-agent-readonly-root, enforce-agent-nonroot, enforce-agent-no-privilege-escalation, enforce-agent-drop-capabilities (Category C — 4 separate rules with per-field messages, Enforce)
  - audit-agent-direct-api-calls (Category D — Audit mode, observability for EX-001)
- `charts/mechanic/values.yaml`: `agent.kyvernoPolicy.enabled: false` and `agent.kyvernoPolicy.allowedImagePrefix: "ghcr.io/lenaxia/mechanic-agent"`
- `internal/jobbuilder/job.go`: main container SecurityContext gets `ReadOnlyRootFilesystem: true` and `RunAsNonRoot: true` (in addition to existing AllowPrivilegeEscalation + drop ALL); added `/tmp` emptyDir volume + mount
- `docs/SECURITY/EXFIL_LEAK_REGISTRY.md` EX-001: added Kyverno audit rule as mitigating control
- `docs/SECURITY/2026-02-27_security_report/findings.md` 2026-02-27-005: updated to note Kyverno image allowlist closes the gap at admission layer

---

## Key Decisions

1. **Volume path `mechanic-cfg` vs `mendabot-cfg`**: The runtime code uses `mechanic-cfg`/`/mechanic-cfg` throughout (consistent with existing dry-run pattern). Story docs referenced the stale `mendabot-cfg` name. Updated all story docs to use `mechanic-cfg`.

2. **4 separate pod-security rules vs. 1 combined**: STORY_06 acceptance criteria required per-field messages. Implemented as 4 separate Kyverno rules (enforce-agent-readonly-root, enforce-agent-nonroot, enforce-agent-no-privilege-escalation, enforce-agent-drop-capabilities) rather than one combined rule.

3. **EXTRA_REDACT_PATTERNS in redact binary**: skips invalid patterns with a warning rather than exiting (never crash the agent Job). Watcher config.go fails hard at startup on invalid patterns. The two behaviors are intentional and complementary.

4. **Default values**: `agent.hardenKubectl: false` and `agent.kyvernoPolicy.enabled: false` — existing deployments are unaffected by all new features.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./...
# All 19 packages: PASS

go build ./...
# Clean — no errors

helm lint charts/mechanic/
# 1 chart(s) linted, 0 chart(s) failed
```

---

## Next Steps

Epic 29 is complete. Remaining unstarted epics:
- epic26-auto-close-resolved (not started)
- epic27-pr-feedback-iteration (not started)
- epic28-manual-trigger (not started)

All of these are independent of epic29.

---

## Files Modified

**Shell scripts:**
- `docker/scripts/redact-wrappers/kubectl` — Tier 1 + Tier 2 blocking
- `docker/scripts/wrapper-test.sh` — extended with 41 test cases total

**Go (domain):**
- `internal/domain/redact.go` — Redactor struct + 5 new patterns
- `internal/domain/redact_test.go` — TestNew + new pattern test cases

**Go (cmd/redact):**
- `cmd/redact/main.go` — EXTRA_REDACT_PATTERNS support
- `cmd/redact/main_test.go` — new test cases

**Go (config):**
- `internal/config/config.go` — HardenAgentKubectl + ExtraRedactPatterns
- `internal/config/config_test.go` — new parsing tests

**Go (jobbuilder):**
- `internal/jobbuilder/job.go` — new Config fields, buildGateCommand, SecurityContext, /tmp volume
- `internal/jobbuilder/job_test.go` — SecurityContext + volume tests

**Go (providers):**
- `internal/provider/native/pod.go`
- `internal/provider/native/deployment.go`
- `internal/provider/native/statefulset.go`
- `internal/provider/native/pvc.go`
- `internal/provider/native/node.go`
- `internal/provider/native/job.go`

**Go (watcher):**
- `cmd/watcher/main.go` — Redactor initialization + injection

**Helm chart:**
- `charts/mechanic/values.yaml` — hardenKubectl, extraRedactPatterns, kyvernoPolicy fields
- `charts/mechanic/templates/deployment-watcher.yaml` — conditional env var injection
- `charts/mechanic/templates/kyverno-policy-agent.yaml` — new file (ClusterPolicy with 10 rules)

**Security docs:**
- `docs/SECURITY/THREAT_MODEL.md` — v1.6 with AV-03 Kyverno controls, AR-01/AR-02 updates
- `docs/SECURITY/EXFIL_LEAK_REGISTRY.md` — EX-001 updated with Kyverno audit note
- `docs/SECURITY/2026-02-27_security_report/findings.md` — 2026-02-27-005 updated

**Backlog docs (path corrections):**
- `docs/BACKLOG/epic29-agent-hardening/README.md`
- `docs/BACKLOG/epic29-agent-hardening/STORY_01_kubectl_hardened_mode.md`
- `docs/BACKLOG/epic29-agent-hardening/STORY_04_config_and_deploy.md`
- `docs/BACKLOG/epic29-agent-hardening/STORY_05_threat_model_update.md`
- `docs/BACKLOG/epic29-agent-hardening/STORY_06_kyverno_policy.md`
