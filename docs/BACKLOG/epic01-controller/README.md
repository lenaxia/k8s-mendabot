# Epic: Controller

## Purpose

Implement the full controller layer across two packages:

1. **K8sGPTProvider + SourceProviderReconciler** (`internal/provider/`) — watches
   `results.core.k8sgpt.ai`, computes fingerprints, and creates `RemediationJob` CRDs
   (with `spec.sourceType: "k8sgpt"`) as the durable deduplication record
2. **RemediationJobReconciler** (`internal/controller/`) — watches `RemediationJob`
   objects, enforces concurrency limits, dispatches `batch/v1 Jobs` with ownerReferences,
   and syncs Job status back to the `RemediationJob`

## Status: Not Started

## Dependencies

- epic00-foundation complete
- epic00.1-interfaces complete (provider + reconciler structs, JobBuilder + SourceProvider interfaces, fakes, envtest suite)

## Blocks

- epic02-jobbuilder (reconciler calls `jobBuilder.Build(rjob)`)
- epic04-deploy (controller is what gets deployed)

## Success Criteria

- [ ] `SourceProviderReconciler` creates a `RemediationJob` (with `sourceType="k8sgpt"`) for each new unique fingerprint
- [ ] `SourceProviderReconciler` skips Results whose fingerprint already has a non-Failed `RemediationJob`
- [ ] `SourceProviderReconciler` deletes Pending/Dispatched `RemediationJob` when its source Result is deleted
- [ ] `RemediationJobReconciler` creates a `batch/v1 Job` with the correct ownerReference
- [ ] `RemediationJobReconciler` enforces `MAX_CONCURRENT_JOBS`
- [ ] `RemediationJobReconciler` syncs Job status back to `RemediationJob.Status.Phase`
- [ ] Results with zero errors are filtered out before entering the reconcile queue
- [ ] All unit tests for `fingerprintFor` pass (see test table in CONTROLLER_LLD.md §11)
- [ ] All integration tests using envtest pass

## Stories

| Story | File | Status |
|-------|------|--------|
| Result CRD scheme registration | [STORY_01_scheme.md](STORY_01_scheme.md) | Not Started |
| fingerprintFor implementation + tests | [STORY_02_fingerprint.md](STORY_02_fingerprint.md) | Not Started |
| SourceProviderReconciler — RemediationJob creation | [STORY_03_dedup_map.md](STORY_03_dedup_map.md) | Not Started |
| RemediationJobReconciler — Job dispatch | [STORY_04_reconcile.md](STORY_04_reconcile.md) | Not Started |
| Error-filter predicate | [STORY_05_predicate.md](STORY_05_predicate.md) | Not Started |
| Manager wiring (main.go) | [STORY_06_manager.md](STORY_06_manager.md) | Not Started |
| Integration tests (envtest) | [STORY_07_integration_tests.md](STORY_07_integration_tests.md) | Not Started |

## Technical Overview

The controller layer is specified in [`docs/DESIGN/lld/CONTROLLER_LLD.md`](../../DESIGN/lld/CONTROLLER_LLD.md).

The key architectural split:
- `SourceProviderReconciler` (in `internal/provider/provider.go`) is the single
  `ctrl.Reconciler` for source watching. It holds the full reconcile loop and calls
  into `K8sGPTProvider` for provider-specific logic. There is no separate
  `ResultReconciler` type — `internal/provider/k8sgpt/reconciler.go` holds only the
  `fingerprintFor()` package-level function.
- `RemediationJobReconciler` lives in `internal/controller/` — it is provider-agnostic.

TDD is mandatory. Write `fingerprintFor` tests before implementing it. Write reconciler
tests before implementing the reconciler body.

## Definition of Done

- [ ] All unit tests pass with race detector
- [ ] All envtest integration tests pass
- [ ] `go vet` is clean
- [ ] CONTROLLER_LLD.md test table fully covered
