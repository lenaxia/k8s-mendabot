# Story 00: Feedback Domain Types + FeedbackPoller Interface

## Status: Not Started

## Objective

Define the `Comment` type and `FeedbackPoller` interface in `internal/domain/feedback.go`,
and implement the actionability classifier. This is the domain-layer foundation that all
other epic27 stories build on.

## Acceptance Criteria

- [ ] `Comment` struct defined in `internal/domain/feedback.go` with fields:
      `ID string`, `Author string`, `Body string`, `CreatedAt time.Time`, `IsActionable bool`
- [ ] `FeedbackPoller` interface defined with method:
      `PollComments(ctx context.Context, ref v1alpha1.SinkRef, since time.Time) ([]Comment, error)`
- [ ] `IsActionableComment(c Comment) bool` function implements the three-rule heuristic:
  1. Author not in known bot list: `github-actions[bot]`, `dependabot[bot]`, `mechanic[bot]`
  2. Body length > 20 characters
  3. Body does not match approval pattern: `(?i)^(lgtm|looks good|approved?|ship it|:\+1:|👍|✅)\s*$`
- [ ] `internal/domain/feedback_test.go` covers:
  - Happy path: comment from human with substantive body → `IsActionable: true`
  - Bot author → `IsActionable: false`
  - Short body (≤ 20 chars) → `IsActionable: false`
  - Pure approval body (lgtm, looks good, 👍, ✅, etc.) → `IsActionable: false`
  - Body exactly 21 chars from human (boundary case) → `IsActionable: true`
  - Edge: empty body → `IsActionable: false`
- [ ] `go test -timeout 30s -race ./internal/domain/...` passes

## New Files

| File | Purpose |
|------|---------|
| `internal/domain/feedback.go` | `Comment` type, `FeedbackPoller` interface, `IsActionableComment` |
| `internal/domain/feedback_test.go` | Classifier tests (table-driven) |

## Modified Files

None.

## Notes

- The `FeedbackPoller` interface lives in domain (not in a specific provider package)
  to keep the reconciler testable with a mock poller.
- `IsActionableComment` is a pure function — no receiver, no interface — so it can
  be called directly in tests without any setup.
- Import `v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"` for `SinkRef`.
- The approval regex must be pre-compiled at package init (not per-call) for performance.
