# Story 02: redact.go — Five New Built-In Credential Patterns

**Epic:** [epic29-agent-hardening](README.md)
**Priority:** High
**Status:** Not Started

---

## User Story

As a **mendabot operator**, I want `domain.RedactSecrets` to recognise common credential
formats beyond the current set, so that LLM API keys, cloud provider credentials, JWTs,
and encryption private keys are caught by the built-in pattern set without requiring
custom configuration.

---

## Background

`internal/domain/redact.go` currently has 11 patterns. Five credential formats that
appear frequently in Kubernetes tool output and agent environments are not covered:

| Format | Example | Current coverage |
|--------|---------|-----------------|
| `age` private key | `AGE-SECRET-KEY-1QEKK0T...` | None — bech32 format is not base64; not caught by the `{40,}` pattern |
| OpenAI / Anthropic key | `sk-proj-abc123...`, `sk-ant-api03-...` | Only caught if the YAML key is named `api_key` — a bare value in `provider-config` JSON is not caught |
| AWS access key ID | `AKIAIOSFODNN7EXAMPLE` | Only caught by base64 pattern if ≥40 chars (AKIA keys are exactly 20 chars — **not caught**) |
| JWT | `eyJhbGci....eyJzdWIi...` | The long base64 pattern catches most JWTs, but the header segment alone may be <40 chars; explicit pattern is more reliable |
| Non-Bearer Authorization header | `Authorization: Token abc123`, `Authorization: AWS4-HMAC-SHA256...` | Only `Bearer` is caught (pattern 2); `Token`, `Basic`, `Digest`, `AWS4-HMAC-SHA256` are not |

The `age` gap is the highest priority: `age-keygen` is an installed and wrapped tool,
but the private key it generates (`AGE-SECRET-KEY-1` prefix, bech32 alphabet `A-Z2-7`)
does not match any existing pattern. If `age-keygen` output slips through to the LLM
via a tool call result, the private key is fully exposed.

The `sk-*` gap is also high priority because `AGENT_PROVIDER_CONFIG` for OpenAI and
Anthropic providers contains these keys. While the env var value itself is not returned
as tool output, a misconfigured tool call that echoes the config or a `helm get values`
that surfaces it would not be caught by the current named-key patterns if the YAML key
name is not `api_key` or `token`.

---

## Acceptance Criteria

- [ ] `AGE-SECRET-KEY-1QEKK0T0PGLH0W3S2VCFQV9XC8YFQY4YXJMCF` is redacted to
      `[REDACTED-AGE-KEY]`
- [ ] `age-secret-key-1...` (lowercase) is also redacted (case-insensitive match)
- [ ] `age1ql3z7hjy54pw3pywairh23x4let...` (age public key) is **not** redacted
- [ ] `sk-proj-abc123XYZdef456GHI789jkl012MNO` is redacted
- [ ] `sk-ant-api03-AbCdEf123456789...` is redacted
- [ ] `sk-abc` (too short, < 20 chars after prefix) is **not** redacted
- [ ] `AKIAIOSFODNN7EXAMPLE` is redacted to `[REDACTED-AWS-KEY]`
- [ ] `AKIAIOSFODNN7EXAMPL` (19 chars — one short) is **not** redacted
- [ ] `ASIA` and `AROA` prefixes (AWS temporary credentials) are **not** redacted by
      this pattern (they have different formats and risk profiles)
- [ ] `eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0` (two-segment JWT) is redacted to
      `[REDACTED-JWT]`
- [ ] `eyJhbGciOiJSUzI1NiJ9` (single segment only, no dot) is **not** redacted by the
      JWT pattern (may still be caught by base64 pattern if ≥40 chars)
- [ ] `Authorization: Token abc123secretvalue` is redacted; `abc123secretvalue` becomes
      `[REDACTED]`
- [ ] `Authorization: Bearer already-handled` is still handled by the existing Bearer
      pattern (not double-redacted, no regression)
- [ ] `Authorization: Basic dXNlcjpwYXNz` is redacted
- [ ] All existing `redact_test.go` test cases continue to pass (no regressions)
- [ ] `go test -timeout 30s -race ./internal/domain/...` passes

---

## Technical Implementation

### New patterns (added to `redactPatterns` slice in `redact.go`)

Patterns are inserted **before** the existing base64 catch-all (pattern 11) to ensure
more specific matches take priority. The age key pattern should be first among the new
additions since bech32 characters overlap with base64.

#### 1. age private key

```go
// age private key: AGE-SECRET-KEY-1 followed by bech32 upper-case chars (A-Z, 2-7)
// Minimum real key length is 62 chars total; match 50+ chars after the prefix.
{
    re:          regexp.MustCompile(`(?i)(AGE-SECRET-KEY-1)[A-Z2-7]{50,}`),
    replacement: `[REDACTED-AGE-KEY]`,
}
```

Note: The full replacement discards the prefix too — `AGE-SECRET-KEY-1` is itself
identifying information about the key type. Replacement is the full match.

#### 2. OpenAI / Anthropic `sk-*` keys

```go
// sk-* API keys: OpenAI (sk-..., sk-proj-...) and Anthropic (sk-ant-...)
// Require at least 20 alphanumeric/dash/underscore chars after the sk- prefix.
{
    re:          regexp.MustCompile(`sk-[a-zA-Z0-9_\-]{4,}[A-Za-z0-9]{16,}`),
    replacement: `[REDACTED-SK-KEY]`,
}
```

The pattern requires the total key to be meaningfully long. `sk-proj-` (8 chars) +
16 chars = 24 chars minimum, which catches real keys while excluding short test
fixtures like `sk-test`.

#### 3. AWS access key ID

```go
// AWS IAM access key ID: AKIA followed by exactly 16 uppercase alphanumeric chars.
// AKIA = long-term key; ASIA = temporary STS key (different risk, excluded).
{
    re:          regexp.MustCompile(`AKIA[A-Z0-9]{16}`),
    replacement: `[REDACTED-AWS-KEY]`,
}
```

#### 4. JWT (two base64url segments)

```go
// JWT: two base64url-encoded segments separated by a dot, each at least 10 chars.
// A full JWT has three segments (header.payload.signature); matching two is sufficient
// to avoid false positives on arbitrary dotted strings while catching truncated JWTs.
{
    re:          regexp.MustCompile(`ey[A-Za-z0-9_\-]{10,}\.ey[A-Za-z0-9_\-]{10,}`),
    replacement: `[REDACTED-JWT]`,
}
```

Both segments start with `ey` because the standard base64url encoding of `{"` (the
opening of a JSON object header/payload) always produces `ey`. This is a reliable
discriminator that avoids false positives.

#### 5. Non-Bearer Authorization header

```go
// Authorization header with any scheme other than Bearer (already handled above).
// Matches: "Authorization: Token abc", "Authorization: Basic dXNlcjpw", etc.
// The negative lookahead (?!(?i)bearer) is not supported in Go's RE2 engine.
// Instead, use a separate pattern that captures the scheme + value and only
// matches schemes that are not "bearer" by matching known non-bearer schemes.
// Simpler approach: match Authorization: <non-whitespace> where the value
// is not already fully redacted.
{
    re:          regexp.MustCompile(`(?i)(authorization\s*:\s*)(?:(?i)(?:token|basic|digest|apikey|aws4-hmac-sha256|ntlm)\s+)\S+`),
    replacement: `${1}[REDACTED]`,
}
```

Note: Go's `regexp` package uses RE2 which does not support negative lookaheads.
Rather than trying to exclude `Bearer` with a lookahead, the pattern explicitly
lists known non-Bearer schemes. This avoids double-redacting `Bearer` tokens
(handled by pattern 2) while still catching all common alternative schemes.
Unknown custom schemes are not caught — acceptable trade-off to avoid false positives
on `Authorization: CustomScheme` in non-auth contexts.

### Placement in the `redactPatterns` slice

Final order of all patterns after this story:

| # | Pattern | New? |
|---|---------|------|
| 1 | URL credentials (`://user:pass@host`) | Existing |
| 2 | Bearer token | Existing |
| 3 | GitHub token (`gh[a-z]_...`) | Existing |
| 4 | JSON `"password"` field | Existing |
| 5 | `password=`/`password:` | Existing |
| 6 | `token=`/`token:` | Existing |
| 7 | `secret=`/`secret:` | Existing |
| 8 | `api_key=`/`api-key=`/`apikey=` | Existing |
| 9 | `x-api-key=`/`x-api-key:` | Existing |
| 10 | PEM private key block | Existing |
| 11 | **age private key** (`AGE-SECRET-KEY-1...`) | **New** |
| 12 | **`sk-*` API key** | **New** |
| 13 | **AWS AKIA access key** | **New** |
| 14 | **JWT two-segment** (`ey...ey...`) | **New** |
| 15 | **Non-Bearer Authorization header** | **New** |
| 16 | Long base64 string (≥40 chars) | Existing — always last |

---

## Test Cases

Add the following cases to the existing table-driven test in `redact_test.go`:

| Name | Input | Expected output |
|------|-------|-----------------|
| `age private key full` | `key: AGE-SECRET-KEY-1QEKK0T0PGLH0W3S2VCFQV9XC8YFQY4YXJMCFABCDEFGH` | `key: [REDACTED-AGE-KEY]` |
| `age private key lowercase` | `key: age-secret-key-1qekk0t0pglh0w3s2vcfqv9xc8yfqy4yxjmcfabcdefgh` | `key: [REDACTED-AGE-KEY]` |
| `age public key not redacted` | `recipient: age1ql3z7hjy54pw3pywairh23x4let4w0g92d6jjmscgjhe` | `recipient: age1ql3z7hjy54pw3pywairh23x4let4w0g92d6jjmscgjhe` |
| `sk-proj key` | `api_key: sk-proj-T2BlbkFJabcdefghij1234567890ABCD` | `api_key: [REDACTED-SK-KEY]` |
| `sk-ant key` | `key: sk-ant-api03-AbCdEfGhIj1234567890KLmnopqrst` | `key: [REDACTED-SK-KEY]` |
| `sk too short` | `key: sk-abc` | `key: sk-abc` |
| `AWS AKIA key` | `aws_access_key_id: AKIAIOSFODNN7EXAMPLE` | `aws_access_key_id: [REDACTED-AWS-KEY]` |
| `AWS AKIA not 16 chars` | `value: AKIAIOSFODNN7EXAMPL` | `value: AKIAIOSFODNN7EXAMPL` |
| `JWT two segments` | `token: eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0` | `token: [REDACTED-JWT]` |
| `JWT single segment` | `value: eyJhbGciOiJSUzI1NiJ9` | `value: [REDACTED-BASE64]` (caught by base64 if ≥40 chars) |
| `Authorization Token` | `Authorization: Token ghp_abc123secretvalue456789` | `Authorization: [REDACTED]` |
| `Authorization Basic` | `Authorization: Basic dXNlcjpwYXNzd29yZA==` | `Authorization: [REDACTED]` |
| `Authorization Bearer no regression` | `Authorization: Bearer eyJhbGci` | `Authorization: Bearer [REDACTED]` (existing pattern) |

---

## Definition of Done

- [ ] `internal/domain/redact.go` has five new patterns in the correct position
      (before the base64 catch-all)
- [ ] All new test cases in `redact_test.go` pass
- [ ] All existing test cases continue to pass (no regressions)
- [ ] `go test -timeout 30s -race ./internal/domain/...` passes
- [ ] `go vet ./internal/domain/...` passes
