# Story 01: Schema Foundations

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **mechanic developer**, I want `ChainDepth` carried from a Finding through
to the `RemediationJobSpec` and the CRD testdata, so that all downstream
stories have a stable, typed field to read and write.

---

## Problem

`domain.Finding` and `api/v1alpha1.FindingSpec` have no `ChainDepth` field.
Without these additions no other story in this epic can be implemented.

---

## Acceptance Criteria

- [x] `domain.Finding` in `internal/domain/provider.go` has a new field:
  ```go
  ChainDepth int
  ```
- [x] `FindingSpec` in `api/v1alpha1/remediationjob_types.go` has a new field:
  ```go
  ChainDepth int32 `json:"chainDepth,omitempty"`
  ```
- [x] `RemediationJob.DeepCopyInto` in `api/v1alpha1/remediationjob_types.go`
  copies `Spec.Finding.ChainDepth` (the struct is copied by value already; verify
  this is still true after the addition and add an explicit copy if a pointer is
  ever introduced).
- [x] `testdata/crds/remediationjob_crd.yaml` has the new field inside the
  `finding.properties` block (after `details: {type: string}` at line 77):
  ```yaml
  chainDepth: {type: integer}
  ```
  The envtest suite in `internal/controller/suite_test.go` loads CRDs from
  `../../testdata/crds` (resolving to `testdata/crds/` at repo root). This is
  also the path used by `internal/provider/suite_test.go`.
- [x] `SourceProviderReconciler` in `internal/provider/provider.go` maps the
  new field when building `RemediationJobSpec.Finding`. The `FindingSpec`
  struct literal is at lines 403–410; add `ChainDepth` after `Details`:
  ```go
  Finding: v1alpha1.FindingSpec{
      Kind:         finding.Kind,
      Name:         finding.Name,
      Namespace:    finding.Namespace,
      ParentObject: finding.ParentObject,
      Errors:       finding.Errors,
      Details:      finding.Details,
      ChainDepth:   int32(finding.ChainDepth),  // ← add this line
  },
  ```
  This is the only change required in `provider.go` for this story.
- [x] All existing tests still pass (`go test -timeout 30s -race ./...`).

---

## What this story does NOT do

- No detection logic (STORY_02).
- No enforcement / gating logic (STORY_03).
- No circuit breaker (STORY_04).

---

## Files to modify

| File | Change |
|------|--------|
| `internal/domain/provider.go` | Add `ChainDepth int` to `Finding` |
| `api/v1alpha1/remediationjob_types.go` | Add `ChainDepth int32` to `FindingSpec` |
| `testdata/crds/remediationjob_crd.yaml` | Add `chainDepth: {type: integer}` inside the `finding.properties` block |
| `internal/provider/provider.go` | Map `finding.ChainDepth` into `RemediationJobSpec.Finding` |

---

## Testing Requirements

**Unit tests** (`internal/domain/provider_test.go` — this file exists at repo
root `internal/domain/provider_test.go`):
- `FindingFingerprint` is unaffected by `ChainDepth` (depth is not part of the
  deduplication key — two findings that differ only in chain depth must produce
  the same fingerprint).

**Integration tests** (`internal/controller/`):
- Existing controller integration tests must still pass with no changes.
- Add one new test function that creates a `RemediationJob` with
  `Spec.Finding.ChainDepth: 2` via the envtest API server (`k8sClient.Create`)
  and reads it back (`k8sClient.Get`), asserting the field value is preserved.
  There is no existing field round-trip test to copy from; write it from scratch
  following the `k8sClient.Create` / `k8sClient.Get` pattern used in the
  existing suite.

---

## Dependencies

**Depends on:** none (this story is the prerequisite for all others)
**Blocks:** STORY_02, STORY_03, STORY_04

---

## Definition of Done

- [x] All tests pass with `-race`
- [x] `go vet` clean
- [x] `go build ./...` clean
- [x] CRD testdata updated and envtest round-trip test added
- [x] `provider.go` mapping updated
