# Story: Domain — Annotation Constants and Skip Logic

**Epic:** [epic16-annotation-control](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want mechanic to respect per-resource annotations so that I
can permanently suppress investigations (`mechanic.io/enabled: "false"`), suppress them for
a time window (`mechanic.io/skip-until: "YYYY-MM-DD"`), or mark a resource as critical
(`mechanic.io/priority: "critical"`), without having to modify the controller configuration.

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

// Annotation key constants for per-resource mechanic control.
const (
    // AnnotationEnabled suppresses all mechanic investigation for this resource
    // when set to "false". Any other value (including absent) means enabled.
    AnnotationEnabled = "mechanic.io/enabled"

    // AnnotationSkipUntil suppresses mechanic investigation until the given date
    // (inclusive). Value must be formatted as "YYYY-MM-DD" in UTC. A malformed
    // value is silently ignored (treated as absent — no suppression).
    AnnotationSkipUntil = "mechanic.io/skip-until"

    // AnnotationPriority, when set to "critical", causes SourceProviderReconciler
    // to bypass the stabilisation window for this resource. Any other value is
    // ignored. This annotation is read by the reconciler (STORY_03), not by
    // ExtractFinding.
    AnnotationPriority = "mechanic.io/priority"
)
```

### Helper: `ShouldSkip`

```go
// ShouldSkip reports whether ExtractFinding should return (nil, nil) for the
// resource that owns annotations, based on the mechanic.io control annotations.
//
// Rules (evaluated in order):
//  1. If annotations["mechanic.io/enabled"] == "false"  → skip (return true).
//  2. If annotations["mechanic.io/skip-until"] is set:
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

- [x] `internal/domain/annotations.go` is a new file in the `domain` package
- [x] It exports exactly three constants: `AnnotationEnabled`, `AnnotationSkipUntil`,
  `AnnotationPriority`
- [x] It exports `ShouldSkip(annotations map[string]string, now time.Time) bool`
- [x] `ShouldSkip` returns `true` when `mechanic.io/enabled` is `"false"`
- [x] `ShouldSkip` returns `true` when `mechanic.io/skip-until` is a future date
- [x] `ShouldSkip` returns `false` when `mechanic.io/skip-until` is today-or-past
  (window expired at midnight UTC)
- [x] `ShouldSkip` returns `false` when `mechanic.io/skip-until` is malformed
- [x] `ShouldSkip` returns `false` when no relevant annotations are present
- [x] `internal/domain/annotations_test.go` covers all five cases above

---

## Test Cases

All tests live in `internal/domain/annotations_test.go`.

| Test Name | `annotations` input | `now` | Expected |
|---|---|---|---|
| `SkipWhenDisabled` | `{"mechanic.io/enabled": "false"}` | any | `true` |
| `SkipWhenSkipUntilInFuture` | `{"mechanic.io/skip-until": "2099-12-31"}` | `2025-01-01T00:00:00Z` | `true` |
| `NoSkipWhenSkipUntilInPast` | `{"mechanic.io/skip-until": "2020-01-01"}` | `2025-06-15T12:00:00Z` | `false` |
| `NoSkipWhenSkipUntilMalformed` | `{"mechanic.io/skip-until": "not-a-date"}` | any | `false` |
| `NoSkipWhenNoAnnotations` | `{}` | any | `false` |

**Additional boundary test** (recommended):

| Test Name | `annotations` input | `now` | Expected |
|---|---|---|---|
| `SkipOnTheDateItself` | `{"mechanic.io/skip-until": "2025-06-01"}` | `2025-06-01T23:59:59Z` | `true` (still within the day) |
| `NoSkipDayAfter` | `{"mechanic.io/skip-until": "2025-06-01"}` | `2025-06-02T00:00:00Z` | `false` (window expired at midnight) |

---

## Tasks

- [x] Write `internal/domain/annotations_test.go` covering all cases above (TDD — verify
  they fail before creating the implementation)
- [x] Create `internal/domain/annotations.go` with the three constants and `ShouldSkip`
- [x] Run `go test -race ./internal/domain/...` — all tests must pass
- [x] Run `go vet ./internal/domain/...` — must be clean

---

## Dependencies

**Depends on:** Nothing (pure domain logic, no external imports beyond `"time"`)
**Blocks:** STORY_02 (provider gate), STORY_03 (priority bypass)

---

## Definition of Done

- [x] `internal/domain/annotations.go` compiles cleanly
- [x] All annotation tests pass with `-race`
- [x] Full test suite `go test -race ./...` passes
- [x] `go vet ./...` clean
