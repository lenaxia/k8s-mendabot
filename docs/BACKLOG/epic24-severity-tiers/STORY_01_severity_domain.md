# Story 01: Domain — Severity Type, Constants, and Level Helper

**Epic:** [epic24-severity-tiers](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **mendabot developer**, I want a strongly-typed `Severity` domain type with ordered
constants so that all components can compare, store, and filter finding severity without
using bare strings or arbitrary integer magic numbers.

---

## Background

Go's type system makes it easy to misassign strings. A named `Severity` type prevents
`Finding.Severity = "CRITICAL"` (wrong case) or `Finding.Severity = "urgent"` (wrong value)
from compiling. All severity comparisons go through a single `SeverityLevel` helper so the
ordering is defined once and tested once.

---

## Design

### New file: `internal/domain/severity.go`

```go
package domain

// Severity represents the impact tier of a Finding.
type Severity string

const (
    SeverityCritical Severity = "critical"
    SeverityHigh     Severity = "high"
    SeverityMedium   Severity = "medium"
    SeverityLow      Severity = "low"
)

// severityOrder maps Severity to a numeric level for comparison.
// Higher numbers = higher severity.
var severityOrder = map[Severity]int{
    SeverityLow:      1,
    SeverityMedium:   2,
    SeverityHigh:     3,
    SeverityCritical: 4,
}

// SeverityLevel returns the numeric level for s (higher = more severe).
// Returns 0 for unrecognised values (including the empty string "").
func SeverityLevel(s Severity) int {
    return severityOrder[s]
}

// MeetsSeverityThreshold reports whether finding severity f is at least as
// severe as the configured minimum threshold min.
//
// Special case: if min is SeverityLow (the default pass-all threshold), any
// non-empty recognised severity passes, AND an empty/unrecognised f also passes.
// This ensures the default MinSeverity=low setting does not silently drop findings
// whose providers have not yet been updated to set Severity.
//
// For any min above SeverityLow, an empty or unrecognised f returns false.
func MeetsSeverityThreshold(f, min Severity) bool {
    minLevel := SeverityLevel(min)
    if minLevel == 0 {
        // unrecognised min — should not happen; fail closed
        return false
    }
    if minLevel == SeverityLevel(SeverityLow) {
        // pass-all mode: any finding (including legacy empty severity) passes
        return true
    }
    return SeverityLevel(f) >= minLevel
}

// ParseSeverity converts a string to a Severity, returning (value, true) on
// success or (SeverityLow, false) if the string is not a recognised value.
func ParseSeverity(s string) (Severity, bool) {
    v := Severity(s)
    if _, ok := severityOrder[v]; ok {
        return v, true
    }
    return SeverityLow, false
}
```

### Finding type update

In `internal/domain/provider.go`, add the `Severity` field to `Finding`. Use a **named
field** addition at the end of the struct — all existing `domain.Finding{...}` struct
literals in the codebase use named fields (confirmed), so adding a new field at the end
is safe and requires no changes to existing initialisations.

```go
type Finding struct {
    Namespace    string
    Kind         string
    Name         string
    ParentObject string
    Errors       string   // JSON-encoded []errorEntry; existing field, unchanged
    Severity     Severity // impact tier; zero value "" passes when MinSeverity=low
}
```

---

## Acceptance Criteria

- [ ] `internal/domain/severity.go` defines `Severity` type and four constants
- [ ] `SeverityLevel` returns `4` for `critical`, `3` for `high`, `2` for `medium`, `1` for `low`, `0` for unknown/empty
- [ ] `MeetsSeverityThreshold(f, SeverityLow)` returns `true` for all `f` including `""` (pass-all semantics for default threshold)
- [ ] `MeetsSeverityThreshold(f, SeverityHigh)` returns `false` for `""` and `"low"`
- [ ] `MeetsSeverityThreshold` returns correct results for all same-tier and cross-tier combinations
- [ ] `ParseSeverity` returns the correct `Severity` and `true` for all four valid values
- [ ] `ParseSeverity` returns `(SeverityLow, false)` for empty string and unknown values
- [ ] `domain.Finding` has a `Severity Severity` field added at the end of the struct using a named field (no positional literal breakage)
- [ ] `internal/domain/severity_test.go` covers all cases including the pass-all edge case

---

## Test Cases

All tests live in `internal/domain/severity_test.go`.

| Test | Input | Expected |
|------|-------|----------|
| `SeverityLevelCritical` | `SeverityCritical` | `4` |
| `SeverityLevelHigh` | `SeverityHigh` | `3` |
| `SeverityLevelMedium` | `SeverityMedium` | `2` |
| `SeverityLevelLow` | `SeverityLow` | `1` |
| `SeverityLevelUnknown` | `Severity("bogus")` | `0` |
| `SeverityLevelEmpty` | `Severity("")` | `0` |
| `MeetsThreshold_CriticalMeetsCritical` | `f=critical, min=critical` | `true` |
| `MeetsThreshold_CriticalMeetsHigh` | `f=critical, min=high` | `true` |
| `MeetsThreshold_LowDoesNotMeetHigh` | `f=low, min=high` | `false` |
| `MeetsThreshold_EmptyDoesNotMeetHigh` | `f="", min=high` | `false` |
| `MeetsThreshold_EmptyMeetsLow` | `f="", min=low` | `true` (pass-all: default threshold passes everything) |
| `MeetsThreshold_LowMeetsLow` | `f=low, min=low` | `true` |
| `MeetsThreshold_UnknownMinFailsClosed` | `f=critical, min="bogus"` | `false` |
| `ParseSeverityValid` | `"critical"` | `(SeverityCritical, true)` |
| `ParseSeverityUnknown` | `"urgent"` | `(SeverityLow, false)` |
| `ParseSeverityEmpty` | `""` | `(SeverityLow, false)` |

---

## Tasks

- [ ] Write `internal/domain/severity_test.go` (TDD — must fail before implementation)
- [ ] Create `internal/domain/severity.go` with type, constants, and helpers
- [ ] Add `Severity Severity` field to `domain.Finding` in `internal/domain/provider.go`
- [ ] Run `go test -race ./internal/domain/...` — all tests must pass
- [ ] Run `go vet ./internal/domain/...` — must be clean
- [ ] Run `go build ./...` — full build must be clean (zero-value `Severity` on existing `Finding` usages is `""` which is fine)

---

## Dependencies

**Depends on:** Nothing (pure domain logic, no external imports)
**Blocks:** STORY_02 (CRD field), STORY_03 (provider severity), STORY_04 (config filter)

---

## Definition of Done

- [ ] `internal/domain/severity.go` compiles cleanly
- [ ] All severity tests pass with `-race`
- [ ] Full test suite `go test -race ./...` passes
- [ ] `go vet ./...` clean
