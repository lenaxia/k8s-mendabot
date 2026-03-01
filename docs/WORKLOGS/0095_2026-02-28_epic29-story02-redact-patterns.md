# Worklog: epic29 STORY_02 — Five New Built-In Redact Patterns

**Date:** 2026-02-28
**Session:** Add 5 new credential patterns to `internal/domain/redact.go` (age key, sk-*, AWS AKIA, JWT, non-Bearer Authorization)
**Status:** Complete

---

## Objective

Extend `domain.RedactSecrets` with five new built-in patterns that cover credential formats not caught by the existing 11 patterns: age private keys, OpenAI/Anthropic sk-* API keys, AWS AKIA access key IDs, two-segment JWTs, and non-Bearer Authorization headers.

---

## Work Completed

### 1. TDD — Failing tests written first

Added 11 new test cases to `internal/domain/redact_test.go` covering:
- `age private key full` — uppercase AGE-SECRET-KEY-1 prefix
- `age private key lowercase` — lowercase variant (case-insensitive)
- `age public key not redacted` — `age1...` prefix must not match
- `sk-proj key` — bare OpenAI sk-proj-* key (not preceded by a named-key prefix)
- `sk-ant key` — Anthropic sk-ant-* key
- `sk too short` — short sk-abc must not match
- `AWS AKIA key` — exact 20-char AKIA key
- `AWS AKIA not 16 chars` — 19-char value must not match
- `JWT two segments` — ey...ey... two-segment JWT (bare, no named prefix)
- `Authorization Token` — non-Bearer Token scheme
- `Authorization Basic` — Basic scheme

### 2. Implementation — 5 new patterns in `redact.go`

All five patterns inserted before the base64 catch-all (position 16), after PEM (position 10):

| # | Pattern | Regex |
|---|---------|-------|
| 11 | age private key | `(?i)AGE-SECRET-KEY-1[A-Z0-9]{40,}` |
| 12 | sk-* API key | `sk-[a-zA-Z0-9_\-]{4,}[A-Za-z0-9]{16,}` |
| 13 | AWS AKIA | `AKIA[A-Z0-9]{16}` |
| 14 | JWT two-segment | `ey[A-Za-z0-9_\-]{10,}\.ey[A-Za-z0-9_\-]{10,}` |
| 15 | Non-Bearer Authorization | `(?i)(authorization\s*:\s*)(?:token\|basic\|digest\|apikey\|aws4-hmac-sha256\|ntlm)\s+\S+` |

### 3. Spec deviation notes

Two test cases in the delegation prompt had incorrect expected outputs due to a pattern ordering conflict:

- `"api_key: sk-proj-..."` → the `api[_-]?key` named-key pattern (position 8) fires before sk-* (position 12), producing `api_key: [REDACTED]` not `[REDACTED-SK-KEY]`. Test input changed to `"provider: sk-proj-..."` (non-matched prefix) so sk-* fires directly.
- `"token: eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0"` → the `token:` named-key pattern fires first, producing `token: [REDACTED]`. Test input changed to `"header: ..."` so JWT pattern fires.

The age key regex was also adjusted: the spec stated `[A-Z2-7]{50,}` (strict bech32, 50 chars) but the test key suffix is 47 chars and contains digits outside `2-7`. Pattern broadened to `[A-Z0-9]{40,}` with `(?i)` to match both uppercase and lowercase.

---

## Key Decisions

1. **`[A-Z0-9]{40,}` for age key suffix** — the test keys in the spec are not strict bech32; they contain `0`, `8`, `9`. Using `[A-Z0-9]` with `(?i)` is broader but still specific enough given the `AGE-SECRET-KEY-1` anchor. Real age keys are 59+ chars; 40-char minimum is conservative and safe.

2. **sk-* and JWT patterns after named-key patterns** — the correct placement per the story's position table (11–15). Named-key patterns (`api_key:`, `token:`) take priority over sk-* and JWT patterns for values preceded by those keys. This is by design: the named-key patterns produce `[REDACTED]` which is acceptable. sk-* and JWT patterns fire for bare values not preceded by a named key.

3. **Non-Bearer Authorization pattern uses explicit scheme list** — Go RE2 does not support negative lookaheads, so Bearer exclusion is done by listing the known non-Bearer schemes explicitly. Unknown custom schemes are not caught.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/domain/...
# ok  github.com/lenaxia/k8s-mechanic/internal/domain

go vet ./internal/domain/...
# (no output — clean)
```

All 11 new test cases pass. All existing test cases pass (no regressions).

---

## Next Steps

STORY_03 of epic29: Add `Redactor` struct to `redact.go` (per the delegation spec — refactor to struct-based pattern registry).

---

## Files Modified

- `internal/domain/redact.go` — 5 new patterns added (positions 11–15)
- `internal/domain/redact_test.go` — 11 new test cases added
- `docs/WORKLOGS/0095_2026-02-28_epic29-story02-redact-patterns.md` — this file
