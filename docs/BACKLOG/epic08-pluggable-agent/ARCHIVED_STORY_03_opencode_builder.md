# Story 03: Extract opencode Builder to Sub-Package

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **developer**, I want the opencode-specific Job building logic isolated in
`internal/jobbuilder/opencode/` so it is clear which code is opencode-specific and
a future maintainer can delete it without touching anything else.

---

## Acceptance Criteria

- [ ] New package `internal/jobbuilder/opencode/` containing:
  - `builder.go` — `Builder` struct with `New(cfg Config) (*Builder, error)`;
    `AgentType() string` returns `"opencode"`; `Build(*v1alpha1.RemediationJob)` produces
    identical output to the current `internal/jobbuilder` `Build()` with one change: the
    prompt ConfigMap name is `"agent-prompt-opencode"` (see STORY_07)
  - `builder_test.go` — identical test coverage to existing `internal/jobbuilder/job_test.go`
  - Compile-time assertion `var _ domain.JobBuilder = (*Builder)(nil)`
- [ ] `internal/jobbuilder/job.go` and `internal/jobbuilder/job_test.go` — **deleted**
  (the package is replaced entirely by the sub-package)
- [ ] `internal/jobbuilder/registry.go` remains in `internal/jobbuilder/` (not moved)
- [ ] All callers updated to import `internal/jobbuilder/opencode` instead of `internal/jobbuilder`
- [ ] All existing tests pass with identical outcomes

---

## Tasks

- [ ] Write tests first in `internal/jobbuilder/opencode/builder_test.go` (copy + adapt
  existing tests; run to confirm they fail before implementing)
- [ ] Implement `internal/jobbuilder/opencode/builder.go`
- [ ] Delete `internal/jobbuilder/job.go` and `internal/jobbuilder/job_test.go`
- [ ] Update `cmd/watcher/main.go` import and constructor call
- [ ] Verify no other files import `internal/jobbuilder` for the builder struct (grep before
  and after)

---

## Dependencies

**Depends on:** STORY_02
**Blocks:** STORY_05

---

## Notes

The only intentional behaviour change from the current `Build()` is the ConfigMap name:
`"opencode-prompt"` → `"agent-prompt-opencode"`. This is coordinated with STORY_07 which
renames the ConfigMap in the deploy manifests. Do not make any other behaviour changes.

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
- [ ] No files in `internal/jobbuilder/` reference opencode directly (only `registry.go`
  and the `opencode/` sub-package)
