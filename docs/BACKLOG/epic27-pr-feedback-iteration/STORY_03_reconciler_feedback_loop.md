# Story 03: RemediationJobReconciler Feedback Watch Loop

## Status: Not Started

## Objective

Add the `AwaitingFeedback` reconcile branch to `RemediationJobReconciler`. This is the
core feedback watch loop: poll for new comments, dispatch follow-up Jobs, enforce the
iteration limit, and emit the `FeedbackExhausted` event.

## Acceptance Criteria

- [ ] `RemediationJobReconciler` has a `FeedbackPoller domain.FeedbackPoller` field
      (nil when `FEEDBACK_WATCH=false` or feedback is disabled)
- [ ] When `FEEDBACK_WATCH=false` (config), `FeedbackPoller` is set to nil and the
      `AwaitingFeedback` branch skips polling entirely (transitions to `Succeeded`)
- [ ] When `PhaseSucceeded` and `SinkRef.URL != ""` and `FeedbackPoller != nil`:
      the reconciler transitions the job to `PhaseAwaitingFeedback`
- [ ] `PhaseAwaitingFeedback` reconcile branch:
  1. If `time.Since(LastFeedbackPollAt) < FeedbackPollInterval` → requeue after remainder
  2. Call `FeedbackPoller.PollComments(ctx, rjob.Status.SinkRef, LastFeedbackPollAt)`
  3. Patch `LastFeedbackPollAt = now`
  4. If any comment has `IsActionable == true`:
     - If `FeedbackIterationCount >= FeedbackMaxIterations`:
       transition to `PhaseFeedbackExhausted`; emit Event: `"Feedback iteration limit reached. Manual review required."`
     - Else: dispatch follow-up Job (via JobBuilder); transition to `PhaseIterating`;
       increment `FeedbackIterationCount`
  5. Else: requeue after `FeedbackPollInterval`
- [ ] `PhaseIterating`: when the follow-up Job succeeds, transition back to `PhaseAwaitingFeedback`
- [ ] `PhaseFeedbackExhausted` is terminal — no further reconciliation
- [ ] `internal/controller/remediationjob_controller_test.go` adds integration tests:
  - `FEEDBACK_WATCH=false` → job stays `Succeeded`, no polling
  - Actionable comment → follow-up Job dispatched, phase becomes `Iterating`
  - `FeedbackIterationCount >= max` → phase becomes `FeedbackExhausted`, Event emitted
  - Non-actionable comment → requeued, no Job dispatched
  - `SinkRef.URL` empty on Succeeded job → no transition to `AwaitingFeedback`

## Modified Files

| File | Change |
|------|--------|
| `internal/controller/remediationjob_controller.go` | Add `FeedbackPoller` field + `AwaitingFeedback`/`Iterating` branches |
| `internal/controller/remediationjob_controller_test.go` | Integration tests for feedback loop |

## Notes

- Follow the existing reconcile switch pattern in `remediationjob_controller.go`.
- `FeedbackPoller` is injected as a field so tests can use a mock.
- The follow-up Job is built by `JobBuilder` using the new `FeedbackMode` flag (Story 04).
- Use `r.Recorder.Event(rjob, corev1.EventTypeWarning, "FeedbackExhausted", "Feedback iteration limit reached. Manual review required.")`.
- For the `PhaseIterating → PhaseAwaitingFeedback` transition, check the follow-up Job
  status the same way the main Job status is checked (watch the `batch/v1` Job by
  label `mechanic.io/remediation-job=<name>`).
- The follow-up Job name pattern is `mechanic-agent-<fingerprint[:12]>-fb<N>` where N
  is the new `FeedbackIterationCount` value (1-based, after increment).
- Pre-test cleanup for integration tests: delete any stale follow-up Job before each test
  (per README-LLM.md §Testing Requirements Rule 2).
