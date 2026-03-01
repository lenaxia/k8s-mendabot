package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// redactRule is a compiled find-replace pair.
type redactRule struct {
	re          *regexp.Regexp
	replacement string
}

// builtinPatternDefs is the ordered list of built-in pattern definitions.
// All 16 patterns (11 original + 5 from STORY_02) remain here as string literals.
var builtinPatternDefs = []struct {
	pattern     string
	replacement string
}{
	// URL credentials must come first — they contain `:` which would otherwise
	// match the generic password/token patterns and produce garbled output.
	{`(?i)://[^:@\s]*:[^@\s]+@`, `://[REDACTED]@`},
	// Bearer tokens (HTTP Authorization header).
	{`(?i)(bearer )\S+`, `${1}[REDACTED]`},
	// GitHub token formats (gh[a-z]_ prefix) must come before the generic
	// token= pattern so that tokens appearing as bare values (not preceded by
	// "token=") always produce [REDACTED-GH-TOKEN], giving consistent audit labels.
	{`gh[a-z]_[A-Za-z0-9]{36,}`, `[REDACTED-GH-TOKEN]`},
	// JSON "password" field.
	{`(?i)("password"\s*:\s*)"[^"]*"`, `${1}"[REDACTED]"`},
	// Generic key=value / key: value credential patterns.
	{`(?i)(password\s*[=:]\s*)\S+`, `${1}[REDACTED]`},
	{`(?i)(token\s*[=:]\s*)\S+`, `${1}[REDACTED]`},
	{`(?i)(secret\s*[=:]\s*)\S+`, `${1}[REDACTED]`},
	{`(?i)(api[_-]?key\s*[=:]\s*)\S+`, `${1}[REDACTED]`},
	{`(?i)(x-api-key\s*[=:]\s*)\S+`, `${1}[REDACTED]`},
	// Multi-line PEM private key blocks (RSA, EC, DSA, OPENSSH, PKCS#8).
	{`(?is)-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`, `[REDACTED-PEM-KEY]`},
	// age private key: AGE-SECRET-KEY-1 followed by bech32 upper-case chars (A-Z, 2-7).
	{`(?i)AGE-SECRET-KEY-1[A-Z0-9]{40,}`, `[REDACTED-AGE-KEY]`},
	// sk-* API keys: OpenAI (sk-..., sk-proj-...) and Anthropic (sk-ant-...).
	{`sk-[a-zA-Z0-9_\-]{4,}[A-Za-z0-9]{16,}`, `[REDACTED-SK-KEY]`},
	// AWS IAM access key ID: AKIA followed by exactly 16 uppercase alphanumeric chars.
	{`AKIA[A-Z0-9]{16}`, `[REDACTED-AWS-KEY]`},
	// JWT: two base64url-encoded segments separated by a dot, each at least 10 chars.
	{`ey[A-Za-z0-9_\-]{10,}\.ey[A-Za-z0-9_\-]{10,}`, `[REDACTED-JWT]`},
	// Authorization header with non-Bearer schemes (Token, Basic, Digest, etc.).
	{`(?i)(authorization\s*:\s*)(?:(?i)(?:token|basic|digest|apikey|aws4-hmac-sha256|ntlm)\s+)\S+`, `${1}[REDACTED]`},
	// Long base64 strings (≥40 chars). Covers Kubernetes Secret data values,
	// age private key suffixes, and other long encoded secrets. Short values
	// (<30 raw bytes → <40 base64 chars) are not matched; they must be caught
	// by the named-key patterns above.
	{`[A-Za-z0-9+/]{40,}={0,2}`, `[REDACTED-BASE64]`},
}

// builtinRules holds the compiled built-in redaction rules.
// defaultRedactor is initialised in init() after builtinRules is populated.
var builtinRules []redactRule

var defaultRedactor *Redactor

func init() {
	builtinRules = compileRules(builtinPatternDefs)
	defaultRedactor = &Redactor{rules: builtinRules}
}

func compileRules(defs []struct{ pattern, replacement string }) []redactRule {
	rules := make([]redactRule, 0, len(defs))
	for _, d := range defs {
		rules = append(rules, redactRule{
			re:          regexp.MustCompile(d.pattern),
			replacement: d.replacement,
		})
	}
	return rules
}

// Redactor applies a set of compiled redaction rules to text.
type Redactor struct {
	rules []redactRule
}

// New returns a Redactor with the built-in rules plus any extra patterns.
// Extra patterns are appended after the entire built-in set (including the base64 catch-all).
// Returns an error if any extra pattern is not a valid RE2 regex.
func New(extraPatterns []string) (*Redactor, error) {
	rules := make([]redactRule, len(builtinRules))
	copy(rules, builtinRules)
	for _, p := range extraPatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("domain.New: invalid extra redact pattern %q: %w", p, err)
		}
		rules = append(rules, redactRule{
			re:          re,
			replacement: "[REDACTED-CUSTOM]",
		})
	}
	return &Redactor{rules: rules}, nil
}

// Redact applies all rules to text and returns the sanitised result.
func (r *Redactor) Redact(text string) string {
	for _, rule := range r.rules {
		text = rule.re.ReplaceAllString(text, rule.replacement)
	}
	return text
}

// RedactSecrets is a package-level convenience wrapper using only built-in rules.
// All existing call sites continue to work unchanged.
func RedactSecrets(text string) string {
	return defaultRedactor.Redact(text)
}
