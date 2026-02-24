# Phase 7: Audit Log Verification

**Date run:**
**Reviewer:**
**Cluster:** yes / no

---

## 7.1 Audit Event Collection

**Status:** Executed / SKIPPED — reason: ______

**Log collection command:**
```bash
kubectl logs -n mendabot deployment/mendabot-watcher --since=10m \
  | jq 'select(.audit == true) | {event: .event, ts: .ts}' 2>/dev/null \
  | sort | uniq
```
```
<!-- paste output -->
```

**Raw log excerpt (first 50 audit lines):**
```
<!-- paste or reference raw/watcher-audit.txt -->
```

---

## 7.2 Event Coverage

For each event, record whether it was observed and whether `audit=true` and `event` field
were present. If an event could not be triggered, document why.

| Event | Triggered? | `audit=true`? | `event` field? | Notes |
|-------|-----------|--------------|----------------|-------|
| `remediationjob.cancelled` | yes / no / N/A | yes / no | yes / no | |
| `finding.injection_detected` | yes / no / N/A | yes / no | yes / no | |
| `finding.suppressed.cascade` | yes / no / N/A | yes / no | yes / no | |
| `finding.suppressed.circuit_breaker` | yes / no / N/A | yes / no | yes / no | |
| `finding.suppressed.max_depth` | yes / no / N/A | yes / no | yes / no | |
| `finding.suppressed.stabilisation_window` | yes / no / N/A | yes / no | yes / no | |
| `remediationjob.created` | yes / no / N/A | yes / no | yes / no | |
| `remediationjob.deleted_ttl` | yes / no / N/A | yes / no | yes / no | |
| `job.succeeded` / `job.failed` | yes / no / N/A | yes / no | yes / no | |
| `job.dispatched` | yes / no / N/A | yes / no | yes / no | |

---

## 7.3 Audit Log Content — Credential Check

For each audit event, verify no credential values appear in the log fields.

```bash
# Check for credential patterns in audit log lines
kubectl logs -n mendabot deployment/mendabot-watcher --since=10m \
  | grep '"audit":true' \
  | grep -i -E '(password|secret|token|key|credential)' \
  | grep -v '"event"' | grep -v '"audit"'
```
```
<!-- paste output — should be empty, or show only safe field names like "event": "job.dispatched" -->
```

**Result:** No credential values in audit logs / FAIL (describe)

---

## Phase 7 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
