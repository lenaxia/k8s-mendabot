package domain

import "regexp"

var redactPatterns = []struct {
	re          *regexp.Regexp
	replacement string
}{
	// URL credentials must come first — they contain `:` which would otherwise
	// match the generic password/token patterns and produce garbled output.
	{regexp.MustCompile(`(?i)://[^:@\s]*:[^@\s]+@`), `://[REDACTED]@`},
	// Bearer tokens (HTTP Authorization header).
	{regexp.MustCompile(`(?i)(bearer )\S+`), `${1}[REDACTED]`},
	// GitHub token formats (gh[a-z]_ prefix) must come before the generic
	// token= pattern so that tokens appearing as bare values (not preceded by
	// "token=") always produce [REDACTED-GH-TOKEN], giving consistent audit labels.
	{regexp.MustCompile(`gh[a-z]_[A-Za-z0-9]{36,}`), `[REDACTED-GH-TOKEN]`},
	// JSON "password" field.
	{regexp.MustCompile(`(?i)("password"\s*:\s*)"[^"]*"`), `${1}"[REDACTED]"`},
	// Generic key=value / key: value credential patterns.
	{regexp.MustCompile(`(?i)(password\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(token\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(secret\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(api[_-]?key\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)(x-api-key\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
	// Multi-line PEM private key blocks (RSA, EC, DSA, OPENSSH, PKCS#8).
	{regexp.MustCompile(`(?is)-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`), `[REDACTED-PEM-KEY]`},
	// Long base64 strings (≥40 chars). Covers Kubernetes Secret data values,
	// age private key suffixes, and other long encoded secrets. Short values
	// (<30 raw bytes → <40 base64 chars) are not matched; they must be caught
	// by the named-key patterns above.
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
