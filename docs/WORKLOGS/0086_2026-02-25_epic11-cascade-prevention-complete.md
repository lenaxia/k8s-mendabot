# Worklog: Epic 11 — Self-Remediation Cascade Prevention Complete

**Date:** 2026-02-25
**Session:** Full implementation of all 4 epic 11 stories on feature/epic11-self-remediation-cascade
**Status:** Complete

---

## Objective

Implement epic 11 from scratch: allow the watcher to investigate a failing mechanic agent job
up to a configurable depth limit (`SELF_REMEDIATION_MAX_DEPTH`) with a cooldown circuit breaker
(`SELF_REMEDIATION_COOLDOWN_SECONDS`), replacing the unconditional silent guard that previously
blocked all self-remediation.

---

## Work Completed

### 1. Branch setup
- Created `feature/epic11-self-remediation-cascade` from `main`
- Followed README-LLM.md 11-step orchestrator workflow throughout

### 2. STORY_04 — In-memory circuit breaker (`internal/circuitbreaker/`)
- Created `circuitbreaker.go`: `Gater` interface + `CircuitBreaker` struct with `sync.Mutex`
- Constructor `New(cooldown time.Duration) *CircuitBreaker`
- `ShouldAllow()` returns `(true, 0)` on first call or after cooldown; `(false, remaining)` during cooldown
- 5 unit tests: first call, within cooldown, cooldown elapsed, zero cooldown, concurrent with start-gate
- Code review found weak concurrent test (no start-gate); fixed before commit

### 3. STORY_01 — Schema foundations
- Added `ChainDepth int` to `domain.Finding`
- Added `ChainDepth int32` to `api/v1alpha1.FindingSpec` with `json:"chainDepth,omitempty"`
- Added `chainDepth: {type: integer}` to `testdata/crds/remediationjob_crd.yaml` (was pre-added by implementation agent; Go types were missing)
- Mapped `ChainDepth: int32(finding.ChainDepth)` in `SourceProviderReconciler.Reconcile`'s FindingSpec literal
- Added `ChainDepthDoesNotAffectFingerprint` sub-case to fingerprint tests
- Added `TestRemediationJobChainDepthRoundTrip` integration test via envtest
- Code review found 6 gaps (all Go source additions were missing); fixed directly

### 4. STORY_02 — jobProvider depth detection
- Replaced unconditional guard `if managed-by == mechanic-watcher { return nil, nil }` with `isMechanicJob` flag
- Added `getChainDepthFromOwner(ctx, job)` helper: looks up owning `RemediationJob` via owner references, returns `depth+1`; returns `(1, nil)` for 404 or missing owner
- `ChainDepth` populated in `domain.Finding` struct literal; zero for non-mechanic jobs
- Added imports: `apierrors "k8s.io/apimachinery/pkg/api/errors"` and `v1alpha1`
- Deleted `TestJobProvider_ExtractFinding_ExcludesMechanicManagedJobs` (old behaviour test)
- Added 9 new test cases: all depth branches + all skip-condition branches with mechanic label
- Code review found all gaps already present in job.go (the review agent had stale state); confirmed correct

### 5. STORY_03 — Reconciler wiring, config, main.go
- `config.Config` gains `SelfRemediationMaxDepth int` (default 2) and `SelfRemediationCooldown time.Duration` (default 300s) with full env-var parsing
- `SourceProviderReconciler` gains `CircuitBreaker circuitbreaker.Gater` field
- Depth gate block inserted after severity filter, before fingerprint: suppresses when maxDepth==0 or chainDepth>maxDepth; calls CB when chainDepth>0
- `main.go` constructs `*circuitbreaker.CircuitBreaker` (nil when cooldown==0); injects `CircuitBreaker: cb` into all provider reconcilers
- Config tests: 8 new cases (defaults, valid, zero, negative, non-integer for both fields)
- Provider tests: `fakeGater` test double + 8 cases covering all depth/CB gate outcomes
- Code review found 3 gaps: missing non-integer cooldown test, depth-within/at-limit tests didn't assert RJob created; all fixed

---

## Key Decisions

- **In-memory CB only.** The earlier planning session decided this deliberately. The depth limit is the hard safety guarantee; the cooldown is LLM cost protection. A restart resets the cooldown, which is acceptable.
- **No ConfigMap persistence.** The README.md for epic 11 originally mentioned ConfigMap, but the story files all specify in-memory. The story files are authoritative.
- **Gater interface has no ctx or error.** STORY_03 spec has an inconsistency — STORY_04 spec (authoritative) says no error return and no ctx. We followed STORY_04.
- **Implementation agents fabricated success.** Two delegation agents (STORY_01 and STORY_02) returned false "complete" status without making actual code changes. All substantive fixes were applied by the orchestrator directly after code review confirmed the gaps.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./...

ok  github.com/lenaxia/k8s-mechanic/api/v1alpha1
ok  github.com/lenaxia/k8s-mechanic/cmd/redact
ok  github.com/lenaxia/k8s-mechanic/cmd/watcher
ok  github.com/lenaxia/k8s-mechanic/internal/circuitbreaker
ok  github.com/lenaxia/k8s-mechanic/internal/config
ok  github.com/lenaxia/k8s-mechanic/internal/controller
ok  github.com/lenaxia/k8s-mechanic/internal/domain
ok  github.com/lenaxia/k8s-mechanic/internal/jobbuilder
ok  github.com/lenaxia/k8s-mechanic/internal/logging
ok  github.com/lenaxia/k8s-mechanic/internal/provider
ok  github.com/lenaxia/k8s-mechanic/internal/provider/native
ok  github.com/lenaxia/k8s-mechanic/internal/readiness
ok  github.com/lenaxia/k8s-mechanic/internal/readiness/llm
ok  github.com/lenaxia/k8s-mechanic/internal/readiness/sink
```

All 14 packages pass with `-race`.

---

## Next Steps

1. Merge `feature/epic11-self-remediation-cascade` to `main` when ready.
2. Update README-LLM.md to move this branch to the Merged table.
3. The `feature/epic11-13-deferred` branch can be deleted (superseded by this branch).
4. Epic 13 (multi-signal correlation) is still deferred; its branch remains.

---

## Files Modified

| File | Change |
|------|--------|
| `internal/circuitbreaker/circuitbreaker.go` | New: Gater interface + CircuitBreaker implementation |
| `internal/circuitbreaker/circuitbreaker_test.go` | New: 5 unit tests |
| `internal/domain/provider.go` | Add `ChainDepth int` to Finding |
| `internal/domain/provider_test.go` | Add ChainDepthDoesNotAffectFingerprint test |
| `api/v1alpha1/remediationjob_types.go` | Add `ChainDepth int32` to FindingSpec |
| `testdata/crds/remediationjob_crd.yaml` | Add `chainDepth: {type: integer}` to finding.properties |
| `internal/provider/provider.go` | Add CircuitBreaker field; depth gate block; ChainDepth mapping; circuitbreaker import |
| `internal/provider/provider_test.go` | Add fakeGater + 8 depth/CB gate tests; add circuitbreaker and native imports |
| `internal/provider/native/job.go` | Replace unconditional guard with isMechanicJob flag; add getChainDepthFromOwner; set ChainDepth in Finding |
| `internal/provider/native/job_test.go` | Delete old ExcludesMechanicManagedJobs test; add 9 new depth/skip tests |
| `internal/config/config.go` | Add SelfRemediationMaxDepth and SelfRemediationCooldown fields + parsing |
| `internal/config/config_test.go` | Add 9 tests for both new config fields |
| `cmd/watcher/main.go` | Construct CircuitBreaker; inject into all provider reconcilers; add circuitbreaker import |
| `docs/BACKLOG/epic11-self-remediation-cascade/README.md` | Update status to Complete; check all DoD items |
| `docs/BACKLOG/epic11-self-remediation-cascade/STORY_01_schema_foundations.md` | Update status |
| `docs/BACKLOG/epic11-self-remediation-cascade/STORY_02_job_provider_detection.md` | Update status |
| `docs/BACKLOG/epic11-self-remediation-cascade/STORY_03_reconciler_wiring.md` | Update status |
| `docs/BACKLOG/epic11-self-remediation-cascade/STORY_04_circuit_breaker.md` | Update status |
| `README-LLM.md` | Add epic11 branch to active branches table |
