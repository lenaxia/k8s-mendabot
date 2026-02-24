# Story 01: Gap Analysis — Secret Redaction Implementation

**Epic:** [epic19-secret-redaction](README.md)
**Priority:** High
**Status:** Complete — No Gaps Found

---

## Objective

Verify that epic12 STORY_01_secret_redaction was fully implemented. If gaps exist,
document them as tasks in this story. If no gaps exist, this story closes as
Superseded by epic12.

---

## Findings

### domain.RedactSecrets

**File:** `internal/domain/redact.go` — exists, 31 lines.

`RedactSecrets(text string) string` applies 8 ordered patterns via
`regexp.ReplaceAllString`:

| # | Pattern (source line) | Matches | Replacement |
|---|----------------------|---------|-------------|
| 1 | line 9: `(?i)://[^:@\s]*:[^@\s]+@` | URL credentials `scheme://user:pass@` or `scheme://:pass@` (empty username allowed) | `://[REDACTED]@` |
| 2 | line 10: `(?i)(bearer )\S+` | HTTP Bearer tokens (case-insensitive) | `bearer [REDACTED]` |
| 3 | line 11: `(?i)("password"\s*:\s*)"[^"]*"` | JSON `"password": "value"` key-value pairs | `"password": "[REDACTED]"` |
| 4 | line 12: `(?i)(password\s*[=:]\s*)\S+` | `password=value` or `password: value` assignments | `password=[REDACTED]` |
| 5 | line 13: `(?i)(token\s*[=:]\s*)\S+` | `token=value` or `token: value` assignments | `token=[REDACTED]` |
| 6 | line 14: `(?i)(secret\s*[=:]\s*)\S+` | `secret=value` or `secret: value` assignments | `secret=[REDACTED]` |
| 7 | line 15: `(?i)(api[_-]?key\s*[=:]\s*)\S+` | `api-key=`, `api_key=`, `apikey=` assignments | `api_key=[REDACTED]` |
| 8 | line 16: `[A-Za-z0-9+/]{40,}={0,2}` | Base64-like strings ≥ 40 characters | `[REDACTED-BASE64]` |

Note: The URL pattern at line 9 uses `[^:@\s]*` (zero-or-more) for the username
segment, which correctly handles the Redis `redis://:password@host` form. Epic12's
spec drafted `[^:@\s]+` (one-or-more); the implementation is more permissive and
correct.

No patterns are missing relative to epic12 STORY_01 acceptance criteria. The
implementation adds two patterns not in the original spec: Bearer token (pattern 2)
and JSON `"password":` (pattern 3), both validated by `redact_test.go` findings
010 and 011.

### Provider coverage

**`internal/provider/native/pod.go`**

- `RedactSecrets` called: **yes**
- Line 84: `domain.RedactSecrets(msg)` — applied to `cs.State.Terminated.Message`
  before inclusion in the terminated-exit-code error entry.
- Line 98: `domain.RedactSecrets(cond.Message)` — applied to the Unschedulable
  pod condition message.
- Line 151: `domain.RedactSecrets(msg)` inside `buildWaitingText` — applied to
  `cs.State.Waiting.Message` for all non-CrashLoopBackOff waiting failures.
- `buildCrashLoopText` (lines 133–142): uses only
  `cs.LastTerminationState.Terminated.Reason`, which is a Kubernetes enum value
  (e.g. `OOMKilled`), not a free-text field. No redaction required.
- Unredacted `.Message` fields: **none**.

**`internal/provider/native/deployment.go`**

- `RedactSecrets` called: **yes**
- Line 67: `domain.RedactSecrets(truncate(cond.Message, 500))` — applied to the
  `DeploymentAvailable=False` condition message.
- Unredacted `.Message` fields: **none**.

**`internal/provider/native/statefulset.go`**

- `RedactSecrets` called: **yes**
- Line 71: `domain.RedactSecrets(truncate(cond.Message, 500))` — applied to the
  `Available=False` condition message.
- Unredacted `.Message` fields: **none**.

**`internal/provider/native/job.go`**

- `RedactSecrets` called: **yes**
- Line 86: `domain.RedactSecrets(truncate(cond.Message, 500))` — applied to the
  `JobFailed` condition message.
- Line 79: `baseText` contains only `job.Name` and `job.Status.Failed` (integer).
  No free-text field. No redaction required.
- Unredacted `.Message` fields: **none**.

**`internal/provider/native/node.go`**

- `RedactSecrets` called: **yes**
- Line 118: inside `buildNodeConditionText`, `domain.RedactSecrets(truncate(cond.Message, 500))` — applied to every node condition message before being included in the error entry.
- All condition types (NodeReady, NodeMemoryPressure, NodeDiskPressure,
  NodePIDPressure, NodeNetworkUnavailable, and the default catch-all) funnel
  through `buildNodeConditionText`; all are therefore redacted.
- Unredacted `.Message` fields: **none**.

**`internal/provider/native/pvc.go`**

- `RedactSecrets` called: **yes**
- Line 61: `domain.RedactSecrets(truncate(eventMsg, 500))` — applied to the
  `ProvisioningFailed` event message returned by `latestProvisioningFailedMessage`.
- Unredacted `.Message` fields: **none**. Note: `ev.Message` is read at line 117
  via `failures[0].Message` and assigned to `eventMsg`; it is redacted at the
  call site (line 61) before serialisation.

### Test coverage

**File:** `internal/domain/redact_test.go` — exists, 135 lines.

`TestRedactSecrets` is table-driven with 20 test cases:

| Case | Pattern exercised |
|------|-------------------|
| `URL credentials postgres` | Pattern 1 — URL creds with username |
| `URL credentials https` | Pattern 1 — URL creds with GitHub token in URL |
| `password= assignment` | Pattern 4 — `password=` |
| `password: assignment with colon` | Pattern 4 — `password:` |
| `token= assignment` | Pattern 5 — `token=` |
| `secret= assignment` | Pattern 6 — `secret=` |
| `api-key= assignment` | Pattern 7 — `api-key=` |
| `api_key= assignment` | Pattern 7 — `api_key=` |
| `apikey= assignment` | Pattern 7 — `apikey=` |
| `base64 string exactly 40 chars` | Pattern 8 — boundary: exactly 40 chars redacted |
| `base64 string longer than 40 chars` | Pattern 8 — >40 chars redacted |
| `base64 string less than 40 chars not redacted` | Pattern 8 — short string not matched |
| `base64 string 39 chars not redacted` | Pattern 8 — boundary: 39 chars not matched |
| `clean text unchanged` | No pattern — `CrashLoopBackOff` passthrough |
| `empty string` | No pattern — empty input passthrough |
| `multiple patterns in one string` | Patterns 4 and 5 in same string |
| `finding 010: JWT bearer token uppercase` | Pattern 2 — `Bearer` token |
| `finding 010: JWT bearer token lowercase` | Pattern 2 — `bearer` token |
| `finding 011: JSON password no space` | Pattern 3 — JSON `"password":"value"` |
| `finding 011: JSON password with space after colon` | Pattern 3 — JSON `"password": "value"` |
| `finding 011: JSON password case-insensitive` | Pattern 3 — `"Password":"value"` |
| `finding 012: Redis URL with empty username` | Pattern 1 — `redis://:pass@host` |

Epic12 STORY_01 required coverage of: URL credentials, `password=`, `token=`,
`secret=`, `api-key=`, base64≥40, and clean text. All are covered. The actual
test suite covers 20 cases vs the 8 cases in the spec, including bearer tokens,
JSON password fields, Redis empty-username URLs, and the 39/40 char base64
boundary.

---

## Gaps Found

No gaps found. This epic is superseded by epic12 STORY_01.

All six native providers call `domain.RedactSecrets` on every free-text field
before it enters the `errors` slice. `internal/domain/redact.go` implements all
patterns required by epic12 plus two additional patterns (Bearer token, JSON
password). `internal/domain/redact_test.go` covers 20 cases, exceeding the
minimum required by the epic12 acceptance criteria.

---

## Tasks (if gaps found)

None. No remediation required.

---

## Definition of Done

- [x] All `.Message` fields in all 6 providers pass through `domain.RedactSecrets`
- [x] `go test -timeout 30s -race ./internal/domain/...` passes (20 test cases,
      all patterns covered)
- [x] `go test -timeout 30s -race ./internal/provider/native/...` passes
