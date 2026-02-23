# Story 02: AgentType() on JobBuilder Interface + Registry

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **developer**, I want a `Registry` that maps agent type strings to `JobBuilder`
implementations so the reconciler can select the correct builder at runtime without a
switch statement or any opencode-specific imports.

---

## Acceptance Criteria

- [ ] `internal/domain/interfaces.go`: `JobBuilder` interface gains `AgentType() string`
  method alongside the existing `Build` method:
  ```go
  type JobBuilder interface {
      AgentType() string
      Build(*v1alpha1.RemediationJob) (*batchv1.Job, error)
  }
  ```
- [ ] `internal/jobbuilder/registry.go` (new file):
  - `Registry` struct with `builders map[string]domain.JobBuilder`
  - `NewRegistry(builders ...domain.JobBuilder) (*Registry, error)` — returns error if
    any two builders share the same `AgentType()` value or if the slice is empty
  - `Get(agentType string) (domain.JobBuilder, error)` — returns a typed error
    `ErrUnknownAgentType` (defined in the same file) for unregistered types
- [ ] Unit tests for `Registry`:
  - Happy path: single builder registered, `Get` returns it
  - Happy path: two builders registered with different types, each `Get` returns correct one
  - Unhappy: duplicate `AgentType()` value → `NewRegistry` returns error
  - Unhappy: empty slice → `NewRegistry` returns error
  - Unhappy: `Get` with unknown type → returns `ErrUnknownAgentType`
- [ ] Compile-time assertion in `registry.go` that `Registry` satisfies no interface
  (it is not an interface itself), but any concrete `JobBuilder` implementation must be
  verified with `var _ domain.JobBuilder = (*ConcreteBuilder)(nil)` in its own file
- [ ] All existing tests continue to pass

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Add `AgentType() string` to `domain.JobBuilder` interface
- [ ] Update existing `jobbuilder.Builder` to implement `AgentType()` returning `"opencode"`
  (temporarily, before STORY_03 moves it)
- [ ] Implement `Registry` in `internal/jobbuilder/registry.go`
- [ ] Define `ErrUnknownAgentType` as a typed error

---

## Dependencies

**Depends on:** STORY_01
**Blocks:** STORY_03, STORY_04, STORY_05

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
