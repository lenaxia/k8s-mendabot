# Worklog: Epic 13 STORY_02 — Correlation Window in RemediationJobReconciler

**Date:** 2026-02-24
**Session:** Implement correlation window hold + Correlator struct + config fields + controller wiring
**Status:** Complete

---

## Objective

Implement STORY_02 of epic13-multi-signal-correlation:
- `internal/correlator/correlator.go` — `Correlator` struct with `Evaluate()` returning `(CorrelationGroup, bool, error)`
- `internal/config/config.go` — three new fields: `CorrelationWindowSeconds`, `DisableCorrelation`, `MultiPodThreshold`
- `internal/controller/remediationjob_controller.go` — correlation window hold, `pendingPeers`, `dispatch`, `transitionSuppressed` helpers, `PhaseSuppressed` terminal case, `Correlator` field
- `cmd/watcher/main.go` — conditional `Correlator` construction via `buildCorrelator()`
- `deploy/kustomize/deployment-watcher.yaml` — three commented-out env var knobs

---

## Work Completed

### 1. TDD test suite written before implementation (all failed initially)

- `internal/correlator/correlator_test.go` — 7 tests:
  - `TestCorrelator_NoRules_NoMatch`
  - `TestCorrelator_FirstRuleMatches`
  - `TestCorrelator_FirstRuleNoMatch_SecondRuleMatches`
  - `TestCorrelator_RuleError_PropagatesError`
  - `TestCorrelator_AllFindings_PopulatedOnMatch`
  - `TestCorrelator_PrimaryUID_FromRule`
  - `TestCorrelator_MultipleRulesNoneMatch`
- `internal/config/config_test.go` — 10 new tests covering all three new fields
- `internal/controller/remediationjob_controller_test.go` — 6 new tests:
  - `TestRemediationJobReconciler_Suppressed_ReturnsNil`
  - `TestCorrelationWindow_HoldsJobDuringWindow`
  - `TestCorrelationWindow_DispatchesAfterWindowElapsed`
  - `TestCorrelationWindow_SecondaryIsSuppressed`
  - `TestCorrelationWindow_PrimaryIsDispatched`
  - `TestCorrelationWindow_NilCorrelator_DispatchesImmediately`

### 2. `internal/correlator/correlator.go` — Correlator struct

- `CorrelationGroup` struct with `GroupID`, `PrimaryUID`, `CorrelatedUIDs`, `Rule`, `AllFindings`
- `Correlator` struct with ordered `Rules []domain.CorrelationRule`
- `Evaluate(ctx, candidate, peers, cl) (CorrelationGroup, bool, error)` — first-match wins, AllFindings populated from candidate + all peers

### 3. `internal/config/config.go` — three new fields

- `CorrelationWindowSeconds int` — default 30, parsed from `CORRELATION_WINDOW_SECONDS`
- `DisableCorrelation bool` — default false, parsed from `DISABLE_CORRELATION`
- `MultiPodThreshold int` — default 3, parsed from `CORRELATION_MULTI_POD_THRESHOLD`

### 4. `internal/controller/remediationjob_controller.go` — full correlation integration

- Added `Correlator *correlator.Correlator` field to `RemediationJobReconciler`
- Added `case v1alpha1.PhaseSuppressed: return ctrl.Result{}, nil` to terminal phase switch
- Inserted window hold block before step 3 (list owned jobs):
  - Returns `RequeueAfter: window - age` while within window
  - After window: calls `pendingPeers`, calls `Correlator.Evaluate`
  - Secondary → `transitionSuppressed`; Primary → label patch + `dispatch` with AllFindings
  - Skipped entirely when `r.Correlator == nil`
- Replaced inline job creation steps 5+6+7 with `r.dispatch(ctx, &rjob, nil)`
- `pendingPeers` helper: lists all Pending jobs in AgentNamespace, excludes candidate
- `dispatch` helper: `Build → Create → status patch to Dispatched` (with AlreadyExists handling)
- `transitionSuppressed` helper: status patch (PhaseSuppressed + CorrelationGroupID + condition) + separate label patch

### 5. `cmd/watcher/main.go` — Correlator wired in

- Added `internal/correlator` and `sigs.k8s.io/controller-runtime/pkg/client` imports
- Added `buildCorrelator(cfg, cl)` function: returns nil when `DisableCorrelation=true`, otherwise creates Correlator with all three built-in rules using `cfg.MultiPodThreshold`
- Passes `Correlator: buildCorrelator(cfg, mgr.GetClient())` to reconciler constructor

### 6. `deploy/kustomize/deployment-watcher.yaml` — operator knobs documented

- Added three commented-out env var entries after `STABILISATION_WINDOW_SECONDS`

---

## Key Decisions

- **Story file as authoritative spec:** STORY_02 specifies `Evaluate` returning `(CorrelationGroup, bool, error)` — this differs slightly from the user prompt context but the story is the authoritative spec per README-LLM.md.
- **`dispatch` helper:** Consolidates all job-creation logic into one function, eliminating duplication between the correlation path and the non-correlation path.
- **`pendingPeers` includes empty-phase jobs:** Empty phase is treated as Pending (new jobs not yet reconciled), matching the existing `Pending` semantics.
- **Correlator nil check as escape hatch:** The reconciler uses `r.Correlator != nil` as the single source of truth, avoiding coupling to the config field name inside the reconciler.

---

## Blockers

None.

---

## Tests Run

```
go clean -testcache && go test -timeout 30s -race ./...
```
All 17 packages passed.

---

## Next Steps

- STORY_03 (jobbuilder multi-finding) was already completed (worklog 0042).
- STORY_04 and STORY_05 (integration tests, end-to-end validation) are the natural next steps.

---

## Files Modified

- `internal/correlator/correlator.go` (created)
- `internal/correlator/correlator_test.go` (created)
- `internal/config/config.go` (updated — 3 new fields + parsing)
- `internal/config/config_test.go` (updated — 10 new tests)
- `internal/controller/remediationjob_controller.go` (updated — Correlator field, window hold, helpers)
- `internal/controller/remediationjob_controller_test.go` (updated — 6 new tests + new imports)
- `cmd/watcher/main.go` (updated — Correlator wiring + buildCorrelator)
- `deploy/kustomize/deployment-watcher.yaml` (updated — 3 commented env var knobs)
- `docs/WORKLOGS/0043_2026-02-24_epic13-story02-correlation-window.md` (created)
- `docs/WORKLOGS/README.md` (updated)
