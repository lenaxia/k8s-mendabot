# Story: No-Error Filtering in ExtractFinding

**Epic:** [Controller](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want Results with zero errors to be silently skipped by the provider
so the controller never dispatches an investigation for a healthy resource.

---

## Acceptance Criteria

- [ ] `K8sGPTProvider.ExtractFinding()` returns `(nil, nil)` when `len(result.Spec.Error) == 0`
- [ ] Results with `len(spec.error) == 0` produce no `RemediationJob` (skipped in `SourceProviderReconciler.Reconcile`)
- [ ] Results with `len(spec.error) > 0` proceed normally
- [ ] **No manager-level predicate** — filtering is provider-specific and belongs in `ExtractFinding()`
  (see CONTROLLER_LLD.md §5.3 and PROVIDER_LLD.md §8)
- [ ] Unit test in `internal/provider/k8sgpt/provider_test.go` verifies the skip directly:
  ```go
  func TestK8sGPTProvider_ExtractFinding_NoErrors(t *testing.T) {
      // result with empty Spec.Error → returns nil, nil
  }
  ```

---

## Tasks

- [ ] Write unit test for `ExtractFinding` with empty errors first (TDD)
- [ ] Implement the early-return in `K8sGPTProvider.ExtractFinding()`
- [ ] Verify `SourceProviderReconciler.Reconcile()` returns nil when `ExtractFinding` returns nil, nil

---

## Dependencies

**Depends on:** epic00.1-interfaces/STORY_03 (K8sGPTProvider stub exists)
**Blocks:** STORY_07 (integration tests)

---

## Definition of Done

- [ ] Test passes with `-race`
- [ ] `go vet` clean
