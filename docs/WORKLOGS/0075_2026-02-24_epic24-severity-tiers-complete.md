# Worklog: Epic 24 — Severity Tiers on Findings

**Date:** 2026-02-24
**Session:** Implement epic24-severity-tiers: Severity type, CRD field, provider classification, MIN_SEVERITY filter, JobBuilder injection, prompt calibration
**Status:** Complete

---

## Objective

Implement all 6 stories of epic24-severity-tiers: add a `Severity` domain type, classify findings by severity in all 6 native providers, filter findings by `MIN_SEVERITY` env var, propagate severity through `RemediationJobSpec` and `JobBuilder`, and add a severity calibration block to the agent prompt.

---

## Work Completed

### 1. STORY_01 — Domain severity type (internal/domain/)

- Created `internal/domain/severity.go`: `Severity` named type, 4 constants (`critical`, `high`, `medium`, `low`), `severityOrder` map, `SeverityLevel()`, `MeetsSeverityThreshold()` (with pass-all semantics for `SeverityLow` default), `ParseSeverity()`
- Created `internal/domain/severity_test.go`: 19 table-driven tests covering all 4 constants, all boundary combinations, and all 4 valid parse inputs plus empty and unknown
- Added `Severity Severity` field to `domain.Finding` in `internal/domain/provider.go` (named field, safe zero-value addition)
- Review gap fixed: `TestParseSeverity` was missing test cases for `"high"`, `"medium"`, `"low"` — added all three

### 2. STORY_02 — CRD severity field (api/, testdata/, charts/)

- Added `Severity string \`json:"severity,omitempty"\`` to `RemediationJobSpec` in `api/v1alpha1/remediationjob_types.go`
- Added `severity: {type: string}` (no enum) to `testdata/crds/remediationjob_crd.yaml` under spec properties
- Added same entry to `charts/mendabot/crds/remediationjob.yaml`
- No `DeepCopyInto` changes needed (all value types)

### 3. STORY_03 — Provider severity assignment (internal/provider/native/)

Updated all 6 providers with local severity compute helpers and test coverage:

- `pod.go`: `computePodSeverity()` — critical (CrashLoopBackOff >5), high (CrashLoopBackOff ≤5, OOMKilled, ImagePullBackOff, Unschedulable), medium (default). Safe `make`+`append` pattern for container iteration.
- `deployment.go`: `computeDeploymentSeverity()` — critical (0 ready), high (<50% ready), medium (some missing or Available=False)
- `statefulset.go`: `computeStatefulSetSeverity()` — mirrors deployment; replica check gated by `generation==observedGeneration`
- `node.go`: `computeNodeSeverity()` — critical (NotReady=False/Unknown), high (all pressure conditions)
- `job.go`: `Severity: domain.SeverityMedium` set directly
- `pvc.go`: `Severity: domain.SeverityHigh` set directly

Review gaps fixed: unsafe `append` at 2 sites in pod.go; dead `availableFalseFired` parameter removed from deployment.go and statefulset.go; 5 missing severity assertions added across deployment_test.go, statefulset_test.go, node_test.go

### 4. STORY_04 — Config + reconciler filter (internal/config/, internal/provider/)

- Added `MinSeverity domain.Severity` to `config.Config`
- Added `MIN_SEVERITY` parsing in `FromEnv()` using `domain.ParseSeverity`; returns error for invalid values; defaults to `domain.SeverityLow`
- Inserted severity filter in `SourceProviderReconciler.Reconcile` at step 6.5 (after namespace annotation gate, before fingerprint) using `domain.MeetsSeverityThreshold`; suppressed findings logged at Info with `audit=true, event=finding.suppressed.min_severity`
- Added `Severity: string(finding.Severity)` to `RemediationJobSpec` creation block

Review gaps fixed: `newTestReconciler` now explicitly sets `MinSeverity: domain.SeverityLow`; added `TestSeverityFilter_EmptySeverity_PassesDefaultLow` for legacy empty-severity findings

### 5. STORY_05 — JobBuilder env injection (internal/jobbuilder/, docker/scripts/)

- Added `{Name: "FINDING_SEVERITY", Value: rjob.Spec.Severity}` to main container env in `internal/jobbuilder/job.go`
- Added `"FINDING_SEVERITY"` to required env list in `TestBuild_EnvVars_AllPresent`
- Added `TestBuild_FindingSeverity_ValueInjected` (sets Severity="critical", asserts FINDING_SEVERITY=critical)
- Added `TestBuild_FindingSeverity_EmptyStringLegacy` (empty Severity, asserts env var present with empty value)
- Added `${FINDING_SEVERITY}` to `VARS` string in `docker/scripts/entrypoint-common.sh`

### 6. STORY_06 — Prompt calibration (charts/mendabot/files/prompts/core.txt)

- Added `Severity:     ${FINDING_SEVERITY}` after Fingerprint in the finding context block
- Added `=== SEVERITY CALIBRATION ===` block before investigation steps, covering critical/high/medium/low/empty

---

## Key Decisions

- `MeetsSeverityThreshold("", SeverityLow)` returns `true` (pass-all semantics for default threshold) — ensures existing providers that have not yet set Severity are not silently dropped under default config
- `availableFalseFired` parameter removed from `computeDeploymentSeverity`/`computeStatefulSetSeverity` — was dead code; Available=False always maps to medium (which is the default return value), so the max computation was a no-op
- No `enum` constraint on the CRD severity field — omitempty means empty strings are never sent, and explicit enum would reject empty strings in tests
- Defensive `MinSeverity` nil-guard kept in provider.go for test constructors that bypass `FromEnv()` — mitigated by explicitly setting `MinSeverity: domain.SeverityLow` in `newTestReconciler`

---

## Blockers

None.

---

## Tests Run

```
go build ./...
# Clean

go test -timeout 120s -race ./...
# All 13 packages PASS:
# ok  github.com/lenaxia/k8s-mendabot/api/v1alpha1
# ok  github.com/lenaxia/k8s-mendabot/cmd/watcher
# ok  github.com/lenaxia/k8s-mendabot/internal/config
# ok  github.com/lenaxia/k8s-mendabot/internal/controller
# ok  github.com/lenaxia/k8s-mendabot/internal/domain
# ok  github.com/lenaxia/k8s-mendabot/internal/jobbuilder
# ok  github.com/lenaxia/k8s-mendabot/internal/logging
# ok  github.com/lenaxia/k8s-mendabot/internal/provider
# ok  github.com/lenaxia/k8s-mendabot/internal/provider/native
# ok  github.com/lenaxia/k8s-mendabot/internal/readiness
# ok  github.com/lenaxia/k8s-mendabot/internal/readiness/llm
# ok  github.com/lenaxia/k8s-mendabot/internal/readiness/sink

helm lint charts/mendabot/
# 0 charts failed
```

---

## Next Steps

Epic 24 is complete. Next epic candidates:
- epic22-token-expiry-guard (GitHub App token expiry guard in agent entrypoint)
- epic13-multi-signal-correlation (deferred in feature/epic11-13-deferred; severity is now a prerequisite — UNBLOCK epic13)
- epic23-structured-audit-log now has severity available via `RemediationJobSpec.Severity` (audit log entries can include it)

---

## Files Modified

- `internal/domain/severity.go` — new file
- `internal/domain/severity_test.go` — new file
- `internal/domain/provider.go` — Severity field added to Finding
- `api/v1alpha1/remediationjob_types.go` — Severity string field added to RemediationJobSpec
- `testdata/crds/remediationjob_crd.yaml` — severity: {type: string} added
- `charts/mendabot/crds/remediationjob.yaml` — severity: {type: string} added
- `internal/provider/native/pod.go` — computePodSeverity, safe append pattern
- `internal/provider/native/pod_test.go` — severity assertions + new severity tests
- `internal/provider/native/deployment.go` — computeDeploymentSeverity, removed dead parameter
- `internal/provider/native/deployment_test.go` — severity assertions
- `internal/provider/native/statefulset.go` — computeStatefulSetSeverity, removed dead parameter
- `internal/provider/native/statefulset_test.go` — severity assertions
- `internal/provider/native/node.go` — computeNodeSeverity
- `internal/provider/native/node_test.go` — severity assertions for 4 tests
- `internal/provider/native/job.go` — Severity: SeverityMedium
- `internal/provider/native/job_test.go` — severity assertion
- `internal/provider/native/pvc.go` — Severity: SeverityHigh
- `internal/provider/native/pvc_test.go` — severity assertion
- `internal/config/config.go` — MinSeverity field + MIN_SEVERITY parsing
- `internal/config/config_test.go` — TestFromEnv_MinSeverity (6 cases)
- `internal/provider/provider.go` — severity filter + Severity population on RemediationJob
- `internal/provider/provider_test.go` — 5 new severity filter tests; newTestReconciler updated
- `internal/jobbuilder/job.go` — FINDING_SEVERITY env var added
- `internal/jobbuilder/job_test.go` — 2 new severity tests; FINDING_SEVERITY in required list
- `docker/scripts/entrypoint-common.sh` — ${FINDING_SEVERITY} added to VARS
- `charts/mendabot/files/prompts/core.txt` — Severity line + calibration block
- `docs/BACKLOG/epic24-severity-tiers/README.md` — status updated to Complete
- `docs/BACKLOG/epic24-severity-tiers/STORY_01_severity_domain.md` — status updated
- `docs/BACKLOG/epic24-severity-tiers/STORY_02_crd_severity_field.md` — status updated
- `docs/BACKLOG/epic24-severity-tiers/STORY_03_provider_severity.md` — status updated
- `docs/BACKLOG/epic24-severity-tiers/STORY_04_min_severity_filter.md` — status updated
- `docs/BACKLOG/epic24-severity-tiers/STORY_05_jobbuilder_severity.md` — status updated
- `docs/BACKLOG/epic24-severity-tiers/STORY_06_prompt_severity.md` — status updated
- `docs/WORKLOGS/0075_2026-02-24_epic24-severity-tiers-complete.md` — this file
