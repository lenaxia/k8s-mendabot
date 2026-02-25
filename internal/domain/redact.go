package domain

import "regexp"

var redactPatterns = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)://[^:@\s]*:[^@\s]+@`), `://[REDACTED]@`},
	{regexp.MustCompile(`(?i)(bearer )\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)("password"\s*:\s*)"[^"]*"`), `${1}"[REDACTED]"`},
	{regexp.MustCompile(`(?i)(password\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(token\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(secret\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(api[_-]?key\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(x-api-key\s*[=:]\t*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?is)-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`), `[REDACTED-PEM-KEY]`},
	{regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`), `[REDACTED-BASE64]`},
}

// RedactSecrets applies heuristic patterns to strip credential-like values from
// error text before it is stored in Finding.Errors.
//
// This is a best-effort defence-in-depth measure. It has both false positives
// (matching non-secret strings) and false negatives (novel credential formats).
// It is not a substitute for proper secret management.
func RedactSecrets(text string) string {
	for _, p := range redactPatterns {
		text = p.re.ReplaceAllString(text, p.replacement)
	}
	return text
}
