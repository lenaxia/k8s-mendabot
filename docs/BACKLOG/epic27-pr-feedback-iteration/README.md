# Epic 27: PR and Issue Feedback Iteration

## Purpose

When the agent opens a PR or issue, the interaction does not end there. Human reviewers
leave comments, request changes, ask clarifying questions, or point out that the proposed
fix is wrong. Today mechanic is deaf to all of this. A reviewed PR sits with unaddressed
comments until a human manually intervenes.

This epic adds a **feedback watch loop**: the watcher polls open sinks (PRs and issues
associated with active `RemediationJob` objects) for new comments, and when it detects
actionable feedback it dispatches a follow-up agent Job to review the comment, update
the proposed fix, and respond.

## Status: Not Started

## Dependencies

- epic26-auto-close-resolved complete (GitHub App token provider in watcher, `SinkRef`
  model, `internal/github/token.go` — all reused directly)
- epic01-controller complete (`RemediationJobReconciler`)
- epic03-agent-image complete (agent image must support `gh pr comment` and `gh pr edit`)

## Blocks

Nothing — this is a standalone enhancement once epic26 is complete.

## Success Criteria

- [ ] `FeedbackPoller` interface exists in `internal/domain/feedback.go` with method
      `PollComments(ctx, sinkRef SinkRef, since time.Time) ([]Comment, error)`
- [ ] `GitHubFeedbackPoller` implementation in `internal/feedback/github/` that calls
      `gh pr view --json comments` or `gh issue view --json comments`
- [ ] `RemediationJobReconciler` reconciles `Succeeded` jobs with a non-empty `SinkRef`
      by polling for new comments at a configurable interval (`FEEDBACK_POLL_INTERVAL`,
      default: `5m`)
- [ ] When actionable feedback is detected, a follow-up agent Job is dispatched with
      the comment text injected as `FEEDBACK_COMMENT` env var
- [ ] The follow-up Job uses a `feedback` prompt mode (separate STEP sequence focused
      on addressing the comment, not re-investigating from scratch)
- [ ] A `RemediationJob` in feedback-iteration state has phase `AwaitingFeedback`;
      transitions to `Iterating` when a follow-up Job is dispatched
- [ ] Maximum iteration count enforced via `FEEDBACK_MAX_ITERATIONS` (default: `3`)
      to prevent infinite back-and-forth; transitions to `FeedbackExhausted` when limit hit
- [ ] `FEEDBACK_WATCH=false` env var disables all feedback polling (default: `true`)
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] Worklog entry created

## Design

### New RemediationJob phases

```
Pending → Dispatched → Succeeded → AwaitingFeedback → Iterating → Succeeded (loop)
                                                                 └→ FeedbackExhausted (terminal)
```

`AwaitingFeedback` is non-terminal: the `RemediationJobReconciler` requeues these
objects at `FEEDBACK_POLL_INTERVAL` to check for new comments.

`FeedbackExhausted` is terminal. The human must take over. A Kubernetes Event is emitted:
`"Feedback iteration limit reached. Manual review required."`

### Comment model

```go
type Comment struct {
    ID        string
    Author    string
    Body      string
    CreatedAt time.Time
    // IsActionable is true if the comment is from a human (not a bot) and is not
    // a simple acknowledgement. The classifier uses basic heuristics (see below).
    IsActionable bool
}
```

### Actionability heuristics

A comment is considered actionable if **all** of the following hold:

1. Author is not a known bot (`github-actions[bot]`, `dependabot[bot]`, `mechanic[bot]`)
2. Body length > 20 characters (filters "+1", "thanks", emoji reactions)
3. Body does not match a pure approval pattern:
   `(?i)^(lgtm|looks good|approved?|ship it|:+1:|👍|✅)\s*$`

The heuristics are intentionally conservative — false negatives (missing a real comment)
are better than false positives (re-running the agent on noise).

### FeedbackPoller interface

```go
// FeedbackPoller polls a sink for comments added after a given time.
type FeedbackPoller interface {
    PollComments(ctx context.Context, ref SinkRef, since time.Time) ([]Comment, error)
}
```

### Follow-up agent Job

The follow-up Job is a standard `batch/v1` Job constructed by `JobBuilder`, with two
additional env vars:

```
FEEDBACK_MODE=true
FEEDBACK_COMMENT=<comment body, truncated to 2000 chars>
FEEDBACK_COMMENT_AUTHOR=<author login>
FEEDBACK_ITERATION=<current iteration number, 1-based>
```

The feedback prompt mode (a separate `feedback-mode.txt` ConfigMap key) instructs the
agent to:
1. Read the existing PR/branch diff to understand what was already proposed
2. Read the reviewer comment in `FEEDBACK_COMMENT`
3. Update the fix to address the feedback
4. Push an amended commit to the same branch (no new PR)
5. Reply to the reviewer comment with a summary of what was changed

The follow-up Job's name includes an iteration suffix:
`mechanic-agent-<fingerprint[:12]>-fb<N>`

### Reconciler changes

`RemediationJobReconciler.Reconcile` gains a new branch for `AwaitingFeedback` phase:

```
AwaitingFeedback:
  if time.Since(rjob.Status.LastFeedbackPollAt) < pollInterval → requeue after remainder
  comments, err := poller.PollComments(ctx, rjob.Status.SinkRef, rjob.Status.LastFeedbackPollAt)
  patch LastFeedbackPollAt = now
  if any comment is actionable:
    if rjob.Status.FeedbackIterationCount >= maxIterations:
      transition to FeedbackExhausted; emit Event
    else:
      dispatch follow-up Job; transition to Iterating; increment FeedbackIterationCount
  else:
    requeue after pollInterval
```

When the follow-up Job succeeds, the reconciler transitions back to `AwaitingFeedback`
to continue watching for further feedback.

### Configuration

```bash
# Disable feedback polling entirely (default: true)
FEEDBACK_WATCH=true

# How often to poll open sinks for new comments (default: 5m)
FEEDBACK_POLL_INTERVAL=5m

# Maximum number of feedback-driven re-investigations per RemediationJob (default: 3)
FEEDBACK_MAX_ITERATIONS=3
```

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| Feedback domain types + FeedbackPoller interface | [STORY_00_domain_types.md](STORY_00_domain_types.md) | Not Started | High | 1h |
| New RemediationJob phases (AwaitingFeedback, Iterating, FeedbackExhausted) | [STORY_01_phases.md](STORY_01_phases.md) | Not Started | High | 2h |
| GitHubFeedbackPoller implementation | [STORY_02_github_feedback_poller.md](STORY_02_github_feedback_poller.md) | Not Started | High | 2h |
| RemediationJobReconciler feedback watch loop | [STORY_03_reconciler_feedback_loop.md](STORY_03_reconciler_feedback_loop.md) | Not Started | Critical | 4h |
| Follow-up Job dispatch via JobBuilder | [STORY_04_followup_job.md](STORY_04_followup_job.md) | Not Started | High | 2h |
| Feedback prompt mode (feedback-mode.txt) | [STORY_05_feedback_prompt.md](STORY_05_feedback_prompt.md) | Not Started | Medium | 2h |
| Config + FEEDBACK_WATCH escape hatch | [STORY_06_config.md](STORY_06_config.md) | Not Started | Medium | 1h |

## Technical Overview

### New files

| File | Purpose |
|------|---------|
| `internal/domain/feedback.go` | `Comment` type, `FeedbackPoller` interface, actionability classifier |
| `internal/domain/feedback_test.go` | Classifier unit tests (happy + unhappy paths) |
| `internal/feedback/github/poller.go` | `GitHubFeedbackPoller` using `gh` CLI |
| `internal/feedback/github/poller_test.go` | Poller tests with mock `gh` output |

### Modified files

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add `AwaitingFeedback`, `Iterating`, `FeedbackExhausted` phases; add `FeedbackIterationCount int`, `LastFeedbackPollAt *metav1.Time` to status |
| `internal/controller/remediationjob_controller.go` | Add `AwaitingFeedback` reconcile branch; follow-up Job dispatch |
| `internal/controller/remediationjob_controller_test.go` | Tests for feedback loop, exhaustion, and FEEDBACK_WATCH=false |
| `internal/jobbuilder/job.go` | Accept `FeedbackMode bool` + feedback env vars in `BuildInput` |
| `internal/jobbuilder/job_test.go` | Tests for feedback Job construction |
| `internal/config/config.go` | Add `FeedbackWatch bool`, `FeedbackPollInterval time.Duration`, `FeedbackMaxIterations int` |
| `internal/config/config_test.go` | Config parsing tests |
| `deploy/kustomize/configmap-prompt.yaml` | Add `feedback-mode.txt` key |
| `charts/mechanic/files/prompts/feedback-mode.txt` | New feedback prompt |
| `testdata/crds/remediationjob_crd.yaml` | Add new phase constants and status fields |

## Definition of Done

- [ ] All unit tests pass: `go test -timeout 30s -race ./...`
- [ ] `go build ./...` succeeds
- [ ] `FEEDBACK_WATCH=false` disables all polling (verified by test)
- [ ] Follow-up Job is dispatched when an actionable comment is detected (integration test)
- [ ] `FeedbackExhausted` phase reached after `FEEDBACK_MAX_ITERATIONS` (integration test)
- [ ] Existing phases (`Pending`, `Dispatched`, `Succeeded`, etc.) are unaffected
- [ ] Worklog entry created in `docs/WORKLOGS/`
