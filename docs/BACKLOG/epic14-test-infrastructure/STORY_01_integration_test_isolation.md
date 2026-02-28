# Story 01: Fix Integration Test Isolation

**Epic:** [epic14-test-infrastructure](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 45 minutes

---

## User Story

As a **mechanic developer**, I want all integration tests in `internal/controller/` to
pass deterministically regardless of how many times they have run previously in the same
envtest process, so that `-count=N` invocations and parallel CI pipelines do not
produce false failures.

---

## Background

All integration tests in `internal/controller/` share a single envtest process (one
real Kubernetes API server, one etcd). Tests run sequentially but write real Kubernetes
objects. `t.Cleanup` is registered to delete objects *after* the test completes. If a
cleanup silently fails, or if a test is interrupted, those objects persist for the
lifetime of the envtest process.

Five tests create `batch/v1` Jobs with deterministic names derived from fixed
fingerprint constants. Each test then uses `waitFor` to poll until a job appears in
the namespace matching the test's label (`remediation.mechanic.io/remediation-job:
<rjob-name>`). If a stale job from a previous run is still present — with the same
label (because both the prior and current runs use the same `rjob` name) — the
`waitFor` condition is satisfied immediately using the *old* job. That old job's
`OwnerReference.UID` points to the `RemediationJob` from the prior run, causing UID
assertions or phase assertions to fail non-deterministically.

The five affected tests and their deterministic job names:

| Test | fp (first 12 chars) | Job name | rjob name |
|------|---------------------|----------|-----------|
| `TestRemediationJobReconciler_CreatesJob` | `aaaa0000bbbb` | `mechanic-agent-aaaa0000bbbb` | `rjob-creates-job` |
| `TestRemediationJobReconciler_SyncsStatus_Running` | `bbbb1111cccc` | `mechanic-agent-bbbb1111cccc` | `rjob-syncs-running` |
| `TestRemediationJobReconciler_SyncsStatus_Succeeded` | `cccc2222dddd` | `mechanic-agent-cccc2222dddd` | `rjob-syncs-succeeded` |
| `TestRemediationJobReconciler_SyncsStatus_Failed` | `dddd3333eeee` | `mechanic-agent-dddd3333eeee` | `rjob-syncs-failed` |
| `TestRemediationJobReconciler_OwnerReference` | `ffff5555aaaa` | `mechanic-agent-ffff5555aaaa` | `rjob-ownerref` |

The correct fix is **pre-test cleanup**: delete any pre-existing job with the
deterministic name *at the start of each test*, before creating the `RemediationJob`
and reconciling. This makes each test self-healing regardless of what prior runs left
behind.

`TestRemediationJobReconciler_MaxConcurrentJobs_Requeues` also uses a deterministic
fingerprint (`eeee4444ffff` prefix) but it does not itself create a Job — the
reconciler is expected to requeue without creating one — so it does not need this fix.

This fix does not change any controller implementation — it only changes the tests.

---

## Acceptance Criteria

- [ ] Each of the five affected tests deletes any pre-existing job with its deterministic
      name in `default` namespace before proceeding
- [ ] The pre-test delete ignores `not-found` errors (idempotent)
- [ ] `go test -count=3 -timeout 300s -race ./internal/controller/...` passes all three
      runs for each test

---

## Technical Implementation

### File to change

**`internal/controller/integration_test.go`**

In each of the five affected tests, add a pre-test cleanup block immediately after
`c := newIntegrationClient(t)` and **before** `const fp = ...`. The pattern is:

```go
// Pre-test cleanup: delete any stale job from a previous run so the waitFor
// loop below sees only the job created by this test.
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mechanic-agent-<fp[:12]>",
    Namespace: integrationCtrlNamespace,
}})
```

The `_ =` is intentional: `Delete` returns a not-found error when the object doesn't
exist, which is the normal case and must not fail the test.

### Why not `t.Cleanup` instead?

`t.Cleanup` runs *after* the test. The problem is that a *prior* run's cleanup may have
been skipped or failed. Pre-test cleanup executes unconditionally before the test body
runs and is therefore immune to prior failures.

### Per-test changes

All line numbers refer to the current file. Each insertion goes **after**
`c := newIntegrationClient(t)` and **before** `const fp = ...`.

---

#### `TestRemediationJobReconciler_CreatesJob` (starts at line 150)

Current structure (lines 154–157):
```go
ctx := context.Background()
c := newIntegrationClient(t)

const fp = "aaaa0000bbbb1111cccc2222dddd3333aaaa0000bbbb1111cccc2222dddd3333"
```

Insert after line 155 (`c := newIntegrationClient(t)`):
```go
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mechanic-agent-aaaa0000bbbb",
    Namespace: integrationCtrlNamespace,
}})
```

---

#### `TestRemediationJobReconciler_SyncsStatus_Running` (starts at line 221)

Current structure (lines 225–228):
```go
ctx := context.Background()
c := newIntegrationClient(t)

const fp = "bbbb1111cccc2222dddd3333eeee4444bbbb1111cccc2222dddd3333eeee4444"
```

Insert after line 226 (`c := newIntegrationClient(t)`):
```go
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mechanic-agent-bbbb1111cccc",
    Namespace: integrationCtrlNamespace,
}})
```

---

#### `TestRemediationJobReconciler_SyncsStatus_Succeeded` (starts at line 278)

Current structure (lines 282–285):
```go
ctx := context.Background()
c := newIntegrationClient(t)

const fp = "cccc2222dddd3333eeee4444ffff5555cccc2222dddd3333eeee4444ffff5555"
```

Insert after line 283 (`c := newIntegrationClient(t)`):
```go
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mechanic-agent-cccc2222dddd",
    Namespace: integrationCtrlNamespace,
}})
```

---

#### `TestRemediationJobReconciler_SyncsStatus_Failed` (starts at line 335)

Current structure (lines 339–342):
```go
ctx := context.Background()
c := newIntegrationClient(t)

const fp = "dddd3333eeee4444ffff5555aaaa6666dddd3333eeee4444ffff5555aaaa6666"
```

Insert after line 340 (`c := newIntegrationClient(t)`):
```go
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mechanic-agent-dddd3333eeee",
    Namespace: integrationCtrlNamespace,
}})
```

---

#### `TestRemediationJobReconciler_OwnerReference` (starts at line 478)

Current structure (lines 482–485):
```go
ctx := context.Background()
c := newIntegrationClient(t)

const fp = "ffff5555aaaa6666bbbb7777cccc8888ffff5555aaaa6666bbbb7777cccc8888"
```

Insert after line 483 (`c := newIntegrationClient(t)`):
```go
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mechanic-agent-ffff5555aaaa",
    Namespace: integrationCtrlNamespace,
}})
```

---

### Verification

```bash
go test -count=3 -timeout 300s -race ./internal/controller/...
```

All three runs of every test must pass. If any run fails, the fix is incomplete.

---

## Implementation Steps

- [ ] Read `internal/controller/integration_test.go` lines 150–220 (`CreatesJob`)
- [ ] Insert pre-test stale job deletion in `TestRemediationJobReconciler_CreatesJob`
- [ ] Read lines 221–275 (`SyncsStatus_Running`)
- [ ] Insert pre-test stale job deletion in `TestRemediationJobReconciler_SyncsStatus_Running`
- [ ] Read lines 278–332 (`SyncsStatus_Succeeded`)
- [ ] Insert pre-test stale job deletion in `TestRemediationJobReconciler_SyncsStatus_Succeeded`
- [ ] Read lines 335–390 (`SyncsStatus_Failed`)
- [ ] Insert pre-test stale job deletion in `TestRemediationJobReconciler_SyncsStatus_Failed`
- [ ] Read lines 478–538 (`OwnerReference`)
- [ ] Insert pre-test stale job deletion in `TestRemediationJobReconciler_OwnerReference`
- [ ] Run `go test -count=3 -timeout 300s -race ./internal/controller/...` — all must pass

---

## Dependencies

**Depends on:** None (standalone test change)
**Blocks:** STORY_02 (documentation)

---

## Definition of Done

- [ ] Pre-test stale job deletion added to all five affected tests
- [ ] Full controller test suite passes under `-count=3`
