# Phase 9: Operational Security

**Date run:**
**Reviewer:**

---

## 9.1 Secret Placeholder Audit

```bash
cat deploy/kustomize/secret-github-app-placeholder.yaml
cat deploy/kustomize/secret-llm-placeholder.yaml
```
```
<!-- paste content of both files -->
```

| Check | Result | Notes |
|-------|--------|-------|
| Both Secrets contain only placeholder values | pass / fail | |
| Neither placeholder is applied by default kustomization | pass / fail | |
| Documentation instructs operators to replace before deployment | pass / fail | |

**Findings:** (none / list → add each to findings.md)

---

## 9.2 Configuration Security

**Relevant config code (`internal/config/config.go`):**
```bash
grep -n 'INJECTION_DETECTION_ACTION\|AGENT_RBAC_SCOPE\|LOG_LEVEL' internal/config/config.go
```
```
<!-- paste output -->
```

| Check | Result | Notes |
|-------|--------|-------|
| `FromEnv()` validates `AGENT_RBAC_SCOPE` — error on invalid value | pass / fail | |
| `AGENT_WATCH_NAMESPACES` required when scope=namespace | pass / fail | |
| Default `INJECTION_DETECTION_ACTION` is `log` — weaker than `suppress` | documented / undocumented | Consider documenting the trade-off |
| No config values from Secrets are logged at any level | pass / fail | |

**Findings:** (none / list → add each to findings.md)

---

## 9.3 Error Message Information Disclosure

```bash
grep -rn 'fmt.Errorf\|errors.New' internal/ --include='*.go' | grep -v '_test.go'
```
```
<!-- paste output -->
```

**Review:** Do any error messages expose internal paths, file contents, stack traces,
or values that originated from Secrets or Finding fields?

**Result:** No disclosure issues / Issues found (describe)

**Findings:** (none / list → add each to findings.md)

---

## 9.4 Job Security Settings

```bash
grep -rn 'ttlSecondsAfterFinished\|activeDeadlineSeconds\|backoffLimit\|restartPolicy' \
  internal/jobbuilder/
```
```
<!-- paste output -->
```

| Setting | Present? | Value | Adequate? |
|---------|---------|-------|----------|
| `activeDeadlineSeconds` | yes / no | | 900s = 15 min |
| `ttlSecondsAfterFinished` | yes / no | | 86400s = 1 day |
| `backoffLimit` | yes / no | | Should be low (≤2) |
| `restartPolicy: Never` | yes / no | | |

**Findings:** (none / list → add each to findings.md)

---

## 9.5 Watcher Deployment Security Settings

```bash
cat deploy/kustomize/deployment-watcher.yaml
```
```
<!-- paste content -->
```

| Check | Result | Notes |
|-------|--------|-------|
| `readOnlyRootFilesystem: true` in securityContext | pass / fail | |
| `allowPrivilegeEscalation: false` | pass / fail | |
| `runAsNonRoot: true` | pass / fail | |
| Resource limits set (CPU + memory) | pass / fail | |
| Liveness and readiness probes configured | pass / fail | |

**Findings:** (none / list → add each to findings.md)

---

## Phase 9 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
