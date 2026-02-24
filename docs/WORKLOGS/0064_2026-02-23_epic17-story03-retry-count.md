# Worklog: Epic 17 STORY_03 — RemediationJobReconciler RetryCount + PermanentlyFailed

**Date:** 2026-02-23
**Session:** Implement RetryCount increment, PermanentlyFailed cap, and terminal-phase switch addition in RemediationJobReconciler
**Status:** Complete

---

## Objective

Implement STORY_03 of epic17-dead-letter-queue:
- Increment `Status.RetryCount` each time a `batch/v1 Job` transitions to `PhaseFailed`
- Set `Phase = PermanentlyFailed` when `RetryCount >= MaxRetries` after increment
- Add `PhasePermanentlyFailed` to the terminal-phase switch (no-dispatch short-circuit)
- Emit `job.permanently_failed` audit log event when permanently tombstoning
- All via TDD: tests written and confirmed failing before implementation

---

## Work Completed

### 1. TDD — Tests written first

Added five new test functions to `internal/controller/remediationjob_controller_test.go`:

- `TestRemediationJobReconciler_PhaseFailed_IncrementsRetryCount` — verifies RetryCount goes
  from 0 to 1 when a job transitions to Failed, and phase stays PhaseFailed (below cap)
- `TestRemediationJobReconciler_PhaseFailed_AtCap_PermanentlyFails` — verifies RetryCount 2→3
  at MaxRetries=3 transitions phase to PermanentlyFailed
- `TestRemediationJobReconciler_RetryCount_Idempotent` — verifies re-reconciling an already-Failed
  rjob does NOT increment RetryCount (the terminal-switch short-circuits before the sync block)
- `TestRemediationJobReconciler_PermanentlyFailed_ReturnsNil` — verifies PermanentlyFailed phase
  returns immediately with no Build() calls
- `TestRemediationJobReconciler_TerminalPhases_NoBuild` — table-driven test covering all three
  terminal-no-dispatch phases: PhaseFailed, PhaseCancelled, PhasePermanentlyFailed

All tests confirmed failing before implementation.

### 2. Controller changes

**Change 1 — terminal-phase switch** (`remediationjob_controller.go` lines 85–93):
- Added `case v1alpha1.PhasePermanentlyFailed: return ctrl.Result{}, nil` between
  PhaseFailed and PhaseCancelled cases

**Change 2 — RetryCount + cap logic** (lines 113–147):
- Replaced simple `condType` selection with a PhaseFailed sub-block
- Guard: `if rjobCopy.Status.Phase != v1alpha1.PhaseFailed { rjob.Status.RetryCount++ }`
  — uses `rjobCopy` (pre-mutation snapshot) to detect genuine first-time transition,
  because `rjob.Status.Phase` has already been set to `newPhase` at line 107
- Fallback: `if maxRetries <= 0 { maxRetries = 3 }`
- Cap check: `if rjob.Status.RetryCount >= maxRetries` → set PhasePermanentlyFailed +
  ConditionPermanentlyFailed condition with reason "RetryCapReached"
- Otherwise: set ConditionJobFailed as before

**Change 3 — audit log extension** (lines 155–186):
- Replaced simple `event` string selection with a `switch` block
- Added `case rjob.Status.Phase == v1alpha1.PhasePermanentlyFailed:` emitting
  event `"job.permanently_failed"` with retryCount and maxRetries fields
- The PermanentlyFailed case uses the post-mutation `rjob.Status.Phase` (which has been
  updated to PermanentlyFailed if the cap was reached), while `newPhase` remains PhaseFailed
  — this is correct ordering for the audit log switch

### 3. Key correctness decision

The idempotency guard must compare `rjobCopy.Status.Phase` (the pre-mutation copy), not
`rjob.Status.Phase`. At line 107, `rjob.Status.Phase = newPhase` runs before the guard.
If we had used `rjob.Status.Phase`, the guard would always be false (Phase was just set to
PhaseFailed = newPhase). Using `rjobCopy.Status.Phase` correctly captures the phase
*before* this reconcile's mutations.

---

## Key Decisions

1. **Use `rjobCopy.Status.Phase` for idempotency guard** — necessary because `rjob.Status.Phase`
   is mutated before the guard check. Story spec says "pre-patch copy", and `rjobCopy` is
   captured at line 106 before any mutation. This is the correct implementation.

2. **Audit log switch uses `rjob.Status.Phase` post-mutation** — after the cap check,
   `rjob.Status.Phase` is either PermanentlyFailed (capped) or still PhaseFailed (below cap).
   The switch discriminates correctly: PermanentlyFailed first, then PhaseFailed for below-cap.

3. **`newPhase` remains PhaseFailed throughout** — `syncPhaseFromJob` always returns
   PhaseFailed when the job has failed. The controller re-maps it to PermanentlyFailed
   internally. The audit log switch must use `rjob.Status.Phase` (post-remap), not `newPhase`.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/controller/...
```

Result: PASS — 33 tests (including 5 new), 0 failures, race detector clean.

```
go build ./...    → BUILD OK
go vet ./internal/controller/...    → VET OK
```

---

## Next Steps

STORY_03 is complete. The next story in epic17 is STORY_04: gate in `SourceProviderReconciler`
to skip re-dispatch when an rjob is in `PhasePermanentlyFailed`. STORY_04 is now unblocked.

---

## Files Modified

- `internal/controller/remediationjob_controller.go` — three changes: terminal switch, retry logic, audit log
- `internal/controller/remediationjob_controller_test.go` — five new test functions
- `docs/BACKLOG/epic17-dead-letter-queue/STORY_03_reconciler_retry_count.md` — status updated to Complete
- `docs/WORKLOGS/0064_2026-02-23_epic17-story03-retry-count.md` — this file
- `docs/WORKLOGS/README.md` — index updated
