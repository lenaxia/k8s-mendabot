# Worklog: Epic 17 — Dead-Letter Queue Complete

**Date:** 2026-02-23
**Session:** Full epic17 implementation via multi-agent orchestration (all 5 stories)
**Status:** Complete

---

## Objective

Implement epic17 (FT-R1: Dead-Letter Queue / Retry Cap) in full — bounding the retry loop on
broken `RemediationJob` objects so that a failed deployment never burns unlimited LLM quota.

---

## Work Completed

### 1. STORY_01 — CRD Types

- Added `PhasePermanentlyFailed RemediationJobPhase = "PermanentlyFailed"` constant
- Added `ConditionPermanentlyFailed = "PermanentlyFailed"` constant
- Added `MaxRetries int32` to `RemediationJobSpec` with `// +kubebuilder:default=3` and `// +kubebuilder:validation:Minimum=1`
- Updated `+kubebuilder:validation:Enum` on `Status.Phase` to include `PermanentlyFailed`
- Added `RetryCount int32` to `RemediationJobStatus`
- Added `out.Status.RetryCount = in.Status.RetryCount` to `DeepCopyInto`
- Created `api/v1alpha1/remediationjob_types_test.go` with 6 test functions (constant pinning, DeepCopy, zero-value)
- Updated `testdata/crds/remediationjob_crd.yaml` with `maxRetries` (minimum:1, default:3), `retryCount` (minimum:0), `PermanentlyFailed` in enum

### 2. STORY_02 — Config

- Added `MaxInvestigationRetries int32` to `Config` struct
- Added `MAX_INVESTIGATION_RETRIES` parsing block in `FromEnv` (default 3, error on ≤0 or non-integer)
- Added table-driven tests in `config_test.go` covering 8 cases
- Removed a hand-rolled `contains` helper and replaced all call sites with `strings.Contains`

### 3. STORY_03 — RemediationJobReconciler RetryCount

- Added `PhasePermanentlyFailed` case to the terminal-phase switch (immediate no-op return)
- Added idempotency-guarded `RetryCount++` in the `PhaseFailed` branch (compares `rjobCopy.Status.Phase` to avoid double-increment on re-reconcile)
- Added cap check: when `RetryCount >= MaxRetries`, phase set to `PhasePermanentlyFailed` and `ConditionPermanentlyFailed` condition set to True
- Fallback: when `Spec.MaxRetries <= 0`, effective max defaults to 3
- Extended audit log block with `job.permanently_failed` event including `retryCount` and `effectiveMaxRetries` fields
- 5 new unit tests covering increment, cap, idempotency, terminal no-op, and table of all terminal phases

### 4. STORY_04 — SourceProviderReconciler Gate

- Refactored fingerprint-dedup loop from `if phase != PhaseFailed` to a `switch` on `rjob.Status.Phase`
- `PhasePermanentlyFailed` case: emits `remediationjob.permanently_failed_suppressed` audit log, returns nil without deleting
- `PhaseFailed` case: deletes rjob (unchanged — triggers re-dispatch)
- `default` case: returns nil (dedup — active/completed job exists)
- Added `MaxRetries: r.Cfg.MaxInvestigationRetries` to the `RemediationJob` Spec literal at creation
- 3 new tests: PermanentlyFailed_Suppressed (tombstone survives + audit log), PhaseFailed_DeletesAndCreatesNew, MaxRetries_PopulatedFromConfig

### 5. STORY_05 — CRD Schema Updates

- `testdata/crds/remediationjob_crd.yaml`: added format:int32 and description to maxRetries/retryCount (partially done in STORY_01; completed here)
- `charts/mechanic/crds/remediationjob.yaml`: added maxRetries, retryCount, PermanentlyFailed; preserved isSelfRemediation, chainDepth, correlationGroupID, Suppressed
- `deploy/kustomize/crd-remediationjob.yaml`: same changes (confirmed standalone copy)

---

## Key Decisions

- **Idempotency guard uses `rjobCopy.Status.Phase`**: The story spec stated to use `rjob.Status.Phase` but the mutation `rjob.Status.Phase = newPhase` runs before the check. Using `rjobCopy` (captured pre-mutation) correctly implements the stated intent — pre-mutation phase comparison.

- **`effectiveMaxRetries` variable hoisted**: The audit log for `job.permanently_failed` must log the fallback-adjusted max retries value, not the raw `Spec.MaxRetries`. A local `effectiveMaxRetries int32` variable is declared before the outer `if` block and assigned after the fallback, ensuring consistency between the condition message and the audit log field.

- **STORY_01 proactively updated `testdata/crds/remediationjob_crd.yaml`**: Per README-LLM.md envtest rules, updating the CRD testdata whenever `RemediationJobSpec`/`RemediationJobStatus` fields change is mandatory. STORY_05 only needed to add `format: int32` metadata that was missing.

---

## Blockers

None.

---

## Tests Run

```
go build ./...                            — clean
go test -timeout 60s -race ./...          — 12/12 packages PASS
go vet ./...                              — clean
helm lint charts/mechanic                 — 1 chart(s) linted, 0 failed
```

---

## Next Steps

epic17 is complete. Recommended next epic: **epic23** (structured audit log gaps) — additive log field changes, low risk, good warm-up for epic21 (EventRecorder wiring).

After epic23: epic21 → epic22 → epic18 → epic15 → epic16 → epic20.

---

## Files Modified

- `api/v1alpha1/remediationjob_types.go`
- `api/v1alpha1/remediationjob_types_test.go` (new)
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/controller/remediationjob_controller.go`
- `internal/controller/remediationjob_controller_test.go`
- `internal/provider/provider.go`
- `internal/provider/provider_test.go`
- `testdata/crds/remediationjob_crd.yaml`
- `charts/mechanic/crds/remediationjob.yaml`
- `deploy/kustomize/crd-remediationjob.yaml`
