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
