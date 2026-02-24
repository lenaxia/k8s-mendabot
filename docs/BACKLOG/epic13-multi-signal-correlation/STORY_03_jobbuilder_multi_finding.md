# Story 03: JobBuilder Multi-Finding Support

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mendabot developer**, I want `JobBuilder.Build()` to accept a slice of correlated
findings and inject them as a `FINDING_CORRELATED_FINDINGS` env var, so that the agent
Job for a correlated group receives the full group context rather than a single finding.

---

## Background

Today `JobBuilder.Build()` builds a Job spec from a single `RemediationJob`. After
correlation, the primary `RemediationJob` may represent a group of related findings. The
agent must know about all of them to produce a coherent investigation and PR.

The change is additive: when no correlated findings are present (the common case), the
env var is absent or empty and existing agent behaviour is unchanged.

---

## Acceptance Criteria

- [x] `JobBuilder.Build()` signature changes to accept an additional
      `correlatedFindings []v1alpha1.FindingSpec` parameter (nil or empty = single-finding mode)
- [x] When `len(correlatedFindings) > 1`, a `FINDING_CORRELATED_FINDINGS` env var is
      injected into the main container as a JSON-encoded array of `FindingSpec` objects
- [x] When `len(correlatedFindings) <= 1`, `FINDING_CORRELATED_FINDINGS` is not set
      (no empty env var pollution)
- [x] `FINDING_CORRELATION_GROUP_ID` env var is injected when the primary
      `RemediationJob` carries a `mendabot.io/correlation-group-id` label
- [x] `internal/jobbuilder/job_test.go` covers:
  - Single finding (no correlated env var set)
  - Two correlated findings (env var set, valid JSON)
  - JSON encodes all `FindingSpec` fields correctly
- [x] `go test -timeout 30s -race ./internal/jobbuilder/...` passes

---

## Technical Implementation

### Signature change

```go
// Before
func (b *Builder) Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error)

// After
func (b *Builder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error)
```

All existing call sites pass `nil` for `correlatedFindings` until STORY_02 wires the
full group context. The reconciler (STORY_02) passes the full findings slice.

### Env var injection

```go
if len(correlatedFindings) > 1 {
    raw, err := json.Marshal(correlatedFindings)
    if err != nil {
        return nil, fmt.Errorf("jobbuilder: marshal correlated findings: %w", err)
    }
    env = append(env, corev1.EnvVar{
        Name:  "FINDING_CORRELATED_FINDINGS",
        Value: string(raw),
    })
}

if groupID, ok := rjob.Labels[domain.CorrelationGroupIDLabel]; ok && groupID != "" {
    env = append(env, corev1.EnvVar{
        Name:  "FINDING_CORRELATION_GROUP_ID",
        Value: groupID,
    })
}
```

### Callers to update

- `internal/controller/remediationjob_controller.go` — primary call site (`r.JobBuilder.Build(&rjob)` at line 158)
- `internal/jobbuilder/job_test.go` — all existing calls `b.Build(testRJob)` become `b.Build(testRJob, nil)`
- `internal/controller/fakes_test.go:25` — `fakeJobBuilder.Build()` implements the
  `domain.JobBuilder` interface. The method signature and the `fakeJobBuilderCall` struct
  must both be updated:
  ```go
  // Before
  func (f *fakeJobBuilder) Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error)

  // After
  func (f *fakeJobBuilder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error)
  ```
  The `fakeJobBuilderCall` struct at `fakes_test.go` should also record `CorrelatedFindings`
  so controller tests can assert on what was passed.
- `internal/domain/interfaces.go:12` — the `JobBuilder` interface must be updated to the
  new two-argument signature. This is a **compiler-breaking change**: the compile-time
  assertion `var _ domain.JobBuilder = (*Builder)(nil)` at `internal/jobbuilder/job.go`
  will fail until both the interface and the concrete type agree:
  ```go
  // internal/domain/interfaces.go
  type JobBuilder interface {
      Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error)
  }
  ```

---

## Tasks

- [x] Write new `job_test.go` cases for multi-finding injection (TDD — must fail first)
- [x] Update `internal/domain/interfaces.go:12` — change `JobBuilder.Build()` to the
      two-argument signature (this is a compiler-breaking change; do it first so the
      compile-time assertion fails clearly and guides the remaining changes)
- [x] Update `Builder.Build()` signature in `internal/jobbuilder/job.go` and inject
      `FINDING_CORRELATED_FINDINGS` and `FINDING_CORRELATION_GROUP_ID` env vars
- [x] Update `fakeJobBuilder.Build()` in `internal/controller/fakes_test.go:25` to the
      new signature; update `fakeJobBuilderCall` struct to record `CorrelatedFindings`
- [x] Update all existing `b.Build(rjob)` calls in `internal/jobbuilder/job_test.go`
      to `b.Build(rjob, nil)`
- [x] Update the call site in `internal/controller/remediationjob_controller.go:158`
      from `r.JobBuilder.Build(&rjob)` to `r.JobBuilder.Build(&rjob, nil)` as a
      placeholder — STORY_02 will update this to pass the actual correlated findings
- [x] Run `go test -timeout 30s -race ./...` — must pass

---

## Dependencies

**Depends on:** STORY_00 (domain types, `CorrelationGroupIDLabel` constant)
**Blocks:** STORY_04 (prompt must reference the env var that this story injects)

---

## Definition of Done

- [x] `Build()` signature updated; all call sites compile
- [x] Env vars injected correctly for both single and multi-finding cases
- [x] All tests pass
