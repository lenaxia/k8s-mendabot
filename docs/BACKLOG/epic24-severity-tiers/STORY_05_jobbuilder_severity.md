# Story 05: JobBuilder — Inject FINDING_SEVERITY into Agent Job

**Epic:** [epic24-severity-tiers](README.md)
**Priority:** Medium
**Status:** Complete
**Estimated Effort:** 45 minutes

---

## User Story

As the **mendabot agent**, I want to receive the finding severity as an environment variable
so that I can calibrate how aggressively to propose a fix and how much to hedge.

---

## Background

`JobBuilder` in `internal/jobbuilder/job.go` constructs the `batch/v1 Job` spec from a
`RemediationJob`. It already injects a set of env vars into the agent container. Adding
`FINDING_SEVERITY` here makes the severity available to the agent at runtime without any
changes to the agent image or prompt template language.

---

## Design

In `internal/jobbuilder/job.go`, inside `Build()`, add to the env var slice:

```go
{Name: "FINDING_SEVERITY", Value: rjob.Spec.Severity},
```

`rjob.Spec.Severity` is a `string`. If it is empty (legacy object), the env var is
present with an empty value — not absent. The prompt handles this gracefully (see STORY_06).

### entrypoint-common.sh VARS list

`docker/scripts/entrypoint-common.sh` line 105 contains a restricted `VARS` string
used by `envsubst "$VARS"` to substitute only the known variables in the rendered prompt:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
```

If `${FINDING_SEVERITY}` is not added to this string, `envsubst` will leave it as the
literal text `${FINDING_SEVERITY}` in the prompt — the agent will never see the actual
value. **This file must be updated as part of this story.**

---

## Acceptance Criteria

- [ ] `JobBuilder.Build()` injects `FINDING_SEVERITY` into the agent container's env
- [ ] When `RemediationJob.Spec.Severity` is `"critical"`, the Job env contains `FINDING_SEVERITY=critical`
- [ ] When `RemediationJob.Spec.Severity` is `""` (legacy object), the env var is present with an empty value
- [ ] `TestBuild_EnvVars_AllPresent` in `internal/jobbuilder/job_test.go` lists `"FINDING_SEVERITY"` in its `required` slice
- [ ] `${FINDING_SEVERITY}` is added to the `VARS` string in `docker/scripts/entrypoint-common.sh` line 105

---

## Tasks

- [ ] Update `internal/jobbuilder/job_test.go` — add `"FINDING_SEVERITY"` to the `required` slice in `TestBuild_EnvVars_AllPresent` (TDD — test will fail until job.go is updated)
- [ ] Add a new test asserting the `FINDING_SEVERITY` value for a specific severity. Use the copy pattern already demonstrated in the test file (e.g. `TestBuild_EmptyErrors` does `rjob := *testRJob; rjob.Spec.Finding.Errors = ""`). For the severity test: `rjob := *testRJob; rjob.Spec.Severity = "critical"` then assert `FINDING_SEVERITY=critical` in the container env.
- [ ] Update `internal/jobbuilder/job.go` — add `{Name: "FINDING_SEVERITY", Value: rjob.Spec.Severity}` to the env slice in `Build()`
- [ ] Update `docker/scripts/entrypoint-common.sh` — append `${FINDING_SEVERITY}` to the `VARS` string on line 105
- [ ] Run `go test -race -timeout 30s ./internal/jobbuilder/...` — must pass
- [ ] Run `go build ./...` — must be clean

---

## Dependencies

**Depends on:** STORY_04 (`RemediationJobSpec.Severity` populated by reconciler)
**Blocks:** STORY_06 (prompt references `FINDING_SEVERITY`)

---

## Definition of Done

- [ ] `FINDING_SEVERITY` env var injected into all new agent Jobs
- [ ] JobBuilder tests assert the env var value for all severity levels
- [ ] Full test suite passes with `-race`
