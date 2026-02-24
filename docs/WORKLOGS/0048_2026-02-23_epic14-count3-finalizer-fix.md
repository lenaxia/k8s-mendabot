# Worklog: Epic 14 — -count=3 Finalizer Fix

**Date:** 2026-02-23
**Session:** Fix batch/v1 Job finalizer blocking -count=3 and namespace counter isolation
**Status:** Complete

---

## Objective

Achieve a clean `go test -count=3 -timeout 300s -race ./internal/controller/...` run,
completing the Definition of Done for Epic 14.

---

## Root Causes Fixed

### 1. batch/v1 Job finalizer (all integration tests)

envtest does not run the batch controller. `batch/v1` Jobs created by the tests acquire
a `batch.kubernetes.io/job-tracking` finalizer from the API server. When `c.Delete` is
called in `t.Cleanup`, the Job enters terminating state and stays there forever — the
batch controller that would normally process the finalizer is not running.

On the next `-count=N` pass, pre-test stale guards call `c.Delete` on the same job name,
then `waitForGone` waits for the object to disappear. Because the object is already
terminating but not gone, `waitForGone` times out. On the subsequent `c.Create`, the API
server returns `"object is being deleted: jobs.batch already exists"`.

**Fix:** Added `deleteJob(ctx, c, job *batchv1.Job)` helper in `integration_test.go`.
It strips all finalizers via `c.Update` before calling `c.Delete`, allowing the API
server to remove the object immediately without needing the batch controller.

Wired `deleteJob` into:
- Pre-test stale deletes in all 6 deterministic-name tests
- `t.Cleanup` for `SyncsStatus_Running`, `SyncsStatus_Succeeded`, `SyncsStatus_Failed`
- `t.Cleanup` for `MaxConcurrentJobs_Requeues` (activeJob)
- `t.Cleanup` loops in `CreatesJob` and `OwnerReference`
- `cleanupJobsInNS` body in `correlation_integration_test.go`

### 2. Namespace termination (correlation tests)

Kubernetes namespace termination is asynchronous and can take minutes even in envtest
because the API server must garbage-collect system resources (service accounts, tokens,
RBAC) before deleting the namespace object. Any attempt to wait for a namespace to be
fully gone before recreating it would stall the test indefinitely.

**Fix:** Added `corrRunCounter int64` (atomic) and `corrNS(base string) string` to
`correlation_integration_test.go`. Each call to `corrNS` returns `"base-N"` where N is
a monotonically increasing counter. Each `-count=N` invocation allocates fresh namespace
names (`tc01-single-1`, `tc01-single-2`, ...) that have never existed, bypassing the
namespace termination window entirely. Old namespaces accumulate harmlessly.

---

## Validation

```
go test -count=3 -timeout 300s -race ./internal/controller/...
```
All passes: PASS (32.6s)

---

## Files Modified

- `internal/controller/integration_test.go`
  - Added `deleteJob` helper (strips finalizers before delete)
  - Changed all `c.Delete` calls on batch/v1 Jobs to use `deleteJob`
- `internal/controller/correlation_integration_test.go`
  - Added `corrRunCounter` and `corrNS` for unique namespace names per test invocation
  - Changed `cleanupJobsInNS` body to use `deleteJob`

---

## Admin

- Updated `docs/BACKLOG/epic14-test-infrastructure/README.md` status to Complete
- Updated all three story files status to Complete
- Checked DoD checkboxes in epic README
