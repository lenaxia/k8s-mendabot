# Worklog: Epic 16 STORY_01 — Annotation Constants and Skip Logic

**Date:** 2026-02-24
**Session:** Implement domain annotation constants and ShouldSkip helper (TDD)
**Status:** Complete

---

## Objective

Implement `internal/domain/annotations.go` with three exported annotation key constants
(`AnnotationEnabled`, `AnnotationSkipUntil`, `AnnotationPriority`) and the `ShouldSkip`
helper function. Write `internal/domain/annotations_test.go` first per TDD protocol,
covering all 7 test cases from the story spec.

---

## Work Completed

### 1. Test file (TDD — written before implementation)

Created `internal/domain/annotations_test.go` with a table-driven `TestShouldSkip`
covering all 7 required cases:

- `SkipWhenDisabled`
- `SkipWhenSkipUntilInFuture`
- `NoSkipWhenSkipUntilInPast`
- `NoSkipWhenSkipUntilMalformed`
- `NoSkipWhenNoAnnotations`
- `SkipOnTheDateItself` (boundary: last second of the annotated day)
- `NoSkipDayAfter` (boundary: midnight UTC after the annotated day)

Tests failed at build with `undefined: AnnotationEnabled`, `undefined: AnnotationSkipUntil`,
`undefined: ShouldSkip` — confirming TDD pre-condition met.

### 2. Implementation file

Created `internal/domain/annotations.go` with:
- Three exported `const` values: `AnnotationEnabled`, `AnnotationSkipUntil`, `AnnotationPriority`
- `ShouldSkip(annotations map[string]string, now time.Time) bool` with godoc
- No inline comments on constants per README-LLM.md §4 (no comments unless strictly necessary)
- Only `"time"` import — zero circular dependency risk

---

## Key Decisions

- Inline comments stripped from constants: story doc showed them but README-LLM.md §4
  forbids comments unless strictly necessary. The godoc on `ShouldSkip` is retained as it
  documents the boundary semantics which are non-obvious.
- `now` passed as parameter (not `time.Now()`) as specified, enabling deterministic tests
  without monkey-patching.
- `skip-until` deadline: `t.UTC().AddDate(0, 0, 1)` — window expires at midnight UTC on
  the day after the annotated date. Malformed values silently treated as absent.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/domain/...
# BEFORE implementation: build failed (undefined symbols) ✓ TDD confirmed

go test -timeout 30s -race ./internal/domain/...
# AFTER implementation: ok  github.com/lenaxia/k8s-mendabot/internal/domain  1.379s ✓

go vet ./internal/domain/...
# Clean — no output ✓

go build ./...
# Clean — no output ✓

go test -timeout 30s -race ./...
# All 12 packages pass ✓
```

---

## Next Steps

- STORY_02: Add `ShouldSkip` gate to all 6 native providers in `internal/provider/native/`
  immediately before the concrete type assertion in each `ExtractFinding` method.
- STORY_03: Add priority bypass in `internal/provider/provider.go` at the stabilisation
  window block (lines 167–181), using `domain.AnnotationPriority`.

---

## Files Modified

- `internal/domain/annotations.go` — created
- `internal/domain/annotations_test.go` — created
- `docs/WORKLOGS/0072_2026-02-24_epic16-story01-annotation-constants.md` — created
- `docs/WORKLOGS/README.md` — index table updated
