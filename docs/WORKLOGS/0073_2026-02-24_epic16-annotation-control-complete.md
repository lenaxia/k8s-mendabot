# Worklog: Epic 16 — Resource Annotation Control Complete

**Date:** 2026-02-24
**Session:** Orchestrated full epic16 implementation across 3 stories, 2-pass skeptical review, 4-gap remediation
**Status:** Complete

---

## Objective

Implement epic16-annotation-control: allow operators to annotate Kubernetes resources with `mechanic.io/enabled`, `mechanic.io/skip-until`, and `mechanic.io/priority` to suppress or fast-track mechanic investigations.

---

## Work Completed

### 1. Branch

- Created `feature/epic16-annotation-control` from `main`
- Recorded in README-LLM.md branch table

### 2. STORY_01 — Domain annotation constants and skip logic

- New file: `internal/domain/annotations.go`
  - `AnnotationEnabled = "mechanic.io/enabled"`
  - `AnnotationSkipUntil = "mechanic.io/skip-until"`
  - `AnnotationPriority = "mechanic.io/priority"`
  - `ShouldSkip(annotations map[string]string, now time.Time) bool`
- New file: `internal/domain/annotations_test.go` — 8 table-driven cases (7 original + nil-map gap fix)
- TDD: tests written and confirmed failing before implementation

### 3. STORY_02 — Provider annotation gate

- Modified all 6 native providers: `pod.go`, `deployment.go`, `statefulset.go`, `job.go`, `node.go`, `pvc.go`
- `domain.ShouldSkip(obj.GetAnnotations(), time.Now())` added as the first statement in each `ExtractFinding`, before any type assertion
- `"time"` import added to each file
- 2 new tests per provider (12 total) in respective `_test.go` files

### 4. STORY_03 — Priority bypass in reconciler

- Modified `internal/provider/provider.go`
- Replaced `if r.Cfg.StabilisationWindow != 0 { ... }` with `priorityCritical := obj.GetAnnotations()[domain.AnnotationPriority] == "critical"` + `if !priorityCritical && r.Cfg.StabilisationWindow != 0 { ... }`
- 2 new tests in `internal/provider/provider_test.go`: `TestStabilisationWindow_PriorityCriticalBypassesWindow`, `TestStabilisationWindow_PriorityCriticalWindowAlreadyZero`

### 5. Skeptical review — 4 gaps found and fixed

**Gap 1 (Major):** `AnnotationEnabled_False` tests in all 6 providers used healthy objects. Fixed by replacing with unhealthy objects that would produce findings without the annotation.

**Gap 2 (Major):** No end-to-end integration test wiring a real native provider through the reconciler with an annotated object. Fixed by adding `TestAnnotationGate_EnabledFalse_NoRemediationJob` to `internal/provider/provider_test.go` using a real `podProvider` with a CrashLoopBackOff pod annotated `mechanic.io/enabled: "false"`.

**Gap 3 (Minor):** Native provider test files used raw string literals for annotation keys. Fixed by replacing with `domain.AnnotationEnabled` and `domain.AnnotationSkipUntil` constants throughout.

**Gap 4 (Minor):** `annotations_test.go` tested empty map but not nil map. Fixed by adding `NoSkipWhenNilAnnotations` test case.

---

## Key Decisions

- `ShouldSkip` receives `now time.Time` as a parameter rather than calling `time.Now()` internally. This keeps the domain layer deterministically testable without monkey-patching.
- `skip-until` window is inclusive: a resource annotated `2025-06-01` is skipped until `2025-06-02T00:00:00Z` (deadline = `t.UTC().AddDate(0,0,1)`). This is the least surprising interpretation of an inclusive end date.
- Malformed `skip-until` values are silently ignored (no suppression). This prevents a typo from permanently disabling investigations.
- `AnnotationPriority` is read by the reconciler only (STORY_03), not by `ExtractFinding`. The provider gate handles suppression; the reconciler handles acceleration. Orthogonal concerns, cleanly separated.
- The `AnnotationEnabled_False` integration test (Gap 2 fix) uses a real `podProvider`, not the fakeSourceProvider, to prove the annotation gate in the provider code path is actually wired into the reconciler's behaviour.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./...
ok  github.com/lenaxia/k8s-mechanic/api/v1alpha1
ok  github.com/lenaxia/k8s-mechanic/cmd/watcher
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

12/12 packages pass. Race detector clean.

---

## Next Steps

Epic 16 is complete and the branch is ready to merge to `main`. The next epic in the backlog is epic15-namespace-filtering (depends on `internal/domain/` — now a natural neighbour to epic16) or epic17 (dead-letter queue — already complete on `main`). Verify merge order by checking the branch table and FEATURE_TRACKER.md.

---

## Files Modified

| File | Action |
|------|--------|
| `internal/domain/annotations.go` | Created |
| `internal/domain/annotations_test.go` | Created (8 test cases) |
| `internal/provider/native/pod.go` | Modified (ShouldSkip guard) |
| `internal/provider/native/pod_test.go` | Modified (2 annotation tests, Gap 1 fix, Gap 3 fix) |
| `internal/provider/native/deployment.go` | Modified (ShouldSkip guard) |
| `internal/provider/native/deployment_test.go` | Modified (2 annotation tests, Gap 1 fix, Gap 3 fix) |
| `internal/provider/native/statefulset.go` | Modified (ShouldSkip guard) |
| `internal/provider/native/statefulset_test.go` | Modified (2 annotation tests, Gap 1 fix, Gap 3 fix) |
| `internal/provider/native/job.go` | Modified (ShouldSkip guard) |
| `internal/provider/native/job_test.go` | Modified (2 annotation tests, Gap 1 fix, Gap 3 fix) |
| `internal/provider/native/node.go` | Modified (ShouldSkip guard) |
| `internal/provider/native/node_test.go` | Modified (2 annotation tests, Gap 1 fix, Gap 3 fix) |
| `internal/provider/native/pvc.go` | Modified (ShouldSkip guard) |
| `internal/provider/native/pvc_test.go` | Modified (2 annotation tests, Gap 1 fix, Gap 3 fix) |
| `internal/provider/provider.go` | Modified (priority bypass) |
| `internal/provider/provider_test.go` | Modified (2 priority tests + 1 integration e2e test) |
| `docs/BACKLOG/epic16-annotation-control/README.md` | Updated (status: Complete, story table) |
| `docs/BACKLOG/epic16-annotation-control/STORY_01_annotation_constants.md` | Updated (status: Complete) |
| `docs/BACKLOG/epic16-annotation-control/STORY_02_provider_gate.md` | Updated (status: Complete) |
| `docs/BACKLOG/epic16-annotation-control/STORY_03_priority_bypass.md` | Updated (status: Complete) |
| `README-LLM.md` | Updated (branch table) |
| `docs/WORKLOGS/0072_2026-02-24_epic16-story01-annotation-constants.md` | Created (by delegate) |
| `docs/WORKLOGS/0073_2026-02-24_epic16-annotation-control-complete.md` | Created (this file) |
| `docs/WORKLOGS/README.md` | Updated (index table) |
