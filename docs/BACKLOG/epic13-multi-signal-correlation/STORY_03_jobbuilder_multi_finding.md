# Story 03: JobBuilder Multi-Finding Support

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **mendabot developer**, I want `JobBuilder.Build()` to accept a slice of correlated
findings and inject them as a `FINDING_CORRELATED_FINDINGS` env var, so that the agent
Job for a correlated group receives the full group context rather than a single finding.

---

## Background

`JobBuilder.Build()` already has the two-argument signature and already injects
`FINDING_CORRELATED_FINDINGS`. The remaining work is injecting `FINDING_CORRELATION_GROUP_ID`
from the primary's labels. The label is applied to the primary by `labelAsPrimary` (STORY_02)
before `dispatch` calls `Build`, so the label is reliably present at the time `Build` runs.

### What is already done

The following work was applied during a prior (incomplete) implementation attempt:

- `internal/domain/interfaces.go` — `JobBuilder.Build()` already uses the two-argument
  signature: `Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec)`
- `internal/jobbuilder/job.go` — `Build()` already accepts the second argument and
  injects `FINDING_CORRELATED_FINDINGS` when `len(correlatedFindings) > 0`
- `internal/controller/fakes_test.go` — `fakeJobBuilder.Build()` already uses the
  two-argument signature
- `internal/controller/remediationjob_controller.go` — the existing `dispatch()` call
  at line 298 already passes `nil` as the second arg: `r.JobBuilder.Build(rjob, nil)`

### What is NOT done (remaining work)

- `FINDING_CORRELATION_GROUP_ID` env var injection is absent from `internal/jobbuilder/job.go`
- Tests for `FINDING_CORRELATION_GROUP_ID` injection are absent from `internal/jobbuilder/job_test.go`

---

## Acceptance Criteria

- [x] `JobBuilder.Build()` signature accepts `correlatedFindings []v1alpha1.FindingSpec`
      (already done)
- [x] When `len(correlatedFindings) > 0`, `FINDING_CORRELATED_FINDINGS` is injected as
      a JSON-encoded array (already done)
- [x] When `len(correlatedFindings) == 0`, `FINDING_CORRELATED_FINDINGS` is not set
      (already done)
- [x] When the primary `RemediationJob` carries a `mendabot.io/correlation-group-id` label,
      `FINDING_CORRELATION_GROUP_ID` is injected as an env var with that value
- [x] When the primary does NOT carry that label, `FINDING_CORRELATION_GROUP_ID` is not set
- [x] `internal/jobbuilder/job_test.go` covers:
  - Single finding — no `FINDING_CORRELATED_FINDINGS`, no `FINDING_CORRELATION_GROUP_ID`
  - Two correlated findings — `FINDING_CORRELATED_FINDINGS` is valid JSON, `FINDING_CORRELATION_GROUP_ID` is set
  - Primary without group ID label — `FINDING_CORRELATION_GROUP_ID` is absent
- [x] `go test -timeout 30s -race ./internal/jobbuilder/...` passes

---

## Technical Implementation

### `FINDING_CORRELATION_GROUP_ID` injection

Add after the existing `FINDING_CORRELATED_FINDINGS` block in `Build()` (at line 185 in
`internal/jobbuilder/job.go`, after the `if len(correlatedFindings) > 0` block closes):

```go
if groupID, ok := rjob.Labels[domain.CorrelationGroupIDLabel]; ok && groupID != "" {
    mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{
        Name:  "FINDING_CORRELATION_GROUP_ID",
        Value: groupID,
    })
}
```

**Ordering guarantee:** `labelAsPrimary` (STORY_02) patches the group ID label onto the
primary before `dispatch` calls `Build`. The label is therefore present on `rjob` by the
time `Build` reads it. No additional synchronisation is needed.

---

## Tasks

- [x] Add `FINDING_CORRELATION_GROUP_ID` injection to `Build()` in
      `internal/jobbuilder/job.go` (after line 185, after the correlated findings block)
- [x] Add `job_test.go` cases for `FINDING_CORRELATION_GROUP_ID` injection (present and absent)
- [x] Run `go test -timeout 30s -race ./internal/jobbuilder/...` — must pass

---

## Dependencies

**Depends on:** STORY_00 (`CorrelationGroupIDLabel` constant)
**Blocks:** STORY_04 (prompt must reference the env var that this story injects)

---

## Definition of Done

- [x] `FINDING_CORRELATION_GROUP_ID` injected when label is present on primary
- [x] Absent when label is not present
- [x] All tests pass
