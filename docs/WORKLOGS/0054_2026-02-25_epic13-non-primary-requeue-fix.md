# Worklog: Epic 13 — Non-Primary Requeue Bug Fix

**Date:** 2026-02-25
**Session:** Fix critical design bug: non-primary candidate was self-suppressing instead of requeueing
**Status:** Complete

---

## Objective

Fix a critical design bug in `internal/controller/remediationjob_controller.go` where a
non-primary candidate was calling `transitionSuppressed` on itself immediately (self-suppression).
This caused the primary to see no pending peers during its own reconcile, dispatching as a solo
job and permanently losing the correlated finding context.

The correct behavior per STORY_02 spec:
- Non-primary: return `ctrl.Result{RequeueAfter: 5 * time.Second}, nil` — stay Pending
- Primary: call `suppressCorrelatedPeers` (new helper) for ALL correlated peers still Pending, then dispatch

---

## Work Completed

### 1. TDD — Failing tests written first

Added two new tests to `internal/controller/remediationjob_controller_test.go`:

- `TestCorrelationWindow_NonPrimary_RequeuesAndStaysPending`: verifies non-primary candidate
  returns `RequeueAfter:5s` and remains `PhasePending`. Confirmed to fail before the fix.
- `TestCorrelationWindow_Primary_SuppressesPeersBeforeDispatch`: verifies primary calls
  `suppressCorrelatedPeers` (suppresses all CorrelatedUIDs peers), then dispatches.
  Confirmed to fail before the fix.

Removed and replaced `TestCorrelationWindow_SecondaryIsSuppressed` — this test was asserting
the buggy self-suppression behavior (`PhaseSuppressed` + zero `RequeueAfter`). The new
`TestCorrelationWindow_NonPrimary_RequeuesAndStaysPending` replaces it with correct assertions.

### 2. Implementation — `remediationjob_controller.go`

Two changes:

**Non-primary path** (line ~110): replaced:
```go
return ctrl.Result{}, r.transitionSuppressed(ctx, &rjob, group.GroupID, group.PrimaryUID)
```
with:
```go
return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
```

**Primary path** (added before status patch): inserted call to new `suppressCorrelatedPeers`
helper before the primary patches its own status and dispatches.

**New helper** `suppressCorrelatedPeers`: iterates `peers`, suppresses any whose UID appears
in `group.CorrelatedUIDs` via `transitionSuppressed`. Added between `pendingPeers` and `dispatch`.

### 3. Integration test fixes — `correlation_integration_test.go`

Three tests were asserting the old self-suppression model and were corrected:

- **`TestCorrelationIntegration_TC02_SameNamespaceParent`**: was asserting rjob2 dispatches
  independently after rjob1 dispatches. Now correctly asserts rjob2 is `PhaseSuppressed`
  (suppressed by rjob1 primary via `suppressCorrelatedPeers`).

- **`TestCorrelationIntegration_TC02b_SecondaryIsSuppressed`**: was asserting rjob2
  self-suppresses and rjob1 dispatches solo. Now correctly asserts: rjob2 gets
  `RequeueAfter:5s` + stays Pending on first reconcile; then rjob1 (primary) suppresses
  rjob2 and dispatches with full correlated findings.

- **`TestCorrelationIntegration_TC03_PVCPod`**: was asserting Pod self-suppresses first,
  then PVC dispatches as solo. Now correctly asserts: Pod gets `RequeueAfter:5s` + stays
  Pending; PVC primary suppresses Pod and dispatches with both findings.

---

## Key Decisions

- **Self-suppression was wrong because** `pendingPeers()` only includes `Phase==Pending`
  jobs. If the non-primary self-suppresses before the primary reconciles, the primary sees
  zero peers, calls `Correlator.Evaluate` with no peers (or no match), and dispatches as a
  solo job — permanently losing the correlated finding context.

- **`suppressCorrelatedPeers` uses `group.CorrelatedUIDs`** (populated by the correlator
  when rules return `MatchedUIDs`). When `CorrelatedUIDs` is nil (legacy fallback path
  with no `MatchedUIDs`), the helper does nothing — this is safe since the fallback path
  means the rule didn't precisely identify correlated peers.

- **The 5-second requeue** gives the primary sufficient time to reconcile and suppress.
  If the primary fails or never reconciles, the non-primary will keep requeueing every 5s
  and eventually dispatch solo when it sees no pending peers (after the primary moves to
  Dispatched and is excluded from `pendingPeers`).

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./internal/controller/...   # PASS
go test -timeout 60s -race ./...                        # ALL PASS (17 packages)
go build ./...                                          # CLEAN
```

---

## Next Steps

No follow-up work required for this fix. The correlation window design is now correct.

---

## Files Modified

- `internal/controller/remediationjob_controller.go` — non-primary path fix + `suppressCorrelatedPeers` helper
- `internal/controller/remediationjob_controller_test.go` — replaced `TestCorrelationWindow_SecondaryIsSuppressed` with two correct tests
- `internal/controller/correlation_integration_test.go` — corrected TC02, TC02b, TC03 to match new model
- `docs/WORKLOGS/0054_2026-02-25_epic13-non-primary-requeue-fix.md` — this file
- `docs/WORKLOGS/README.md` — index updated
