# Story: ResultReconciler — RemediationJob Creation

**Epic:** [Controller](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **developer**, I want the `ResultReconciler.Reconcile()` method to fetch a Result,
compute its fingerprint, check for an existing `RemediationJob` via the Kubernetes API
(CRD-as-state, no in-memory map), and create a `RemediationJob` if none exists — handling
all error cases correctly.

---

## Acceptance Criteria

- [ ] `Reconcile()` fetches the Result; if NotFound, finds and deletes any
  Pending/Dispatched `RemediationJob` with annotation `opencode.io/result-name=req.Name`
  in the Result's namespace, then returns nil
- [ ] Computes fingerprint via `fingerprintFor(namespace, spec)`
- [ ] Lists `RemediationJob` objects with label `remediation.mendabot.io/fingerprint=fp[:12]`
  in `cfg.AgentNamespace`; if a match exists with `spec.fingerprint == fp` and phase is
  not Failed, returns nil immediately (deduplicated via CRD, no in-memory map)
- [ ] Builds `RemediationJob` with:
  - `spec.sourceType: "k8sgpt"`
  - `spec.fingerprint: fp`
  - `spec.sourceResultRef`
  - `spec.finding` (errors redacted of Sensitive fields before serialisation)
  - appropriate labels and annotations
- [ ] Calls `client.Create(ctx, rjob)` to create the object
- [ ] On `IsAlreadyExists`: returns nil (race-safe, CRD persisted)
- [ ] On any other create error: returns wrapped error (controller-runtime requeues)
- [ ] Logs with structured fields on every significant path

---

## Integration Test Cases (envtest — write tests first, in `internal/provider/k8sgpt/reconciler_test.go`)

| Test | Expected |
|------|----------|
| `TestResultReconciler_CreatesRemediationJob` | New Result → RemediationJob created with sourceType="k8sgpt" |
| `TestResultReconciler_DuplicateFingerprint_Skips` | Same fingerprint, non-Failed phase → no second RemediationJob |
| `TestResultReconciler_FailedPhase_ReDispatches` | Existing Failed RemediationJob → new one created |
| `TestResultReconciler_NoErrors_Skipped` | Result with no errors → no RemediationJob (predicate filtered) |
| `TestResultReconciler_ResultDeleted_CancelsPending` | Result deleted → Pending RemediationJob deleted |
| `TestResultReconciler_DifferentParents_TwoJobs` | Two Results, different parents → two RemediationJobs |
| `TestResultReconciler_ErrorTextChanges_NewJob` | Same parent, changed error text → new fingerprint, new RemediationJob |

---

## Tasks

- [ ] Write all 7 envtest integration tests first (must fail before implementation)
- [ ] Implement `Reconcile()` method body in `internal/provider/k8sgpt/reconciler.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_02 (fingerprint)
**Blocks:** STORY_04 (RemediationJobReconciler), STORY_07 (integration tests)

---

## Definition of Done

- [ ] All 7 integration tests pass with `-race`
- [ ] `go vet` clean
- [ ] No in-memory map — deduplication is entirely via CRD label lookup
