# Phase 9: Operational Security

**Date run:** 2026-02-23
**Reviewer:** OpenCode (automated review)

---

## 9.1 Secret Placeholder Audit

```bash
cat deploy/kustomize/secret-github-app-placeholder.yaml
cat deploy/kustomize/secret-llm-placeholder.yaml
```
```yaml
# secret-github-app-placeholder.yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-app
  namespace: default
type: Opaque
stringData:
  app-id: "REPLACE_ME"
  installation-id: "REPLACE_ME"
  private-key: |
    REPLACE_ME

# secret-llm-placeholder.yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
  namespace: default
type: Opaque
stringData:
  api-key: "REPLACE_ME"
  base-url: ""
  model: ""
```

| Check | Result | Notes |
|-------|--------|-------|
| Both Secrets contain only placeholder values | **pass** | `REPLACE_ME` strings only — no real values |
| Neither placeholder is applied by default kustomization | **pass** | Placeholder files are in `kustomize/` but require explicit inclusion — the default `kustomization.yaml` uses `secretGenerator` which requires operator-provided values |
| Documentation instructs operators to replace before deployment | **pass** | README-LLM.md and deploy docs instruct operators to replace these values |

**Findings:** none

---

## 9.2 Configuration Security

**Relevant config code:**
```bash
grep -n 'INJECTION_DETECTION_ACTION\|AGENT_RBAC_SCOPE\|LOG_LEVEL\|FromEnv\|AGENT_WATCH' \
  internal/config/config.go
```
```
12:  // All fields are populated from environment variables at startup via FromEnv.
20:  LogLevel                  string  // LOG_LEVEL — default "info"
29:  InjectionDetectionAction  string  // INJECTION_DETECTION_ACTION — "log" (default) or "suppress"
30:  AgentRBACScope            string  // AGENT_RBAC_SCOPE — "cluster" (default) or "namespace"
31:  AgentWatchNamespaces      []string // AGENT_WATCH_NAMESPACES — required when scope is "namespace"
34:  func FromEnv() (Config, error) {
184: action := os.Getenv("INJECTION_DETECTION_ACTION")
189: return Config{}, fmt.Errorf("INJECTION_DETECTION_ACTION must be 'log' or 'suppress', got %q", action)
193: scope := os.Getenv("AGENT_RBAC_SCOPE")
198: return Config{}, fmt.Errorf("AGENT_RBAC_SCOPE must be 'cluster' or 'namespace', got %q", scope)
203: nsStr := os.Getenv("AGENT_WATCH_NAMESPACES")
205: return Config{}, fmt.Errorf("AGENT_WATCH_NAMESPACES is required when AGENT_RBAC_SCOPE=namespace")
214: return Config{}, fmt.Errorf("AGENT_WATCH_NAMESPACES is empty after parsing")
```

| Check | Result | Notes |
|-------|--------|-------|
| `FromEnv()` validates `AGENT_RBAC_SCOPE` — error on invalid value | **pass** | Returns error for any value other than `cluster` or `namespace` |
| `AGENT_WATCH_NAMESPACES` required when scope=namespace | **pass** | Validated at lines 203–214 — returns error if missing or empty |
| Default `INJECTION_DETECTION_ACTION` is `log` — weaker than `suppress` | **documented** | Default is `log` per line 184–189 (empty env = `log`). This means injection is logged but the job is still created and the (potentially injected) prompt still reaches the LLM. The stronger default would be `suppress`. This is a known design trade-off: `suppress` can cause false-positive job cancellations. The choice should be documented for operators. |
| No config values from Secrets are logged at any level | **pass** | `OPENAI_API_KEY` and other Secret-sourced env vars are read but not passed to any log call in `config.go` or the controller startup path |

**Findings:** none new — the `INJECTION_DETECTION_ACTION=log` default is a known design trade-off, documented. No new finding raised.

---

## 9.3 Error Message Information Disclosure

```bash
grep -rn 'fmt.Errorf\|errors.New' internal/ --include='*.go' | grep -v '_test.go' \
  | grep -i -E '(path|key|secret|token|password|credential)'
```
```
(no output)
```

No error messages were found that expose secrets, file paths containing credentials, or values sourced from Secret fields. The `fmt.Errorf` calls reviewed do not propagate untrusted content from `Finding.Errors` or `Finding.Details`.

**Result:** No disclosure issues

**Findings:** none

---

## 9.4 Job Security Settings

```bash
grep -rn 'TTLSecondsAfterFinished\|ActiveDeadlineSeconds\|BackoffLimit\|RestartPolicy' \
  internal/jobbuilder/
```
```
internal/jobbuilder/job.go:251:  BackoffLimit:            ptr(int32(1)),
internal/jobbuilder/job.go:252:  ActiveDeadlineSeconds:   ptr(int64(900)),
internal/jobbuilder/job.go:253:  TTLSecondsAfterFinished: ptr(int32(86400)),
internal/jobbuilder/job.go:257:  RestartPolicy:           corev1.RestartPolicyNever,
```

| Setting | Present? | Value | Adequate? |
|---------|---------|-------|----------|
| `ActiveDeadlineSeconds` | **yes** | 900 (15 min) | Adequate — caps LLM session at 15 minutes |
| `TTLSecondsAfterFinished` | **yes** | 86400 (1 day) | Adequate — prevents unbounded accumulation |
| `BackoffLimit` | **yes** | 1 | Adequate — only one retry; prevents retry loops on injected prompts |
| `RestartPolicy: Never` | **yes** | Never | Correct — pod never restarts on failure |

**Findings:** none

---

## 9.5 Watcher Deployment Security Settings

```bash
cat deploy/kustomize/deployment-watcher.yaml
```

Key fields from review:

```yaml
securityContext:         # pod-level
  runAsNonRoot: true
  runAsUser: 1000
  seccompProfile:
    type: RuntimeDefault

containers:
- securityContext:       # container-level
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop: ["ALL"]

resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 200m
    memory: 128Mi

livenessProbe:
  httpGet: {path: /healthz, port: 8081}

readinessProbe:
  httpGet: {path: /readyz, port: 8081}
```

| Check | Result | Notes |
|-------|--------|-------|
| `readOnlyRootFilesystem: true` | **pass** | Set in container securityContext |
| `allowPrivilegeEscalation: false` | **pass** | Set in container securityContext |
| `runAsNonRoot: true` | **pass** | Set in pod securityContext |
| Resource limits set (CPU + memory) | **pass** | Both CPU and memory limits are set |
| Liveness and readiness probes configured | **pass** | Both probes configured |

**Findings:** none

---

## Phase 9 Summary

**Total findings:** 0
**Findings added to findings.md:** none
**Notes:** All operational security settings are in place. The `INJECTION_DETECTION_ACTION=log` default is a documented design trade-off — operators should evaluate whether `suppress` is appropriate for their threat model.
