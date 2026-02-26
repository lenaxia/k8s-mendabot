# Worklog 0058 — Epic 13 STORY_05: Integration Tests and DISABLE_CORRELATION Escape Hatch

**Date:** 2026-02-25
**Branch:** feature/epic13-multi-signal-correlation
**Story:** STORY_05_integration_escape_hatch.md

## Summary

Fixed gaps in the integration test file `internal/controller/correlation_integration_test.go`
identified via skeptical code review. All five test cases (TC-01 through TC-05) now fully
match the story acceptance criteria. Full test suite passes.

## Gaps Fixed

1. **TC-05 single-job gap**: Story requires two correlated jobs to verify the escape hatch
   in a multi-job scenario. Added `tc05-rjob2` (same `ParentObject` as rjob1) and verified
   both dispatch immediately without any window hold or grouping when `Correlator==nil`.

2. **TC-02 window-hold assertion missing**: Added an explicit `Reconcile()` call on rjob1
   *before* the window elapses and asserted `RequeueAfter > 0`. This verifies the window-hold
   path, not just the dispatch path. (Note: the blank-phase `"" → Pending` init call returns
   `Requeue:true`, not `RequeueAfter`; the window-hold check fires on the *subsequent* Pending
   reconcile before the window has elapsed.)

3. **TC-02 CorrelationGroupID cross-check**: Added assertion that the suppressed peer's
   `Status.CorrelationGroupID` equals the primary's `Labels[domain.CorrelationGroupIDLabel]`.

4. **TC-02b CorrelationGroupID cross-check**: Same cross-check added to TC-02b.

5. **RBAC comment wording**: Fixed file-level comment from
   `"pendingPeers list call is covered by the existing ClusterRole grant on remediationjobs"` to
   the exact required wording: `"pendingPeers calls r.List on RemediationJobs; covered by existing ClusterRole grant"`.

6. **SourceProvider comment wording**: Fixed test doc comment from
   `"Suppressed is handled by the existing != PhaseFailed check; no code change needed"` to
   `"Suppressed is handled by the existing default: case at provider.go:383; no code change needed"`.

## Unchanged (correctly implemented)

- TC-01: single finding, no correlation, dispatched without group label — correct
- TC-02: SameNamespaceParent primary dispatch + peer suppression — correct (plus gaps above)
- TC-02b: non-primary RequeueAfter:5s path — correct (plus CorrelationGroupID cross-check)
- TC-03: PVCPod correlation (PVC primary, Pod suppressed) — correct
- TC-04: no correlation across namespaces — correct
- TC-05: Correlator==nil escape hatch — extended with second job
- SourceProvider skip-Suppressed test — correct (comment wording fixed)

## Verification

- `go test -timeout 60s -race ./internal/controller/...` — PASS (15s)
- `go test -timeout 30s -race ./...` — all 17 packages PASS
- `go build ./...` — clean
- `go vet ./...` — clean
