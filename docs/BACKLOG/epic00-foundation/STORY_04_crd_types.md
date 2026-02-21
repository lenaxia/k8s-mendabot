# Story: Vendored CRD Types

**Epic:** [Foundation](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want the k8sgpt-operator `Result` CRD types available in
`api/v1alpha1/` without depending on the full operator module so that the watcher can
watch and read Result objects with full type safety.

---

## Acceptance Criteria

- [x] `api/v1alpha1/result_types.go` defines `Result`, `ResultList`, `ResultSpec`,
  `ResultStatus`, `Failure`, `Sensitive`
  (`AutoRemediationStatus` is intentionally omitted — the watcher does not use it;
  see CONTROLLER_LLD.md §3.1)

  The types are defined as follows:
  ```go
  type ResultSpec struct {
      Backend      string    `json:"backend"`
      Kind         string    `json:"kind"`
      Name         string    `json:"name"`
      Error        []Failure `json:"error"`
      Details      string    `json:"details"`
      ParentObject string    `json:"parentObject"`
  }

  type Failure struct {
      Text      string      `json:"text,omitempty"`
      Sensitive []Sensitive `json:"sensitive,omitempty"`
  }

  type Sensitive struct {
      Unmasked string `json:"unmasked,omitempty"`
      Masked   string `json:"masked,omitempty"`
  }

  // ResultStatus is intentionally minimal — the watcher reads Results but never
  // writes their status. Only the fields needed for scheme completeness are defined.
  type ResultStatus struct{}
  ```

- [x] Both `Result` and `ResultList` implement `runtime.Object` via `DeepCopyObject()`
- [x] `DeepCopyInto()` performs a true deep copy (no shared slice references)
- [x] `AddToScheme` registers both types with a `runtime.Scheme`
- [x] `GroupVersion` is `core.k8sgpt.ai/v1alpha1`
- [x] Unit tests verify `DeepCopyObject()` produces an independent copy (mutating the
  copy does not affect the original)

---

## Tasks

- [x] Write tests in `api/v1alpha1/result_types_test.go` first (TDD)
- [x] Implement `result_types.go` with all types
- [x] Implement `DeepCopyObject()` and `DeepCopyInto()` for both `Result` and `ResultList`
- [x] Verify scheme registration compiles and registers correctly

---

## Dependencies

**Depends on:** STORY_01 (module setup)
**Blocks:** Controller epic STORY_01 (scheme registration)

---

## Definition of Done

- [x] All deep copy tests pass with `-race`
- [x] `go vet` clean
- [x] No import of the full k8sgpt-operator module
