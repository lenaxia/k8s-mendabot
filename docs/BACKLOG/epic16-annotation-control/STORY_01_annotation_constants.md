# Story: Domain — Annotation Constants and Skip Logic

**Epic:** [epic16-annotation-control](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want mendabot to respect per-resource annotations so that I
can permanently suppress investigations (`mendabot.io/enabled: "false"`), suppress them for
a time window (`mendabot.io/skip-until: "YYYY-MM-DD"`), or mark a resource as critical
(`mendabot.io/priority: "critical"`), without having to modify the controller configuration.

---

## Background

The six native providers all call `ExtractFinding(obj client.Object)` which returns a
`*domain.Finding`. Currently there is no mechanism to suppress individual resources.
Annotation constants and the `ShouldSkip` helper must live in `internal/domain/` so they
can be imported by both the providers (STORY_02) and the reconciler (STORY_03) without
creating a circular dependency.

---

## Design

### New file: `internal/domain/annotations.go`

Define three typed string constants and one helper function. No other logic belongs here.

```go
package domain

import "time"

// Annotation key constants for per-resource mendabot control.
const (
    // AnnotationEnabled suppresses all mendabot investigation for this resource
    // when set to "false". Any other value (including absent) means enabled.
    AnnotationEnabled = "mendabot.io/enabled"

    // AnnotationSkipUntil suppresses mendabot investigation until the given date
    // (inclusive). Value must be formatted as "YYYY-MM-DD" in UTC. A malformed
    // value is silently ignored (treated as absent — no suppression).
    AnnotationSkipUntil = "mendabot.io/skip-until"

    // AnnotationPriority, when set to "critical", causes SourceProviderReconciler
    // to bypass the stabilisation window for this resource. Any other value is
    // ignored. This annotation is read by the reconciler (STORY_03), not by
    // ExtractFinding.
    AnnotationPriority = "mendabot.io/priority"
)
```

### Helper: `ShouldSkip`

```go
// ShouldSkip reports whether ExtractFinding should return (nil, nil) for the
// resource that owns annotations, based on the mendabot.io control annotations.
//
// Rules (evaluated in order):
//  1. If annotations["mendabot.io/enabled"] == "false"  → skip (return true).
//  2. If annotations["mendabot.io/skip-until"] is set:
//       - Parse the value as "2006-01-02" in UTC.
//       - If parsing fails: do NOT skip (treat as absent).
//       - If now is before the end of the skip-until day (i.e. before midnight
//         UTC at the start of the day after skip-until): skip (return true).
//       - Otherwise: do not skip (return false).
//  3. No relevant annotations present → do not skip (return false).
//
// now is passed as a parameter (not time.Now()) so that tests can use a fixed
// clock without monkey-patching.
func ShouldSkip(annotations map[string]string, now time.Time) bool {
    if annotations[AnnotationEnabled] == "false" {
        return true
    }
    if raw, ok := annotations[AnnotationSkipUntil]; ok {
        t, err := time.Parse("2006-01-02", raw)
        if err != nil {
            // Malformed value — silently ignore, do not skip.
            return false
        }
        // Skip while now is before the start of the day *after* skip-until.
        // Example: skip-until=2025-06-01 means skip on 2025-06-01 and
        // resume at 2025-06-02T00:00:00Z.
        deadline := t.UTC().AddDate(0, 0, 1)
        return now.UTC().Before(deadline)
    }
    return false
}
```

**Key implementation note on `skip-until` boundary:**
`t.UTC().AddDate(0, 0, 1)` advances by exactly one calendar day in UTC regardless of
daylight saving time (UTC has none). The resource is skipped as long as
`now.UTC().Before(deadline)`, meaning the skip window expires at `YYYY-MM-DDT00:00:00Z`
on the day *after* the annotated date. This is the least surprising interpretation of an
inclusive end date.

---

## Acceptance Criteria

- [ ] `internal/domain/annotations.go` is a new file in the `domain` package
- [ ] It exports exactly three constants: `AnnotationEnabled`, `AnnotationSkipUntil`,
  `AnnotationPriority`
- [ ] It exports `ShouldSkip(annotations map[string]string, now time.Time) bool`
- [ ] `ShouldSkip` returns `true` when `mendabot.io/enabled` is `"false"`
- [ ] `ShouldSkip` returns `true` when `mendabot.io/skip-until` is a future date
- [ ] `ShouldSkip` returns `false` when `mendabot.io/skip-until` is today-or-past
  (window expired at midnight UTC)
- [ ] `ShouldSkip` returns `false` when `mendabot.io/skip-until` is malformed
- [ ] `ShouldSkip` returns `false` when no relevant annotations are present
- [ ] `internal/domain/annotations_test.go` covers all five cases above

---

## Test Cases

All tests live in `internal/domain/annotations_test.go`.

| Test Name | `annotations` input | `now` | Expected |
|---|---|---|---|
| `SkipWhenDisabled` | `{"mendabot.io/enabled": "false"}` | any | `true` |
| `SkipWhenSkipUntilInFuture` | `{"mendabot.io/skip-until": "2099-12-31"}` | `2025-01-01T00:00:00Z` | `true` |
| `NoSkipWhenSkipUntilInPast` | `{"mendabot.io/skip-until": "2020-01-01"}` | `2025-06-15T12:00:00Z` | `false` |
| `NoSkipWhenSkipUntilMalformed` | `{"mendabot.io/skip-until": "not-a-date"}` | any | `false` |
| `NoSkipWhenNoAnnotations` | `{}` | any | `false` |

**Additional boundary test** (recommended):

| Test Name | `annotations` input | `now` | Expected |
|---|---|---|---|
| `SkipOnTheDateItself` | `{"mendabot.io/skip-until": "2025-06-01"}` | `2025-06-01T23:59:59Z` | `true` (still within the day) |
| `NoSkipDayAfter` | `{"mendabot.io/skip-until": "2025-06-01"}` | `2025-06-02T00:00:00Z` | `false` (window expired at midnight) |

---

## Tasks

- [ ] Write `internal/domain/annotations_test.go` covering all cases above (TDD — verify
  they fail before creating the implementation)
- [ ] Create `internal/domain/annotations.go` with the three constants and `ShouldSkip`
- [ ] Run `go test -race ./internal/domain/...` — all tests must pass
- [ ] Run `go vet ./internal/domain/...` — must be clean

---

## Dependencies

**Depends on:** Nothing (pure domain logic, no external imports beyond `"time"`)
**Blocks:** STORY_02 (provider gate), STORY_03 (priority bypass)

---

## Definition of Done

- [ ] `internal/domain/annotations.go` compiles cleanly
- [ ] All annotation tests pass with `-race`
- [ ] Full test suite `go test -race ./...` passes
- [ ] `go vet ./...` clean
