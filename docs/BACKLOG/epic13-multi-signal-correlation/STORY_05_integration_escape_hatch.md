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

- [ ] `internal/controller/suite_test.go` (existing envtest suite) is extended with
      correlation integration tests:
  - **TC-01 — No correlation, single finding:** one `RemediationJob` created, window
    elapses, job dispatched without `CorrelationGroupID`
  - **TC-02 — SameNamespaceParent correlation:** two jobs with matching parent prefix
    created, window elapses, primary dispatched with group label, secondary `Suppressed`
  - **TC-03 — PVCPod correlation:** PVC finding + Pod finding in same namespace with
    matching volume reference, window elapses, PVC job primary, Pod job `Suppressed`
  - **TC-04 — No correlation across namespaces:** two jobs with identical parent names
    but different namespaces, each dispatched independently (no group)
  - **TC-05 — DISABLE_CORRELATION=true:** two correlated jobs, escape hatch enabled,
    both dispatched immediately without any hold or grouping
- [ ] `DISABLE_CORRELATION=true` skips the window requeue and calls no correlator code
- [ ] `go test -timeout 60s -race ./internal/controller/...` passes (envtest tests need
      60s timeout to allow for the 30s window in TC-01 through TC-04; tests should use
      a reduced window via `CORRELATION_WINDOW_SECONDS=1` in test config)
- [ ] `go test -timeout 30s -race ./...` passes for all non-envtest packages

---

## Technical Implementation

### Test configuration

All correlation integration tests set `cfg.CorrelationWindowSeconds = 1` on the
reconciler's `Cfg` directly. The envtest suite instantiates the reconciler manually
(see `internal/controller/suite_test.go`) — inject config by constructing the reconciler
with the desired `Cfg` value before each test case.

### Critical: tests must call `Reconcile()` twice

The existing envtest pattern in this project calls `rec.Reconcile(ctx, req)` directly
and synchronously — there is no background controller loop. The `ctrl.Result{RequeueAfter: ...}`
returned by the window hold is not automatically acted upon by a manager. Tests must
replicate the requeue manually:

```go
// TC-02 pattern: two-call test
ctx := context.Background()
// First call: window not elapsed → expect RequeueAfter, job still Pending
result, err := rec.Reconcile(ctx, ctrlReq(rjob1.Name, rjob1.Namespace))
require.NoError(t, err)
require.Greater(t, result.RequeueAfter, time.Duration(0))

// Wait for the window to elapse.
// A 1.1× sleep (110ms over a 1s window) provides enough margin for CI scheduling
// jitter without making the test suite slow. This is the accepted trade-off:
// a deterministic fake clock would require injecting a clock interface into the
// reconciler, which adds significant complexity for marginal gain given the 1s
// window and the 100ms margin. If this test flakes on a heavily loaded CI runner,
// raise the margin to 1.5× (1500ms) or inject a fakeclock.
time.Sleep(1100 * time.Millisecond) // 10% over the 1s window

// Second call: window elapsed → correlator runs
_, err = rec.Reconcile(ctx, ctrlReq(rjob1.Name, rjob1.Namespace))
require.NoError(t, err)

// Now check phase transitions
var updated v1alpha1.RemediationJob
require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated))
require.Equal(t, v1alpha1.PhaseDispatched, updated.Status.Phase)
```

Apply the same two-call pattern for TC-01, TC-03, and TC-04.

### TC-05 escape hatch verification

Set `r.Correlator = nil` directly on the reconciler before the test. With no correlator,
a single call to `Reconcile()` must dispatch the job without any `RequeueAfter`:

```go
rec.Correlator = nil
result, err := rec.Reconcile(ctx, ctrlReq(rjob.Name, rjob.Namespace))
require.NoError(t, err)
require.Zero(t, result.RequeueAfter, "expected immediate dispatch with no correlation hold")
```

### `SourceProviderReconciler` treatment of `Suppressed` (Gap 5 clarification)

The existing check at `internal/provider/provider.go:248`:
```go
if rjob.Status.Phase != v1alpha1.PhaseFailed {
    return ctrl.Result{}, nil
}
```
already skips any `RemediationJob` whose phase is not `Failed` — including `Suppressed`.
**No code change is required here.** Verify this by writing a test: create a
`RemediationJob` with `Phase=Suppressed`, reconcile the source provider, and assert no
new `RemediationJob` is created. If the test passes without any code change, document
the finding with a comment in the test: `// Suppressed is handled by the existing
!= PhaseFailed check; no code change needed`.

Remove the task "Add `Suppressed` to the non-failed phase check in
`internal/provider/provider.go`" — it is a no-op.

---

## Tasks

- [ ] Extend `internal/controller/suite_test.go` (or a new `correlation_integration_test.go`
      in the same package) with TC-01 through TC-05, using the two-call `Reconcile()` pattern
      (first call returns `RequeueAfter`, sleep 1.1s, second call triggers correlation)
- [ ] Verify RBAC is covered: the `Correlator` calls `r.List` on `RemediationJob` objects in
      `r.Cfg.AgentNamespace`. Confirm that `deploy/kustomize/clusterrole-watcher.yaml` already
      includes `list` on `remediationjobs` — it does (verified in the existing ClusterRole rules).
      No new RBAC changes are required. Add a comment in the integration test file noting that
      the `pendingPeers` list call is covered by the existing `ClusterRole` grant.
- [ ] Verify via test (no code change needed) that `SourceProviderReconciler` already skips
      `Suppressed`-phase jobs via the existing `!= PhaseFailed` check at `provider.go:248`;
      add a comment in the test explaining this
- [ ] Verify `go test -timeout 60s -race ./internal/controller/...` passes
- [ ] Verify `go test -timeout 30s -race ./...` passes for all other packages
- [ ] Run `go build ./...` and `go vet ./...` — must be clean

---

## Dependencies

**Depends on:** STORY_00, STORY_01, STORY_02, STORY_03, STORY_04 (all prior stories)
**Blocks:** Epic Definition of Done

---

## Definition of Done

- [ ] All five test cases pass in envtest
- [ ] `DISABLE_CORRELATION=true` verified to bypass all correlation logic
- [ ] `Suppressed` phase treated correctly by `SourceProviderReconciler`
- [ ] Full repository test suite passes: `go test -timeout 60s -race ./...`
- [ ] `go build ./...` and `go vet ./...` clean
- [ ] Worklog entry written in `docs/WORKLOGS/`
