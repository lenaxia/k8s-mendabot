# Phase 2: Architecture and Design Review

**Date run:**
**Reviewer:**

---

## 2.1 Data Flow — Path 1: Error message → LLM prompt

### PodProvider (`internal/provider/native/pod.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `State.Waiting.Message` truncation before `RedactSecrets` | pass / fail / N/A | | |
| `State.Waiting.Message` — `RedactSecrets` called | pass / fail | | |
| `State.Terminated.Message` — `RedactSecrets` called | pass / fail | | |
| Condition messages — `RedactSecrets` called | pass / fail | | |
| No bypass path around truncation + redaction | pass / fail | | |

### DeploymentProvider (`internal/provider/native/deployment.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass / fail | | |
| All free-form text — `RedactSecrets` called | pass / fail | | |

### StatefulSetProvider (`internal/provider/native/statefulset.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass / fail | | |
| All free-form text — `RedactSecrets` called | pass / fail | | |

### JobProvider (`internal/provider/native/job.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass / fail | | |
| All free-form text — `RedactSecrets` called | pass / fail | | |

### NodeProvider (`internal/provider/native/node.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass / fail | | |
| All free-form text — `RedactSecrets` called | pass / fail | | |

### PVCProvider (`internal/provider/native/pvc.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| All free-form text — truncation applied | pass / fail | | |
| All free-form text — `RedactSecrets` called | pass / fail | | |

### SourceProviderReconciler (`internal/provider/provider.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `DetectInjection` called on `finding.Errors` | pass / fail | | |
| `DetectInjection` called on `finding.Details` | pass / fail / N/A | | |
| Detection fires before job creation | pass / fail | | |
| `INJECTION_DETECTION_ACTION=suppress` returns before creation | pass / fail | | |

### JobBuilder (`internal/jobbuilder/job.go`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `FINDING_ERRORS` is the only env var with untrusted error text | pass / fail | | |
| `FINDING_DETAILS` assessed for untrusted content | assessed | | |
| All `Finding` fields injected as env vars reviewed | pass / fail | | |

### Agent entrypoint (`docker/scripts/agent-entrypoint.sh`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `envsubst` restricts substitutions to known variable list | pass / fail | | |
| Rendered prompt written to temp file (not inline) | pass / fail | | |
| Temp file path not attacker-controlled | pass / fail | | |
| No double-expansion of variables | pass / fail | | |

### Prompt template (`deploy/kustomize/configmap-prompt.yaml`)

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| Untrusted-data envelope around `${FINDING_ERRORS}` | pass / fail | | |
| HARD RULE 8 present and unambiguous | pass / fail | | |
| `${FINDING_DETAILS}` envelope assessed | assessed | | |

**Findings:** (none / list → add each to findings.md)

---

## 2.2 RBAC Audit

### ClusterRole: mechanic-agent

```bash
# Command run:
cat deploy/kustomize/clusterrole-agent.yaml
```

```yaml
<!-- paste content -->
```

| Check | Result | Notes |
|-------|--------|-------|
| No write verbs | pass / fail | |
| No `pods/exec` | pass / fail | |
| No `nodes/proxy` | pass / fail | |
| Namespace scope replaces (not supplements) ClusterRole | pass / fail / N/A | |

### ClusterRole: mechanic-watcher

```yaml
<!-- paste content -->
```

| Check | Result | Notes |
|-------|--------|-------|
| ConfigMap write is namespace-scoped | pass / fail | |
| No write outside mechanic ns (except RemediationJobs) | pass / fail | |
| `delete` on remediationjobs — blast radius acceptable | pass / fail | |

### Role: mechanic-agent (namespace-scoped)

```yaml
<!-- paste content -->
```

| Check | Result | Notes |
|-------|--------|-------|
| Status patch scoped to `remediationjobs/status` only | pass / fail | |
| Agent cannot update full remediationjobs spec | pass / fail | |

**Findings:** (none / list → add each to findings.md)

---

## 2.3 Secret Handling Audit

### GitHub App private key (`internal/jobbuilder/job.go`)

```
grep -n 'github-app' internal/jobbuilder/job.go
<!-- paste output -->
```

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `github-app-secret` volume in init container VolumeMounts only | pass / fail | | |
| `GITHUB_APP_PRIVATE_KEY` in init container Env only | pass / fail | | |
| `GITHUB_APP_ID` in init container Env only | pass / fail | | |
| `GITHUB_APP_INSTALLATION_ID` in init container Env only | pass / fail | | |
| Main container has no `github-app-secret` reference | pass / fail | | |
| Shared emptyDir contains only the token | pass / fail | | |

### LLM API key

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `OPENAI_API_KEY` sourced from Secret | pass / fail | | |
| Key not printed or logged in entrypoint | pass / fail | | |
| opencode config built in-memory | pass / fail | | |

### Token file

| Check | Result | Notes |
|-------|--------|-------|
| Token read from file, not env var | pass / fail | |
| Entrypoint does not log/echo token value | pass / fail | |
| Token file path not attacker-influenced | pass / fail | |

**Findings:** (none / list → add each to findings.md)

---

## 2.4 Container Security Audit

### Dockerfile.agent

| Check | Result | Notes |
|-------|--------|-------|
| Non-root USER instruction | pass / fail | |
| All binary downloads have SHA256 checksum | pass / fail | Missing: |
| `--no-install-recommends` on apt-get | pass / fail | |
| Package lists cleaned after install | pass / fail | |
| No secrets in ARG or ENV | pass / fail | |
| Base image pinned to digest | pass / fail | Current: |
| Multi-stage build or justified reason why not | pass / fail | |

### Dockerfile.watcher

| Check | Result | Notes |
|-------|--------|-------|
| Non-root USER instruction | pass / fail | |
| Multi-stage build — build tools excluded from final image | pass / fail | |
| No secrets in ARG or ENV | pass / fail | |

**Findings:** (none / list → add each to findings.md)

---

## 2.5 CI/CD Pipeline Audit

| Check | Workflow | Result | Notes |
|-------|---------|--------|-------|
| `permissions: contents: read` or equivalent | build-watcher.yaml | pass / fail | |
| `permissions: contents: read` or equivalent | build-agent.yaml | pass / fail | |
| Actions pinned to commit SHA | all | pass / fail | Not pinned: |
| No fork PR secret exposure | all | pass / fail | |
| Vulnerability scan step in CI | all | pass / fail | |
| Builds gated on protected branch/tag only | all | pass / fail | |

**Findings:** (none / list → add each to findings.md)

---

## Phase 2 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
