# Story: Slim SourceProvider Interface and Update SourceProviderReconciler

**Epic:** [epic09-native-provider](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the `SourceProvider` interface to contain only the three
methods that vary per provider so that implementing a new provider is as simple as
possible and carries no hidden algorithmic responsibilities.

---

## Background

After STORY_01 adds `domain.FindingFingerprint`, there are now two copies of the
fingerprint algorithm: the new standalone function and `K8sGPTProvider.Fingerprint`.
This story atomically removes the second copy by:

1. Deleting `Fingerprint` from the `SourceProvider` interface
2. Deleting `K8sGPTProvider.Fingerprint` from `internal/provider/k8sgpt/provider.go`
3. Updating `SourceProviderReconciler.Reconcile` to call `domain.FindingFingerprint`
   instead of `r.Provider.Fingerprint`
4. Cleaning up all test infrastructure in `internal/provider/provider_test.go` that
   was written to support the old `Fingerprint`-on-interface contract

This story is explicitly split from STORY_01 so that after STORY_01 the codebase
compiles (interface still has `Fingerprint`), and STORY_02 removes it atomically.

---

## Acceptance Criteria

- [ ] `domain.SourceProvider` interface in `internal/domain/provider.go` has exactly
  three methods:
  ```go
  type SourceProvider interface {
      ProviderName() string
      ObjectType() client.Object
      ExtractFinding(obj client.Object) (*Finding, error)
  }
  ```
- [ ] `K8sGPTProvider.Fingerprint` method deleted from `internal/provider/k8sgpt/provider.go`
- [ ] All fingerprint tests from `internal/provider/k8sgpt/provider_test.go` that test
  `K8sGPTProvider.Fingerprint` directly are deleted (they tested a method that no longer
  exists; the algorithm is now tested in `internal/domain/provider_test.go`)
- [ ] `SourceProviderReconciler.Reconcile` in `internal/provider/provider.go` calls
  `domain.FindingFingerprint(finding)` where it previously called
  `r.Provider.Fingerprint(finding)`
- [ ] `internal/provider/provider_test.go` updated:
  - `fakeSourceProvider` struct: `fp` and `fpErr` fields removed; `Fingerprint` method
    removed
  - `trackingFakeProvider` struct: `Fingerprint` method removed
  - `TestSourceProviderReconciler_FingerprintError_ReturnsError`: this test verified that
    a `Fingerprint` error propagated; since `domain.FindingFingerprint` only fails on
    malformed JSON, the test must be rewritten to inject a finding with malformed
    `Errors` JSON (e.g. `Errors: "not-json"`) to exercise the error path
  - All `var _ domain.SourceProvider = ...` compile-time assertions still pass
- [ ] `go build ./...` succeeds — no remaining references to `Provider.Fingerprint`

---

## Tasks

- [ ] Remove `Fingerprint` from `domain.SourceProvider` interface in
  `internal/domain/provider.go`
- [ ] Delete `K8sGPTProvider.Fingerprint` from `internal/provider/k8sgpt/provider.go`
- [ ] Delete `TestK8sGPTProvider_Fingerprint_*` tests from
  `internal/provider/k8sgpt/provider_test.go` (these tested `K8sGPTProvider.Fingerprint`
  directly; the algorithm is now covered by `domain.FindingFingerprint` tests in STORY_01)
- [ ] Update `SourceProviderReconciler.Reconcile` in `internal/provider/provider.go`
  to call `domain.FindingFingerprint(finding)` in place of `r.Provider.Fingerprint(finding)`
- [ ] Update `internal/provider/provider_test.go`:
  - Remove `fp` and `fpErr` fields and `Fingerprint` method from `fakeSourceProvider`
  - Remove `Fingerprint` method from `trackingFakeProvider`
  - Rewrite `TestSourceProviderReconciler_FingerprintError_ReturnsError` to use malformed
    `Errors` JSON to trigger the error path in `domain.FindingFingerprint`
- [ ] Run full test suite: `go test -timeout 120s -race ./...`

---

## Dependencies

**Depends on:** STORY_01 (FindingFingerprint in domain)
**Blocks:** STORY_03, STORY_04, STORY_05, STORY_06, STORY_07, STORY_10, STORY_11, STORY_12

---

## Definition of Done

- [ ] Full test suite passes with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
- [ ] `SourceProvider` interface has exactly three methods
- [ ] No remaining `Fingerprint` method implementations in any provider or test file
