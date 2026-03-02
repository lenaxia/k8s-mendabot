# Story 01: New RemediationJob Phases

## Status: Not Started

## Objective

Add three new `RemediationJobPhase` constants (`AwaitingFeedback`, `Iterating`,
`FeedbackExhausted`) and two new `RemediationJobStatus` fields (`FeedbackIterationCount`,
`LastFeedbackPollAt`) to the CRD types. Also update the envtest CRD testdata file.

## Acceptance Criteria

- [ ] Three new phase constants in `api/v1alpha1/remediationjob_types.go`:
  ```go
  PhaseAwaitingFeedback  RemediationJobPhase = "AwaitingFeedback"
  PhaseIterating         RemediationJobPhase = "Iterating"
  PhaseFeedbackExhausted RemediationJobPhase = "FeedbackExhausted"
  ```
- [ ] Two new fields in `RemediationJobStatus`:
  ```go
  // FeedbackIterationCount is the number of feedback-driven re-investigations dispatched so far.
  FeedbackIterationCount int `json:"feedbackIterationCount,omitempty"`
  // LastFeedbackPollAt is the timestamp of the last comment poll for this job.
  LastFeedbackPollAt *metav1.Time `json:"lastFeedbackPollAt,omitempty"`
  ```
- [ ] `DeepCopyInto` updated to handle `LastFeedbackPollAt *metav1.Time` (pointer copy)
- [ ] `testdata/crds/remediationjob_crd.yaml` updated with:
  - `feedbackIterationCount: {type: integer}` under status.properties
  - `lastFeedbackPollAt: {type: string, format: date-time}` under status.properties
- [ ] `go test -timeout 30s -race ./...` still passes (no regressions)

## Modified Files

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add 3 phase constants + 2 status fields + DeepCopyInto fix |
| `testdata/crds/remediationjob_crd.yaml` | Add 2 status field entries |

## Notes

- `PhaseFeedbackExhausted` is terminal (like `PhasePermanentlyFailed`).
- `PhaseAwaitingFeedback` is non-terminal — the reconciler requeues at `FEEDBACK_POLL_INTERVAL`.
- `PhaseIterating` is transitional — set when a follow-up Job is dispatched; the reconciler
  transitions back to `PhaseAwaitingFeedback` once the follow-up Job succeeds.
- `LastFeedbackPollAt` uses `*metav1.Time` (pointer) so it can be `nil` (never polled).
  `DeepCopyInto` must deep-copy this pointer: if non-nil, allocate new `metav1.Time` and copy.
- The CRD testdata rule: see README-LLM.md §Testing Requirements / Rule 1.
