# Story: RemediationJobReconciler — Job Dispatch

**Epic:** [Controller](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **developer**, I want the `RemediationJobReconciler.Reconcile()` method to fetch a
`RemediationJob`, check concurrency limits, call `jobBuilder.Build()`, create the
`batch/v1 Job`, and patch the `RemediationJob` status — handling all error cases correctly.

---

## Acceptance Criteria

- [ ] `Reconcile()` fetches the `RemediationJob`; returns nil if NotFound
- [ ] Returns nil immediately if phase is `Succeeded` or `Failed` (terminal state, no action)
- [ ] Looks for an owned Job (label `remediation.mendabot.io/remediation-job=rjob.Name`);
  if found, syncs phase and returns nil (covered in STORY_05)
- [ ] Checks `MAX_CONCURRENT_JOBS` — counts active Jobs with label
  `app.kubernetes.io/managed-by=mendabot-watcher`; if at limit, requeues after 30s
- [ ] Calls `jobBuilder.Build(rjob)` to produce the Job spec
- [ ] Calls `client.Create(ctx, job)` to create the Job
- [ ] On `IsAlreadyExists`: re-fetches job, syncs phase, returns nil
- [ ] On any other create error: returns wrapped error (controller-runtime requeues)
- [ ] Patches `rjob.Status`:
  - `Phase = PhaseDispatched`
  - `JobRef = job.Name`
  - `DispatchedAt = now`
  - `Condition ConditionJobDispatched = True`
- [ ] Logs with structured fields on every significant path

---

## Integration Test Cases (envtest — write tests first, in `internal/controller/remediationjob_controller_test.go`)

| Test | Expected |
|------|----------|
| `TestRemediationJobReconciler_CreatesJob` | Pending RemediationJob → Job created, phase = Dispatched |
| `TestRemediationJobReconciler_MaxConcurrentJobs_Requeues` | At MAX_CONCURRENT_JOBS limit → no Job created, requeued |
| `TestRemediationJobReconciler_JobAlreadyExists_SyncsStatus` | Job already exists → status synced, no duplicate |
| `TestRemediationJobReconciler_Terminal_NoOp` | Succeeded/Failed phase → no action taken |
| `TestRemediationJobReconciler_OwnerReference` | Created Job has ownerRef pointing to RemediationJob |
| `TestRemediationJobReconciler_BuildError_Requeues` | `jobBuilder.Build()` returns error → reconciler returns error |

---

## Tasks

- [ ] Write all 6 envtest integration tests first (must fail before implementation)
- [ ] Implement `Reconcile()` method body in `internal/controller/remediationjob_controller.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (RemediationJob creation by ResultReconciler), STORY_05_fakes (fakeJobBuilder)
**Blocks:** STORY_05 (status sync), STORY_07 (integration tests)

---

## Definition of Done

- [ ] All 6 integration tests pass with `-race`
- [ ] `go vet` clean
