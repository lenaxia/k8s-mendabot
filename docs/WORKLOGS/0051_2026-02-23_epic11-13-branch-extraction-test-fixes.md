# Worklog 0051 — Epic 11/13 Branch Extraction: Test Fixes and Docs

**Date:** 2026-02-23
**Branch:** `main`

## Summary

Completed the epic 11/13 branch extraction started in the previous session.
All epic 11 (self-remediation cascade prevention) and epic 13 (multi-signal
correlation) code was moved to `feature/epic11-13-deferred`. This session
fixed all failing test packages after the code removal.

## Changes

### Test files fixed

- `internal/config/config_test.go` — removed `TestFromEnv_ValidationSkipOption`
  and all `CorrelationWindow`, `DisableCorrelation`, `MultiPodThreshold` test
  functions (lines 521–686)

- `internal/controller/remediationjob_controller_test.go` — removed `correlator`
  and `domain` imports; removed `TestRemediationJobReconciler_Suppressed_ReturnsNil`,
  `correlationWindowCfg`, `newRJobCreatedAt`, `newReconcilerWithCorrelator`,
  `fakeCorrelatorRule`, and all `TestCorrelationWindow_*` test functions

- `internal/provider/native/job_test.go` — removed `SelfRemediationMaxDepth` from
  `newTestConfig()` helper; removed `sync`, `client`, `v1alpha1` imports; removed
  all `TestMendabotJob_*`, `TestNonMendabotJob_*`, `TestConcurrentChainDepthRace`,
  `TestAtomicChainDepthTracking`, `TestJobProvider_SelfRemediation*`, and related
  test functions (~750 lines)

- `internal/provider/provider_test.go` — removed `cascade`, `circuitbreaker`,
  `metrics`, `testutil`, `strings`, `record` imports; removed all
  `TestReconcile_SelfRemediation_*`, `TestReconcile_MaxDepthExceeded_*`,
  `TestReconcile_CircuitBreakerBlocked_*`, `TestReconcile_DeepCascade_*`,
  `TestReconcile_InfrastructureCascadeSuppressed_*`, `TestReconcile_NilEventRecorder_*`,
  `TestReconcile_DisableCascadeCheck_*` test functions and the `fakeCascadeChecker` type

- `internal/jobbuilder/job_test.go` — removed `domain` import; removed
  `TestBuild_CorrelationGroupID_*` test functions (3 tests)

### New file

- `internal/provider/export_test.go` — added `SetFirstSeenForTest` and `FirstSeen`
  helper methods on `SourceProviderReconciler` to support the surviving stabilisation
  window tests (which pre-existed the epic 11/13 work)

### Docs updated

- `docs/BACKLOG/epic11-self-remediation-cascade/README.md` — status changed to
  "Deferred — moved to `feature/epic11-13-deferred`"
- `docs/BACKLOG/epic13-multi-signal-correlation/README.md` — status changed to
  "Deferred — moved to `feature/epic11-13-deferred`"
- `README-LLM.md` — added `feature/epic11-13-deferred` to the Active Branches table

## Test result

`go test ./...` passes cleanly across all 12 packages.
