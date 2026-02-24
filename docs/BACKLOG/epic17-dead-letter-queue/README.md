# Epic 17: Dead-Letter Queue (Retry Cap)

**Feature Tracker:** FT-R1
**Area:** Reliability

## Purpose

Add a `RetryCount` field to `RemediationJobStatus` and a `MaxRetries` field to
`RemediationJobSpec` (default: 3, configurable via `MAX_INVESTIGATION_RETRIES` env var).
When `RetryCount >= MaxRetries`, the `RemediationJob` enters a `PermanentlyFailed` phase
and is never re-dispatched.

Without this, a broken deployment (bad git credentials, invalid prompt, LLM API outage)
creates an infinite retry loop that burns LLM quota and fills the namespace with failed
Jobs. This is the highest reliability risk in the current implementation.

## Status: Complete

## Deep-Dive Findings (2026-02-23)

### CRD Types (STORY_01)
- `api/v1alpha1/remediationjob_types.go`: phase constants at lines 49–69, spec at 84–118,
  status at 152–179, `DeepCopyInto` at 203–226.
- `PhasePermanentlyFailed` must be added after `PhaseCancelled` (line 68).
- `ConditionPermanentlyFailed` constant added after `ConditionJobFailed` (line 80).
- `MaxRetries int32` added to `RemediationJobSpec` after `AgentSA` (line 117) with
  kubebuilder markers `+kubebuilder:default=3` and `+kubebuilder:validation:Minimum=1`.
- `RetryCount int32` added to `RemediationJobStatus` after `Message` (line 173).
- `+kubebuilder:validation:Enum` marker on `Status.Phase` must include `PermanentlyFailed`.
- `DeepCopyInto` needs `out.Status.RetryCount = in.Status.RetryCount` added.
  (`MaxRetries` in Spec is covered by `out.Spec = in.Spec` at line 208 — no change.)
- New test file: `api/v1alpha1/remediationjob_types_test.go` (package has no test file
  today).

### Config (STORY_02)
- New field `MaxInvestigationRetries int32` in `internal/config/config.go`.
- Parsing pattern mirrors `MaxConcurrentJobs` block (lines 66–78): `strconv.Atoi`,
  validate `> 0`, cast to `int32`. Default: `3`.
- Consumed in STORY_04: `rjob.Spec.MaxRetries = r.Cfg.MaxInvestigationRetries`.

### RemediationJobReconciler — RetryCount (STORY_03)
- Job failure detected at `internal/controller/remediationjob_controller.go` lines 100–142
  via `syncPhaseFromJob` (lines 167–182).
- **Idempotency guard:** only increment `RetryCount` when transitioning *into* `PhaseFailed`
  for the first time. Check: `if rjob.Status.Phase != v1alpha1.PhaseFailed { rjob.Status.RetryCount++ }`.
  (Uses the pre-patch phase because `rjobCopy` was captured before mutation.)
- When `RetryCount >= MaxRetries`: set `Phase = PermanentlyFailed`, set
  `ConditionPermanentlyFailed = True`, emit audit log `event=job.permanently_failed`.
- `PhasePermanentlyFailed` must be added to the terminal-phase switch at lines 85–90.
- Fallback when `MaxRetries <= 0`: use `3` (prevents panic if field not populated).

### SourceProviderReconciler — Gate (STORY_04)
- Fingerprint-dedup loop at `internal/provider/provider.go` lines 190–203 refactored
  to a `switch` on `rjob.Status.Phase`.
- `PhasePermanentlyFailed` case: do **not** delete the rjob; return immediately with
  audit log `event=remediationjob.permanently_failed_suppressed`.
- `PhaseFailed` case: delete and re-dispatch (existing behaviour unchanged).
- `default` case: any other phase (Pending, Running, Succeeded, Cancelled) → return,
  dedup suppresses creation.
- `MaxRetries: r.Cfg.MaxInvestigationRetries` added to the `RemediationJob` Spec literal
  at object creation (~line 257).

### CRD Schema Updates (STORY_05)
- Two files to update: `testdata/crds/remediationjob_crd.yaml` (98 lines today) and
  `charts/mendabot/crds/remediationjob.yaml` (105 lines today, has extra fields
  `isSelfRemediation`, `chainDepth`, `correlationGroupID`, `Suppressed` phase — preserve).
- `deploy/kustomize/crd-remediationjob.yaml` must be read first to determine if it is a
  standalone copy or a reference.
- Changes: extend `status.phase` enum with `PermanentlyFailed`; add `spec.maxRetries`
  (int32, minimum 1, default 3); add `status.retryCount` (int32, minimum 0).

## Dependencies

- epic00.1-interfaces complete (`api/v1alpha1/remediationjob_types.go`)
- epic01-controller complete (`internal/controller/remediationjob_controller.go`)
- epic09-native-provider complete (`internal/provider/provider.go`)

## Blocks

- epic23 (dispatch audit log gap in controller; permanently_failed event needed)

## Stories

| Story | File | Status |
|-------|------|--------|
| CRD types — RetryCount, MaxRetries, PermanentlyFailed phase | [STORY_01_crd_types.md](STORY_01_crd_types.md) | Complete |
| Config — MAX_INVESTIGATION_RETRIES env var | [STORY_02_config.md](STORY_02_config.md) | Complete |
| RemediationJobReconciler — increment RetryCount on Job failure | [STORY_03_reconciler_retry_count.md](STORY_03_reconciler_retry_count.md) | Complete |
| SourceProviderReconciler — respect PermanentlyFailed; no re-dispatch | [STORY_04_source_provider_gate.md](STORY_04_source_provider_gate.md) | Complete |
| CRD schema — testdata and Helm chart updates | [STORY_05_schema_updates.md](STORY_05_schema_updates.md) | Complete |

## Implementation Order

```
STORY_01 (CRD types) ──> STORY_02 (config)
                     ──> STORY_03 (controller retry count)
                     ──> STORY_04 (source provider gate)  [needs STORY_02 + STORY_03]
                     ──> STORY_05 (schema updates)
```

STORY_02 and STORY_03 are independent once STORY_01 is complete.
STORY_04 depends on STORY_01, STORY_02, and STORY_03.

## Definition of Done

- [ ] `RemediationJobStatus` has `RetryCount int32` field
- [ ] `RemediationJobSpec` has `MaxRetries int32` field (default 3, populated by SourceProviderReconciler from config)
- [ ] `RemediationJobPhase` gains `PhasePermanentlyFailed` constant
- [ ] `RemediationJobReconciler` increments `RetryCount` idempotently (only on first transition to PhaseFailed)
- [ ] When `RetryCount >= MaxRetries`, `RemediationJobReconciler` sets phase to `PermanentlyFailed`
- [ ] `SourceProviderReconciler` skips `PermanentlyFailed` objects (does not delete and re-create)
- [ ] Audit log emits `job.permanently_failed` and `remediationjob.permanently_failed_suppressed`
- [ ] `config.Config` gains `MaxInvestigationRetries int32`; `FromEnv` parses `MAX_INVESTIGATION_RETRIES` with default 3
- [ ] `testdata/crds/remediationjob_crd.yaml` updated with new fields
- [ ] `charts/mendabot/crds/remediationjob.yaml` updated with new fields
- [ ] All unit and integration tests pass with `-race`
- [ ] Worklog written
