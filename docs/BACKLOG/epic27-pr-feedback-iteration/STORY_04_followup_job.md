# Story 04: Follow-up Job Dispatch via JobBuilder

## Status: Not Started

## Objective

Extend `JobBuilder` to support feedback-mode follow-up Jobs. A follow-up Job is
identical to a normal investigation Job except for its name pattern and three additional
env vars (`FEEDBACK_MODE`, `FEEDBACK_COMMENT`, `FEEDBACK_COMMENT_AUTHOR`, `FEEDBACK_ITERATION`).

## Acceptance Criteria

- [ ] `BuildInput` in `internal/jobbuilder/job.go` gains three fields:
  ```go
  FeedbackMode          bool
  FeedbackComment       string  // truncated to 2000 chars if longer
  FeedbackCommentAuthor string
  FeedbackIteration     int     // 1-based
  ```
- [ ] When `FeedbackMode == true`, the Job name is `mechanic-agent-<fingerprint[:12]>-fb<N>`
      where N is `FeedbackIteration`
- [ ] When `FeedbackMode == true`, three env vars are added to the agent container:
  ```
  FEEDBACK_MODE=true
  FEEDBACK_COMMENT=<body, truncated to 2000 chars>
  FEEDBACK_COMMENT_AUTHOR=<author login>
  FEEDBACK_ITERATION=<N as string>
  ```
- [ ] When `FeedbackMode == false` (normal job), behaviour is unchanged
- [ ] `FeedbackComment` longer than 2000 chars is silently truncated to 2000 chars before
      being set as the env var value
- [ ] `internal/jobbuilder/job_test.go` adds tests:
  - Feedback Job name has `-fb<N>` suffix
  - Feedback env vars are present when `FeedbackMode=true`
  - Normal Job (FeedbackMode=false) does not have feedback env vars
  - Comment truncation: 2001-char body → 2000-char env var value
  - Comment exactly 2000 chars → not truncated
- [ ] `go test -timeout 30s -race ./internal/jobbuilder/...` passes

## Modified Files

| File | Change |
|------|--------|
| `internal/jobbuilder/job.go` | Add `FeedbackMode`, `FeedbackComment`, `FeedbackCommentAuthor`, `FeedbackIteration` to `BuildInput`; update `Build()` |
| `internal/jobbuilder/job_test.go` | Tests for feedback Job construction |

## Notes

- Check the existing `BuildInput` struct and `Build()` function signature before modifying.
- The truncation must happen in `Build()`, not at the call site, to keep callers simple.
- All existing tests must continue to pass — `FeedbackMode` defaults to `false`.
- `fmt.Sprintf("mechanic-agent-%s-fb%d", fingerprint[:12], feedbackIteration)` for job name.
