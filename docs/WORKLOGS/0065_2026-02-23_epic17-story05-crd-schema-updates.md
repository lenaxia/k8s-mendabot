# Worklog: Epic 17 STORY_05 — CRD Schema Updates

**Date:** 2026-02-23
**Session:** Update CRD YAML files with maxRetries, retryCount, and PermanentlyFailed
**Status:** Complete

---

## Objective

Add `maxRetries` (spec), `retryCount` (status), and `PermanentlyFailed` (phase enum) to
all three CRD YAML files so that envtest integration tests and the Helm chart reflect the
fields introduced by stories 01–03.

---

## Work Completed

### 1. Pre-change state assessment

- `testdata/crds/remediationjob_crd.yaml` (103 lines): Already updated by STORY_01 with
  `PermanentlyFailed` in the phase enum, `maxRetries` in spec, and `retryCount` in status.
  However, `maxRetries` was missing `format: int32` and `description`; `retryCount` was
  a bare `{type: integer}` inline missing `format: int32`, `minimum: 0`, and `description`.
- `charts/mechanic/crds/remediationjob.yaml` (105 lines): None of the three changes were
  present. Phase enum had `Suppressed` but not `PermanentlyFailed`. No `maxRetries` or
  `retryCount` fields. Existing fields `isSelfRemediation`, `chainDepth`, `correlationGroupID`,
  and `Suppressed` were preserved.
- `deploy/kustomize/crd-remediationjob.yaml` (105 lines): Standalone copy — identical to
  the chart CRD. Same three changes required.

### 2. Changes applied

**`testdata/crds/remediationjob_crd.yaml`:**
- Added `format: int32` and `description` to existing `maxRetries` field
- Replaced bare `retryCount: {type: integer}` with full expanded form including
  `format: int32`, `minimum: 0`, and `description`

**`charts/mechanic/crds/remediationjob.yaml`:**
- Added `maxRetries` block after `chainDepth` in spec.properties
- Added `PermanentlyFailed` to the phase enum (alongside existing `Suppressed`)
- Added `retryCount` block after `correlationGroupID` in status.properties

**`deploy/kustomize/crd-remediationjob.yaml`:**
- Applied identical changes as the chart CRD (confirmed standalone copy, not a symlink)

### 3. Validation

- `go test -timeout 60s -race ./internal/controller/...` — PASS (envtest loads CRD cleanly)
- `go test -timeout 60s -race ./internal/provider/...` — PASS (all tests green)
- `helm lint charts/mechanic` — 0 chart(s) failed (INFO/WARN about icon and required
  values are expected for a lint run without `--values`)

---

## Key Decisions

- `deploy/kustomize/crd-remediationjob.yaml` is a standalone copy, not a Kustomize
  reference or symlink — it has the same full CRD body as the chart. Therefore it requires
  the same three changes as the chart CRD.
- Confirmed via `git stash` test that pre-existing failures in `./internal/provider/...`
  (two tests: `PermanentlyFailed_Suppressed` and `MaxRetries_PopulatedFromConfig`) are
  caused by story 04 implementation being incomplete — they are NOT caused by this story's
  YAML-only changes.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./internal/controller/...  → ok (8.972s)
go test -timeout 60s -race ./internal/provider/...    → ok (10.308s)
helm lint charts/mechanic                             → 1 chart(s) linted, 0 chart(s) failed
```

---

## Next Steps

Story 04 (provider logic for `PermanentlyFailed` re-dispatch and `MaxRetries` propagation)
needs to be implemented to fix the two pre-existing failing tests in `./internal/provider/...`:
- `TestSourceProviderReconciler_PermanentlyFailed_Suppressed` (provider.go: suppress
  re-dispatch when `Status.Phase == PhasePermanentlyFailed`)
- `TestSourceProviderReconciler_MaxRetries_PopulatedFromConfig` (provider.go: set
  `rjob.Spec.MaxRetries = r.Cfg.MaxInvestigationRetries` when creating a new RemediationJob)

---

## Files Modified

- `testdata/crds/remediationjob_crd.yaml` — added `format: int32`, `description`, and
  `minimum: 0` to existing maxRetries/retryCount fields
- `charts/mechanic/crds/remediationjob.yaml` — added maxRetries, retryCount, PermanentlyFailed
- `deploy/kustomize/crd-remediationjob.yaml` — added maxRetries, retryCount, PermanentlyFailed
