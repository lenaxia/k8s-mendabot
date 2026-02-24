# Epic 16: Resource Annotation Control

**Feature Tracker:** FT-A2
**Area:** Accuracy & Precision

## Purpose

Allow operators to annotate individual Kubernetes resources to suppress mendabot
investigations permanently (`mendabot.io/enabled: "false"`), for a time window
(`mendabot.io/skip-until: "YYYY-MM-DD"`), or to bypass the stabilisation window
for critical resources (`mendabot.io/priority: "critical"`).

Without this, operators have no per-resource escape hatch. A canary pod that crashes
by design, a load-test Job, or a resource under active manual investigation will
continuously trigger new `RemediationJob` objects that have to be manually deleted.

## Status: Not Started

## Deep-Dive Findings (2026-02-23)

### Domain Constants (STORY_01)
- New file: `internal/domain/annotations.go` — three typed constants and one helper.
- `ShouldSkip(annotations map[string]string, now time.Time) bool` takes `now` as a
  parameter (not `time.Now()`) so tests can use a fixed clock without monkey-patching.
- `skip-until` boundary: `t.UTC().AddDate(0, 0, 1)` — the window expires at midnight UTC
  on the day *after* the annotated date (inclusive end date, unambiguous semantics).
- `AnnotationPriority` is **read by the reconciler** (STORY_03), not by `ExtractFinding`.
- No external imports beyond `"time"` — pure domain logic, zero circular dependency risk.
- Test file: `internal/domain/annotations_test.go`, 7 cases including two boundary tests
  (skip on the date itself, no skip the day after).

### Provider Gate (STORY_02)
- `obj.GetAnnotations()` is available on the `client.Object` interface **before any type
  assertion** — cheapest possible guard, avoids all reflection when resource is suppressed.
- Insertion point in all 6 providers: **immediately before** the concrete type assertion.
- Exact files: `pod.go`, `deployment.go`, `statefulset.go`, `job.go`, `node.go`, `pvc.go`
  (lines 49, 39, 39, 42, 53, 39 respectively).
- Each file needs `"time"` added to its import block.
- Node is cluster-scoped but still has `ObjectMeta.Annotations` — the guard applies.

### Priority Bypass (STORY_03)
- Stabilisation window block at `internal/provider/provider.go` lines 167–181.
- `obj` (the fetched Kubernetes object) is available via `r.Get` at line 64 — no extra
  API call needed for `obj.GetAnnotations()`.
- The preferred formulation:
  ```go
  priorityCritical := obj.GetAnnotations()[domain.AnnotationPriority] == "critical"
  if !priorityCritical && r.Cfg.StabilisationWindow != 0 { ... }
  ```
- Key invariant: when `mendabot.io/priority: "critical"`, `firstSeen` map is **never
  consulted** and no requeue is issued.
- `firstSeen.Clear()` calls on the not-found/nil-finding paths are unchanged.
- `domain.AnnotationPriority` constant must be used — no bare string literals.

## Dependencies

- epic09-native-provider complete (all six providers in `internal/provider/native/`)
- epic15-namespace-filtering complete or in progress (annotation constants share `internal/domain/`)

## Blocks

- epic23 (new annotation-suppression paths require audit log coverage)

## Stories

| Story | File | Status |
|-------|------|--------|
| Domain — annotation constants and skip logic | [STORY_01_annotation_constants.md](STORY_01_annotation_constants.md) | Not Started |
| Providers — ExtractFinding annotation gate | [STORY_02_provider_gate.md](STORY_02_provider_gate.md) | Not Started |
| Reconciler — priority annotation bypasses stabilisation window | [STORY_03_priority_bypass.md](STORY_03_priority_bypass.md) | Not Started |

## Implementation Order

```
STORY_01 (domain constants) ──> STORY_02 (providers)
                            ──> STORY_03 (reconciler)
```

STORY_02 and STORY_03 are independent once STORY_01 is complete.

## Definition of Done

- [ ] Annotation keys defined as typed constants in `internal/domain/annotations.go`
- [ ] `ShouldSkip` helper implemented with correct `skip-until` boundary semantics (inclusive, midnight UTC)
- [ ] `ExtractFinding` in each provider returns `(nil, nil)` when `mendabot.io/enabled: "false"` is set
- [ ] `ExtractFinding` in each provider returns `(nil, nil)` when `mendabot.io/skip-until` is set to a future date
- [ ] `SourceProviderReconciler` bypasses stabilisation window when `mendabot.io/priority: "critical"` is set
- [ ] `firstSeen` map is never consulted for priority-critical resources
- [ ] All unit tests pass with `-race`
- [ ] Worklog written
