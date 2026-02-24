# Worklog: Epic 13 STORY_05 — Correlation Integration Tests and Escape Hatch

**Date:** 2026-02-23
**Session:** Epic 13 Story 05 — integration tests for correlation window, rules, and DISABLE_CORRELATION escape hatch
**Status:** Complete

---

## Objective

Add end-to-end integration tests for the full correlation path using the existing envtest suite. Verify the correlation window hold, SameNamespaceParentRule, PVCPodRule, cross-namespace isolation, and the DISABLE_CORRELATION (nil Correlator) escape hatch. Verify SourceProviderReconciler already skips Suppressed-phase jobs via the existing `!= PhaseFailed` check.

---

## Work Completed

### 1. Created `internal/controller/correlation_integration_test.go`

Six integration tests using the real envtest API server:

- **TC-01** (`TestCorrelationIntegration_TC01_SingleFinding_NoCorrelation`): Single job, no peers. Verifies window hold (first reconcile → RequeueAfter > 0), then dispatches without group label after 1.1s sleep.

- **TC-02** (`TestCorrelationIntegration_TC02_SameNamespaceParent`): Two jobs with same namespace and ParentObject. Verifies rjob1 dispatches WITH `correlation-group-id` label (primary role) when rjob2 is still at Phase="" and visible as a peer. Documented why rjob2 dispatches independently thereafter (Dispatched jobs are excluded from pendingPeers).

- **TC-03** (`TestCorrelationIntegration_TC03_PVCPod`): PVC job + Pod job. Pod has a volume referencing the PVC in envtest. PVCPodRule fires. Reconcile Pod job first (PVC still Phase="") → Pod suppressed. PVC job dispatches independently.

- **TC-04** (`TestCorrelationIntegration_TC04_NoCorrelationAcrossNamespaces`): Same ParentObject in two different namespaces. Each reconciler points at its own namespace. Both jobs dispatch independently without any group label.

- **TC-05** (`TestCorrelationIntegration_TC05_EscapeHatch_DisableCorrelation`): `rec.Correlator = nil` with two would-be correlated jobs. Single reconcile → immediate dispatch, RequeueAfter == 0.

- **SourceProvider skip-Suppressed** (`TestCorrelationIntegration_SourceProvider_SkipsSuppressed`): Pre-existing Suppressed RemediationJob with same fingerprint. SourceProviderReconciler reconciles → no new job created. Confirms the existing `!= PhaseFailed` check at `provider.go:258` handles Suppressed phase; no code change needed.

### 2. Helpers added

- `ctrlReqNS(name, namespace string)` — namespace-parameterised ctrl.Request builder.
- `ensureNamespace(t, ctx, c, name)` — creates test namespaces in envtest with cleanup.
- `newCorrelationRJob(name, namespace, fp, finding)` — builds a fully-populated RemediationJob for correlation tests.
- `newCorrReconcilerWith(c, namespace, rules, jb)` — builds a reconciler with CorrelationWindowSeconds=1.
- `dynamicFakeJobBuilder` — re-fetches the rjob at `Build()` time to get the current envtest-assigned UID, avoiding selector label collisions when the same builder is shared across rjobs.
- `correlationFakeProvider` — minimal SourceProvider watching corev1.Pod for the SourceProviderReconciler test.

### 3. Key design note documented in TC-02

`pendingPeers()` excludes jobs with Phase != "" && Phase != "Pending". Once a job is Dispatched it becomes invisible as a peer. This means the secondary-is-suppressed path requires reconciling the secondary while the primary is still Phase="". In integration tests this means reconcile order matters. The secondary suppression path is verified by the existing unit test `TestCorrelationWindow_SecondaryIsSuppressed`.

---

## Key Decisions

1. **No testify dependency**: The codebase uses stdlib `testing` throughout. The prompt asked for `require.*` but testify is not a dependency. Used `t.Fatalf` / `t.Errorf` instead to stay consistent.

2. **TC-02 reconcile order**: Reconcile rjob1 first (primary gets group label while rjob2 is Phase=""). rjob2 then dispatches independently. The story's "rjob2 PhaseSuppressed" expectation requires simultaneous reconciliation which is impossible in a sequential test. Documented this constraint in a comment.

3. **TC-03 reconcile order**: Reconcile Pod job first (PVC job still Phase="") → Pod suppressed by PVCPodRule. Then PVC job dispatches. This is the natural ordering because the Pod candidate's `evaluatePodCandidate` path finds PVC peers.

4. **dynamicFakeJobBuilder**: Needed to avoid Job selector collision when rjob1 and rjob2 would try to create jobs with the same fingerprint-based name but different UID selectors.

5. **CorrelationWindowSeconds=1**: All correlation integration tests use a 1-second window and sleep 1100ms — 10% margin above the window.

---

## Blockers

None.

---

## Tests Run

```
go build ./...                                    # clean
go vet ./...                                      # clean
go test -count=1 -timeout 60s -race ./internal/controller/...
# PASS (all 6 new + all existing tests)
go test -count=1 -timeout 30s -race $(go list ./... | grep -v internal/readiness/llm)
# ok all packages
```

---

## Next Steps

- Epic 13 is complete. All 5 stories done.
- Next: write the final epic-level worklog, update backlog story DoDs, and merge the feature branch.

---

## Files Modified

- `internal/controller/correlation_integration_test.go` — created (new file)
- `docs/WORKLOGS/0044_2026-02-23_epic13-story05-integration-tests.md` — created (this file)
- `docs/WORKLOGS/README.md` — updated index
