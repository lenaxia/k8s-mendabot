# Story 01: SourceProviderReconciler — EventRecorder wiring and finding events

**Epic:** epic21-kubernetes-events (FT-U3)
**Status:** Complete
**Estimate:** S

## Context

`SourceProviderReconciler` in `internal/provider/provider.go` already declares an
`EventRecorder record.EventRecorder` field (line 33) and `main.go` already assigns it
via `mgr.GetEventRecorderFor("mechanic-watcher")` (line 148). The field is wired but
never called: no `r.EventRecorder.Event(...)` calls exist anywhere in `Reconcile`.

This story adds the four missing `Event` calls, one at each decision point in
`Reconcile`, so that `kubectl describe` on the watched object shows a live timeline.

## What is already true (do not re-implement)

- `SourceProviderReconciler.EventRecorder record.EventRecorder` — field exists.
- `cmd/watcher/main.go:148` — `EventRecorder: mgr.GetEventRecorderFor("mechanic-watcher")` — already present in the loop that constructs each `SourceProviderReconciler`.

## Decision points in Reconcile

The following table maps each significant outcome to the Event that must be emitted.
The reconciled object is always `obj` (the `client.Object` fetched via `r.Get`).
The sole exception is the source-deleted / cancel path: `obj` could not be fetched, so
the event is emitted on each `rjob` that gets cancelled.

| Location in provider.go | Reason | Type | Message pattern |
|---|---|---|---|
| After `r.Create(ctx, rjob)` succeeds (line 264) | `FindingDetected` | Normal | `Provider <name> detected <Kind>/<name> in namespace <ns>` |
| Inside the dedup loop when `rjob.Status.Phase != PhaseFailed` returns early (line 196) | `DuplicateFingerprint` | Normal | `Existing RemediationJob mechanic-<fp[:12]> already covers this finding` |
| After `finding == nil` check (line 122) | `FindingCleared` | Normal | `Finding cleared; no active finding on this object` |
| After successful `r.Delete(ctx, rjob)` in the source-deleted cancel loop (line 100) | `SourceDeleted` | Normal | `Source object deleted; investigation cancelled` |

## Exact Go code to add

### FindingDetected — after `r.Create` succeeds

Location: `internal/provider/provider.go`, immediately after the existing `r.Log.Info` block at line 271.

```go
if r.EventRecorder != nil {
    r.EventRecorder.Eventf(obj, corev1.EventTypeNormal, "FindingDetected",
        "Provider %s detected %s/%s in namespace %s",
        r.Provider.ProviderName(), finding.Kind, finding.Name, finding.Namespace)
}
```

`obj` here is the `client.Object` fetched at the top of `Reconcile` — the Pod, Deployment,
PVC, etc. that the provider is watching.

### DuplicateFingerprint — inside the dedup loop

Location: `internal/provider/provider.go`, inside `for i := range rjobList.Items`, when
the non-Failed early-return fires (after the `if rjob.Spec.Fingerprint != fp { continue }` check).

```go
if rjob.Status.Phase != v1alpha1.PhaseFailed {
    if r.EventRecorder != nil {
        r.EventRecorder.Eventf(obj, corev1.EventTypeNormal, "DuplicateFingerprint",
            "Existing RemediationJob %s already covers this finding", rjob.Name)
    }
    return ctrl.Result{}, nil
}
```

### FindingCleared — when ExtractFinding returns nil

Location: `internal/provider/provider.go`, inside `if finding == nil { ... }`.

```go
if finding == nil {
    r.firstSeen.Clear()
    if r.EventRecorder != nil {
        r.EventRecorder.Event(obj, corev1.EventTypeNormal, "FindingCleared",
            "Finding cleared; no active finding on this object")
    }
    return ctrl.Result{}, nil
}
```

### SourceDeleted — in the cancel loop (event on rjob)

Location: `internal/provider/provider.go`, after the successful `r.Delete(ctx, rjob)` call
inside the source-deleted cancel loop. Because `obj` cannot be fetched (IsNotFound), the
event target is `rjob` itself.

```go
if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
    cancelErrs = append(cancelErrs, delErr)
} else {
    if r.EventRecorder != nil {
        r.EventRecorder.Event(rjob, corev1.EventTypeNormal, "SourceDeleted",
            "Source object deleted; investigation cancelled")
    }
    if r.Log != nil {
        r.Log.Info("RemediationJob cancelled", ...)
    }
}
```

## Import change required

Add `corev1 "k8s.io/api/core/v1"` to the import block in `internal/provider/provider.go`.
The file already imports `"k8s.io/apimachinery/pkg/runtime"` and
`"k8s.io/client-go/tools/record"`, so only the core/v1 types package is missing.

## main.go — no change required

`cmd/watcher/main.go` already wires `EventRecorder: mgr.GetEventRecorderFor("mechanic-watcher")`
for every `SourceProviderReconciler`. No modification needed.

## Test approach

### Recorder construction in unit tests

Use `record.NewFakeRecorder(bufferSize)` from `k8s.io/client-go/tools/record`.
The fake recorder is a channel-backed `EventRecorder`; events are read from the returned
`chan string`.

```go
import "k8s.io/client-go/tools/record"

fakeRecorder := record.NewFakeRecorder(10)
r := &provider.SourceProviderReconciler{
    Client:        c,
    Scheme:        newTestScheme(),
    Cfg:           config.Config{AgentNamespace: agentNamespace},
    Provider:      p,
    EventRecorder: fakeRecorder,
}
```

### Asserting on emitted events

Events are written as `"<Type> <Reason> <message>"` strings to `fakeRecorder.Events`.

```go
// Helper to drain all buffered events.
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

events := drainEvents(fakeRecorder.Events)
// assert one of the events contains "FindingDetected"
var found bool
for _, e := range events {
    if strings.Contains(e, "FindingDetected") {
        found = true
        break
    }
}
if !found {
    t.Errorf("expected FindingDetected event, got: %v", events)
}
```

### Test cases to add

Add the following tests to `internal/provider/provider_test.go`:

| Test name | Scenario | Expected event |
|---|---|---|
| `TestReconcile_EmitsEvent_FindingDetected` | Valid finding, no existing rjob, `r.Create` succeeds | `"Normal FindingDetected ..."` on `obj` |
| `TestReconcile_EmitsEvent_DuplicateFingerprint` | Non-Failed rjob with same fingerprint already exists | `"Normal DuplicateFingerprint ..."` on `obj` |
| `TestReconcile_EmitsEvent_FindingCleared` | `ExtractFinding` returns `nil, nil` | `"Normal FindingCleared ..."` on `obj` |
| `TestReconcile_EmitsEvent_SourceDeleted` | Watched object IsNotFound, Pending rjob exists | `"Normal SourceDeleted ..."` on `rjob` |
| `TestReconcile_NilRecorder_NoPanic` | `EventRecorder` is `nil`, valid finding | No panic, rjob still created |

### Nil-guard pattern

All event calls must be guarded with `if r.EventRecorder != nil` to match the existing
`if r.Log != nil` pattern used throughout the file.  The `newTestReconciler` helper in
`provider_test.go` currently omits `EventRecorder` (it is `nil`); existing tests must
continue to pass without modification.

## Definition of Done

- [ ] `corev1` import added to `internal/provider/provider.go`
- [ ] `FindingDetected` event emitted after successful `r.Create(ctx, rjob)`
- [ ] `DuplicateFingerprint` event emitted before the early return in the dedup loop
- [ ] `FindingCleared` event emitted when `finding == nil`
- [ ] `SourceDeleted` event emitted on `rjob` after successful delete in the cancel loop
- [ ] All event calls guarded with `if r.EventRecorder != nil`
- [ ] New unit tests pass with `-race`
- [ ] Existing unit and integration tests unchanged and passing
- [ ] `kubectl describe pod <name>` (or whichever native object) shows the Events section
