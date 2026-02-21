# Story: Fake and Stub Implementations

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want a fake `JobBuilder` implementation so that `RemediationJobReconciler`
unit tests can inject a controllable builder without constructing real `batch/v1 Job`
objects or touching the Kubernetes API.

---

## Acceptance Criteria

- [ ] `internal/controller/fakes_test.go` (package `controller_test`) defines:

  ```go
  type fakeJobBuilder struct {
      calls     []fakeJobBuilderCall
      returnJob *batchv1.Job
      returnErr error
  }

  type fakeJobBuilderCall struct {
      RemediationJob *v1alpha1.RemediationJob
  }

  func (f *fakeJobBuilder) Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error) {
      f.calls = append(f.calls, fakeJobBuilderCall{rjob})
      return f.returnJob, f.returnErr
  }
  ```

- [ ] `fakeJobBuilder` satisfies `domain.JobBuilder` — verified by a compile-time assertion:
  ```go
  var _ domain.JobBuilder = (*fakeJobBuilder)(nil)
  ```

- [ ] `internal/provider/k8sgpt/fakes_test.go` (package `k8sgpt_test`) defines:

  ```go
  // No fake needed for SourceProvider in unit tests — SourceProviderReconciler
  // is tested directly in envtest integration tests (no fake needed).
  ```

  For now, `K8sGPTProvider` is tested by exercising `SourceProviderReconciler`
  directly in envtest integration tests (no fake needed).

- [ ] A `defaultFakeJob(rjob *v1alpha1.RemediationJob) *batchv1.Job` helper returns a
  minimal valid `*batchv1.Job` with:
  - Correct name (`mendabot-agent-<fp[:12]>`)
  - Correct namespace
  - The ownerReference pointing at `rjob`
  - Label `remediation.mendabot.io/remediation-job=rjob.Name`
  - `Spec.BackoffLimit: ptr(int32(1))` — required so that `syncPhaseFromJob` can
    safely dereference `BackoffLimit` without a nil pointer panic. The Kubernetes API
    server applies a default of 6, but test environments (fakeJobBuilder, envtest without
    defaulting) do not; always set it explicitly in test helpers.
  This allows controller tests to call `fakeJobBuilder.returnJob = defaultFakeJob(rjob)`
  and proceed past the `Build()` call without asserting on the full Job spec.

- [ ] Unit tests in `fakes_test.go` verify:
  - `fakeJobBuilder.Build()` records each call
  - `fakeJobBuilder` with `returnErr != nil` propagates the error
  - `defaultFakeJob()` returns a Job with the correct name pattern

---

## Scope

This story defines `fakeJobBuilder` for `RemediationJobReconciler` tests. For logger
fakes, use `zap.NewNop()` directly. For the scheme, use `runtime.NewScheme()`. No client
fake — controller integration tests use envtest's real client.

`SourceProvider` has no fake in v1: `SourceProviderReconciler` is tested directly in envtest.

---

## Tasks

- [ ] Create `internal/controller/fakes_test.go` with `fakeJobBuilder`,
  `fakeJobBuilderCall`, `defaultFakeJob()`, and the compile-time assertion
- [ ] Write unit tests for the fake itself (TDD — fakes need tests too)

---

## Dependencies

**Depends on:** STORY_01 (RemediationJob type)
**Depends on:** STORY_02 (JobBuilder interface)
**Blocks:** epic01-controller/STORY_04 (RemediationJobReconciler unit tests inject fakeJobBuilder)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] Compile-time interface assertion present
