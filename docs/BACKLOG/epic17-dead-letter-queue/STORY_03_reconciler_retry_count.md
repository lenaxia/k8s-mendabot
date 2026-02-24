# Story 03: RemediationJobReconciler — Increment RetryCount on Job Failure

**Epic:** [epic17-dead-letter-queue](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot operator**, I want the `RemediationJobReconciler` to increment
`Status.RetryCount` each time the owned `batch/v1 Job` transitions to the `Failed`
state, and to set `Phase = PermanentlyFailed` when `RetryCount >= MaxRetries`, so that
the retry loop is bounded and I never burn unlimited LLM quota on a broken job.

---

## Background

### Where job failure is detected today

In `internal/controller/remediationjob_controller.go`, the reconcile loop reaches the
owned-job-sync block at lines 100–142 when at least one batch/v1 Job exists with the
matching label. The helper `syncPhaseFromJob` (lines 167–182) returns `PhaseFailed`
when `job.Status.Failed >= backoffLimit+1`.

The transition to `PhaseFailed` is applied at lines 103–126:

```go
// remediationjob_controller.go:100-126
if len(ownedJobs.Items) > 0 {
    job := &ownedJobs.Items[0]
    newPhase := syncPhaseFromJob(job)
    rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
    rjob.Status.Phase = newPhase
    if newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed {
        if rjob.Status.CompletedAt == nil {
            now := metav1.Now()
            rjob.Status.CompletedAt = &now
        }
        condType := v1alpha1.ConditionJobComplete
        if newPhase == v1alpha1.PhaseFailed {
            condType = v1alpha1.ConditionJobFailed
        }
        apimeta.SetStatusCondition(...)
    }
    // ... patch
}
```

The `switch` at lines 62–90 already short-circuits for `PhaseFailed` and
`PhaseCancelled` — returning immediately with no work. After this story, the switch
must also short-circuit for `PhasePermanentlyFailed`.

### What must change

When `newPhase == v1alpha1.PhaseFailed` is detected in the owned-job-sync block:

1. **Increment `RetryCount`** on the `RemediationJob` status.
2. **Check `RetryCount >= MaxRetries`** — if so, transition to `PhasePermanentlyFailed`
   instead of leaving phase at `PhaseFailed`, set the `ConditionPermanentlyFailed`
   condition, and log an audit event.
3. The status patch already at line 124 persists both fields in one call.

The `PhasePermanentlyFailed` short-circuit must be added to the terminal-phase switch
at lines 85–90.

### Idempotency

The reconciler may fire multiple times after a job has already reached `PhaseFailed`.
On the second reconcile the owned job still has `Status.Failed >= backoffLimit+1`
so `syncPhaseFromJob` returns `PhaseFailed` again. Without a guard, `RetryCount`
would be incremented on every reconcile. The guard is: **only increment `RetryCount`
when the `rjob.Status.Phase` is transitioning from a non-terminal phase to
`PhaseFailed`** (i.e., the incoming phase is not already `PhaseFailed`).

Concretely: `if rjob.Status.Phase != v1alpha1.PhaseFailed { rjob.Status.RetryCount++ }`.
This check uses the pre-patch copy of the phase (still in `rjob.Status.Phase` because
`rjobCopy` was captured before the mutation at line 104).

---

## Acceptance Criteria

- [x] `RetryCount` is incremented exactly once per `PhaseFailed` transition
- [x] When `RetryCount < MaxRetries` after increment, phase stays `PhaseFailed` (SourceProviderReconciler will re-dispatch via delete+create)
- [x] When `RetryCount >= MaxRetries` after increment, phase is set to `PermanentlyFailed`
- [x] `ConditionPermanentlyFailed` condition is set to `True` when permanently failing
- [x] `PhasePermanentlyFailed` is added to the terminal-phase switch (returns immediately, no dispatch)
- [x] An audit log line is emitted when permanently failing (event `"job.permanently_failed"`)
- [x] All new and existing tests pass: `go test -timeout 30s -race ./internal/controller/...`

---

## Technical Implementation

### `internal/controller/remediationjob_controller.go`

#### Change 1 — Add `PhasePermanentlyFailed` to the terminal-phase switch (lines 85–90)

```go
// Before (lines 85-90):
case v1alpha1.PhaseFailed:
    return ctrl.Result{}, nil

case v1alpha1.PhaseCancelled:
    return ctrl.Result{}, nil

// After:
case v1alpha1.PhaseFailed:
    return ctrl.Result{}, nil

case v1alpha1.PhasePermanentlyFailed:
    return ctrl.Result{}, nil

case v1alpha1.PhaseCancelled:
    return ctrl.Result{}, nil
```

#### Change 2 — Increment RetryCount and check cap in the owned-job-sync block
(insert inside the `if newPhase == v1alpha1.PhaseFailed` branch, after setting
`condType` and before the `apimeta.SetStatusCondition` call at line 114)

Replace the existing `PhaseFailed` terminal block (approximately lines 105–120) with:

```go
if newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed {
    if rjob.Status.CompletedAt == nil {
        now := metav1.Now()
        rjob.Status.CompletedAt = &now
    }
    if newPhase == v1alpha1.PhaseFailed {
        // Only increment RetryCount when transitioning *into* Failed for the
        // first time (not on subsequent reconciles of an already-Failed rjob).
        if rjob.Status.Phase != v1alpha1.PhaseFailed {
            rjob.Status.RetryCount++
        }
        maxRetries := rjob.Spec.MaxRetries
        if maxRetries <= 0 {
            maxRetries = 3 // fallback in case the field was not populated
        }
        if rjob.Status.RetryCount >= maxRetries {
            // Permanently tombstone — SourceProviderReconciler will not re-dispatch.
            rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed
            apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
                Type:               v1alpha1.ConditionPermanentlyFailed,
                Status:             metav1.ConditionTrue,
                Reason:             "RetryCapReached",
                Message:            fmt.Sprintf("RetryCount %d reached MaxRetries %d", rjob.Status.RetryCount, maxRetries),
                LastTransitionTime: metav1.Now(),
            })
        } else {
            apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
                Type:               v1alpha1.ConditionJobFailed,
                Status:             metav1.ConditionTrue,
                Reason:             string(newPhase),
                LastTransitionTime: metav1.Now(),
            })
        }
    } else {
        // PhaseSucceeded
        apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
            Type:               v1alpha1.ConditionJobComplete,
            Status:             metav1.ConditionTrue,
            Reason:             string(newPhase),
            LastTransitionTime: metav1.Now(),
        })
    }
}
```

#### Change 3 — Audit log when permanently failing
(extend the existing audit log block at lines 127–140 to cover `PermanentlyFailed`)

```go
if r.Log != nil {
    switch {
    case newPhase == v1alpha1.PhaseSucceeded:
        r.Log.Info("agent job terminal",
            zap.Bool("audit", true),
            zap.String("event", "job.succeeded"),
            zap.String("remediationJob", rjob.Name),
            zap.String("job", job.Name),
            zap.String("namespace", rjob.Namespace),
            zap.String("prRef", rjob.Status.PRRef),
        )
    case rjob.Status.Phase == v1alpha1.PhasePermanentlyFailed:
        r.Log.Info("agent job permanently failed",
            zap.Bool("audit", true),
            zap.String("event", "job.permanently_failed"),
            zap.String("remediationJob", rjob.Name),
            zap.String("job", job.Name),
            zap.String("namespace", rjob.Namespace),
            zap.Int32("retryCount", rjob.Status.RetryCount),
            zap.Int32("maxRetries", rjob.Spec.MaxRetries),
        )
    case newPhase == v1alpha1.PhaseFailed:
        r.Log.Info("agent job terminal",
            zap.Bool("audit", true),
            zap.String("event", "job.failed"),
            zap.String("remediationJob", rjob.Name),
            zap.String("job", job.Name),
            zap.String("namespace", rjob.Namespace),
            zap.String("prRef", rjob.Status.PRRef),
        )
    }
}
```

---

## Test Cases

### Unit tests — `internal/controller/remediationjob_controller_test.go`

Add a new test group after `TestRemediationJobReconciler_Cancelled_ReturnsNil`:

```go
// TestRemediationJobReconciler_PhaseFailed_IncrementsRetryCount verifies that
// when the owned batch/v1 Job transitions to Failed, RetryCount is incremented
// exactly once.
func TestRemediationJobReconciler_PhaseFailed_IncrementsRetryCount(t *testing.T) {
    const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
    rjob := newRJob("test-retry-count", fp)
    rjob.Status.Phase = v1alpha1.PhasePending
    rjob.Spec.MaxRetries = 3

    backoffLimit := int32(1)
    failedJob := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mendabot-agent-" + fp[:12],
            Namespace: testNamespace,
            Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-retry-count"},
        },
        Spec:   batchv1.JobSpec{BackoffLimit: &backoffLimit},
        Status: batchv1.JobStatus{Failed: backoffLimit + 1},
    }

    c := newFakeClient(t, rjob, failedJob)
    jb := &fakeJobBuilder{}
    r := newReconciler(t, c, jb, defaultCfg())

    _, err := r.Reconcile(context.Background(), rjobReqFor("test-retry-count"))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var updated v1alpha1.RemediationJob
    if err := c.Get(context.Background(), types.NamespacedName{Name: "test-retry-count", Namespace: testNamespace}, &updated); err != nil {
        t.Fatalf("get rjob: %v", err)
    }
    if updated.Status.RetryCount != 1 {
        t.Errorf("RetryCount = %d, want 1", updated.Status.RetryCount)
    }
    if updated.Status.Phase != v1alpha1.PhaseFailed {
        t.Errorf("Phase = %q, want %q (below cap)", updated.Status.Phase, v1alpha1.PhaseFailed)
    }
}

// TestRemediationJobReconciler_PhaseFailed_AtCap_PermanentlyFails verifies that
// when RetryCount reaches MaxRetries, phase transitions to PermanentlyFailed.
func TestRemediationJobReconciler_PhaseFailed_AtCap_PermanentlyFails(t *testing.T) {
    const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
    rjob := newRJob("test-perm-fail", fp)
    rjob.Status.Phase = v1alpha1.PhasePending
    rjob.Spec.MaxRetries = 3
    rjob.Status.RetryCount = 2 // one more failure will hit the cap

    backoffLimit := int32(1)
    failedJob := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mendabot-agent-" + fp[:12],
            Namespace: testNamespace,
            Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-perm-fail"},
        },
        Spec:   batchv1.JobSpec{BackoffLimit: &backoffLimit},
        Status: batchv1.JobStatus{Failed: backoffLimit + 1},
    }

    c := newFakeClient(t, rjob, failedJob)
    jb := &fakeJobBuilder{}
    r := newReconciler(t, c, jb, defaultCfg())

    _, err := r.Reconcile(context.Background(), rjobReqFor("test-perm-fail"))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var updated v1alpha1.RemediationJob
    if err := c.Get(context.Background(), types.NamespacedName{Name: "test-perm-fail", Namespace: testNamespace}, &updated); err != nil {
        t.Fatalf("get rjob: %v", err)
    }
    if updated.Status.RetryCount != 3 {
        t.Errorf("RetryCount = %d, want 3", updated.Status.RetryCount)
    }
    if updated.Status.Phase != v1alpha1.PhasePermanentlyFailed {
        t.Errorf("Phase = %q, want %q", updated.Status.Phase, v1alpha1.PhasePermanentlyFailed)
    }
}

// TestRemediationJobReconciler_RetryCount_Idempotent verifies that re-reconciling
// an already-Failed rjob does NOT increment RetryCount again.
func TestRemediationJobReconciler_RetryCount_Idempotent(t *testing.T) {
    const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
    rjob := newRJob("test-retry-idem", fp)
    // Already in Failed phase with RetryCount=1 — simulates second reconcile
    rjob.Status.Phase = v1alpha1.PhaseFailed
    rjob.Status.RetryCount = 1
    rjob.Spec.MaxRetries = 3

    c := newFakeClient(t, rjob)
    jb := &fakeJobBuilder{}
    r := newReconciler(t, c, jb, defaultCfg())

    _, err := r.Reconcile(context.Background(), rjobReqFor("test-retry-idem"))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var updated v1alpha1.RemediationJob
    if err := c.Get(context.Background(), types.NamespacedName{Name: "test-retry-idem", Namespace: testNamespace}, &updated); err != nil {
        t.Fatalf("get rjob: %v", err)
    }
    // Phase is already Failed → short-circuit, no change
    if updated.Status.RetryCount != 1 {
        t.Errorf("RetryCount = %d, want 1 (idempotent — must not re-increment on already-Failed rjob)", updated.Status.RetryCount)
    }
}

// TestRemediationJobReconciler_PermanentlyFailed_ReturnsNil verifies
// PermanentlyFailed phase → returns immediately, no dispatch.
func TestRemediationJobReconciler_PermanentlyFailed_ReturnsNil(t *testing.T) {
    const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
    rjob := newRJob("test-perm-noop", fp)
    rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed

    c := newFakeClient(t, rjob)
    jb := &fakeJobBuilder{}
    r := newReconciler(t, c, jb, defaultCfg())

    result, err := r.Reconcile(context.Background(), rjobReqFor("test-perm-noop"))
    if err != nil {
        t.Errorf("expected nil error for PermanentlyFailed phase, got %v", err)
    }
    if result.RequeueAfter != 0 || result.Requeue {
        t.Errorf("expected zero Result for PermanentlyFailed phase, got %+v", result)
    }
    if len(jb.calls) != 0 {
        t.Error("expected no Build() calls for PermanentlyFailed phase")
    }
}
```

### Table-driven test for all terminal phases

Add this test to replace/augment the existing `TestRemediationJobReconciler_Failed_ReturnsNil`
so all three terminal-no-dispatch phases are verified together:

```go
func TestRemediationJobReconciler_TerminalPhases_NoBuild(t *testing.T) {
    const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
    tests := []struct {
        phase v1alpha1.RemediationJobPhase
    }{
        {v1alpha1.PhaseFailed},
        {v1alpha1.PhaseCancelled},
        {v1alpha1.PhasePermanentlyFailed},
    }
    for _, tt := range tests {
        t.Run(string(tt.phase), func(t *testing.T) {
            rjob := newRJob("test-terminal-"+string(tt.phase), fp)
            rjob.Status.Phase = tt.phase

            c := newFakeClient(t, rjob)
            jb := &fakeJobBuilder{}
            r := newReconciler(t, c, jb, defaultCfg())

            result, err := r.Reconcile(context.Background(),
                rjobReqFor("test-terminal-"+string(tt.phase)))
            if err != nil {
                t.Errorf("phase %q: unexpected error: %v", tt.phase, err)
            }
            if result.RequeueAfter != 0 || result.Requeue {
                t.Errorf("phase %q: expected zero Result, got %+v", tt.phase, result)
            }
            if len(jb.calls) != 0 {
                t.Errorf("phase %q: expected no Build() calls", tt.phase)
            }
        })
    }
}
```

---

## Tasks

- [x] Write the four unit tests above (TDD — run first; `PhasePermanentlyFailed` won't compile until STORY_01 is merged)
- [x] Add `PhasePermanentlyFailed` case to terminal-phase switch (lines 85–90)
- [x] Add RetryCount increment + cap check in the owned-job-sync `PhaseFailed` branch (lines 105–120)
- [x] Add `ConditionPermanentlyFailed` set logic when cap is reached
- [x] Update audit log block to emit `job.permanently_failed` event
- [x] Run: `go test -timeout 30s -race ./internal/controller/...` — must pass
- [x] Run: `go vet ./internal/controller/...` — must be clean

---

## Dependencies

**Depends on:** STORY_01 (`PhasePermanentlyFailed`, `ConditionPermanentlyFailed`,
`RetryCount`, `MaxRetries` must exist in v1alpha1)
**Blocks:** STORY_04 (the gate in the source provider is meaningless until the
controller actually enters `PhasePermanentlyFailed`)

---

## Definition of Done

- [x] `RetryCount` is incremented exactly once per `PhaseFailed` transition (idempotent on re-reconcile)
- [x] `Phase == PermanentlyFailed` when `RetryCount >= MaxRetries` after increment
- [x] `ConditionPermanentlyFailed = True` set when phase becomes `PermanentlyFailed`
- [x] `PhasePermanentlyFailed` is a no-op short-circuit in the terminal-phase switch
- [x] Audit log event `job.permanently_failed` is emitted
- [x] `go test -timeout 30s -race ./internal/controller/...` green
- [x] `go vet ./internal/controller/...` clean
