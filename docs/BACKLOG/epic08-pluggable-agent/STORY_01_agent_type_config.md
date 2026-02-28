# Story 01: AgentType Config Field

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **mechanic operator**, I want to configure which agent runner the watcher dispatches
via an `AGENT_TYPE` environment variable, so that the correct agent binary, secret, and
prompt ConfigMap are selected at runtime without code changes.

---

## Background

Today the watcher has no concept of which agent runner it is using — it is implicitly
always OpenCode. This story introduces `AgentType` as a first-class typed config field
so the watcher can select the correct secret name and prompt ConfigMap at dispatch time.

---

## Acceptance Criteria

- [ ] `type AgentType string` and constants `AgentTypeOpenCode`, `AgentTypeClaude` exist
      in `internal/config/config.go`
- [ ] `Config.AgentType AgentType` field populated by `AGENT_TYPE` env var
- [ ] Default is `AgentTypeOpenCode` (`"opencode"`) — existing deployments with no
      `AGENT_TYPE` set continue to work unchanged
- [ ] Startup returns an error for any unknown value
- [ ] `internal/config/config_test.go` has table-driven tests covering:
  - default (unset → `"opencode"`)
  - explicit `"opencode"` → `AgentTypeOpenCode`
  - explicit `"claude"` → `AgentTypeClaude`
  - unknown value → error
- [ ] `go test -timeout 30s -race ./internal/config/...` passes

---

## Technical Implementation

### `internal/config/config.go`

Add after the imports:

```go
// AgentType identifies which agent runner binary the watcher dispatches.
type AgentType string

const (
    AgentTypeOpenCode AgentType = "opencode"
    AgentTypeClaude   AgentType = "claude"
)
```

Add to `Config` struct:

```go
AgentType AgentType // AGENT_TYPE — default "opencode"
```

Add to `FromEnv()`:

```go
agentTypeStr := os.Getenv("AGENT_TYPE")
if agentTypeStr == "" {
    agentTypeStr = string(AgentTypeOpenCode)
}
switch AgentType(agentTypeStr) {
case AgentTypeOpenCode, AgentTypeClaude:
    cfg.AgentType = AgentType(agentTypeStr)
default:
    return Config{}, fmt.Errorf("AGENT_TYPE %q is not supported; accepted values: opencode, claude", agentTypeStr)
}
```

---

## Dependencies

None — standalone config change.

## Definition of Done

- [ ] Types and constants defined
- [ ] `FromEnv()` parses and validates `AGENT_TYPE`
- [ ] All tests pass
