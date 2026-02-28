# Story: RemediationJobReconciler — Job Dispatch

**Epic:** [Controller](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **developer**, I want the `RemediationJobReconciler.Reconcile()` method to fetch a
`RemediationJob`, check concurrency limits, call `jobBuilder.Build()`, create the
`batch/v1 Job`, and patch the `RemediationJob` status — handling all error cases correctly.

---

## Acceptance Criteria

- [ ] `Reconcile()` fetches the `RemediationJob`; returns nil if NotFound
- [ ] If phase is `Succeeded`: applies TTL check per CONTROLLER_LLD.md §6.2 step 2 —
  if `CompletedAt + RemediationJobTTL <= now` deletes the object and returns nil;
  if TTL is not yet due returns `ctrl.Result{RequeueAfter: time.Until(deadline)}, nil`;
  if `CompletedAt` is not yet set returns nil (will be set when Job syncs)
- [ ] If phase is `Failed`: returns nil immediately (terminal, retained indefinitely, no TTL)
- [ ] Looks for an owned Job (label `remediation.mechanic.io/remediation-job=rjob.Name`);
  if found, syncs phase via `syncPhaseFromJob` (defined in CONTROLLER_LLD.md §6.3) and returns nil
- [ ] Checks `MAX_CONCURRENT_JOBS` — counts active Jobs with label
  `app.kubernetes.io/managed-by=mechanic-watcher`; if at limit, requeues after 30s
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

**Depends on:** STORY_03 (RemediationJob creation by SourceProviderReconciler), epic00.1-interfaces/STORY_05 (fakeJobBuilder)
**Blocks:** STORY_05 (error-filter predicate), STORY_07 (integration tests)

---

## Definition of Done

- [ ] All 6 integration tests pass with `-race`
- [ ] `go vet` clean
