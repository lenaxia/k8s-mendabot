# Story: Domain Interfaces

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 45 minutes

---

## User Story

As a **developer**, I want the job builder and source provider exposed as interfaces so
that reconcilers depend on abstractions rather than concrete implementations, enabling
unit testing without real Kubernetes objects or external signal sources.

---

## Acceptance Criteria

- [x] `internal/domain/interfaces.go` defines:
  ```go
  type JobBuilder interface {
      Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error)
  }
  ```
- [x] `internal/domain/provider.go` defines `SourceProvider` with the full four-method
  interface from PROVIDER_LLD.md §3:
  ```go
  type SourceProvider interface {
      ProviderName() string
      ObjectType() client.Object
      ExtractFinding(obj client.Object) (*Finding, error)
      Fingerprint(f *Finding) string
  }
  ```
  (This file is created in STORY_01; this story verifies it is correct and the interface
  is used consistently throughout the codebase.)
- [x] The `JobBuilder` interface is the only thing the `RemediationJobReconciler` imports
  from the job builder package — the reconciler never references `jobbuilder.Builder` directly
- [ ] The concrete `*jobbuilder.Builder` satisfies `domain.JobBuilder` (verified by a
  compile-time assertion in `internal/jobbuilder/job.go`):
  ```go
  var _ domain.JobBuilder = (*Builder)(nil)
  ```
  **DEFERRED to STORY_03** — `internal/jobbuilder/` package does not yet exist.
- [ ] `K8sGPTProvider` satisfies `domain.SourceProvider` (verified by a
  compile-time assertion in `internal/provider/k8sgpt/provider.go`):
  ```go
  var _ domain.SourceProvider = (*K8sGPTProvider)(nil)
  ```
  **DEFERRED to STORY_03** — `internal/provider/k8sgpt/` package does not yet exist.
- [x] No functional logic is added in this story — interface definitions only

---

## Tasks

- [x] Add `JobBuilder` interface to `internal/domain/interfaces.go`
- [x] Verify `internal/domain/provider.go` (from STORY_01) has the correct 4-method
  `SourceProvider` interface matching PROVIDER_LLD.md §3
- [ ] Add compile-time assertion TODOs in `internal/jobbuilder/` and
  `internal/provider/k8sgpt/` for when those packages are created
  **DEFERRED to STORY_03** — packages do not yet exist.
- [x] Write compile-level test in `internal/domain/interfaces_test.go` that references
  both interfaces by name (ensures they stay importable)

---

## Dependencies

**Depends on:** STORY_01 (RemediationJob type — interface signatures use `*v1alpha1.RemediationJob`)
**Blocks:** STORY_03 (ReconcilerSkeleton has a `JobBuilder domain.JobBuilder` field and uses provider loop)
**Blocks:** STORY_05 (fake implementations satisfy these interfaces)

---

## Definition of Done

- [x] `go build ./...` clean
- [x] `go vet ./...` clean
- [x] `JobBuilder` interface lives in `internal/domain`, not in `internal/controller` or `internal/jobbuilder`
- [x] `SourceProvider` interface lives in `internal/domain`, not in `internal/provider` or `internal/provider/k8sgpt`
