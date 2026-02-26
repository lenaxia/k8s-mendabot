# Worklog: Epic 13 STORY_01 — Built-in Correlation Rules

**Date:** 2026-02-25
**Session:** Delegation agent implementing and validating STORY_01 built-in correlation rules
**Status:** Complete

---

## Objective

Implement and validate all STORY_01 acceptance criteria: three built-in correlation rules
(`SameNamespaceParentRule`, `PVCPodRule`, `MultiPodSameNodeRule`) plus supporting changes
to `domain.Finding`, `PodProvider.ExtractFinding`, and `SourceProviderReconciler`.

---

## Work Completed

### 1. State assessment (read-only)

All prior-session work was already present on the branch:

- `internal/domain/provider.go` — `NodeName string` already added to `Finding` struct
  (lines 124–128, added in epic13 prior sessions).
- `internal/provider/native/pod.go` — `ExtractFinding` already populates
  `NodeName: pod.Spec.NodeName` in the returned `domain.Finding{}` literal (line 118).
- `internal/provider/provider.go` — The `rjob` construction block already writes
  `rjob.Annotations["mendabot.io/node-name"] = finding.NodeName` when non-empty
  (lines 331–333).
- `internal/correlator/rules.go` — All three rules fully implemented:
  `SameNamespaceParentRule`, `PVCPodRule` (both orientations), `MultiPodSameNodeRule`.
- `internal/correlator/rules_test.go` — 29 tests covering all rules with happy paths,
  no-match paths, edge cases, and determinism checks.
- `internal/correlator/correlator.go` — `Correlator` struct (STORY_02 work) also present.
- `internal/correlator/correlator_test.go` — 8 correlator-level tests also present.

### 2. Gap identified: missing lexicographic tiebreaker in MultiPodSameNodeRule

The spec (STORY_01 section on `MultiPodSameNodeRule`) explicitly requires:
> "On a tie, use lexicographic order of `Name` as a stable tiebreaker."

The primary selection loop at `internal/correlator/rules.go:312–317` only compared
`CreationTimestamp` — it did not apply the Name tiebreaker. This is a correctness gap
that would cause nondeterministic primary selection when two pod RemediationJobs have
identical CreationTimestamps (which is common in tests and possible in production when
multiple pods are created near-simultaneously).

### 3. TDD fix: added failing test, then fixed implementation

**Test added** (`internal/correlator/rules_test.go:703–730`):
`TestMultiPodSameNodeRule_PrimaryUID_Tiebreaker_LexicographicName` — three pods with the
same `CreationTimestamp`, names `rjob-aaa`, `rjob-mmm`, `rjob-zzz`; expects `rjob-aaa`
as primary. Test failed before the fix.

**Implementation fix** (`internal/correlator/rules.go:312–320`):
Added a secondary comparison branch: when `pTime.Equal(&cTime)`, compare `p.Name <
primary.Name` and replace if lexicographically smaller. This makes primary selection
fully deterministic in all cases.

### 4. Tests

All tests pass after the fix:

```
go test -timeout 30s -race ./internal/correlator/...
# ok  github.com/lenaxia/k8s-mendabot/internal/correlator  1.093s  (41 tests)

go test -timeout 30s -race ./...
# All 17 packages pass
```

### 5. Build

```
go build ./...
# Clean — no errors
```

### 6. Backlog update

Updated `STORY_01_builtin_rules.md`: status changed from `Not Started` to `Complete`,
all acceptance criteria checkboxes and task checkboxes marked `[x]`.

---

## Key Decisions

- The lexicographic-tiebreaker gap was a pre-existing spec non-compliance from prior
  sessions. Fixing it is the correct action — the spec is unambiguous and the fix is
  minimal (3-line change).
- No other gaps found. The prior work on this branch is substantively correct.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race -run TestMultiPodSameNodeRule_PrimaryUID_Tiebreaker_LexicographicName ./internal/correlator/... -v
# FAIL before fix, PASS after fix

go test -timeout 30s -race ./internal/correlator/... ./internal/domain/... ./internal/provider/...
# ok  github.com/lenaxia/k8s-mendabot/internal/correlator  1.093s
# ok  github.com/lenaxia/k8s-mendabot/internal/domain      (cached)
# ok  github.com/lenaxia/k8s-mendabot/internal/provider    (cached)
# ok  github.com/lenaxia/k8s-mendabot/internal/provider/native (cached)

go test -timeout 30s -race ./...
# All 17 packages pass

go build ./...
# Clean
```

---

## Next Steps

STORY_01 is complete. The next story in epic13 is STORY_02 (correlation window) — already
implemented on this branch per worklog 0043. Validation of STORY_02 should proceed next.

---

## Files Modified

- `internal/correlator/rules.go` — Added lexicographic Name tiebreaker in
  `MultiPodSameNodeRule` primary selection loop (lines 312–320)
- `internal/correlator/rules_test.go` — Added
  `TestMultiPodSameNodeRule_PrimaryUID_Tiebreaker_LexicographicName` test (lines 703–730)
- `docs/BACKLOG/epic13-multi-signal-correlation/STORY_01_builtin_rules.md` — Status and
  all checkboxes updated to Complete
