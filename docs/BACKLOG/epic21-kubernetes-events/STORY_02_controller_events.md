# Story 02: RemediationJobReconciler — EventRecorder wiring and lifecycle events

**Epic:** epic21-kubernetes-events (FT-U3)
**Status:** Not Started
**Estimate:** S

## Context

`RemediationJobReconciler` in `internal/controller/remediationjob_controller.go` has no
`EventRecorder` field. The controller already emits structured log lines at each phase
transition, but nothing is written to the Kubernetes Events API, so
`kubectl describe rjob <name>` shows no Events section.

This story adds the `Recorder record.EventRecorder` field, wires it from the manager in
`cmd/watcher/main.go`, and inserts four `Recorder.Event` calls — one per lifecycle transition.

## Current struct (before this story)

```go
// internal/controller/remediationjob_controller.go:29-36
type RemediationJobReconciler struct {
    client.Client
    Scheme     *runtime.Scheme
    Log        *zap.Logger
    JobBuilder domain.JobBuilder
    Cfg        config.Config
}
```

## Required struct change

Add `Recorder record.EventRecorder` as a new field:

```go
type RemediationJobReconciler struct {
    client.Client
    Scheme     *runtime.Scheme
    Log        *zap.Logger
    JobBuilder domain.JobBuilder
    Cfg        config.Config
    Recorder   record.EventRecorder
}
```

Add the import `"k8s.io/client-go/tools/record"` to the import block.

## main.go wiring

Location: `cmd/watcher/main.go`, the `RemediationJobReconciler` literal at line 122.

Before:

```go
if err := (&controller.RemediationJobReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jb,
    Cfg:        cfg,
}).SetupWithManager(mgr); err != nil {
```

After:

```go
if err := (&controller.RemediationJobReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jb,
    Cfg:        cfg,
    Recorder:   mgr.GetEventRecorderFor("mendabot-watcher"),
}).SetupWithManager(mgr); err != nil {
```

`mgr.GetEventRecorderFor` is the exact method name on `ctrl.Manager`. Using
`"mendabot-watcher"` as the component name is consistent with the
`SourceProviderReconciler` wiring already present in `main.go:148`.

## Phase transitions and required Event calls

All events are emitted on `&rjob` — the `*v1alpha1.RemediationJob` that is being
reconciled in the current request.

### JobDispatched — in `dispatch()`, after status patch succeeds

Location: `internal/controller/remediationjob_controller.go`, `dispatch()` method, after
the `r.Status().Patch` call at line 225.

```go
if r.Recorder != nil {
    r.Recorder.Eventf(rjob, corev1.EventTypeNormal, "JobDispatched",
        "Created agent Job %s", job.Name)
}
```

`job.Name` is set by `r.JobBuilder.Build(rjob, nil)` and is available in scope.

### JobSucceeded — in the owned-job sync block

Location: `internal/controller/remediationjob_controller.go`, after the
`r.Status().Patch(ctx, &rjob, ...)` call at line 124, inside the existing
`if r.Log != nil && (newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed)`
block. Emit distinct events per phase:

```go
if r.Log != nil && (newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed) {
    // existing log lines ...
}
if r.Recorder != nil {
    switch newPhase {
    case v1alpha1.PhaseSucceeded:
        prRef := rjob.Status.PRRef  // set by agent via status patch; may be ""
        if prRef != "" {
            r.Recorder.Eventf(&rjob, corev1.EventTypeNormal, "JobSucceeded",
                "Agent Job completed; PR: %s", prRef)
        } else {
            r.Recorder.Event(&rjob, corev1.EventTypeNormal, "JobSucceeded",
                "Agent Job completed")
        }
    case v1alpha1.PhaseFailed:
        r.Recorder.Eventf(&rjob, corev1.EventTypeWarning, "JobFailed",
            "Agent Job failed after %d attempt(s)", job.Status.Failed)
    }
}
```

**Where does the PR URL come from?**
`rjob.Status.PRRef` is defined in `RemediationJobStatus` (line 163 of
`api/v1alpha1/remediationjob_types.go`):

```go
// PRRef is the GitHub PR URL opened or commented on by the agent.
// Set by the agent via a status patch before it exits (best-effort).
PRRef string `json:"prRef,omitempty"`
```

The controller already reads it in the existing TTL-deletion log at line 78:
`zap.String("prRef", rjob.Status.PRRef)`. It may be empty if the agent did not open a
PR or did not patch the status before exiting; the conditional message handles both cases.

**Where does the failure reason come from?**
There is no structured failure message field on `batchv1.Job` that the controller
currently extracts. The most reliable signal available without querying Pod logs is
`job.Status.Failed` (the count of failed Pod attempts). The event message uses this
count. If a richer reason is needed in a future story, it can be extracted from Job
conditions (`job.Status.Conditions` with type `Failed`) — that is out of scope here.

### SourceDeleted — emitted in SourceProviderReconciler (Story 01)

The `PhaseCancelled` transition is driven by `SourceProviderReconciler.Reconcile`, not by
`RemediationJobReconciler`. The `SourceDeleted` event is therefore emitted from the
provider reconciler (Story 01) on the `rjob` object. No additional event is needed here.

## Import change required

Add the following to the import block in
`internal/controller/remediationjob_controller.go`:

```go
corev1 "k8s.io/api/core/v1"
"k8s.io/client-go/tools/record"
```

`corev1` is needed for `corev1.EventTypeNormal` and `corev1.EventTypeWarning`.

## Test approach

### Recorder construction in unit tests

Use `record.NewFakeRecorder(bufferSize)` from `k8s.io/client-go/tools/record`:

```go
import "k8s.io/client-go/tools/record"

fakeRecorder := record.NewFakeRecorder(10)
r := &controller.RemediationJobReconciler{
    Client:     c,
    Scheme:     newTestScheme(t),
    Log:        zap.NewNop(),
    JobBuilder: jb,
    Cfg:        defaultCfg(),
    Recorder:   fakeRecorder,
}
```

### Reading emitted events

The fake recorder writes events as `"<Type> <Reason> <message>"` strings to
`fakeRecorder.Events` (a `chan string`). Drain them non-blockingly:

```go
func drainEvents(ch <-chan string) []string {
    var out []string
    for {
        select {
        case e := <-ch:
            out = append(out, e)
        default:
            return out
        }
    }
}
```

### Test cases to add

Add the following tests to
`internal/controller/remediationjob_controller_test.go`:

| Test name | Scenario | Expected event |
|---|---|---|
| `TestReconcile_EmitsEvent_JobDispatched` | `PhasePending` rjob, no existing Job, Build succeeds | `"Normal JobDispatched Created agent Job mendabot-agent-..."` |
| `TestReconcile_EmitsEvent_JobSucceeded_WithPR` | Owned Job with `Succeeded > 0`, `rjob.Status.PRRef` set | `"Normal JobSucceeded Agent Job completed; PR: https://..."` |
| `TestReconcile_EmitsEvent_JobSucceeded_NoPR` | Owned Job with `Succeeded > 0`, `rjob.Status.PRRef` empty | `"Normal JobSucceeded Agent Job completed"` |
| `TestReconcile_EmitsEvent_JobFailed` | Owned Job with `Failed >= BackoffLimit+1` | `"Warning JobFailed Agent Job failed after N attempt(s)"` |
| `TestReconcile_NilRecorder_NoPanic` | `Recorder` is `nil`, Pending rjob dispatched | No panic, job still created |

#### Example: TestReconcile_EmitsEvent_JobDispatched

```go
func TestReconcile_EmitsEvent_JobDispatched(t *testing.T) {
    const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
    rjob := newRJob("test-rjob", fp)
    rjob.Status.Phase = v1alpha1.PhasePending
    c := newFakeClient(t, rjob)

    job := defaultFakeJob(rjob)
    jb := &fakeJobBuilder{returnJob: job}

    fakeRecorder := record.NewFakeRecorder(10)
    r := &controller.RemediationJobReconciler{
        Client:     c,
        Scheme:     newTestScheme(t),
        Log:        zap.NewNop(),
        JobBuilder: jb,
        Cfg:        defaultCfg(),
        Recorder:   fakeRecorder,
    }

    _, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    events := drainEvents(fakeRecorder.Events)
    var found bool
    for _, e := range events {
        if strings.Contains(e, "JobDispatched") {
            found = true
            break
        }
    }
    if !found {
        t.Errorf("expected JobDispatched event, got: %v", events)
    }
}
```

#### Example: TestReconcile_EmitsEvent_JobFailed

```go
func TestReconcile_EmitsEvent_JobFailed(t *testing.T) {
    const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
    rjob := newRJob("test-rjob", fp)
    rjob.Status.Phase = v1alpha1.PhasePending

    // BackoffLimit=1 means PhaseFailed when Failed >= 2
    failedJob := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mendabot-agent-" + fp[:12],
            Namespace: testNamespace,
            Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-rjob"},
        },
        Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
        Status: batchv1.JobStatus{Failed: 2},
    }

    c := newFakeClient(t, rjob, failedJob)
    jb := &fakeJobBuilder{}

    fakeRecorder := record.NewFakeRecorder(10)
    r := &controller.RemediationJobReconciler{
        Client:     c,
        Scheme:     newTestScheme(t),
        Log:        zap.NewNop(),
        JobBuilder: jb,
        Cfg:        defaultCfg(),
        Recorder:   fakeRecorder,
    }

    _, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    events := drainEvents(fakeRecorder.Events)
    var found bool
    for _, e := range events {
        if strings.Contains(e, "JobFailed") {
            found = true
            break
        }
    }
    if !found {
        t.Errorf("expected JobFailed event, got: %v", events)
    }
}
```

### Nil-guard pattern

All `r.Recorder.Event(...)` calls must be guarded with `if r.Recorder != nil`. The
existing `newReconciler` helper in `remediationjob_controller_test.go` does not set
`Recorder` (zero value is `nil`). All existing tests must continue to pass without
modification.

## Summary of all file changes

| File | Change |
|---|---|
| `internal/controller/remediationjob_controller.go` | Add `Recorder record.EventRecorder` field; add `corev1` and `record` imports; add 4 `Recorder.Event` calls |
| `cmd/watcher/main.go` | Add `Recorder: mgr.GetEventRecorderFor("mendabot-watcher")` to `RemediationJobReconciler` literal |
| `internal/controller/remediationjob_controller_test.go` | Add 5 new test functions (see above) |

## Definition of Done

- [ ] `Recorder record.EventRecorder` field added to `RemediationJobReconciler`
- [ ] `corev1` and `record` imports added to `remediationjob_controller.go`
- [ ] `Recorder: mgr.GetEventRecorderFor("mendabot-watcher")` added in `cmd/watcher/main.go`
- [ ] `JobDispatched` event emitted in `dispatch()` after status patch
- [ ] `JobSucceeded` event emitted (with PR URL if present) when `newPhase == PhaseSucceeded`
- [ ] `JobFailed` event (Warning type) emitted with attempt count when `newPhase == PhaseFailed`
- [ ] All event calls guarded with `if r.Recorder != nil`
- [ ] New unit tests pass with `-race`
- [ ] Existing unit and integration tests unchanged and passing
- [ ] `kubectl describe rjob <name>` shows Events section with dispatched/succeeded/failed entries
