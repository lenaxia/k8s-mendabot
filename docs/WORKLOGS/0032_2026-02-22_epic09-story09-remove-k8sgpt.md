# Worklog: Epic 09 STORY_09 — Remove k8sgpt provider

**Date:** 2026-02-22
**Session:** Remove k8sgpt provider package, result_types, and all k8sgpt dependencies
**Status:** Complete

---

## Objective

Delete everything that depended on k8sgpt: the `K8sGPTProvider`, `result_types.go`, all
`SourceTypeK8sGPT` references, the k8sgpt binary from the agent image, and the k8sgpt
analysis step from the agent prompt. Migrate the 6 reconciler integration test scenarios
(previously in `internal/provider/k8sgpt/integration_test.go`) to a new
`internal/provider/provider_integration_test.go` using native providers.

---

## Work Completed

### 1. Integration tests created (TDD — written before deletion)

- `internal/provider/suite_test.go` — new envtest suite setup in `package provider_test`
  with `TestMain` and extended scheme (clientgoscheme + v1alpha1). CRD path: `../../testdata/crds`.
- `internal/provider/provider_integration_test.go` — 6 reconciler integration scenarios
  using `integrationFakeProvider` (watches `corev1.Pod`, returns configured finding):
  1. `TestIntegration_CreateRemediationJob`
  2. `TestIntegration_DuplicateFingerprint_Skips`
  3. `TestIntegration_FailedPhase_ReDispatches`
  4. `TestIntegration_NoErrors_Skipped`
  5. `TestIntegration_ResultDeleted_CancelsPending`
  6. `TestIntegration_ResultDeleted_CancelsDispatched`

### 2. Files deleted

- `internal/provider/k8sgpt/provider.go`
- `internal/provider/k8sgpt/provider_test.go`
- `internal/provider/k8sgpt/integration_test.go`
- `internal/provider/k8sgpt/suite_test.go`
- `api/v1alpha1/result_types.go`
- `api/v1alpha1/result_types_test.go`

### 3. api/v1alpha1/remediationjob_types.go updated

- `NewScheme()` now only registers remediation types (no `AddResultToScheme` call)
- `SourceTypeK8sGPT = "k8sgpt"` replaced with `SourceTypeNative = "native"`
- Stale comments referencing k8sgpt removed

### 4. cmd/watcher/main.go updated

- `k8sgpt` import removed
- `AddResultToScheme` call removed from scheme registration
- `&k8sgpt.K8sGPTProvider{}` removed from `enabledProviders`

### 5. Test files updated

- `api/v1alpha1/remediationjob_types_test.go` — `SourceTypeK8sGPT` → `SourceTypeNative`; removed
  `TestNewScheme_RegistersBothGroupVersions` (tested Result which no longer exists), replaced
  with `TestNewScheme_RegistersRemediationGroupVersion`
- `cmd/watcher/scheme_test.go` — removed `AddResultToScheme` and `core.k8sgpt.ai` GVK tests
- `internal/provider/provider_test.go` — replaced `*v1alpha1.Result` watched object with
  `*corev1.ConfigMap`; added `clientgoscheme` to `newTestScheme()`; updated provider names
  from `"k8sgpt"` to `"native"`; updated `SourceType` assertions
- `internal/controller/integration_test.go` — `SourceTypeK8sGPT` → `SourceTypeNative`

### 6. internal/domain/provider.go updated

- Removed k8sgpt-specific examples from ProviderName, Details, and SourceRef.APIVersion comments

### 7. docker/Dockerfile.agent updated

- `K8SGPT_VERSION` ARG removed
- k8sgpt download/install block removed

### 8. deploy/kustomize/configmap-prompt.yaml updated

- Preamble updated (k8sgpt-operator reference removed)
- STEP 4 (k8sgpt analyze) deleted; steps 5–9 renumbered to 4–8
- Branch names changed from `fix/k8sgpt-…` to `fix/mechanic-…`
- Tools list updated (k8sgpt removed)
- Rule 5 updated (Result CRD → RemediationJob CRD)

### 9. deploy/kustomize/clusterrole-watcher.yaml updated

- `core.k8sgpt.ai/results` rule removed
- Added native provider resource permissions: pods, pvcs, nodes, events, deployments,
  statefulsets, replicasets, daemonsets

### 10. deploy/kustomize/crd-remediationjob.yaml updated

- `sourceType` description: "e.g. k8sgpt" → "e.g. native"

### 11. go mod tidy

- k8sgpt dependency entries removed from go.mod and go.sum

---

## Key Decisions

- **Watched object type in unit tests**: Changed from `*v1alpha1.Result` to `*corev1.ConfigMap`.
  The reconciler unit tests test reconciler logic, not type-specific logic, so any registered
  type works. ConfigMap is available via clientgoscheme added to the test scheme.
- **Integration test provider**: Used `integrationFakeProvider` watching `corev1.Pod` with
  a configurable finding. The envtest suite now adds clientgoscheme to the k8sClient scheme
  so Pod CRUD operations work in envtest.
- **Suite TestMain**: The `provider_test` package already had tests in `provider_test.go`.
  The new `suite_test.go` adds `TestMain` which is valid since there was no existing
  `TestMain` in `provider_test.go` (unit tests used fake clients only).
- **Branch name update**: Changed `fix/k8sgpt-…` to `fix/mechanic-…` in the prompt to
  remove the k8sgpt brand association.

---

## Blockers

None.

---

## Tests Run

```
go build ./...          — clean
go vet ./...            — clean
go test -timeout 120s -race ./...  — all 9 packages pass
```

Full output (no cache):
```
ok  github.com/lenaxia/k8s-mechanic/api/v1alpha1           1.031s
ok  github.com/lenaxia/k8s-mechanic/cmd/watcher             1.109s
ok  github.com/lenaxia/k8s-mechanic/internal/config         1.022s
ok  github.com/lenaxia/k8s-mechanic/internal/controller     7.980s
ok  github.com/lenaxia/k8s-mechanic/internal/domain         1.083s
ok  github.com/lenaxia/k8s-mechanic/internal/jobbuilder     1.087s
ok  github.com/lenaxia/k8s-mechanic/internal/logging        1.023s
ok  github.com/lenaxia/k8s-mechanic/internal/provider       9.101s
ok  github.com/lenaxia/k8s-mechanic/internal/provider/native 1.222s
```

---

## Next Steps

Epic 09 STORY_09 is complete. Epic 09 is the final epic. All native providers are wired,
the k8sgpt dependency is fully removed, and the test suite is clean.

---

## Files Modified

**Created:**
- `internal/provider/suite_test.go`
- `internal/provider/provider_integration_test.go`
- `docs/WORKLOGS/0032_2026-02-22_epic09-story09-remove-k8sgpt.md`

**Deleted:**
- `internal/provider/k8sgpt/` (entire directory: provider.go, provider_test.go,
  integration_test.go, suite_test.go)
- `api/v1alpha1/result_types.go`
- `api/v1alpha1/result_types_test.go`

**Modified:**
- `api/v1alpha1/remediationjob_types.go`
- `api/v1alpha1/remediationjob_types_test.go`
- `cmd/watcher/main.go`
- `cmd/watcher/scheme_test.go`
- `internal/provider/provider_test.go`
- `internal/controller/integration_test.go`
- `internal/domain/provider.go`
- `internal/domain/interfaces_test.go`
- `docker/Dockerfile.agent`
- `deploy/kustomize/configmap-prompt.yaml`
- `deploy/kustomize/clusterrole-watcher.yaml`
- `deploy/kustomize/crd-remediationjob.yaml`
- `go.mod`
- `go.sum`
