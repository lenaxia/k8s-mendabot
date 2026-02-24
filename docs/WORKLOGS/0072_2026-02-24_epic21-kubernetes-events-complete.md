# Worklog: Epic 21 — Kubernetes Events on RemediationJob

**Date:** 2026-02-24
**Session:** Epic 21 full implementation — SourceProviderReconciler + RemediationJobReconciler Kubernetes Events
**Status:** Complete

---

## Objective

Add `record.EventRecorder` calls to both `SourceProviderReconciler` and
`RemediationJobReconciler` so that `kubectl describe rjob <name>` shows a live
lifecycle timeline in the Events section (FT-U3).

---

## Work Completed

### 1. STORY_01 — SourceProviderReconciler event calls (internal/provider/provider.go)

The `EventRecorder` field was already wired (struct + main.go). Added 4 missing call sites:

- **FindingDetected** — after `r.Create(ctx, rjob)` succeeds; emits on `obj`; replaces the
  prior `RemediationJobCreated` event reason with the spec-correct `FindingDetected` reason
  and includes provider name, Kind, object name and namespace in the message.
- **DuplicateFingerprint** — in the dedup switch `default:` branch before early return;
  emits on `obj`; message includes rjob name.
- **FindingCleared** — inside `if finding == nil` block; emits on `obj`.
- **SourceDeleted** — after successful `r.Delete(ctx, rjob)` in the cancel loop; emits on
  `rjob` (not `obj`, which is unavailable at IsNotFound time); replaces the prior
  incorrect `RemediationJobCancelled`/Warning call.

All calls nil-guarded with `if r.EventRecorder != nil`.

### 2. STORY_02 — RemediationJobReconciler event calls (internal/controller/remediationjob_controller.go)

- Added `Recorder record.EventRecorder` field to `RemediationJobReconciler` struct.
- Added `corev1` and `record` imports.
- Added `Recorder: mgr.GetEventRecorderFor("mendabot-watcher")` in `cmd/watcher/main.go`.
- **JobDispatched** — in `dispatch()` after status patch succeeds; also added to the
  `IsAlreadyExists` branch (GAP 8 fix); emits on `rjob`.
- **JobSucceeded** — when `newPhase == PhaseSucceeded`; includes PR URL if `rjob.Status.PRRef != ""`.
- **JobFailed** — when `newPhase == PhaseFailed && rjob.Status.Phase != PermanentlyFailed`; Warning type; includes `job.Status.Failed` attempt count.
- **JobPermanentlyFailed** — when `newPhase == PhaseFailed && rjob.Status.Phase == PermanentlyFailed`; Warning type; includes `rjob.Status.RetryCount`.

All calls nil-guarded with `if r.Recorder != nil`.

### 3. Code review + gap remediation

11 gaps identified by skeptical review. 6 fixed in scope:

| Gap | Fix |
|-----|-----|
| GAP 1 | Removed redundant `TestReconcile_EventRecorder_EmitsRemediationJobCreated` (duplicate of FindingDetected test) |
| GAP 5 | PermanentlyFailed path now emits `JobPermanentlyFailed` instead of `JobFailed` |
| GAP 6 | Added `TestReconcile_EmitsEvent_JobPermanentlyFailed` |
| GAP 8 | `JobDispatched` event added to AlreadyExists dispatch path |
| GAP 9 | `TestReconcile_EmitsEvent_JobFailed` now asserts Warning type |
| GAP 10 | `TestReconcile_EmitsEvent_JobDispatched` now asserts Normal type |

5 gaps deferred / accepted:
- GAP 2: DuplicateFingerprint on PhaseSucceeded — pre-existing dedup logic, not introduced here
- GAP 3: Blank-phase dedup test variant — minor, follow-up story
- GAP 4: FindingCleared on every poll — spec-compliant; story does not require transition guard
- GAP 7: Fragile PRRef test comment — minor cosmetic, out of scope
- GAP 11: Integration test for SourceDeleted round-trip — no event broadcaster in envtest; follow-up

### 4. Tests added

`internal/provider/provider_test.go`:
- `TestReconcile_EmitsEvent_FindingDetected`
- `TestReconcile_EmitsEvent_DuplicateFingerprint`
- `TestReconcile_EmitsEvent_FindingCleared`
- `TestReconcile_EmitsEvent_SourceDeleted`
- `TestReconcile_NilRecorder_NoPanic`

`internal/controller/remediationjob_controller_test.go`:
- `TestReconcile_EmitsEvent_JobDispatched`
- `TestReconcile_EmitsEvent_JobSucceeded_WithPR`
- `TestReconcile_EmitsEvent_JobSucceeded_NoPR`
- `TestReconcile_EmitsEvent_JobFailed`
- `TestReconcile_EmitsEvent_JobPermanentlyFailed`
- `TestReconcile_NilRecorder_NoPanic`

---

## Key Decisions

1. **PermanentlyFailed emits `JobPermanentlyFailed` not `JobFailed`** — the `newPhase`
   variable holds the raw `syncPhaseFromJob` result (always `PhaseFailed`), but
   `rjob.Status.Phase` may have been mutated to `PhasePermanentlyFailed` by the retry-cap
   logic. The switch checks `rjob.Status.Phase` to distinguish the two cases and emits a
   distinct `Warning JobPermanentlyFailed` event. This makes the terminal-no-retry state
   visible in `kubectl describe`.

2. **`JobDispatched` emitted in both dispatch paths** — both the happy path (new Job
   created) and the AlreadyExists path (existing Job found on restart) emit `JobDispatched`.
   This ensures the event always appears regardless of watcher restart timing.

3. **`SourceDeleted` emits on `rjob`** — at IsNotFound time, `obj` is unavailable as an
   event target. Emitting on `rjob` is correct and matches the story spec.

---

## Blockers

None.

---

## Tests Run

```
go build ./...                         → clean
go test -timeout 60s -race ./...       → 12/12 packages pass
```

---

## Next Steps

- Epic 21 is complete. No immediate follow-up required.
- Follow-up stories to consider: GAP 2 (DuplicateFingerprint semantics on PhaseSucceeded),
  GAP 11 (integration test for SourceDeleted via event broadcaster).
- Next epic to implement per backlog: check docs/BACKLOG/ for highest-priority unstarted epic.

---

## Files Modified

- `internal/provider/provider.go` — 4 event call sites updated/added
- `internal/provider/provider_test.go` — 5 new tests; 1 redundant test removed
- `internal/controller/remediationjob_controller.go` — Recorder field, imports, 4 event calls
- `internal/controller/remediationjob_controller_test.go` — 6 new tests; type assertions strengthened
- `cmd/watcher/main.go` — `Recorder: mgr.GetEventRecorderFor("mendabot-watcher")` added
- `docs/BACKLOG/epic21-kubernetes-events/STORY_01_source_provider_events.md` — status → Complete
- `docs/BACKLOG/epic21-kubernetes-events/STORY_02_controller_events.md` — status → Complete
- `docs/BACKLOG/epic21-kubernetes-events/README.md` — status → Complete
- `docs/WORKLOGS/README.md` — index updated
- `README-LLM.md` — branch table updated
