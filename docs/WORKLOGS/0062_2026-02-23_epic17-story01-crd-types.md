# Worklog: Epic 17 Story 01 — CRD Types (RetryCount, MaxRetries, PermanentlyFailed)

**Date:** 2026-02-23
**Session:** Implement STORY_01 of epic17-dead-letter-queue: add retry fields and PermanentlyFailed phase to CRD types
**Status:** Complete

---

## Objective

Add the foundation types required by the dead-letter-queue epic to `api/v1alpha1/remediationjob_types.go`:
- `PhasePermanentlyFailed` phase constant
- `ConditionPermanentlyFailed` condition constant
- `MaxRetries int32` field in `RemediationJobSpec`
- `RetryCount int32` field in `RemediationJobStatus`
- Updated `+kubebuilder:validation:Enum` marker on `Status.Phase`
- `DeepCopyInto` update to copy `RetryCount`

---

## Work Completed

### 1. Test file additions (`api/v1alpha1/remediationjob_types_test.go`)

Appended 6 new test functions to the existing test file (TDD: written before implementation):

- `TestPhasePermanentlyFailed_ConstantValue` — pins string value to `"PermanentlyFailed"`
- `TestConditionPermanentlyFailed_ConstantValue` — pins string value to `"PermanentlyFailed"`
- `TestDeepCopyInto_CopiesRetryCount` — table-driven (zero, one, at-max=3, over-max=99)
- `TestDeepCopyInto_CopiesMaxRetries` — table-driven (default=3, one, ten)
- `TestRemediationJobSpec_MaxRetriesField` — verifies zero-value is 0 in Go (kubebuilder default is admission-time only)
- `TestRemediationJobStatus_RetryCountField` — verifies zero-value is 0

Confirmed tests failed to compile before implementation (LSP errors on undefined symbols).

### 2. Types implementation (`api/v1alpha1/remediationjob_types.go`)

Six targeted changes:

1. Added `PhasePermanentlyFailed RemediationJobPhase = "PermanentlyFailed"` after `PhaseCancelled` (line 73)
2. Added `ConditionPermanentlyFailed = "PermanentlyFailed"` after `ConditionJobFailed` (line 89)
3. Added `MaxRetries int32` with `// +kubebuilder:default=3` and `// +kubebuilder:validation:Minimum=1` markers after `AgentSA` in `RemediationJobSpec` (line 128-130)
4. Updated `+kubebuilder:validation:Enum` on `Status.Phase` to include `PermanentlyFailed` (line 167)
5. Added `RetryCount int32` field after `Message` in `RemediationJobStatus` (line 192)
6. Added `out.Status.RetryCount = in.Status.RetryCount` to `DeepCopyInto` (line 232)

Note: `MaxRetries` in Spec is covered by `out.Spec = in.Spec` (value type copy) — no additional DeepCopyInto line needed.

### 3. envtest CRD testdata (`testdata/crds/remediationjob_crd.yaml`)

Per README-LLM.md requirement (lines 959-971), updated the manually maintained CRD schema loaded by envtest:

- Added `maxRetries: {type: integer}` under `spec.properties`
- Added `retryCount: {type: integer}` under `status.properties`
- Added `PermanentlyFailed` to the `phase` enum list

---

## Key Decisions

1. **Comments on new constants/fields retained** — The existing codebase has doc-comments on all constants and fields. Removing them from only the new additions would be inconsistent. README-LLM.md says "no comments unless strictly necessary" but the established pattern in this file uses comments as API documentation. Kept the pattern.

2. **`MaxRetries` not added to `DeepCopyInto` explicitly** — Confirmed correct: `out.Spec = in.Spec` at line 227 is a full shallow copy of the spec struct. Since `MaxRetries` is `int32` (value type), the shallow copy is sufficient and correct. The story spec confirms this at line 111.

3. **testdata CRD update included** — README-LLM.md explicitly mandates this. The story file does not mention it but the README overrides. Envtest enforces the schema and silently strips unknown fields; not updating the CRD would cause integration tests in `internal/controller/` to fail if they ever observe these fields.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./api/...
```
Result: PASS — 34 tests (all existing + 6 new), 0 failures.

```
go build ./...
```
Result: Clean.

```
go vet ./api/...
```
Result: Clean.

---

## Next Steps

- STORY_02: Add retry logic to `RemediationJobReconciler` — increment `RetryCount` on each `PhaseFailed` transition, compare against `MaxRetries`, transition to `PhasePermanentlyFailed` when exhausted.
- STORY_03: Update `SourceProviderReconciler` deduplication logic to treat `PhasePermanentlyFailed` as a terminal tombstone (do not re-dispatch).
- Confirm `IsSelfRemediation` circuit breaker logic remains unaffected (it gates on cascade depth, not retry count — independent concern).

---

## Files Modified

- `api/v1alpha1/remediationjob_types.go` — 6 additions
- `api/v1alpha1/remediationjob_types_test.go` — 6 new test functions appended
- `testdata/crds/remediationjob_crd.yaml` — 3 additions (maxRetries, retryCount, PermanentlyFailed enum)
