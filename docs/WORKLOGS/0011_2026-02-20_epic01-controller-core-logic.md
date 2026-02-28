# Worklog: Epic 01 Stories 02–05 — Controller Core Logic

**Date:** 2026-02-20
**Session:** Implement fingerprintFor, K8sGPTProvider, SourceProviderReconciler, RemediationJobReconciler with full TDD
**Status:** Complete

---

## Objective

Implement the four core controller stories in one session following strict TDD:
- STORY_02: `fingerprintFor` + full `K8sGPTProvider` implementation
- STORY_05: nil-return filter in `ExtractFinding` (covered by STORY_02)
- STORY_03: `SourceProviderReconciler.Reconcile()` full implementation
- STORY_04: `RemediationJobReconciler.Reconcile()` full implementation

---

## Work Completed

### 1. STORY_02 — `fingerprintFor` + `K8sGPTProvider`

**TDD workflow followed:**

1. Wrote `internal/provider/k8sgpt/reconciler_test.go` (package `k8sgpt`, white-box) with 8 tests:
   `TestFingerprintFor_SameParentDifferentPods`, `TestFingerprintFor_DifferentErrors`,
   `TestFingerprintFor_ErrorOrderIndependent`, `TestFingerprintFor_DifferentParents`,
   `TestFingerprintFor_DifferentNamespaces`, `TestFingerprintFor_EmptyErrors`,
   `TestFingerprintFor_Deterministic`, `TestFingerprintEquivalence` (table-driven, includes
   `<`, `>`, `&` HTML escape guard case).
2. Confirmed tests failed (compile error: `fingerprintFor` undefined).
3. Wrote `internal/provider/k8sgpt/provider_test.go` (package `k8sgpt_test`, black-box) with
   9 tests replacing the old panic-check stubs.
4. Implemented `fingerprintFor` in `reconciler.go` using `json.NewEncoder` + `SetEscapeHTML(false)`.
5. Implemented all four `K8sGPTProvider` methods in `provider.go`:
   - `ProviderName()` → `v1alpha1.SourceTypeK8sGPT`
   - `ObjectType()` → `&v1alpha1.Result{}`
   - `ExtractFinding()` — type assert, nil-return for empty errors, redaction, JSON marshal
   - `Fingerprint()` — re-parse JSON, sort texts, `SetEscapeHTML(false)` on payload
6. All 17 k8sgpt tests pass.

**HTML escape equivalence:** Both `fingerprintFor` and `K8sGPTProvider.Fingerprint` use
`json.NewEncoder` with `SetEscapeHTML(false)` on the payload struct. `ExtractFinding` uses
standard `json.Marshal` for the `[]Failure` slice — this is safe because `Fingerprint` re-parses
the JSON to get raw text strings, which are then re-encoded with `SetEscapeHTML(false)`. The
round-trip is lossless and both paths hash identical bytes for the same logical finding.

### 2. STORY_05 — nil-return filter

Covered entirely by `ExtractFinding`'s `len(result.Spec.Error) == 0` early return and the
`TestK8sGPTProvider_ExtractFinding_NoErrors` + `TestK8sGPTProvider_ExtractFinding_EmptySlice` tests.

### 3. STORY_03 — `SourceProviderReconciler.Reconcile()`

**TDD workflow followed:**

1. Replaced `internal/provider/provider_test.go` — changed from `package provider` (white-box
   panic tests) to `package provider_test` (black-box real tests) with 7 tests:
   `CallsExtractFinding`, `SkipsOnNilFinding`, `CreatesRemediationJob`,
   `SkipsDuplicateFingerprint`, `ReDispatchesFailedRemediationJob`,
   `NotFound_DeletesPendingRJobs`, `NotFound_DeletesDispatchedRJobs`.
   Defined `fakeSourceProvider` and `trackingFakeProvider` in test file.
2. Confirmed tests failed (panics from stub Reconcile).
3. Implemented `SourceProviderReconciler.Reconcile()` and `SetupWithManager()` in `provider.go`.
4. All 7 provider tests pass.

**Key decisions:**
- Used fake client (`sigs.k8s.io/controller-runtime/pkg/client/fake`) for all unit tests — no envtest.
- NotFound handler iterates all RemediationJobs in AgentNamespace and deletes those matching
  the source ref with phase Pending, Dispatched, or empty string.
- Label filtering `remediation.mechanic.io/fingerprint=fp[:12]` for dedup lookup, then full
  fingerprint comparison to handle label prefix collisions.

### 4. STORY_04 — `RemediationJobReconciler.Reconcile()`

**TDD workflow followed:**

1. Replaced `internal/controller/remediationjob_controller_test.go` — changed from
   `package controller` (white-box panic tests) to `package controller_test` (black-box real
   tests) with 8 tests:
   `NotFound_ReturnsNil`, `Pending_CreatesJob`, `MaxConcurrent_Requeues`,
   `JobExists_SyncsStatus`, `BuildError_ReturnsError`, `Succeeded_TTLNotDue_Requeues`,
   `Failed_ReturnsNil`, `OwnerRef`.
2. Added `app.kubernetes.io/managed-by: mechanic-watcher` label to `defaultFakeJob` in
   `fakes_test.go` (required for active-job counting in step 4 of the reconcile loop).
3. Confirmed tests failed (panics from stub Reconcile).
4. Implemented `RemediationJobReconciler.Reconcile()`, `syncPhaseFromJob()`, and
   `SetupWithManager()` in `remediationjob_controller.go`.
5. All 8 controller tests pass (plus 7 existing fakes tests).

**Key decisions:**
- Used `apimeta.SetStatusCondition` from `k8s.io/apimachinery/pkg/api/meta` for condition upsert.
- Active job counting matches spec: `Active > 0 OR (Succeeded == 0 AND CompletionTime == nil)`.
  This counts freshly-created Jobs (Active==0, Succeeded==0, CompletionTime==nil) before any
  pod starts, preventing overrun.
- Status patches use `r.Status().Patch(ctx, &rjob, client.MergeFrom(rjobCopy))` with
  `WithStatusSubresource` on the fake client.

---

## Key Decisions

1. **`fingerprintFor` package**: Kept in `package k8sgpt` (unexported). `reconciler_test.go`
   uses `package k8sgpt` (white-box) to access it directly. `provider_test.go` uses
   `package k8sgpt_test` (black-box). Both coexist in the same directory — valid Go.

2. **HTML escaping**: Both `fingerprintFor` and `K8sGPTProvider.Fingerprint` use
   `SetEscapeHTML(false)` on the payload encoding. `ExtractFinding` uses standard `json.Marshal`
   for the Failure slice, which is safe due to the re-parse round-trip in `Fingerprint`.
   `TestFingerprintEquivalence/html_special_chars` with `<nil> pointer & invalid > comparison`
   confirms correctness.

3. **Test package migration**: Replaced white-box panic-check tests with black-box real tests
   in both `provider_test.go` and `remediationjob_controller_test.go`. The old compile-time
   struct-field assertions were removed — behavior tests provide stronger coverage.

4. **`SourceProviderReconciler` Log field**: Nil-guarded before use (`if r.Log != nil`) to
   allow test construction without requiring a logger. Matches the pattern used in the
   RemediationJobReconciler.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/provider/k8sgpt/...  → PASS (17 tests)
go test -timeout 30s -race ./internal/provider/...         → PASS (7 tests)
go test -timeout 30s -race ./internal/controller/...       → PASS (15 tests)
go test -timeout 30s -race ./...                           → PASS (all packages)
go build ./...                                             → clean
go vet ./...                                               → clean
```

---

## Next Steps

1. Implement envtest integration tests per CONTROLLER_LLD.md §11 and STORY_03:
   - `TestSourceProviderReconciler_CreatesRemediationJob` (envtest, `internal/provider/k8sgpt/`)
   - `TestSourceProviderReconciler_DuplicateFingerprint_Skips`
   - `TestSourceProviderReconciler_FailedPhase_ReDispatches`
   - `TestSourceProviderReconciler_NoErrors_Skipped`
   - `TestSourceProviderReconciler_ResultDeleted_CancelsPending`
   - `TestSourceProviderReconciler_ResultDeleted_CancelsDispatched`
2. Implement envtest integration tests for `RemediationJobReconciler` per CONTROLLER_LLD.md §11:
   - `TestRemediationJobReconciler_CreatesJob`, `SyncsStatus_Running`, `SyncsStatus_Succeeded`, etc.
3. Implement `JobBuilder` (epic02).
4. Mark STORY_02, STORY_03, STORY_04, STORY_05 as complete in backlog.

---

## Files Modified

| File | Change |
|------|--------|
| `internal/provider/k8sgpt/reconciler.go` | Implemented `fingerprintFor()` |
| `internal/provider/k8sgpt/reconciler_test.go` | New: 8 unit tests for `fingerprintFor` + `TestFingerprintEquivalence` |
| `internal/provider/k8sgpt/provider.go` | Replaced stubs: full `K8sGPTProvider` implementation |
| `internal/provider/k8sgpt/provider_test.go` | Replaced panic tests: 9 real unit tests for `K8sGPTProvider` |
| `internal/provider/provider.go` | Replaced stub: full `SourceProviderReconciler.Reconcile()` + `SetupWithManager()` |
| `internal/provider/provider_test.go` | Replaced panic tests: 7 real unit tests for `SourceProviderReconciler` |
| `internal/controller/remediationjob_controller.go` | Replaced stub: full `RemediationJobReconciler.Reconcile()` + `syncPhaseFromJob()` + `SetupWithManager()` |
| `internal/controller/remediationjob_controller_test.go` | Replaced panic tests: 8 real unit tests for `RemediationJobReconciler` |
| `internal/controller/fakes_test.go` | Added `app.kubernetes.io/managed-by` label to `defaultFakeJob` |
