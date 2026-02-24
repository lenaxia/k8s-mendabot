package domain

import "time"

const (
	AnnotationEnabled   = "mendabot.io/enabled"
	AnnotationSkipUntil = "mendabot.io/skip-until"
	AnnotationPriority  = "mendabot.io/priority"
)

// ShouldSkip reports whether ExtractFinding should return (nil, nil) for the
// resource that owns annotations, based on the mendabot.io control annotations.
//
// Rules (evaluated in order):
//  1. If annotations["mendabot.io/enabled"] == "false"  → skip (return true).
//  2. If annotations["mendabot.io/skip-until"] is set:
//     - Parse the value as "2006-01-02" in UTC.
//     - If parsing fails: do NOT skip (treat as absent).
//     - If now is before the end of the skip-until day (i.e. before midnight
//     UTC at the start of the day after skip-until): skip (return true).
//     - Otherwise: do not skip (return false).
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
			return false
		}
		deadline := t.UTC().AddDate(0, 0, 1)
		return now.UTC().Before(deadline)
	}
	return false
}
