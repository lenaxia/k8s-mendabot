# Story 05: Wire Registry into RemediationJobReconciler

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **developer**, I want the `RemediationJobReconciler` to select the correct
`JobBuilder` from the registry based on `rjob.Spec.AgentType` so no switch statement
or agent-specific import exists in the reconciler.

---

## Acceptance Criteria

- [ ] `internal/controller/remediationjob_controller.go`:
  - `RemediationJobController` struct field changes from `jobBuilder domain.JobBuilder`
    to `registry *jobbuilder.Registry`
  - `New(...)` constructor accepts `*jobbuilder.Registry` instead of `domain.JobBuilder`
  - Reconcile loop calls `b, err := r.registry.Get(rjob.Spec.AgentType)` before calling
    `b.Build(rjob)`; an `ErrUnknownAgentType` result causes the reconciler to set
    `RemediationJob` phase to `Failed` with a descriptive message and stop requeuing
    (this is a configuration error, not a transient error — do not requeue)
- [ ] `cmd/watcher/main.go`:
  - Constructs both the opencode and claude builders
  - Calls `jobbuilder.NewRegistry(opencodeBuilder, claudeBuilder)`; fails fast if
    `NewRegistry` returns an error
  - Passes the registry to the controller constructor
- [ ] Unit tests in `remediationjob_controller_test.go`:
  - Happy path: known agent type → builder called, Job created
  - Unhappy path: unknown agent type → phase set to `Failed`, no Job created, no requeue
- [ ] All existing controller tests pass

---

## Tasks

- [ ] Write tests first (TDD) — update existing controller tests to use registry; add
  the unknown-agent-type test case
- [ ] Update `RemediationJobController` struct and constructor
- [ ] Update reconcile loop with `registry.Get` call and error handling
- [ ] Update `cmd/watcher/main.go`

---

## Dependencies

**Depends on:** STORY_02, STORY_03, STORY_04
**Blocks:** STORY_08

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
- [ ] No agent-specific imports in `internal/controller/`
