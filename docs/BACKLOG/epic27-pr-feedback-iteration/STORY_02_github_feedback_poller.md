# Story 02: GitHubFeedbackPoller Implementation

## Status: Not Started

## Objective

Implement `GitHubFeedbackPoller` in `internal/feedback/github/poller.go`. This uses
the `gh` CLI (already present in the agent image) to poll GitHub PR or issue comments.

## Acceptance Criteria

- [ ] `internal/feedback/github/poller.go` defines `GitHubFeedbackPoller` struct
- [ ] `GitHubFeedbackPoller` implements `domain.FeedbackPoller`
- [ ] `PollComments` calls `gh pr view <number> --repo <owner/repo> --json comments`
      (or `gh issue view`) and returns all comments created after `since`
- [ ] `PollComments` deserializes the JSON response into `[]domain.Comment`, calls
      `domain.IsActionableComment` on each, and sets `IsActionable` accordingly
- [ ] `SinkRef.Type` determines which gh subcommand to use:
      `"github-pr"` → `gh pr view`, `"github-issue"` → `gh issue view`
- [ ] Unknown `SinkRef.Type` returns an error: `"unsupported sink type: <type>"`
- [ ] `PollComments` returns `nil, nil` (not an error) when `SinkRef.URL` is empty
- [ ] `internal/feedback/github/poller_test.go` uses a mock `gh` executor (inject via
      interface or function field) to test:
  - Happy path: PR with 2 comments (1 actionable, 1 bot) → correct slice returned
  - Happy path: comments before `since` are filtered out
  - Empty comments → returns empty slice, no error
  - Unknown sink type → returns error
  - Empty SinkRef.URL → returns nil, nil
  - `gh` CLI error (non-zero exit) → returns wrapped error
- [ ] `go test -timeout 30s -race ./internal/feedback/...` passes

## New Files

| File | Purpose |
|------|---------|
| `internal/feedback/github/poller.go` | `GitHubFeedbackPoller` implementation |
| `internal/feedback/github/poller_test.go` | Poller unit tests with mock executor |

## Notes

- `SinkRef` is defined in `api/v1alpha1`. Import as
  `v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"`.
- Check `internal/sink/github/closer.go` for the existing pattern of parsing `SinkRef`
  fields (`Number`, `Repo`). Follow the same approach.
- The `gh` executor should be injected via a field (e.g. `RunGH func(args ...string) ([]byte, error)`)
  to allow test mocks without spawning real processes.
- `gh pr view --json comments` returns:
  ```json
  {"comments": [{"author": {"login": "alice"}, "body": "LGTM", "createdAt": "2026-01-01T00:00:00Z", "id": "IC_abc123"}]}
  ```
- `gh issue view --json comments` returns the same shape.
- Map `comment.author.login` → `Comment.Author`, `comment.body` → `Comment.Body`,
  `comment.createdAt` → `Comment.CreatedAt`, `comment.id` → `Comment.ID`.
- Filter: only include comments where `CreatedAt.After(since)`.
