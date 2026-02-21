# Story: Typed Configuration

**Epic:** [Foundation](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want all runtime configuration read from environment variables into a
strongly-typed `Config` struct at startup so that missing or invalid configuration causes
an immediate, descriptive failure rather than a silent runtime error.

---

## Acceptance Criteria

- [ ] `Config` struct in `internal/config/config.go` with all fields from HLD §13
- [ ] `FromEnv()` constructor reads each field from the environment
- [ ] Missing required fields cause `FromEnv()` to return a descriptive error
- [ ] `MAX_CONCURRENT_JOBS` defaults to `3` if not set
- [ ] `REMEDIATION_JOB_TTL_SECONDS` defaults to `604800` (7 days) if not set
- [ ] `LOG_LEVEL` defaults to `info` if not set
- [ ] Unit tests cover: all fields present, missing required field, invalid int value,
  defaults applied

---

## Config Fields

```go
type Config struct {
    GitOpsRepo               string // GITOPS_REPO — required
    GitOpsManifestRoot       string // GITOPS_MANIFEST_ROOT — required
    AgentImage               string // AGENT_IMAGE — required
    AgentNamespace           string // AGENT_NAMESPACE — required; must equal watcher namespace
    AgentSA                  string // AGENT_SA — required
    SinkType                 string // SINK_TYPE — default "github"
    LogLevel                 string // LOG_LEVEL — default "info"
    MaxConcurrentJobs        int    // MAX_CONCURRENT_JOBS — default 3
    RemediationJobTTLSeconds int    // REMEDIATION_JOB_TTL_SECONDS — default 604800 (7 days)
}
```

---

## Tasks

- [ ] Create `internal/config/config.go` with `Config` struct and `FromEnv()`
- [ ] Write tests in `internal/config/config_test.go` (TDD — tests first)
- [ ] Wire `FromEnv()` call into `cmd/watcher/main.go` with fatal on error

---

## Dependencies

**Depends on:** STORY_01 (module setup)
**Blocks:** STORY_03 (logging), Controller epic

---

## Definition of Done

- [ ] All unit tests pass with `-race`
- [ ] `go vet` clean
- [ ] `FromEnv()` called in main.go with `log.Fatal` on error
