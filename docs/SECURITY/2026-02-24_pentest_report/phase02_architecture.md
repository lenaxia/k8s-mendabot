# Phase 2: Architecture and Design Review

**Date run:** 2026-02-24
**Cluster:** yes (v0.3.9, default namespace)

---

## 2.1 Data Flow — Path 1: Error message → LLM prompt

### PodProvider (`internal/provider/native/pod.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `State.Waiting.Message` truncation before `RedactSecrets` | PASS | 207 | `truncate(msg, 500)` then `RedactSecrets` |
| `State.Waiting.Message` — `RedactSecrets` called | PASS | 208 | After truncation |
| `State.Terminated.Message` — `RedactSecrets` called | PASS | 90 | `RedactSecrets(msg)` |
| Unschedulable condition `cond.Message` — truncation before `RedactSecrets` | **FAIL** | 104 | `domain.RedactSecrets(cond.Message)` — no prior truncation. Carry-over of 2026-02-24-003. |
| No bypass path around truncation + redaction | PASS | — | All code paths reviewed |

### DeploymentProvider (`internal/provider/native/deployment.go`)

| Check | Result | Notes |
|-------|--------|-------|
| All free-form text — truncation applied | PASS | Uses `truncate()` helpers throughout |
| All free-form text — `RedactSecrets` called | PASS | All message fields go through `RedactSecrets` |

### StatefulSetProvider, JobProvider, NodeProvider, PVCProvider

All confirmed: truncation applied before `RedactSecrets` on all free-form message fields.

### SourceProviderReconciler (`internal/provider/provider.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `DetectInjection` called on `finding.Errors` | PASS | 137 | Before job creation |
| `DetectInjection` called on `finding.Details` | PASS | 153 | Before job creation |
| Detection fires before job creation | PASS | 137–167 | Both checks occur in the gate block |
| `INJECTION_DETECTION_ACTION=suppress` returns before creation | PASS | 142–149 | Returns `ctrl.Result{}` when action=suppress |
| Priority annotation bypass emits audit log | **FAIL** | 260–261 | `priorityCritical == true` path has no audit log event. Carry-over of 2026-02-24-004. |

**Key observation:** `DetectInjection` is called **only** in `SourceProviderReconciler.Reconcile` (provider pipeline). It is **not** called in `RemediationJobController.Reconcile` (controller pipeline). A `RemediationJob` created directly (bypassing the provider) will be dispatched without injection detection. This is the documented AV-09 vector. **Pentest confirmed** this in Phase 3 (live test).

### JobBuilder (`internal/jobbuilder/job.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `FINDING_ERRORS` is the only env var with untrusted error text | PASS | 130–170 | `FINDING_DETAILS` is also injected but is from provider analysis, not raw pod output |
| `FINDING_DETAILS` assessed | PASS | — | Passed through provider pipeline; `DetectInjection` called on it at line 153 |
| `FINDING_CORRELATED_FINDINGS` redaction | FAIL (latent) | 179–184 | Not redacted before marshal — carry-over 2026-02-24-006 (deferred/latent) |

### Agent entrypoint (`docker/scripts/entrypoint-common.sh`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `envsubst` restricts substitutions to known variable list | PASS | VARS= line | Explicit `$VARS` passed to `envsubst` — only 9 named vars expanded |
| Rendered prompt written to temp file | PASS | `/tmp/rendered-prompt.txt` | Not inline |
| Temp file path not attacker-controlled | PASS | Hardcoded path | Cannot be influenced by env vars |
| No double-expansion of variables | PASS | `printf '%s'` | Uses printf, not echo |

### Prompt template (`charts/mendabot/files/prompts/core.txt`)

| Check | Result | Notes |
|-------|--------|-------|
| Untrusted-data envelope around `${FINDING_ERRORS}` | PASS | `=== BEGIN FINDING ERRORS (UNTRUSTED INPUT ...) ===` present |
| HARD RULE 8 present and unambiguous | PASS | Rule 8 in HARD RULES section |
| `${FINDING_DETAILS}` envelope assessed | PASS | `=== BEGIN AI ANALYSIS (UNTRUSTED INPUT ...) ===` present |

---

## 2.2 RBAC Audit

### ClusterRole: mendabot-agent (deployed v0.3.9)

```yaml
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
```

| Check | Result | Notes |
|-------|--------|-------|
| No write verbs | PASS | Only get/list/watch |
| No `pods/exec` (create verb) | PASS | No create verb |
| No `nodes/proxy` explicit grant | PASS | Not listed explicitly |
| Wildcard `resources: ["*"]` includes `nodes/proxy` implicitly | **FAIL** | Live test confirmed: agent can GET nodes/proxy → reads kubelet metrics and node logs. See finding 2026-02-24-P-004. |

**Live test results:**
```
kubectl auth can-i get nodes/proxy --as=system:serviceaccount:default:mendabot-agent
→ yes

kubectl get --raw "/api/v1/nodes/cp-00/proxy/metrics" --as=...mendabot-agent
→ # HELP aggregator_discovery_aggregation_count_total ...   (SUCCESS)

kubectl get --raw "/api/v1/nodes/cp-00/proxy/logs/" --as=...mendabot-agent
→ <listing of node log files>  (SUCCESS)
```

### ClusterRole: mendabot-watcher (deployed v0.3.9)

```yaml
rules:
- apiGroups: [""]
  resources: ["pods", "persistentvolumeclaims", "nodes", "namespaces", "secrets"]
  verbs: ["get", "list", "watch"]
```

| Check | Result | Notes |
|-------|--------|-------|
| `secrets` removed from ClusterRole | **FAIL** | 2026-02-24-002 was remediated in the Helm chart source (`charts/`) but the live deployed instance still has `secrets` in ClusterRole. Regression from not re-deploying. See 2026-02-24-P-005. |
| ConfigMap write cluster-wide | PASS | Not in ClusterRole — namespace Role handles it |
| No write outside mendabot ns (except RemediationJobs) | PASS | Only remediationjobs/jobs have write verbs |
| RemediationJob delete cluster-wide | Note | ClusterRole grants delete on `remediationjobs` — functionally this is cluster-wide since the CRD is not namespace-specific. This is by design for watcher cleanup. No cross-namespace RemediationJob objects exist. Acceptable. |

**Live test results:**
```
kubectl get secret -n kube-system --as=...mendabot-watcher
→ NAME ... bootstrap-token-m1akoo ... (SUCCESS — confirmed credential access)
```

### Role: mendabot-agent (namespace-scoped, default)

```yaml
rules:
- apiGroups: ["remediation.mendabot.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch"]
```

| Check | Result | Notes |
|-------|--------|-------|
| Status patch scoped to `remediationjobs/status` only | PASS | Only subresource — correct |
| Agent cannot update full remediationjobs spec | PASS | Live test: impersonation patch to spec → Forbidden |
| `auth can-i` false negative on CRD subresources | INFO | `kubectl auth can-i patch remediationjobs/status` returns `no` but actual patch succeeds. Known `kubectl` limitation with CRD subresources. Not a security issue — actual RBAC is enforced correctly. |

---

## 2.3 Secret Handling Audit

### GitHub App private key

**Live job spec inspection** (`kubectl get job mendabot-agent-0cd2345e0966 -n default`):

Init container env vars:
- `GITHUB_APP_ID` → secretKeyRef `github-app`
- `GITHUB_APP_INSTALLATION_ID` → secretKeyRef `github-app`
- `GITHUB_APP_PRIVATE_KEY` → secretKeyRef `github-app`

Main container env vars: `FINDING_KIND`, `FINDING_NAME`, `FINDING_NAMESPACE`, `FINDING_PARENT`, `FINDING_ERRORS`, `FINDING_DETAILS`, `FINDING_FINGERPRINT`, `FINDING_SEVERITY`, `GITOPS_REPO`, `GITOPS_MANIFEST_ROOT`, `SINK_TYPE`, `AGENT_PROVIDER_CONFIG`, `AGENT_TYPE` — **no GITHUB_APP_* vars**.

| Check | Result |
|-------|--------|
| `GITHUB_APP_PRIVATE_KEY` in init container only | PASS |
| Main container has no GitHub App key env vars | PASS |
| Token written to `/workspace/github-token` | PASS (entrypoint-common.sh line 64) |
| Entrypoint does not log/echo token | PASS |
| Token file path hardcoded | PASS |

### LLM API key

`AGENT_PROVIDER_CONFIG` sourced from `secretKeyRef` on main container. Not logged in entrypoint. Content is opaque to mendabot.

---

## 2.4 Container Security Audit

### Dockerfile.agent

| Check | Result | Notes |
|-------|--------|-------|
| Non-root USER | PASS | `USER agent` (uid 1000) |
| All binary downloads SHA256-verified | PASS | kubectl, helm, flux, talosctl, kustomize, yq, kubeconform, stern, sops, opencode — all verified |
| `gh` CLI verified | PASS | Via GitHub's GPG-signed apt repo |
| age compiled from source | PASS | Two-stage build with golang:1.25.7 |
| `--no-install-recommends` on apt-get | PASS |  |
| Package lists cleaned | PASS | `rm -rf /var/lib/apt/lists/*` after each apt step |
| Base image pinned to digest | PASS | `debian:bookworm-slim@sha256:6458e6ce2b6448...` |
| Build stage pinned to digest | PASS | `golang:1.25.7-bookworm@sha256:0b5f101af6e4f...` |
| No secrets in ARG/ENV | PASS | No credential values in build args |
| GIT_AUTHOR_EMAIL hardcoded | INFO | `mendabot-agent@users.noreply.github.com` — not a secret, acceptable |

### Dockerfile.watcher

| Check | Result | Notes |
|-------|--------|-------|
| Non-root USER | PASS | `USER watcher` (uid 1000) |
| Multi-stage build | PASS | Builder + debian-slim runtime |
| Base image pinned to digest | PASS | Same digest as agent |
| Build image pinned to digest | PASS | golang:1.25.7 |
| No secrets in ARG/ENV | PASS |  |
| `--no-install-recommends` | PASS | Only `ca-certificates` installed |

---

## 2.5 CI/CD Pipeline Audit

### build-watcher.yaml / build-agent.yaml

| Check | Result | Notes |
|-------|--------|-------|
| `permissions: contents: read` | PASS | Both workflows use `contents: read, packages: write` |
| Actions pinned to commit SHA | PASS | All third-party actions pinned to SHAs: `actions/checkout@34e114876b0b...`, `docker/setup-qemu-action@c7c53464...`, etc. |
| No fork PR secret exposure | PASS | Trigger is `push: tags: v*` and `workflow_dispatch` only — no `pull_request` trigger |
| Trivy scan step in CI | PASS | `aquasecurity/trivy-action@b2933f...` runs after build, exits 1 on CRITICAL/HIGH unfixed |
| `.trivyignore` present and documented | PASS | 8 CVEs suppressed for third-party tool binaries with expiry 2026-06-01 |
| Builds gated on tag push only | PASS | Not on every commit push |

---

## Phase 2 Summary

**Total new findings:** 2 (P-004, P-005)
**Carry-over confirmed:** 003, 004, 006
**Findings added to findings.md:** 2026-02-24-P-004, 2026-02-24-P-005
