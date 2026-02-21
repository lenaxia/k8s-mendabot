# Story: Structured Logging

**Epic:** [Foundation](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want structured logging (zap) initialised in main.go and passed
explicitly to all components so that log output is machine-parseable and log level is
controllable at runtime.

---

## Acceptance Criteria

- [ ] `go.uber.org/zap` is used throughout — no `fmt.Println` or `log.Printf` in
  production code paths
- [ ] Log level is driven by `Config.LogLevel`
- [ ] Logger is constructed once in `main.go` and passed to components — no global logger
- [ ] All reconciler log lines include structured fields (fingerprint, kind, job name, etc.)
- [ ] `zap.NewProduction()` used in production; `zap.NewDevelopment()` used in tests

---

## Tasks

- [ ] Add zap initialisation to `cmd/watcher/main.go`
- [ ] Create `internal/logging/logging.go` with `New(level string) (*zap.Logger, error)`
- [ ] Write test for `New()` with valid and invalid level strings
- [ ] Ensure logger is passed as a dependency — never accessed globally

---

## Dependencies

**Depends on:** STORY_02 (config)
**Blocks:** Controller epic (reconciler uses the logger)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] No `fmt.Println` or `log.Print` in `internal/` or `cmd/`
- [ ] `go vet` clean
