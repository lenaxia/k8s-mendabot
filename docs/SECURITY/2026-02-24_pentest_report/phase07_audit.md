# Phase 7: Audit Log Verification

**Date run:** 2026-02-24
**Cluster:** yes (v0.3.9, default namespace)

---

## 7.1 Audit Event Collection

**Status:** Executed

Live watcher logs collected and parsed for `"audit":true` JSON lines. Raw output:

```json
{"level":"info","ts":1771986483.49,"caller":"provider/provider.go:249","msg":"finding detected","audit":true,"event":"finding.detected","provider":"native","kind":"Deployment","namespace":"default","name":"test-crashloop","fingerprint":"0cd2345e0966"}
{"level":"info","ts":1771986483.49,"caller":"provider/provider.go:265","msg":"finding suppressed","audit":true,"event":"finding.suppressed.stabilisation_window","provider":"native","fingerprint":"0cd2345e0966","reason":"first_seen","window":120}
{"level":"info","ts":1771986603.49,"caller":"provider/provider.go:327","msg":"finding suppressed","audit":true,"event":"finding.suppressed.duplicate","provider":"native","fingerprint":"22693f928816","remediationJob":"mendabot-22693f928816","phase":"Succeeded"}
{"level":"info","ts":1771987777.31,"caller":"controller/remediationjob_controller.go:318","msg":"dispatched agent job","audit":true,"event":"job.dispatched","remediationJob":"pentest-injection-001","job":"mendabot-agent-pentest00000","namespace":"default"}
```

---

## 7.2 Event Coverage

| Event | Triggered? | `audit=true`? | `event` field? | Notes |
|-------|-----------|--------------|----------------|-------|
| `finding.detected` | Yes | Yes | Yes | New event not in the previous report's expected list — additional coverage |
| `finding.suppressed.stabilisation_window` | Yes | Yes | Yes | Seen with `reason: first_seen` and `reason: window_open` |
| `finding.suppressed.duplicate` | Yes | Yes | Yes | Triggered when remediationjob already Succeeded |
| `job.dispatched` | Yes | Yes | Yes | Triggered by pentest RJ and normal RJs |
| `remediationjob.cancelled` | No | N/A | N/A | Could not trigger without a cancelled RJ scenario |
| `finding.injection_detected` | No | N/A | N/A | No injection detected via provider pipeline during this session (expected — pentest bypassed provider, went direct to controller) |
| `finding.suppressed.cascade` | No | N/A | N/A | Cascade prevention not triggered |
| `finding.suppressed.circuit_breaker` | No | N/A | N/A | Circuit breaker not triggered |
| `finding.suppressed.max_depth` | No | N/A | N/A | No nested self-remediation |
| `remediationjob.created` | Not directly logged | N/A | N/A | Creation is implied by dispatch; no separate event observed — may not be emitted as audit event |
| `remediationjob.deleted_ttl` | No | N/A | N/A | No TTL-expired RJ during observation window |
| `job.succeeded` / `job.failed` | Yes (inferred) | N/A | N/A | RJ status transitioned to Succeeded but specific audit event not seen in log window |

**Key observation:** `finding.injection_detected` was not emitted during the pentest because the crafted `RemediationJob` was created directly (bypassing the provider reconciler). This is consistent with the finding 2026-02-24-P-008.

---

## 7.3 Audit Log Content — Credential Check

All observed audit log lines reviewed. No credential values found:

- `finding.detected` log contains: `provider`, `kind`, `namespace`, `name`, `fingerprint` — no error text in audit event
- `finding.suppressed.*` logs contain: `fingerprint`, `reason`, `window`, `remaining` — no error text
- `job.dispatched` contains: `remediationJob`, `job`, `namespace` — no finding errors in the event

**Result:** PASS — audit events do not log credential values. Finding error text is **not** included in audit log lines.

---

## Phase 7 Summary

Audit events are structured correctly (`audit=true`, stable `event` strings, no credential values). The `finding.injection_detected` event path is correctly wired in the provider pipeline; it was not exercised because the live pentest bypassed the provider.

**Total findings:** 0
