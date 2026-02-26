# Worklog 0059 — Epic 13 Round-5 Skeptical Validator Fixes

**Date:** 2026-02-26
**Branch:** feature/epic13-multi-signal-correlation
**Commit:** 18acf63

## Summary

Ran a final skeptical validator pass on all Epic 13 files. Found 8 real defects
(3 HIGH, 4 MEDIUM, 1 LOW). All fixed in this session with 7 new tests added.

---

## Defects Fixed

### HIGH: GroupID idempotency ordering bug (DEFECT 19/20)

**File:** `internal/controller/remediationjob_controller.go:200-238`

**Bug:** `suppressCorrelatedPeers` was called BEFORE the GroupID idempotency
guard. On retry (when `rjob.Status.CorrelationGroupID` was already set from a prior
run), the correlator generated a new GroupID, peers were suppressed with that new
GroupID, and then the idempotency guard overwrote `group.GroupID` with the original
stable ID. Result: the primary dispatch used the original GroupID but suppressed peers
had a different GroupID. The recovery path looks for suppressed peers by
`CorrelationGroupID == primary.Status.CorrelationGroupID` and finds none → dispatches
solo → permanently loses correlated context.

**Fix:** Moved the idempotency guard to run BEFORE `suppressCorrelatedPeers`. Peers
now always receive the stable GroupID from the first successful run.

**New test:** `TestCorrelation_GroupID_IdempotencyBeforeSuppress` asserts that after a
retry, the peer's `Status.CorrelationGroupID` equals the original stable ID (not the
newly generated one).

---

### HIGH: PVCPodRule silent error swallow (DEFECTS 1 & 2)

**File:** `internal/correlator/rules.go:183-184` and `rules.go:253-256`

**Bug:** Both `evaluatePodCandidate` and `evaluatePVCCandidate` caught ALL `client.Get`
errors and silently returned `Matched=false, nil`. A transient etcd timeout or RBAC
denial was indistinguishable from NotFound. This caused API blips to permanently
miss correlation (the controller would dispatch as a solo job and never retry the
correlation).

**Fix:** Both now check `apierrors.IsNotFound(err)`:
- `IsNotFound` → non-fatal miss (pod is gone), return `Matched=false, nil`
- Other errors → propagate the error so the controller requeues

**New tests:**
- `TestPVCPodRule_CandidatePod_ClientGetGenericError_PropagatesError`
- `TestPVCPodRule_CandidatePod_ClientGetNotFound_NoError`
- `TestPVCPodRule_CandidatePVC_PeerPod_ClientGetGenericError_PropagatesError`
- `TestPVCPodRule_CandidatePVC_PeerPod_ClientGetNotFound_NoError`

Updated `TestPVCPodRule_CandidatePod_ClientGetGenericError_NoMatch` (renamed and
inverted — it now correctly asserts that the error IS propagated).

---

### HIGH: PhaseDispatched/Running fall-through (DEFECT 12)

**File:** `internal/controller/remediationjob_controller.go:91-109`

**Bug:** The `PhaseDispatched`/`PhaseRunning` switch case had no `return` statement.
When the owned `batch/v1` Job was GC'd (zero ownedJobs) AND the correlator was nil,
execution fell through to the dispatch block at the bottom of the function and created
a second Job. This was only non-fatal because `dispatch()` handled `AlreadyExists` via
the deterministic job name, but it was an unintentional reliance on side-effects.

**Fix:** Added an explicit guard after the owned-jobs sync block:
```go
if rjob.Status.Phase == v1alpha1.PhaseDispatched || rjob.Status.Phase == v1alpha1.PhaseRunning {
    return ctrl.Result{}, nil
}
```
Simplified the switch comment to reflect the new code structure.

**New test:** `TestRemediationJobReconciler_Dispatched_OwnedJobGCd_NilCorrelator_NoDoubleDispatch`
verifies that no `Build()` call is made and phase remains `Dispatched`.

---

### MEDIUM: Contradictory zero-threshold contracts (DEFECTS 4 & 36)

**File:** `internal/correlator/rules.go:340-348` and `cmd/watcher/main.go:177-179`

**Bug:** `MultiPodSameNodeRule` had a silent default of 3 when `Threshold <= 0`. But
`buildCorrelator` in `main.go` panicked when `MultiPodThreshold <= 0`. These are
contradictory contracts: the rule's silent default made the panic dead code, while the
panic made the silent default untestable. Tests using `MultiPodSameNodeRule{}` would
silently get threshold=3 instead of an error, masking misconfiguration.

**Fix:**
- `MultiPodSameNodeRule.Evaluate` now returns `fmt.Errorf(...)` for `Threshold <= 0`
- `buildCorrelator` now returns `(*Correlator, error)` instead of panicking
- Updated `TestMultiPodSameNodeRule_ZeroThreshold_UsesDefault` → `_ReturnsError`

---

### MEDIUM: Hardcoded annotation string in correlator_test.go (DEFECT 9)

**File:** `internal/correlator/correlator_test.go:272,284,296,308,320`

**Bug:** Five test cases used the raw string `"mendabot.io/node-name"` instead of
`domain.NodeNameAnnotation`. If the constant is ever renamed, these tests would silently
pass (the annotation key wouldn't match; rule returns no match; test expects no match).

**Fix:** Replaced all five occurrences with `domain.NodeNameAnnotation`.

---

### LOW: DisableCorrelation emits no log (DEFECT 37)

**File:** `cmd/watcher/main.go`

**Fix:** Added a log line at the call site: `logger.Info("multi-signal correlation disabled (DISABLE_CORRELATION=true)")` when `buildCorrelator` returns nil.

---

### LOW: zapr.New() drops controller-runtime internal log events (DEFECT 38)

**File:** `cmd/watcher/main.go`

**Bug:** `ctrl.SetLogger(zapr.New())` used the controller-runtime zapr package's
`New()` with no options, creating a no-op production logger. This dropped all
controller-runtime internal log events (leader election, reconcile error propagation).

**Fix:** Replaced with `ctrl.SetLogger(gozapr.NewLogger(logger))` using the
`github.com/go-logr/zapr` package to wrap the configured `*zap.Logger`.

---

## Dismissed / Out of Scope

| Defect | Disposition |
|--------|-------------|
| DEFECT 7/8 (correlator.go CorrelatedUIDs/AllFindings source-of-truth) | LOW, undocumented but not an active bug — added comment |
| DEFECT 10 (missing test: candidate UID not in MatchedUIDs) | LOW, degenerate case not reachable by any production rule today |
| DEFECT 13 (AgentNamespace assumption in recovery path) | MEDIUM/DESIGN — documented constraint, not a new bug |
| DEFECT 14 (duplicated active-job counting logic) | MEDIUM — code smell, correctness unaffected |
| DEFECT 15 (PhaseSucceeded + nil CompletedAt) | LOW — status patch is atomic, window is minimal |
| DEFECT 18 (hardcoded 5s requeue) | LOW / operational, not a correctness issue |
| DEFECT 25 (12-char fingerprint collision) | MEDIUM — design trade-off, acknowledged in comments |
| DEFECT 26/27/28/29/30 (job.go LOW issues) | LOW / design-level, no action needed |
| DEFECT 32 (flaky timing assertion) | LOW — timing guard was already addressed in round 3 |
| DEFECT 34/35 (suite_test.go operational) | LOW — portability/operational, not correctness |

---

## Verification

```
go build ./...                             → clean (17 packages)
go test -timeout 60s -race ./...          → all 17 packages PASS
go vet ./...                              → clean
```

---

## Next

Epic 12 — Security Review (Not Started). STORY_00 → STORY_01 (secret redaction, CRITICAL) and STORY_05 (prompt injection, CRITICAL) should run first.
