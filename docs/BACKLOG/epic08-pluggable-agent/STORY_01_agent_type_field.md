# Story 01: AgentType in Config and CRD Types

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **cluster operator**, I want to set `AGENT_TYPE=claude` in the watcher deployment so
the watcher creates `RemediationJob` objects that record which agent should handle them,
without changing any other behaviour.

---

## Acceptance Criteria

- [ ] `internal/config/config.go`: `Config` gains `AgentType string`; loaded from env var
  `AGENT_TYPE`; default value `"opencode"` when env var is absent; no validation of the
  value at config load time (validation happens in the registry at startup)
- [ ] `api/v1alpha1/remediationjob_types.go`: `RemediationJobSpec` gains
  `AgentType string \`json:"agentType"\``
- [ ] `internal/provider/provider.go`: `SourceProviderReconciler` propagates
  `cfg.AgentType` into `RemediationJobSpec.AgentType` when creating a new `RemediationJob`
- [ ] Unit tests for config: `AGENT_TYPE` present → value used; `AGENT_TYPE` absent →
  defaults to `"opencode"`
- [ ] All existing tests continue to pass without modification

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Add `AgentType` to `Config` with default in `FromEnv()`
- [ ] Add `AgentType` field to `RemediationJobSpec`
- [ ] Update `SourceProviderReconciler` to set `spec.agentType` when creating a `RemediationJob`

---

## Dependencies

**Depends on:** epic02 complete
**Blocks:** STORY_02, STORY_03, STORY_04

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
