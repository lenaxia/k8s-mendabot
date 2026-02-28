# Story: JobProvider

**Epic:** [epic09-native-provider](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want mechanic to detect genuinely failed Jobs directly from
cluster state so that Jobs that have exhausted their retry budget trigger a remediation
Job without requiring k8sgpt-operator.

---

## Acceptance Criteria

- [ ] `jobProvider` struct defined in `internal/provider/native/job.go`
  (unexported; exported constructor `NewJobProvider(c client.Client) *jobProvider`
  in same file; panics if `c == nil`)
- [ ] Compile-time assertion `var _ domain.SourceProvider = (*jobProvider)(nil)` present
- [ ] `ProviderName()` returns `"native"`
- [ ] `ObjectType()` returns `&batchv1.Job{}`
- [ ] `ExtractFinding` returns `(nil, nil)` for healthy Jobs (succeeded, still active, or
  suspended)
- [ ] `ExtractFinding` returns `(nil, nil)` for Jobs owned by a CronJob — those are
  transient by design and remediation should target the CronJob, not the individual Job
  instance
- [ ] `ExtractFinding` returns a populated `*Finding` only when **all three** of the
  following are true: `status.failed > 0`, `status.active == 0`,
  `status.completionTime == nil`
- [ ] Error text includes the `Failed` condition's `Reason` and `Message` from
  `status.conditions` when present
- [ ] `Finding.ParentObject` traverses ownerReferences: `Job → CronJob` if present;
  otherwise `"Job/<name>"`. Call: `getParent(ctx, p.client, job.ObjectMeta, "Job")`
  (CronJob-owned Jobs are excluded before this point — see CronJob exclusion section)
- [ ] `Finding.Kind` is `"Job"`, `Finding.Name` is the Job name
- [ ] `Finding.Namespace` is the Job namespace
- [ ] `Finding.SourceRef` identifies the Job (`APIVersion: "batch/v1"`, `Kind: "Job"`)
- [ ] `jobProvider` holds a `client.Client` field for the `getParent` call

---

## Failure detection logic

The three-part condition is intentional and precise:

| Condition | Meaning |
|---|---|
| `status.failed > 0` | At least one pod attempt has failed |
| `status.active == 0` | The Job is no longer running (has given up retrying) |
| `status.completionTime == nil` | The Job did not succeed (a successful Job sets `completionTime`) |

This combination identifies Jobs that have exhausted their `backoffLimit` and are stuck.
It excludes:
- Jobs that are still running (`active > 0`)
- Jobs that succeeded (`completionTime != nil`)
- Jobs that have never failed yet (`failed == 0`)
- Jobs that are suspended (`status.conditions` contains `Suspended=True`) — these are
  deliberate pauses, not failures

CronJob-owned Jobs are excluded because they are short-lived instances. A failed CronJob
run that repeats is a scheduling/template issue, not a one-off Job failure. Detecting at
the CronJob level is more accurate — but CronJob detection is out of scope for v1.

---

## CronJob exclusion: exact mechanism

The exclusion check is performed **before** calling `getParent` and **before** the
three-part failure check. The implementation must check the Job's own `ownerReferences`
directly:

```go
for _, ref := range job.OwnerReferences {
    if ref.Kind == "CronJob" {
        return nil, nil
    }
}
```

**Why check before `getParent`?** `getParent` traverses up to the root owner — it may
return `"CronJob/name"` when the Job is owned by a CronJob. But parsing a string result
is fragile. Checking the raw `ownerReferences[i].Kind` field before any traversal is
direct, unambiguous, and matches how Kubernetes sets the field (`"CronJob"` — the
`batch/v1.CronJob` Kind string, not `"CronJob.batch"` or any qualified variant).

**On `Kind` field values:** The `ownerReference.Kind` field is set by the CronJob
controller and is always `"CronJob"` (unqualified, no group prefix). This is guaranteed
by the Kubernetes API machinery for built-in types. Custom controllers may use other
formats for custom resource kinds, but CronJob is a built-in type so the value is stable.

**`getParent` for non-CronJob-owned Jobs:** After the CronJob exclusion check, if the
Job has other owner references (unusual but possible), `getParent` is called normally.
For the common case of a standalone Job with no owner references, `getParent` returns
`"Job/<name>"` (the input fallback).

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `SucceededJob` | `status.succeeded=1`, `status.completionTime` set | `(nil, nil)` |
| `StillActiveJob` | `status.active=1`, `status.failed=0` | `(nil, nil)` |
| `ActiveWithFailures` | `status.active=1`, `status.failed=2` — still retrying | `(nil, nil)` |
| `FailedJobExhausted` | `status.failed=3`, `status.active=0`, `status.completionTime=nil` | Finding |
| `FailedJobWithReason` | Exhausted Job with `Failed` condition containing `Reason` and `Message` | Finding; error text contains `Reason` and `Message` |
| `SuspendedJob` | `status.conditions` contains `Suspended=True` | `(nil, nil)` |
| `CronJobOwned` | Job with ownerReference `Kind: CronJob` | `(nil, nil)` — excluded |
| `StandaloneJob` | Exhausted Job with no ownerReferences | `Finding.ParentObject == "Job/<name>"` |
| `WrongType` | Non-Job object passed | `(nil, error)` |
| `ZeroFailedZeroActive` | `status.failed=0`, `status.active=0`, `status.completionTime=nil` — Job never ran | `(nil, nil)` |

---

## Tasks

- [ ] Write all 10 tests in `internal/provider/native/job_test.go` (TDD — must fail first)
- [ ] Implement `JobProvider` in `internal/provider/native/job.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (getParent)
**Blocks:** STORY_08 (main wiring)

---

## Definition of Done

- [ ] All 10 tests pass with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
