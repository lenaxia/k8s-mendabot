# Epic 19: Secret Redaction in Finding.Errors

**Feature Tracker:** FT-S1
**Area:** Security

## Status: SUPERSEDED by epic12

## Summary

This epic was opened to track secret redaction in `Finding.Errors`. Epic 12
(security review) fully delivered this feature as STORY_01_secret_redaction.

A gap analysis was conducted on 2026-02-23 (see [STORY_01_gap_analysis.md](STORY_01_gap_analysis.md)).
**No gaps were found.** This epic is closed as superseded.

## What epic12 delivered (verified by gap analysis)

### `internal/domain/redact.go`
`RedactSecrets(text string) string` applies 8 ordered regex patterns:
1. URL credentials (`scheme://user:pass@` or `scheme://:pass@`)
2. HTTP Bearer tokens (case-insensitive)
3. JSON `"password": "value"` key-value pairs
4. `password=value` / `password: value` assignments
5. `token=value` / `token: value` assignments
6. `secret=value` / `secret: value` assignments
7. `api-key=`, `api_key=`, `apikey=` assignments
8. Base64-like strings ≥ 40 characters

The implementation adds patterns 2 (Bearer) and 3 (JSON password) beyond the original
epic12 spec — both are validated by `redact_test.go`.

### Provider coverage — all 6 providers confirmed

| Provider | File | Call site |
|----------|------|-----------|
| Pod | `pod.go` | Lines 84, 98, 151 (terminated message, condition message, waiting text) |
| Deployment | `deployment.go` | Line 67 (DeploymentAvailable=False condition) |
| StatefulSet | `statefulset.go` | Line 71 (Available=False condition) |
| Job | `job.go` | Line 86 (JobFailed condition) |
| Node | `node.go` | Line 118 inside `buildNodeConditionText` (all condition types) |
| PVC | `pvc.go` | Line 61 (ProvisioningFailed event message) |

All six providers call `domain.RedactSecrets` on every free-text field before it
enters the `errors` slice. **No unredacted `.Message` fields in any provider.**

### Test coverage

`internal/domain/redact_test.go`: 20 table-driven test cases, exceeding the 8-case
minimum from the epic12 spec. Includes: 40/39-char base64 boundary, Redis
`redis://:pass@host` empty-username URL, JWT Bearer tokens, JSON password fields.

## No Action Required

This epic requires no implementation. All acceptance criteria are already satisfied
by epic12 STORY_01.

## Stories

| Story | File | Status |
|-------|------|--------|
| Gap analysis — verify epic12 STORY_01 completeness | [STORY_01_gap_analysis.md](STORY_01_gap_analysis.md) | **Complete — No Gaps Found** |
