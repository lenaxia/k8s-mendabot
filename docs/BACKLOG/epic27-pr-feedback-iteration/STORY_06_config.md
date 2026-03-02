# Story 06: Config + FEEDBACK_WATCH Escape Hatch

## Status: Not Started

## Objective

Add three new config fields (`FeedbackWatch`, `FeedbackPollInterval`, `FeedbackMaxIterations`)
to `internal/config/config.go` and wire them into `main.go`.

## Acceptance Criteria

- [ ] `Config` struct in `internal/config/config.go` gains:
  ```go
  // FeedbackWatch enables feedback comment polling on open sinks (default: true).
  FeedbackWatch bool
  // FeedbackPollInterval is how often to poll for new comments (default: 5m).
  FeedbackPollInterval time.Duration
  // FeedbackMaxIterations is the max number of feedback iterations per job (default: 3).
  FeedbackMaxIterations int
  ```
- [ ] `FromEnv()` parses:
  - `FEEDBACK_WATCH` (bool, default `true`) — any value other than `"false"` is treated as `true`
  - `FEEDBACK_POLL_INTERVAL` (duration string, default `"5m"`) — use `time.ParseDuration`
  - `FEEDBACK_MAX_ITERATIONS` (int, default `3`) — use `strconv.Atoi`
- [ ] `internal/config/config_test.go` adds tests:
  - All three fields default correctly when env vars are absent
  - `FEEDBACK_WATCH=false` → `FeedbackWatch == false`
  - `FEEDBACK_WATCH=true` → `FeedbackWatch == true`
  - `FEEDBACK_POLL_INTERVAL=1m` → `1 * time.Minute`
  - `FEEDBACK_MAX_ITERATIONS=5` → `5`
  - `FEEDBACK_POLL_INTERVAL=invalid` → error returned from `FromEnv()`
  - `FEEDBACK_MAX_ITERATIONS=0` → `0` (zero is valid; disables all iterations)
- [ ] `cmd/watcher/main.go` passes the new config fields to `RemediationJobReconciler`:
  - If `cfg.FeedbackWatch == false`, set `FeedbackPoller = nil` on the reconciler
  - If `cfg.FeedbackWatch == true`, construct and set `GitHubFeedbackPoller`
- [ ] `go test -timeout 30s -race ./internal/config/...` passes
- [ ] `go test -timeout 30s -race ./...` passes (no regressions)

## Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `FeedbackWatch`, `FeedbackPollInterval`, `FeedbackMaxIterations` |
| `internal/config/config_test.go` | Tests for new config fields |
| `cmd/watcher/main.go` | Wire feedback poller into `RemediationJobReconciler` |

## Notes

- `FEEDBACK_WATCH` default is `true` — opt-out, not opt-in.
- Invalid `FEEDBACK_POLL_INTERVAL` (non-parseable duration) should return an error from
  `FromEnv()`, consistent with how other invalid config values are handled.
- `FEEDBACK_MAX_ITERATIONS=0` is technically valid (disables feedback entirely without
  disabling the polling path); this is acceptable.
- Check `internal/config/config.go` for the pattern used by other env var parsers
  (e.g. `PR_AUTO_CLOSE`, `STABILISATION_WINDOW`) and follow it exactly.
