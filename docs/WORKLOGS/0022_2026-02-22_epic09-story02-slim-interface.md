# Worklog: Epic 09 STORY_02 — Slim SourceProvider Interface

**Date:** 2026-02-22
**Session:** Remove Fingerprint from SourceProvider interface; reconciler calls domain.FindingFingerprint directly
**Status:** Complete

---

## Objective

Remove the duplicate `Fingerprint` method from the `domain.SourceProvider` interface and from
`K8sGPTProvider`, update `SourceProviderReconciler.Reconcile` to call `domain.FindingFingerprint`
directly, and clean up all test fakes that implemented the now-removed method.

---

## Work Completed

### 1. TDD — Tests modified to fail first
- Removed `fp`, `fpErr` fields and `Fingerprint` method from `fakeSourceProvider` in
  `internal/provider/provider_test.go` — caused compile errors as expected (interface still
  required `Fingerprint`)
- Removed `Fingerprint` method from `trackingFakeProvider` in same file
- Rewrote `TestSourceProviderReconciler_FingerprintError_ReturnsError` to inject a finding with
  `Errors: "not-json"` (malformed JSON) to exercise the error path in `domain.FindingFingerprint`
- Updated `TestSourceProviderReconciler_CreatesRemediationJob`, `_SkipsDuplicateFingerprint`,
  and `_ReDispatchesFailedRemediationJob` to compute the expected fingerprint via
  `domain.FindingFingerprint` rather than a hardcoded constant

### 2. Interface change — `domain.SourceProvider`
- Removed `Fingerprint(f *Finding) (string, error)` method from `domain.SourceProvider` in
  `internal/domain/provider.go`; interface now has exactly 3 methods: `ProviderName`,
  `ObjectType`, `ExtractFinding`

### 3. Reconciler update — `SourceProviderReconciler`
- Updated `internal/provider/provider.go` line 82: changed `r.Provider.Fingerprint(finding)`
  to `domain.FindingFingerprint(finding)`

### 4. Implementation removal — `K8sGPTProvider`
- Deleted `Fingerprint` method from `internal/provider/k8sgpt/provider.go` (41 lines removed)
- Removed unused imports: `bytes`, `crypto/sha256`, `sort`
- Compile-time assertion `var _ domain.SourceProvider = (*K8sGPTProvider)(nil)` still passes

### 5. Test cleanup — `k8sgpt/provider_test.go`
- Deleted `TestK8sGPTProvider_Fingerprint_Deterministic`, `_OrderIndependent`,
  `_MalformedErrors`, and `_EmptyErrors` tests (algorithm is now covered by
  `internal/domain/provider_test.go` from STORY_01)

### 6. Interface test rename — `internal/domain/interfaces_test.go`
- Renamed `TestSourceProvider_HasFourMethods` to `TestSourceProvider_HasThreeMethods` to
  reflect accurate method count

---

## Key Decisions

- The `TestSourceProviderReconciler_FingerprintError_ReturnsError` test was rewritten rather
  than deleted: the test intent (verify fingerprint errors propagate) remains valid; only the
  injection mechanism changed from a fake error return to malformed JSON input.
- Tests `_CreatesRemediationJob`, `_SkipsDuplicateFingerprint`, and `_ReDispatchesFailedRemediationJob`
  now compute fingerprints via `domain.FindingFingerprint` rather than using hardcoded constants,
  making them more accurate and resilient to algorithm changes.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./...
```

All 9 packages pass:
- api/v1alpha1: ok (cached)
- cmd/watcher: ok (cached)
- internal/config: ok (cached)
- internal/controller: ok (cached)
- internal/domain: ok
- internal/jobbuilder: ok (cached)
- internal/logging: ok (cached)
- internal/provider: ok
- internal/provider/k8sgpt: ok

`go build ./...` — clean
`go vet ./...` — clean

---

## Next Steps

STORY_03: Implement native provider skeleton (or whichever story follows in epic09).
Read `docs/BACKLOG/epic09-native-provider/` for the next story and its acceptance criteria.

---

## Files Modified

- `internal/domain/provider.go` — removed `Fingerprint` from `SourceProvider` interface
- `internal/domain/interfaces_test.go` — renamed test to `TestSourceProvider_HasThreeMethods`
- `internal/provider/provider.go` — changed `r.Provider.Fingerprint` to `domain.FindingFingerprint`
- `internal/provider/provider_test.go` — removed `fp`/`fpErr` fields and `Fingerprint` methods
  from fakes; rewrote fingerprint error test; updated fingerprint-using tests to call
  `domain.FindingFingerprint`
- `internal/provider/k8sgpt/provider.go` — removed `Fingerprint` method and unused imports
- `internal/provider/k8sgpt/provider_test.go` — deleted 4 `TestK8sGPTProvider_Fingerprint_*`
  tests
