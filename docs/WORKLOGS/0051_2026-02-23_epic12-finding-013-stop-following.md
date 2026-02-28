# Worklog: Epic 12 Finding 013 — Stop Following/Obeying Injection Pattern

**Date:** 2026-02-23
**Session:** Add missing "stop following/obeying the rules" prompt injection detection pattern
**Status:** Complete

---

## Objective

Fix security finding 2026-02-23-013: `DetectInjection` did not catch the variant phrases
"stop following the rules", "stop obeying the rules", "stop following these instructions",
and related forms. These are common prompt injection phrases that bypass the existing
pattern set.

---

## Work Completed

### 1. TDD — failing tests added first

Added 5 new table-driven test cases to `internal/domain/injection_test.go`:
- `"stop following the rules"` → want: true
- `"stop obeying the rules"` → want: true
- `"stop following these instructions"` → want: true
- `"stop obeying all guidelines"` → want: true
- `"stop running the pod"` → want: false (regression guard — "running" not in keyword list)

Ran `go test -timeout 30s -race ./internal/domain/...` and confirmed 4 failures before
any implementation.

### 2. Pattern implementation

Added a fifth pattern to `injectionPatterns` in `internal/domain/injection.go`:

```go
regexp.MustCompile(`(?i)stop\s+(following|obeying)\s+((the|these|all)\s+)?(rules?|instructions?|guidelines?|prompts?)`),
```

**Key design note:** The suggested pattern in the brief was
`(the|these|all\s+)?` (article with `\s+` only on `all`). This is incorrect: when the
article `the` or `these` is consumed, no trailing `\s+` follows, leaving the engine to
match the noun directly against ` rules` (with a leading space), which fails. The correct
structure wraps the entire article+space unit as `(the|these|all)\s+` inside an outer
optional group so the space is always consumed when an article is present.

### 3. Validation

All 20 tests pass with `-race` flag.

---

## Key Decisions

- The outer optional group structure `((the|these|all)\s+)?` rather than the naive
  `(the|these|all\s+)?` ensures the trailing space is consumed consistently regardless
  of which article matches.
- No existing patterns or test cases were modified.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/domain/...
ok  	github.com/lenaxia/k8s-mechanic/internal/domain	1.123s
```

---

## Next Steps

No immediate follow-up needed for this finding. Continue with remaining epic 12 findings
if any are outstanding.

---

## Files Modified

- `internal/domain/injection.go` — added fifth pattern
- `internal/domain/injection_test.go` — added 5 new test cases
- `docs/WORKLOGS/0051_2026-02-23_epic12-finding-013-stop-following.md` — this file
- `docs/WORKLOGS/README.md` — index updated
