package domain

import "regexp"

var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(ignore|disregard|forget)\s{0,10}(all\s+)?(previous|prior|above|earlier)\s+(instructions?|rules?|prompts?|context)`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(in\s+)?(a\s+)?(different|new|maintenance|admin|root|debug)\s+mode`),
	regexp.MustCompile(`(?i)(override|bypass|disable)\s+(all\s+)?(hard\s+)?rules?`),
	regexp.MustCompile(`(?i)system\s*:\s*(you\s+are|act\s+as|behave\s+as)`),
	regexp.MustCompile(`(?i)stop\s+(following|obeying)\s+((the|these|all)\s+)?(rules?|instructions?|guidelines?|prompts?)`),
}

// DetectInjection returns true if text contains patterns consistent with a
// prompt injection attempt. This is a best-effort heuristic — not a guarantee.
func DetectInjection(text string) bool {
	for _, p := range injectionPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}
