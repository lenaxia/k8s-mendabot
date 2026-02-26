# Worklog: Epic 13 — AllFindings primary exclusion and ordering gap fixes

**Date:** 2026-02-25
**Session:** Fix 3 gaps: AllFindings includes primary's own finding, correlation block runs before owned-jobs sync, test asserts wrong behaviour
**Status:** Complete

---

## Objective

Fix 3 specific gaps identified in a delegation review of `feature/epic13-multi-signal-correlation`:

1. **GAP 1 (CRITICAL)**: `AllFindings` in `CorrelationGroup` included the primary's own finding, duplicating it at dispatch time since it is already present in `rjob.Spec.Finding`.
2. **GAP 2 (MEDIUM)**: The correlation window hold block ran BEFORE the owned-jobs sync block, causing a rjob with an active batch/v1 Job to return a window-hold requeue instead of syncing its status.
3. **GAP 3 (HIGH)**: `TestCorrelationWindow_PrimaryIsDispatched` asserted the primary's own finding IS in `CorrelatedFindings` — the exact opposite of the correct behaviour.

---

## Work Completed

### 1. TDD — Wrote failing tests FIRST

- `TestCorrelator_AllFindings_ExcludesPrimaryFinding` (correlator_test.go): asserts that when candidate is primary, its finding is NOT in AllFindings; only the peer's finding is present. Uses `MatchedUIDs` path. Confirmed failing before fix.
- `TestCorrelator_AllFindings_ExcludesPrimaryFinding_FallbackPath` (correlator_test.go): same assertion for the fallback path (no MatchedUIDs). Confirmed failing.
- `TestReconcile_WindowHold_DoesNotRunBeforeOwnedJobsSync` (controller test): rjob freshly created (within window) with an existing active batch/v1 Job. Reconcile must sync to PhaseRunning, not return a window-hold requeue. Confirmed failing before fix.
- Fixed GAP 3 assertion in `TestCorrelationWindow_PrimaryIsDispatched`: inverted TC-3a from "primary's finding IS present" to "primary's finding must NOT be present". Confirmed failing before fix.

### 2. GAP 1 Fix — `correlator.go` `Evaluate` method

Both code paths in the AllFindings building logic now filter out the primary's finding:

- **MatchedUIDs path**: `if matchedSet[candidate.UID] && candidate.UID != result.PrimaryUID` (was `if matchedSet[candidate.UID]`); same filter on peers `if matchedSet[p.UID] && p.UID != result.PrimaryUID`.
- **Fallback path**: Added `if candidate.UID != result.PrimaryUID` guard before appending candidate; added `if p.UID != result.PrimaryUID` guard before appending each peer.

### 3. GAP 2 Fix — `remediationjob_controller.go` Reconcile ordering

Moved the owned-jobs list/sync block (previously "Step 3", lines ~145–199) to run BEFORE the correlation window hold block. The correlation block now runs AFTER the owned-jobs sync, so jobs with an existing batch/v1 Job always get their status synced immediately without waiting for the correlation window.

New order in Reconcile:
1. Phase initialisation (blank → Pending)
2. Terminal phase switch (Succeeded/Failed/Cancelled/Suppressed)
3. **Owned-jobs sync** (list batch/v1 Jobs by label, sync phase from Job)
4. **Correlation window hold + correlator Evaluate** (moved here)
5. MAX_CONCURRENT_JOBS check
6. dispatch(nil)

### 4. Cascading test fixes required by GAP 1

`TestCorrelator_AllFindings_PopulatedOnMatch` previously asserted the candidate's (primary's) finding was in AllFindings. Updated to assert only the peer's finding is present and the primary's finding is absent.

`TestCorrelator_MultiPodSameNodeRule_AllFindingsOnlyMatchedNode` previously expected 3 findings (pod1 as primary + pod2 + pod3). After fix, pod1 (primary) is excluded: updated expected count from 3 to 2, added assertion that pod1 is not in AllFindings.

---

## Key Decisions

- **AllFindings = non-primary findings only**: The primary's finding is already available at `rjob.Spec.Finding` in `JobBuilder.Build()`. Including it in AllFindings duplicates data and would cause the agent to see the primary finding twice. AllFindings is the "additional context" passed alongside the primary finding, not a replacement for it.
- **Owned-jobs sync before correlation block**: A rjob that has already dispatched a batch/v1 Job should never be held by the correlation window. The window hold is for pre-dispatch correlation only. Moving owned-jobs sync earlier is the correct fix without changing any other logic.
- **Fallback path also needs the primary exclusion**: The fallback path (MatchedUIDs=nil) is a backward-compat path for stub/test rules. Even there, the primary's finding must not appear in AllFindings to maintain the invariant.

---

## Blockers

None.

---

## Tests Run

```
go build ./...
go test -timeout 60s -race -count=1 ./internal/correlator/... ./internal/controller/...
go test -timeout 60s -race -count=1 ./...
```

All 17 packages pass with zero failures.

---

## Next Steps

- Verify no remaining gaps in the epic13 branch before merge.

---

## Files Modified

- `internal/correlator/correlator.go` — GAP 1: AllFindings excludes primary in both code paths
- `internal/correlator/correlator_test.go` — Updated 2 existing tests + added 2 new TDD tests
- `internal/controller/remediationjob_controller.go` — GAP 2: moved owned-jobs sync before correlation block
- `internal/controller/remediationjob_controller_test.go` — GAP 3: inverted wrong assertion + added new ordering test
- `docs/WORKLOGS/0055_2026-02-25_epic13-allfindings-ordering-gaps.md` — this file
- `docs/WORKLOGS/README.md` — index updated
