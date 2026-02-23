# Worklog: Epic09 — Native Cluster Provider Complete

**Date:** 2026-02-22
**Session:** Epic09 full implementation — all 12 stories, 6 native providers, k8sgpt removed
**Status:** Complete

---

## Objective

Implement epic09-native-provider: replace the k8sgpt operator dependency with a native
`SourceProvider` implementation that watches core Kubernetes resources directly. After this
epic, mendabot no longer requires k8sgpt-operator to be installed in the cluster.

---

## Work Completed

### 1. STORY_01 — Promote FindingFingerprint to domain function
- Added `domain.FindingFingerprint(f *Finding) (string, error)` to `internal/domain/provider.go`
- 10 unit tests covering determinism, order-independence, field exclusion, HTML characters, hex length
- Byte-for-byte compatible with the deleted `K8sGPTProvider.Fingerprint`

### 2. STORY_02 — Slim SourceProvider interface
- Removed `Fingerprint(f *Finding) (string, error)` from `domain.SourceProvider` — now 3 methods only
- `SourceProviderReconciler.Reconcile` calls `domain.FindingFingerprint` directly
- All fakes updated; `K8sGPTProvider.Fingerprint` method deleted

### 3. STORY_03 — getParent owner-reference traversal
- `internal/provider/native/parent.go`: `getParent(ctx, c, meta, kind) string`
- Max depth 10, circular reference guard via UID set, no error return (log + fallback)
- 9 unit tests covering all traversal scenarios

### 4. STORY_04 — PodProvider
- Detects: CrashLoopBackOff, ImagePullBackOff, ErrImagePull, OOMKilled, non-zero exit, unschedulable pending
- Checks both `ContainerStatuses` and `InitContainerStatuses`
- 26 unit tests including `TestInitContainerCrashLoop`

### 5. STORY_05 — DeploymentProvider
- Replicas mismatch with scaling-down transient exclusion
- Available=False always reported
- 13 unit tests

### 6. STORY_06 — PVCProvider
- Phase=Pending + ProvisioningFailed event detection
- In-process event filtering (no field selectors — fake client compatible)
- 11 unit tests

### 7. STORY_07 — NodeProvider
- NodeReady=False/Unknown + non-standard conditions True
- Conditions only, no taints
- 16 unit tests

### 8. STORY_10 — StatefulSetProvider
- Same pattern as DeploymentProvider; Available=False always reported
- 13 unit tests

### 9. STORY_11 — JobProvider
- CronJob exclusion before failure check
- Three-part condition: failed>0 AND active==0 AND completionTime==nil
- 17 unit tests

### 10. STORY_12 — Stabilisation window
- `StabilisationWindow time.Duration` in `config.Config`
- `STABILISATION_WINDOW_SECONDS` env var, default 120s
- `window==0` fast path; `firstSeen` map in `SourceProviderReconciler`
- 6 stabilisation window unit tests

### 11. STORY_08 — Wire native providers into main.go
- All 6 providers registered: Pod, Deployment, PVC, Node, StatefulSet, Job

### 12. STORY_09 — Remove k8sgpt
- `internal/provider/k8sgpt/` deleted
- `api/v1alpha1/result_types.go` deleted
- `SourceTypeNative = "native"` replaces `SourceTypeK8sGPT`
- `NewScheme()` no longer registers Result types
- k8sgpt removed from `Dockerfile.agent` and `configmap-prompt.yaml`
- `go mod tidy` removed k8sgpt dependency entries
- Integration tests migrated to `internal/provider/suite_test.go` + `provider_integration_test.go`

---

## Key Decisions

- `getParent` has no error return — on lookup failure it logs debug and falls back to self. This avoids error noise from transient API unavailability.
- `firstSeen` map has no mutex — relies on single-worker controller-runtime default (documented in source).
- PVC event listing uses in-process filtering (no field selectors) to maintain compatibility with fake client in tests.
- CronJob exclusion in JobProvider checks ownerReferences BEFORE failure detection, not after `getParent` traversal.

---

## Blockers

None.

---

## Tests Run

```
go clean -testcache && go test -timeout 120s -race ./...
```

All 9 packages pass:
- api/v1alpha1
- cmd/watcher
- internal/config
- internal/controller
- internal/domain
- internal/jobbuilder
- internal/logging
- internal/provider
- internal/provider/native

`go build ./...` and `go vet ./...` both clean.

---

## Next Steps

- Merge `feature/epic09-native-provider` to `main` when ready
- Consider epic08 (pluggable agent types) as the next epic
- k8sgpt-operator is no longer a deployment prerequisite

---

## Files Modified

**New files:**
- `internal/domain/provider_test.go` (expanded)
- `internal/provider/native/parent.go`
- `internal/provider/native/parent_test.go`
- `internal/provider/native/pod.go`
- `internal/provider/native/pod_test.go`
- `internal/provider/native/deployment.go`
- `internal/provider/native/deployment_test.go`
- `internal/provider/native/pvc.go`
- `internal/provider/native/pvc_test.go`
- `internal/provider/native/node.go`
- `internal/provider/native/node_test.go`
- `internal/provider/native/statefulset.go`
- `internal/provider/native/statefulset_test.go`
- `internal/provider/native/job.go`
- `internal/provider/native/job_test.go`
- `internal/provider/suite_test.go`
- `internal/provider/provider_integration_test.go`

**Modified files:**
- `internal/domain/provider.go` (FindingFingerprint added, interface slimmed)
- `internal/provider/provider.go` (calls domain.FindingFingerprint, stabilisation window)
- `internal/provider/provider_test.go` (fakes updated, window tests added)
- `internal/config/config.go` (StabilisationWindow field)
- `internal/config/config_test.go` (window tests added)
- `cmd/watcher/main.go` (native providers wired, k8sgpt removed)
- `api/v1alpha1/remediationjob_types.go` (SourceTypeNative)
- `docker/Dockerfile.agent` (k8sgpt removed)
- `deploy/kustomize/configmap-prompt.yaml` (k8sgpt analyze step removed)
- `deploy/kustomize/clusterrole-watcher.yaml` (native resource permissions)

**Deleted files:**
- `internal/provider/k8sgpt/` (entire package)
- `api/v1alpha1/result_types.go`
- `api/v1alpha1/result_types_test.go`
