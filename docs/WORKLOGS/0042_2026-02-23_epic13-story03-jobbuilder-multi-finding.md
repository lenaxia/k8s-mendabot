# Worklog: Epic 13 STORY_03 — JobBuilder Multi-Finding Support

**Date:** 2026-02-23
**Session:** Complete STORY_03: Build() two-arg signature, correlated findings env var injection, all call sites updated
**Status:** Complete

---

## Objective

Complete STORY_03 from epic13-multi-signal-correlation. The story requires:
- Updating `JobBuilder.Build()` to accept `correlatedFindings []v1alpha1.FindingSpec` as a second arg
- Injecting `FINDING_CORRELATED_FINDINGS` env var (JSON array) when `len(correlatedFindings) > 1`
- Injecting `FINDING_CORRELATION_GROUP_ID` env var when the rjob carries the correlation group label
- Fixing all compile errors caused by the interface change across all call sites
- Writing TDD tests (fail first, then pass)

---

## Work Completed

### 1. TDD Tests Added (internal/jobbuilder/job_test.go)

Six new tests added at the end of the file:
- `TestBuild_SingleFinding_NoCorrelatedEnvVar` — nil correlatedFindings → no env vars injected
- `TestBuild_SingleElementSlice_NoCorrelatedEnvVar` — len == 1 → FINDING_CORRELATED_FINDINGS absent
- `TestBuild_TwoCorrelatedFindings_EnvVarSet` — len == 2 → env var present, valid JSON, correct fields
- `TestBuild_CorrelatedFindings_AllFieldsEncoded` — all FindingSpec fields survive round-trip
- `TestBuild_CorrelationGroupID_InjectedWhenLabelPresent` — label present → env var set with correct value
- `TestBuild_CorrelationGroupID_NotInjectedWhenLabelAbsent` — label absent → env var absent
- `TestBuild_CorrelationGroupID_NotInjectedWhenLabelEmpty` — label empty string → env var absent

Added `encoding/json` and `internal/domain` imports to job_test.go.

### 2. Compile-time assertion updated (internal/jobbuilder/job_test.go line 107)

Changed from:
```go
var _ func(*v1alpha1.RemediationJob) (*batchv1.Job, error) = (*Builder)(nil).Build
```
To:
```go
var _ func(*v1alpha1.RemediationJob, []v1alpha1.FindingSpec) (*batchv1.Job, error) = (*Builder)(nil).Build
```

### 3. Env var injection implemented (internal/jobbuilder/job.go)

Added after mainContainer construction, before volumes:
```go
if len(correlatedFindings) > 1 {
    raw, err := json.Marshal(correlatedFindings)
    if err != nil {
        return nil, fmt.Errorf("jobbuilder: marshal correlated findings: %w", err)
    }
    mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{Name: "FINDING_CORRELATED_FINDINGS", Value: string(raw)})
}
if groupID, ok := rjob.Labels[domain.CorrelationGroupIDLabel]; ok && groupID != "" {
    mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{Name: "FINDING_CORRELATION_GROUP_ID", Value: groupID})
}
```
This resolves the pre-existing unused `encoding/json` import compiler error.

### 4. fakeJobBuilder updated (internal/controller/fakes_test.go)

- `fakeJobBuilderCall` struct: added `CorrelatedFindings []v1alpha1.FindingSpec` field
- `Build()` method: updated to two-arg signature, records both args in call struct
- All three `f.Build(rjob)` call sites updated to `f.Build(rjob, nil)`

### 5. Controller call site updated (internal/controller/remediationjob_controller.go)

Line 158: `r.JobBuilder.Build(&rjob)` → `r.JobBuilder.Build(&rjob, nil)` (STORY_02 placeholder)

### 6. All existing b.Build() calls in job_test.go updated

- `buildJob()` helper: `b.Build(testRJob)` → `b.Build(testRJob, nil)`
- `TestBuild_JobNameDeterministic`: both calls updated
- `TestBuild_EmptyErrors`: updated
- `TestBuild_LongDetails`: updated
- `TestBuild_NilRJob`: `b.Build(nil)` → `b.Build(nil, nil)`
- `TestBuild_EmptyFingerprint`: updated
- `TestBuild_ShortFingerprint`: updated
- `TestBuild_InitScript_UsesGitHubAppToken`: updated
- `TestBuild_InitScript_HasErrorHandling`: updated

---

## Key Decisions

- `FINDING_CORRELATED_FINDINGS` is injected only when `len > 1`, not `len > 0`. This matches the story spec exactly: a single-element slice is not a "correlated group", it is just redundant noise. The threshold of `> 1` ensures no empty or single-entry env var pollution.
- `FINDING_CORRELATION_GROUP_ID` guards on both `ok` (label exists in map) and `groupID != ""` (non-empty value), preventing injection of a blank string.
- The controller call site passes `nil` as a placeholder per the story spec; STORY_02 will wire the actual group findings.

---

## Blockers

None.

---

## Tests Run

```
go build ./...       — clean, zero errors
go test -timeout 30s -race ./...   — all 14 packages pass
```

Notable packages exercised:
- `internal/jobbuilder`: 1.111s (new TDD tests pass)
- `internal/controller`: 9.144s (envtest integration tests pass)

---

## Next Steps

STORY_02 (correlation_window): the reconciler should look up the correlation group and pass the full `[]FindingSpec` slice to `r.JobBuilder.Build(&rjob, correlatedFindings)`. The `nil` placeholder at `remediationjob_controller.go:158` is the integration point.

---

## Files Modified

- `internal/jobbuilder/job.go` — env var injection + `encoding/json` now used
- `internal/jobbuilder/job_test.go` — compile-time assertion updated, all existing calls updated to nil second arg, 7 new TDD tests added, imports updated
- `internal/controller/fakes_test.go` — `fakeJobBuilderCall` struct updated, `Build()` signature updated, 3 call sites updated
- `internal/controller/remediationjob_controller.go` — Build call updated to two-arg
- `docs/BACKLOG/epic13-multi-signal-correlation/STORY_03_jobbuilder_multi_finding.md` — status and checklists updated to Complete
- `docs/WORKLOGS/README.md` — index updated
