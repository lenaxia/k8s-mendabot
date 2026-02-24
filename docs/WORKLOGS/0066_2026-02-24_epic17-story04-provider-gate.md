# Worklog: Epic 17 Story 04 — SourceProviderReconciler PermanentlyFailed Gate

**Date:** 2026-02-24
**Session:** Implemented STORY_04: switch-based dedup loop + MaxRetries population in provider.go
**Status:** Complete

---

## Objective

Refactor the fingerprint-dedup loop in `SourceProviderReconciler` to an explicit
`switch` on `rjob.Status.Phase`, adding a `PhasePermanentlyFailed` case that emits
an audit log and returns without deleting the tombstone object. Also populate
`MaxRetries` from `r.Cfg.MaxInvestigationRetries` when creating a new `RemediationJob`.

---

## Work Completed

### 1. TDD — Three new tests written first

Added to `internal/provider/provider_test.go`:

- `TestSourceProviderReconciler_PermanentlyFailed_Suppressed` — verifies the tombstone
  is not deleted, no new job is created, and the audit log event
  `remediationjob.permanently_failed_suppressed` is emitted.
- `TestSourceProviderReconciler_PhaseFailed_DeletesAndCreatesNew` — verifies the
  existing PhaseFailed delete-and-recreate behaviour is unchanged after the switch refactor.
- `TestSourceProviderReconciler_MaxRetries_PopulatedFromConfig` — verifies that newly
  created RemediationJobs have `MaxRetries == Cfg.MaxInvestigationRetries`.

Also added `newObserverInfoLogger()` helper (captures `Info`-level logs; the existing
`newObserverLogger` only captures `Warn` and above).

### 2. Implementation — provider.go

**Change 1 (lines 190–211):** Replaced the if-based dedup loop body with a `switch`
on `rjob.Status.Phase`:
- `case PhasePermanentlyFailed`: emit audit Info log, return nil (no delete).
- `case PhaseFailed`: delete rjob (existing behaviour, unchanged).
- `default`: return nil (dedup — active/completed job exists; unchanged).

**Change 2 (line ~261):** Added `MaxRetries: r.Cfg.MaxInvestigationRetries` to the
`RemediationJobSpec` literal at object creation time.

### 3. Test deviations from story spec

The story's `TestSourceProviderReconciler_PhaseFailed_DeletesAndCreatesNew` uses a
`c.Get` to assert the old rjob is gone. This is incorrect because the reconciler
immediately creates a replacement with the same name, so `Get` succeeds and the test
would always fail. Fixed by checking the list count and phase instead — matching the
pattern of the existing `TestSourceProviderReconciler_ReDispatchesFailedRemediationJob`.

The story's `TestSourceProviderReconciler_PermanentlyFailed_Suppressed` used the generic
`newTestReconciler` (no logger), which cannot capture the audit log. Updated to construct
the reconciler with `newObserverInfoLogger()` and added a log assertion.

Both adaptations are correct and faithful to the story's intent.

---

## Key Decisions

- Used `zapcore.InfoLevel` for the new observer logger helper so audit log entries
  (logged at `Info`) are captured. The existing `newObserverLogger` uses `WarnLevel`
  which is appropriate for injection-detection tests only.
- The `PhaseFailed_DeletesAndCreatesNew` test validates the net-count and phase rather
  than a negative `Get` because the fake client persists the recreated object under the
  same name.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/provider/...   → PASS (all tests)
go vet ./internal/provider/...                        → clean
go build ./...                                        → clean
```

---

## Next Steps

STORY_04 is complete. All epic17 stories (01–04) are now implemented. The orchestrator
should run the full suite `go test -timeout 30s -race ./...` and commit.

---

## Files Modified

- `internal/provider/provider.go` — switch refactor + MaxRetries field
- `internal/provider/provider_test.go` — three new tests + newObserverInfoLogger helper
