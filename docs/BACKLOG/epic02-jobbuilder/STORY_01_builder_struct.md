# Story: Builder Struct and Config

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want a `Builder` struct with a `New(cfg Config)` constructor so the
job builder has a clean, testable entry point with all configuration injected at creation time.

---

## Acceptance Criteria

- [ ] `Config` struct defined with all fields from JOBBUILDER_LLD.md §3 in `internal/jobbuilder/job.go`:
  ```go
  type Config struct {
      AgentNamespace string // namespace where Jobs are created
  }
  ```
  **Note:** This is `jobbuilder.Config`, not `domain.JobBuilderConfig`. There is no
  `domain.JobBuilderConfig` type. All other Job parameters (`AgentImage`, `AgentSA`,
  `GitOpsRepo`, `GitOpsManifestRoot`) are read from `rjob.Spec` in `Build()`.
- [ ] `Builder` struct holds a `jobbuilder.Config`
- [ ] `New(cfg Config) (*Builder, error)` constructor validates no required field
  is empty, returning an error if any is missing
- [ ] Compile-time assertion `var _ domain.JobBuilder = (*Builder)(nil)` is present
- [ ] `Build(*v1alpha1.RemediationJob) (*batchv1.Job, error)` method signature matches the
  `domain.JobBuilder` interface
- [ ] Unit tests: valid config constructs successfully; each required field missing returns
  a descriptive error

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Define `Config` and `Builder` in `internal/jobbuilder/job.go`
- [ ] Implement `New()` with validation

---

## Dependencies

**Depends on:** Foundation epic00 complete
**Blocks:** All other jobbuilder stories

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
