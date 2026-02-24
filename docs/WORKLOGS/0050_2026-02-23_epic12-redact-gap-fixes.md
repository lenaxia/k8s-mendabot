# Worklog: Epic 12 — Redaction Gap Fixes (findings 010–012)

**Date:** 2026-02-23
**Session:** Fix three redaction gaps in RedactSecrets identified by security pentest
**Status:** Complete

---

## Objective

Fix three distinct redaction gaps in `internal/domain/redact.go` uncovered by the epic 12 security review:

- **010** — JWT Bearer tokens not redacted (JWT survives because it lacks a `=` key-value pattern)
- **011** — JSON-encoded password values not redacted (quoted `"password":"..."` syntax not matched)
- **012** — Redis URL with empty username (`:password@` host) not redacted (URL pattern required one-or-more chars before `:`)

TDD mandatory: tests written and confirmed failing before implementation.

---

## Work Completed

### 1. Tests (TDD — written before implementation)

Added 6 new test cases to `internal/domain/redact_test.go`:

- `finding 010: JWT bearer token uppercase` — `Authorization: Bearer eyJ...` → `Authorization: Bearer [REDACTED]`
- `finding 010: JWT bearer token lowercase` — `bearer eyJ...` → `bearer [REDACTED]`
- `finding 011: JSON password no space` — `{"password":"s3cr3t"}` → `{"password":"[REDACTED]"}`
- `finding 011: JSON password with space after colon` — `{"password": "hunter2"}` → `{"password": "[REDACTED]"}`
- `finding 011: JSON password case-insensitive` — `{"Password":"MySecret"}` → `{"Password":"[REDACTED]"}`
- `finding 012: Redis URL with empty username` — `redis://:s3cr3tpassword@host` → `redis://[REDACTED]@host`

All 6 tests confirmed failing before implementation.

### 2. Implementation (`internal/domain/redact.go`)

Three changes applied:

1. **URL pattern** (finding 012): `[^:@\s]+` → `[^:@\s]*` in the username position — allows zero characters before `:` so `:password@` is now matched.

2. **Bearer JWT pattern** (finding 010): Added `(?i)(bearer )\S+` → `${1}[REDACTED]` inserted **before** the base64 pattern. This is order-critical: if base64 ran first, the JWT header would be redacted as `[REDACTED-BASE64]` and the `bearer ` prefix would be lost.

3. **JSON password pattern** (finding 011): Added `(?i)("password"\s*:\s*)"[^"]*"` → `${1}"[REDACTED]"` inserted before the generic `password\s*[=:]` pattern. Matches only quoted JSON values (`"password": "value"`) to avoid false positives on other forms.

---

## Key Decisions

- **Bearer pattern position**: Must precede base64 pattern. A JWT header like `eyJhbGciOiJSUzI1NiJ9` is valid base64 >40 chars. If base64 ran first, the `bearer ` prefix would survive unmatched and the token would be redacted as `[REDACTED-BASE64]` instead of the intended `bearer [REDACTED]`.
- **JSON password pattern specificity**: Used `"password"` (with quotes) to match only JSON object keys, not arbitrary `password:` YAML or env-var assignments (which are already covered by the generic password pattern below it).
- **URL fix is minimal**: Only the `+` → `*` quantifier change for the pre-colon username segment. All other URL pattern behaviour is preserved.
- **No other patterns changed**: The three new/modified patterns are the minimum set of changes.

---

## Blockers

None.

---

## Tests Run

```
# Before implementation — 6 failures confirmed:
go test -timeout 30s -race ./internal/domain/...
FAIL github.com/lenaxia/k8s-mendabot/internal/domain

# After implementation — all pass:
go test -timeout 30s -race ./internal/domain/...
ok  github.com/lenaxia/k8s-mendabot/internal/domain

# Full suite — no regressions:
go test -timeout 30s -race ./...
ok  github.com/lenaxia/k8s-mendabot/api/v1alpha1
ok  github.com/lenaxia/k8s-mendabot/cmd/watcher
ok  github.com/lenaxia/k8s-mendabot/internal
ok  github.com/lenaxia/k8s-mendabot/internal/cascade
ok  github.com/lenaxia/k8s-mendabot/internal/circuitbreaker
ok  github.com/lenaxia/k8s-mendabot/internal/config
ok  github.com/lenaxia/k8s-mendabot/internal/controller
ok  github.com/lenaxia/k8s-mendabot/internal/domain
ok  github.com/lenaxia/k8s-mendabot/internal/jobbuilder
ok  github.com/lenaxia/k8s-mendabot/internal/logging
ok  github.com/lenaxia/k8s-mendabot/internal/metrics
ok  github.com/lenaxia/k8s-mendabot/internal/provider
ok  github.com/lenaxia/k8s-mendabot/internal/provider/native
```

---

## Next Steps

Epic 12 redaction gaps closed. No further work required for findings 010–012.

---

## Files Modified

- `internal/domain/redact.go` — three pattern changes (URL quantifier, bearer pattern added, JSON password pattern added)
- `internal/domain/redact_test.go` — 6 new test cases added (findings 010, 011, 012)
- `docs/WORKLOGS/0050_2026-02-23_epic12-redact-gap-fixes.md` — this file
- `docs/WORKLOGS/README.md` — index updated
