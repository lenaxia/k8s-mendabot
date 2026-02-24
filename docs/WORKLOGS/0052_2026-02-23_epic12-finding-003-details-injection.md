# Worklog: Fix finding 2026-02-23-003 — Details injection detection and prompt envelope

**Date:** 2026-02-23
**Session:** Security remediation: add DetectInjection check on finding.Details and untrusted-data envelope in prompt
**Status:** Complete

---

## Objective

Fix security finding 2026-02-23-003: `finding.Details` was passed to the agent prompt without
being checked by `domain.DetectInjection`, and was rendered in the prompt template without an
untrusted-input envelope.

Two parts:
- Part A: add `domain.DetectInjection(finding.Details)` check in `internal/provider/provider.go`
  with the same log/suppress logic used for `finding.Errors`
- Part B: wrap `${FINDING_DETAILS}` in `deploy/kustomize/configmap-prompt.yaml` with the same
  BEGIN/END envelope used for `${FINDING_ERRORS}`, and update HARD RULE 8 to cover both fields

---

## Work Completed

### 1. TDD — wrote failing tests first (`internal/provider/provider_test.go`)

Added four new tests to cover the `finding.Details` injection path:

- `TestReconcile_DetailsInjection_LogsEvent` — verifies audit log entry with
  `event=finding.injection_detected_in_details` when Details contains injection text
- `TestReconcile_DetailsInjection_Suppresses` — verifies no RemediationJob created and log
  entry present when `InjectionDetectionAction="suppress"` and Details contains injection text
- `TestReconcile_DetailsInjection_CleanDetails_NoEvent` — verifies no spurious log entry for
  benign Details text
- `TestReconcile_DetailsInjection_NilLogger_NoPanic` — verifies nil Log does not panic

Used `go.uber.org/zap/zaptest/observer` (already available in the module cache via `go.uber.org/zap v1.27.0`) to capture and assert on structured log entries.

All four tests failed before implementation (confirmed with `-race`).

### 2. Implemented Part A (`internal/provider/provider.go`)

Added a second injection check block immediately after the existing `finding.Errors` block
(lines 134–147 after patch), using:
- log message: `"potential prompt injection detected in finding details"`
- event field: `"finding.injection_detected_in_details"` (distinct from `"finding.injection_detected"`)
- same `InjectionDetectionAction == "suppress"` early-return logic

### 3. Implemented Part B (`deploy/kustomize/configmap-prompt.yaml`)

- Wrapped `${FINDING_DETAILS}` with the same BEGIN/END envelope pattern used for
  `${FINDING_ERRORS}`
- Updated HARD RULE 8 to cover both the FINDING ERRORS and FINDING DETAILS blocks

---

## Key Decisions

- Event field name `finding.injection_detected_in_details` kept distinct from
  `finding.injection_detected` so operators can filter audit logs by field type.
- `zaptest/observer` used for log assertions rather than log-to-buffer hacks — this is
  the idiomatic zap testing approach and avoids test coupling to log formatting.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race -run "TestReconcile_DetailsInjection" ./internal/provider/...
# FAIL before implementation (4 tests failed)

go test -timeout 30s -race -run "TestReconcile_DetailsInjection" ./internal/provider/...
# ok after implementation (4 tests pass)

go test -timeout 30s -race ./...
# All packages PASS
```

---

## Next Steps

Finding 2026-02-23-003 is fully remediated. The security checklist item
`domain.DetectInjection called on finding.Details` is now satisfied.

---

## Files Modified

- `internal/provider/provider.go` — added DetectInjection check for finding.Details
- `internal/provider/provider_test.go` — added 4 new TDD tests for Details injection
- `deploy/kustomize/configmap-prompt.yaml` — added FINDING DETAILS envelope and updated HARD RULE 8
- `docs/WORKLOGS/README.md` — added this entry
- `docs/WORKLOGS/0052_2026-02-23_epic12-finding-003-details-injection.md` — this file
