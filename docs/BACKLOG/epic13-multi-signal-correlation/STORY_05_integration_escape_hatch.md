# Story 05: Integration Tests and DISABLE_CORRELATION Escape Hatch

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot developer**, I want end-to-end integration tests that exercise the full
correlation path from `RemediationJob` creation through window hold, correlator
evaluation, and `Suppressed` phase transition, and I want a `DISABLE_CORRELATION=true`
escape hatch that restores the pre-epic dispatch behaviour completely.

---

## Background

Unit tests in STORY_01 and STORY_02 cover individual components. This story adds
integration tests using `envtest` that exercise the full flow: multiple `RemediationJob`
objects created in the same namespace, the window hold, the correlator running, and the
correct phase transitions resulting.

The escape hatch is critical for operators who hit unexpected correlation behaviour in
production and need an immediate rollback without re-deploying a different binary.

---

## Acceptance Criteria

- [ ] Integration tests for correlation exist in `internal/controller/` (either extending
      `suite_test.go` or in a new `correlation_integration_test.go` in the same package):
  - **TC-01 — No correlation, single finding:** one `RemediationJob` created, window
    elapses, job dispatched without `CorrelationGroupID`
  - **TC-02 — SameNamespaceParent correlation:** two jobs with matching parent prefix
    created, window elapses, candidate that is primary suppresses peer and dispatches
    with group label; peer transitions to `Suppressed`
  - **TC-03 — PVCPod correlation:** PVC finding + Pod finding in same namespace with
    matching volume reference, window elapses, PVC job is primary, Pod job `Suppressed`.
    Note: `AllFindings` will contain exactly one entry (the pod's finding); no sort is
    needed for two-job groups, but use sorted comparison if the group size can vary.
  - **TC-04 — No correlation across namespaces:** two jobs with identical parent names
    but different namespaces, each dispatched independently (no group)
  - **TC-05 — DISABLE_CORRELATION=true:** two correlated jobs, escape hatch enabled,
    both dispatched immediately without any hold or grouping
- [ ] `DISABLE_CORRELATION=true` skips the window requeue and calls no correlator code
- [ ] `go test -timeout 60s -race ./internal/controller/...` passes (60s timeout because
      tests use a 1s window and need margin for the two-reconcile pattern)
- [ ] `go test -timeout 30s -race ./...` passes for all non-envtest packages

---

## Technical Implementation

### Test configuration

Set `cfg.CorrelationWindowSeconds = 1` on the reconciler's `Cfg` directly in each
test. The envtest suite instantiates the reconciler manually (see
`internal/controller/suite_test.go`) — inject config by constructing the reconciler with
the desired `Cfg` value before each test case.

### Critical: tests must call `Reconcile()` twice

The existing envtest pattern calls `rec.Reconcile(ctx, req)` directly and synchronously
— there is no background controller loop. The `ctrl.Result{RequeueAfter: ...}` returned
by the window hold is not automatically acted upon by a manager. Tests must replicate the
requeue manually:

```go
// TC-02 pattern: two-call test (candidate that IS the primary)
ctx := context.Background()
// First call: window not elapsed → expect RequeueAfter, job still Pending
result, err := rec.Reconcile(ctx, ctrlReq(primary.Name, primary.Namespace))
require.NoError(t, err)
require.Greater(t, result.RequeueAfter, time.Duration(0))

// Wait for window to elapse (1.1× the 1s window)
time.Sleep(1100 * time.Millisecond)

// Second call: window elapsed → primary suppresses peer and dispatches
_, err = rec.Reconcile(ctx, ctrlReq(primary.Name, primary.Namespace))
require.NoError(t, err)

// Primary is dispatched
var updatedPrimary v1alpha1.RemediationJob
require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(primary), &updatedPrimary))
require.Equal(t, v1alpha1.PhaseDispatched, updatedPrimary.Status.Phase)

// Peer is Suppressed (set by the primary's reconcile call)
var updatedPeer v1alpha1.RemediationJob
require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(peer), &updatedPeer))
require.Equal(t, v1alpha1.PhaseSuppressed, updatedPeer.Status.Phase)
require.Equal(t, updatedPrimary.Labels[domain.CorrelationGroupIDLabel],
    updatedPeer.Status.CorrelationGroupID)

// AllFindings ordering from client.List is non-deterministic.
// When asserting on FINDING_CORRELATED_FINDINGS env var content, sort both the
// expected and actual FindingSpec slices (e.g. by finding.Name) before comparing.
```

For the non-primary candidate case (TC-02 variant), test that the non-primary's first
post-window reconcile returns `RequeueAfter: 5s` and the job stays Pending:

```go
// Non-primary candidate's post-window reconcile returns RequeueAfter
result, err := rec.Reconcile(ctx, ctrlReq(nonPrimary.Name, nonPrimary.Namespace))
require.NoError(t, err)
require.Equal(t, 5*time.Second, result.RequeueAfter,
    "non-primary should requeue, not self-suppress")
var unchanged v1alpha1.RemediationJob
require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(nonPrimary), &unchanged))
require.Equal(t, v1alpha1.PhasePending, unchanged.Status.Phase,
    "non-primary must remain Pending until primary suppresses it")
```

### TC-05 escape hatch verification

Set `r.Correlator = nil` directly on the reconciler before the test. With no correlator,
a single call to `Reconcile()` must dispatch the job without any `RequeueAfter`:

```go
rec.Correlator = nil
result, err := rec.Reconcile(ctx, ctrlReq(rjob.Name, rjob.Namespace))
require.NoError(t, err)
require.Zero(t, result.RequeueAfter, "expected immediate dispatch with no correlation hold")
var dispatched v1alpha1.RemediationJob
require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(rjob), &dispatched))
require.Equal(t, v1alpha1.PhaseDispatched, dispatched.Status.Phase)
```

### `SourceProviderReconciler` treatment of `Suppressed`

The existing dedup logic in `internal/provider/provider.go` uses a `switch` statement
(starting at line 364) with cases for `PhasePermanentlyFailed` and `PhaseFailed`.
Any phase other than those two falls into the `default:` case at line 383, which logs
a duplicate-suppressed event and returns early. `PhaseSuppressed` lands in `default:`
and is correctly blocked from re-dispatch. **No code change is required here.**

Verify this by writing a test: create a `RemediationJob` with `Phase=Suppressed`,
reconcile the source provider, and assert no new `RemediationJob` is created. Add a
comment in the test: `// Suppressed is handled by the existing default: case at
provider.go:383; no code change needed`.

### CRD testdata

The envtest suite loads CRDs from `testdata/crds/remediationjob_crd.yaml`
(see `internal/controller/suite_test.go` line 41: `CRDDirectoryPaths: []string{"../../testdata/crds"}`).
STORY_00 updates this file to add `Suppressed` to the phase enum and add
`correlationGroupID` to the status schema. These changes must be present before the
integration tests run, or the API server will reject `Phase=Suppressed` status patches
and silently strip `CorrelationGroupID` from responses.

### RBAC

The `Correlator` calls `r.List` on `RemediationJob` objects in `r.Cfg.AgentNamespace`
via `pendingPeers`. The Helm chart's ClusterRole for the watcher already includes `list`
on `remediationjobs` — no new RBAC changes are required. Add a comment in the integration
test file noting this:
`// pendingPeers calls r.List on RemediationJobs; covered by existing ClusterRole grant`

---

## Tasks

- [ ] Extend `internal/controller/suite_test.go` or create
      `internal/controller/correlation_integration_test.go` with TC-01 through TC-05
      using the two-call `Reconcile()` pattern described above
- [ ] Include a non-primary-requeues test case in TC-02 to verify the requeue path
      (not just the primary-dispatches path)
- [ ] Verify via test (no code change) that `SourceProviderReconciler` already skips
      `Suppressed`-phase jobs via the `default:` case at `provider.go:383`; add a
      comment explaining this
- [ ] Confirm `testdata/crds/remediationjob_crd.yaml` has been updated by STORY_00
      (if STORY_00 is not yet complete, this story cannot proceed)
- [ ] Run `go test -timeout 60s -race ./internal/controller/...` — must pass
- [ ] Run `go test -timeout 30s -race ./...` for all other packages — must pass
- [ ] Run `go build ./...` and `go vet ./...` — must be clean

---

## Dependencies

**Depends on:** STORY_00, STORY_01, STORY_02, STORY_03, STORY_04 (all prior stories)
**Blocks:** Epic Definition of Done

---

## Definition of Done

- [ ] All five test cases pass in envtest
- [ ] Non-primary requeue path tested explicitly
- [ ] `DISABLE_CORRELATION=true` verified to bypass all correlation logic
- [ ] `Suppressed` phase treated correctly by `SourceProviderReconciler` (verified by test)
- [ ] Full repository test suite passes: `go test -timeout 60s -race ./...`
- [ ] `go build ./...` and `go vet ./...` clean
- [ ] Worklog entry written in `docs/WORKLOGS/`
