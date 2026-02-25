# Phase 9: Operational Security

**Date run:** 2026-02-24
**Cluster:** yes (v0.3.9, default namespace)

---

## 9.1 Secret Placeholder Audit

The project has moved from Kustomize to Helm chart packaging. Secret placeholders are now
defined in `charts/mendabot/templates/secret-agent-token.yaml` (SA token) and configured
via `values.yaml`. No plaintext credential YAML files are present in the chart or `deploy/`
that would be applied with real values.

`values.yaml` has placeholder-style structures for `llmCredentials.*` and `github.*` fields
with comments instructing operators to provide values. No actual credential strings are
committed in any tracked file.

| Check | Result | Notes |
|-------|--------|-------|
| No literal credentials in chart | PASS | All Secret content is templated from values |
| Operator documentation present | PASS | `values.yaml` has inline comments |

---

## 9.2 Configuration Security

```
grep output:
config.go:38  InjectionDetectionAction — "log" (default) or "suppress"
config.go:39  AgentRBACScope — "cluster" (default) or "namespace"
config.go:159 validation: must be 'log' or 'suppress'
config.go:173 validation: must be 'cluster' or 'namespace'
config.go:180 AGENT_WATCH_NAMESPACES required when AGENT_RBAC_SCOPE=namespace
```

| Check | Result | Notes |
|-------|--------|-------|
| `FromEnv()` validates `INJECTION_DETECTION_ACTION` | PASS | Error returned on invalid value |
| `FromEnv()` validates `AGENT_RBAC_SCOPE` | PASS | Error returned on invalid value |
| `AGENT_WATCH_NAMESPACES` required when scope=namespace | PASS | config.go:180 |
| Default `INJECTION_DETECTION_ACTION` is `log` | INFO | Weaker than `suppress`; operators should be aware. Documented as accepted risk. |
| No config values from Secrets are logged | PASS | `AGENT_PROVIDER_CONFIG` and `GITHUB_APP_PRIVATE_KEY` only appear as `secretKeyRef`, not logged |

---

## 9.3 Error Message Information Disclosure

Error messages in `internal/` examined. Pattern: `fmt.Errorf("... %s ...", err)` — standard Go error wrapping. No errors incorporate secret values, file contents, or finding error text. Errors propagate to the controller-runtime reconcile loop which logs them at `error` level as the `msg` or `error` field.

**Result:** No information disclosure issues found.

---

## 9.4 Job Security Settings

```
internal/jobbuilder/job.go:260  BackoffLimit:            ptr(int32(1))
internal/jobbuilder/job.go:261  ActiveDeadlineSeconds:   ptr(int64(900))
internal/jobbuilder/job.go:262  TTLSecondsAfterFinished: ptr(b.cfg.TTLSeconds)  [default: 86400]
internal/jobbuilder/job.go:266  RestartPolicy:           corev1.RestartPolicyNever
```

| Setting | Present? | Value | Adequate? |
|---------|---------|-------|----------|
| `activeDeadlineSeconds` | Yes | 900 (15 min) | Yes — hard cap on LLM session |
| `ttlSecondsAfterFinished` | Yes | 86400 (7 days configurable, default 1 day) | Yes — prevents Job accumulation |
| `backoffLimit` | Yes | 1 | Yes — allows one retry, then fails permanently |
| `restartPolicy: Never` | Yes | Never | Yes — no auto-restart |

---

## 9.5 Watcher Deployment Security Settings

Live cluster pod spec (Helm chart `deployment-watcher.yaml`):

| Check | Result | Notes |
|-------|--------|-------|
| `readOnlyRootFilesystem: true` | PASS | Present in container securityContext |
| `allowPrivilegeEscalation: false` | PASS | Present |
| `runAsNonRoot: true` | PASS | Pod-level securityContext |
| `runAsUser: 1000` | PASS | Explicit non-root UID |
| `capabilities: drop: ["ALL"]` | PASS | All capabilities dropped |
| `seccompProfile: RuntimeDefault` | PASS | Present |
| CPU + memory limits | PASS | `cpu: 200m, memory: 128Mi` |
| Liveness + readiness probes | PASS | Both configured (confirmed from chart) |

---

## Phase 9 Summary

Operational security configuration is strong. Job security settings are correctly set. Watcher deployment has a comprehensive security context. The one INFO-level note is that the default `INJECTION_DETECTION_ACTION=log` is weaker than `suppress` — this is known and acceptable.

**Total findings:** 0
