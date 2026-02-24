# Story: Reconciler — Priority Annotation Bypasses Stabilisation Window

**Epic:** [epic16-annotation-control](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want to annotate a resource with
`mendabot.io/priority: "critical"` so that mendabot bypasses the stabilisation window
for that resource and creates a `RemediationJob` immediately on the first reconcile,
even when `STABILISATION_WINDOW_SECONDS` is configured to a non-zero value.

---

## Background

The stabilisation window (implemented in epic09 STORY_12) is intentionally conservative:
it prevents `RemediationJob` creation until a finding has been continuously present for
the configured duration. This is the right default for most resources. However, for
critical infrastructure — a database primary that has just crashed, a certificate that
has just expired — operators need an escape hatch that bypasses the window and triggers
immediate investigation.

The `mendabot.io/priority: "critical"` annotation provides that escape hatch. It is
read by `SourceProviderReconciler.Reconcile` from the already-fetched `obj` — no extra
API call is required.

---

## Design

### Location in `internal/provider/provider.go`

The stabilisation window check occupies lines 167–181 of the current
`SourceProviderReconciler.Reconcile`:

```go
if r.Cfg.StabilisationWindow != 0 {
    if first, seen := r.firstSeen.Get(fp); !seen {
        r.firstSeen.Set(fp)
        return ctrl.Result{RequeueAfter: r.Cfg.StabilisationWindow}, nil
    } else {
        elapsed := time.Since(first)
        if elapsed < r.Cfg.StabilisationWindow {
            remaining := r.Cfg.StabilisationWindow - elapsed
            return ctrl.Result{RequeueAfter: remaining}, nil
        }
        // Window has elapsed — fall through to dedup + Job creation.
    }
}
```

The `obj` variable (the fetched Kubernetes object) is available immediately above this
block: it was fetched on line 64 via `r.Get(ctx, req.NamespacedName, obj)`. Its
annotations are accessible via `obj.GetAnnotations()` on the `client.Object` interface
— no type assertion required.

### Guard to insert

Insert the following block **immediately before** the `if r.Cfg.StabilisationWindow != 0`
check:

```go
// Priority bypass: if the resource is annotated mendabot.io/priority=critical,
// skip the stabilisation window entirely and proceed directly to dedup + Job creation.
if obj.GetAnnotations()[domain.AnnotationPriority] == "critical" {
    // fall through — do not enter the stabilisation window block below
} else if r.Cfg.StabilisationWindow != 0 {
    if first, seen := r.firstSeen.Get(fp); !seen {
        r.firstSeen.Set(fp)
        return ctrl.Result{RequeueAfter: r.Cfg.StabilisationWindow}, nil
    } else {
        elapsed := time.Since(first)
        if elapsed < r.Cfg.StabilisationWindow {
            remaining := r.Cfg.StabilisationWindow - elapsed
            return ctrl.Result{RequeueAfter: remaining}, nil
        }
        // Window has elapsed — fall through to dedup + Job creation.
    }
}
```

This replaces the existing `if r.Cfg.StabilisationWindow != 0 { ... }` block with the
combined priority-bypass + window logic.

**Design rationale for if/else structure over a separate early-return:**
The existing block is a single `if` statement. Wrapping it in an `else` makes the
relationship explicit: either the priority bypass applies (and we fall through), or
the window logic applies. This is easier to read than an early-return guard that skips
to a label or restructures the flow.

**Alternative accepted formulation** (equivalent, may be preferred for readability):

```go
priorityCritical := obj.GetAnnotations()[domain.AnnotationPriority] == "critical"
if !priorityCritical && r.Cfg.StabilisationWindow != 0 {
    if first, seen := r.firstSeen.Get(fp); !seen {
        r.firstSeen.Set(fp)
        return ctrl.Result{RequeueAfter: r.Cfg.StabilisationWindow}, nil
    } else {
        elapsed := time.Since(first)
        if elapsed < r.Cfg.StabilisationWindow {
            remaining := r.Cfg.StabilisationWindow - elapsed
            return ctrl.Result{RequeueAfter: remaining}, nil
        }
    }
}
```

Either formulation is acceptable. The key invariant is:
> When `obj.GetAnnotations()["mendabot.io/priority"] == "critical"`, the `firstSeen`
> map is **never consulted** and no requeue is issued — the reconcile falls through
> directly to the dedup + `RemediationJob` creation logic.

### No change to `firstSeen` eviction

The `firstSeen.Clear()` calls on the not-found path and the nil-finding path are
**not** changed by this story. A critical-priority object whose finding later clears
will still evict its `firstSeen` entry normally. If the resource subsequently loses
the `critical` annotation and the finding returns, the stabilisation window will apply
again from scratch.

---

## Acceptance Criteria

- [ ] When a resource has annotation `mendabot.io/priority: "critical"` and
  `r.Cfg.StabilisationWindow > 0`:
  - `r.firstSeen` is **not** consulted (no `Get` or `Set`)
  - `ctrl.Result{RequeueAfter: ...}` is **not** returned
  - Execution falls through to the dedup + `RemediationJob` creation logic
- [ ] When the annotation is absent (or has any value other than `"critical"`),
  the existing stabilisation window behaviour is completely unchanged
- [ ] When `r.Cfg.StabilisationWindow == 0`, the existing fast-path behaviour is
  completely unchanged (priority annotation has no additional effect because
  there is no window to bypass)
- [ ] `obj.GetAnnotations()` is called on the `client.Object` value fetched by
  `r.Get` — no separate API call is made
- [ ] `domain.AnnotationPriority` constant (from STORY_01) is used — no bare string
  literal `"mendabot.io/priority"` in the reconciler code
- [ ] New test added to `internal/provider/provider_test.go` (see Test Cases below)

---

## Test Cases

Add to `internal/provider/provider_test.go`.

| Test Name | Setup | Expected |
|---|---|---|
| `PriorityCriticalBypassesWindow` | `StabilisationWindow = 2 * time.Minute`; object has annotation `mendabot.io/priority: "critical"`; `firstSeen` is empty (finding never seen before) | `RemediationJob` created immediately; no requeue; `firstSeen` remains empty |
| `PriorityCriticalWindowAlreadyZero` | `StabilisationWindow = 0`; object has annotation `mendabot.io/priority: "critical"` | `RemediationJob` created immediately (same as without annotation — fast path unchanged) |
| `NoPriorityAnnotationWindowApplies` | `StabilisationWindow = 2 * time.Minute`; no priority annotation; `firstSeen` is empty | `RequeueAfter = 2 * time.Minute`; no `RemediationJob` created (existing behaviour preserved) |

**Test implementation note for `PriorityCriticalBypassesWindow`:**

The fake object returned by the test provider's `ExtractFinding` must be the same `obj`
that carries the annotation, or the test must construct a reconciler whose provider
returns a finding and whose `r.Get` populates an object with the annotation. In practice:
use `envtest` or a `fake.Client` that returns a pre-built object with the annotation in
`ObjectMeta.Annotations`. The existing provider tests use `envtest`; follow the same
pattern.

---

## Tasks

- [ ] Ensure STORY_01 is complete (`domain.AnnotationPriority` constant exists)
- [ ] Write the failing test `PriorityCriticalBypassesWindow` in
  `internal/provider/provider_test.go` (TDD — verify failure before modifying the
  reconciler)
- [ ] Replace the `if r.Cfg.StabilisationWindow != 0 { ... }` block in
  `internal/provider/provider.go` with the combined priority-bypass + window guard
  described above
- [ ] Verify that `NoPriorityAnnotationWindowApplies` (i.e. the existing `WindowNotElapsed`
  test) still passes without modification — if it does not, the refactor broke existing
  behaviour
- [ ] Run `go test -race ./internal/provider/...` — all tests must pass
- [ ] Run `go vet ./internal/provider/...` — must be clean

---

## Dependencies

**Depends on:** STORY_01 (`domain.AnnotationPriority` constant)
**Does not depend on:** STORY_02 (provider gate is independent once STORY_01 is complete)
**Blocks:** Nothing

---

## Definition of Done

- [ ] `SourceProviderReconciler.Reconcile` bypasses the stabilisation window when
  `mendabot.io/priority: "critical"` is annotated on the resource
- [ ] All new and existing reconciler tests pass with `-race`
- [ ] Full test suite `go test -race ./...` passes
- [ ] `go vet ./...` clean
