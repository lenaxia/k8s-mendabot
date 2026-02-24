# Epic 21: Kubernetes Events on RemediationJob

**Feature Tracker:** FT-U3
**Area:** Usability & Operability

## Purpose

Add controller-runtime `record.EventRecorder` to both `SourceProviderReconciler` and
`RemediationJobReconciler` so that `kubectl describe rjob <name>` shows the full
lifecycle timeline in the Events section.

Today there is no Events section in `kubectl describe rjob`. Diagnosing a stuck or
failed `RemediationJob` requires inspecting the `status.message` field, the owned
`batch/v1 Job`, and log lines separately.

## Status: Not Started

## Deep-Dive Findings (2026-02-23)

### SourceProviderReconciler — field is already wired (STORY_01)
`internal/provider/provider.go` line 33 already declares:
```go
EventRecorder record.EventRecorder
```
`cmd/watcher/main.go` line 148 already assigns:
```go
EventRecorder: mgr.GetEventRecorderFor("mendabot-watcher"),
```
**The field is wired but never called.** No struct or `main.go` changes are needed for
STORY_01 — only the four missing `r.EventRecorder.Event(...)` calls must be added.

Only one import is needed: `corev1 "k8s.io/api/core/v1"` (for `EventTypeNormal`).

### RemediationJobReconciler — field not yet present (STORY_02)
`internal/controller/remediationjob_controller.go` struct (lines 29–36) has no
`EventRecorder` field. This story adds:
```go
Recorder record.EventRecorder
```
`cmd/watcher/main.go` `RemediationJobReconciler` literal at line 122 must add:
```go
Recorder: mgr.GetEventRecorderFor("mendabot-watcher"),
```
Imports needed: `corev1 "k8s.io/api/core/v1"` and `"k8s.io/client-go/tools/record"`.

### Event emission — all calls are nil-guarded
All calls use `if r.EventRecorder != nil` (provider) / `if r.Recorder != nil` (controller),
matching the existing `if r.Log != nil` pattern. Existing tests that omit the recorder
continue to pass without modification.

### `SourceDeleted` event target
When the watched object is not found (`IsNotFound`), `obj` cannot be used as the event
target. The `SourceDeleted` event is emitted on `rjob` instead — the RemediationJob being
cancelled.

## Dependencies

- epic01-controller complete (`internal/provider/provider.go`, `internal/controller/remediationjob_controller.go`)
- epic09-native-provider complete

## Blocks

- Nothing

## Stories

| Story | File | Status |
|-------|------|--------|
| SourceProviderReconciler — EventRecorder wiring and finding events | [STORY_01_source_provider_events.md](STORY_01_source_provider_events.md) | Not Started |
| RemediationJobReconciler — EventRecorder wiring and lifecycle events | [STORY_02_controller_events.md](STORY_02_controller_events.md) | Not Started |

## Implementation Order

```
STORY_01 (source provider events)
STORY_02 (controller events)
```

These two stories are independent and can be worked in parallel.

## Key Events

| Reconciler | Reason | Type | Message pattern |
|---|---|---|---|
| SourceProvider | `FindingDetected` | Normal | `Provider native detected Pod/my-app in namespace default` |
| SourceProvider | `DuplicateFingerprint` | Normal | `Existing RemediationJob mendabot-<fp[:12]> already covers this finding` |
| SourceProvider | `FindingCleared` | Normal | `Finding cleared; no active finding on this object` |
| SourceProvider | `SourceDeleted` | Normal | `Source object deleted; investigation cancelled` (emitted on rjob) |
| Controller | `JobDispatched` | Normal | `Created agent Job mendabot-agent-<fp[:12]>` |
| Controller | `JobSucceeded` | Normal | `Agent Job completed; PR: <url>` (or `Agent Job completed` if no PR) |
| Controller | `JobFailed` | **Warning** | `Agent Job failed after N attempt(s)` |

## File Changes Summary

| File | Change |
|---|---|
| `internal/provider/provider.go` | Add `corev1` import; add 4 `EventRecorder.Event` calls (nil-guarded) |
| `internal/controller/remediationjob_controller.go` | Add `Recorder record.EventRecorder` field; add `corev1` + `record` imports; add 3 `Recorder.Event` calls (nil-guarded) |
| `cmd/watcher/main.go` | Add `Recorder: mgr.GetEventRecorderFor("mendabot-watcher")` to `RemediationJobReconciler` literal |

## Definition of Done

- [ ] `SourceProviderReconciler` `EventRecorder` field is called (already wired at line 33 / main.go:148)
- [ ] `RemediationJobReconciler` gains `Recorder record.EventRecorder` field
- [ ] `Recorder: mgr.GetEventRecorderFor("mendabot-watcher")` wired in `cmd/watcher/main.go`
- [ ] All key lifecycle events emitted as listed above
- [ ] All event calls guarded with nil-guard (`if r.EventRecorder != nil` / `if r.Recorder != nil`)
- [ ] `kubectl describe rjob <name>` shows Events section with lifecycle entries
- [ ] All unit and integration tests pass with `-race`
- [ ] Worklog written
