# Story 03: Redactor Struct + Custom Pattern Support + redact Binary Propagation

**Epic:** [epic29-agent-hardening](README.md)
**Priority:** High
**Status:** Not Started
**Depends on:** STORY_02 (new built-in patterns — `Redactor` struct wraps them)

---

## User Story

As a **mendabot operator**, I want to define custom regex redaction patterns in
`values.yaml` that are applied by both the watcher's finding redaction and the `redact`
binary inside agent Job containers, so that organisation-specific credential formats are
sanitised before they can reach the LLM API.

---

## Background

`internal/domain/redact.go` currently exposes a single package-level function
`RedactSecrets(text string) string` backed by a hardcoded pattern slice. There is no way
to add patterns at runtime without recompiling the binary.

Two consumers need custom patterns:

1. **The watcher** — applies `domain.RedactSecrets` to `Finding.Errors`, `Finding.Details`,
   and correlated finding text in `internal/provider/native/*.go` before storing results
   in `RemediationJob` CR specs.

2. **The `redact` binary** (`cmd/redact/main.go`) — runs inside agent Job containers,
   filtering all tool call output via the shell wrapper pipeline.

Both need to compile and apply the same set of built-in + custom patterns. The cleanest
way to share this is a `Redactor` struct that holds compiled patterns and is instantiated
at startup with any extras. The existing `RedactSecrets` shim preserves all current call
sites with zero changes.

Custom patterns are validated as legal Go RE2 regexes at construction time. Invalid
patterns cause a startup error (watcher) or a logged warning + skip (redact binary, which
must not crash the agent Job over a misconfigured pattern).

---

## Acceptance Criteria

- [ ] `domain.New(extraPatterns []string) (*Redactor, error)` exists and compiles
      built-in patterns + extras
- [ ] `(*Redactor).Redact(text string) string` applies all patterns in order
      (built-in first, extras after, base64 catch-all always last among built-ins)
- [ ] `domain.RedactSecrets(text string) string` is preserved as a package-level function
      backed by a zero-extras `Redactor` — all existing call sites compile unchanged
- [ ] `domain.New` returns an error if any extra pattern is not a valid RE2 regex
- [ ] Extra patterns are applied **after** all built-in patterns and **before** the
      base64 catch-all — no, extra patterns are appended after all built-ins including
      the base64 catch-all (user patterns are additive, position is after the full
      built-in set)
- [ ] `cmd/redact/main.go` reads `EXTRA_REDACT_PATTERNS` env var (comma-separated),
      constructs a `Redactor` via `domain.New(extras)`, and applies it to stdin
- [ ] Invalid patterns in `EXTRA_REDACT_PATTERNS` are logged to stderr and skipped —
      the binary does not crash; remaining valid patterns are still applied
- [ ] The watcher initialises a `domain.Redactor` at startup from
      `cfg.ExtraRedactPatterns` and uses it in place of `domain.RedactSecrets` in the
      provider layer (see STORY_04 for config wiring)
- [ ] An invalid extra pattern in the watcher config causes a startup error (logged +
      fatal — the watcher should not start with broken redaction config)
- [ ] `go test -timeout 30s -race ./internal/domain/...` passes
- [ ] `go test -timeout 30s -race ./cmd/redact/...` passes

---

## Technical Implementation

### `internal/domain/redact.go` refactor

```go
package domain

import (
    "regexp"
    "strings"
)

// redactRule is a compiled find-replace pair.
type redactRule struct {
    re          *regexp.Regexp
    replacement string
}

// builtinRules holds the compiled built-in redaction patterns.
// Populated by init() so they are compiled once at package load.
var builtinRules []redactRule

func init() {
    builtinRules = compileRules(builtinPatterns)
}

// builtinPatterns is the ordered list of built-in pattern definitions.
// (Contains all 16 patterns from STORY_02 — existing 11 + 5 new.)
var builtinPatterns = []struct {
    pattern     string
    replacement string
}{
    // ... all 16 patterns as string literals ...
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
// Extra patterns are appended after the built-in set (including the base64 catch-all).
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
// All existing call sites in internal/provider/native/*.go use this — no changes needed.
func RedactSecrets(text string) string {
    return defaultRedactor.Redact(text)
}

// defaultRedactor is the zero-extras Redactor used by RedactSecrets.
var defaultRedactor = &Redactor{rules: builtinRules}
```

### `cmd/redact/main.go` update

```go
func main() {
    // Read extra patterns from env var (comma-separated).
    var extras []string
    if raw := os.Getenv("EXTRA_REDACT_PATTERNS"); raw != "" {
        for _, p := range strings.Split(raw, ",") {
            p = strings.TrimSpace(p)
            if p != "" {
                extras = append(extras, p)
            }
        }
    }

    // Build redactor; skip invalid patterns with a warning (never crash the agent Job).
    var validExtras []string
    for _, p := range extras {
        if _, err := regexp.Compile(p); err != nil {
            fmt.Fprintf(os.Stderr, "[redact] WARNING: skipping invalid pattern %q: %v\n", p, err)
            continue
        }
        validExtras = append(validExtras, p)
    }

    r, err := domain.New(validExtras)
    if err != nil {
        // Should not happen since we pre-validated above, but be safe.
        fmt.Fprintf(os.Stderr, "[redact] ERROR: failed to build redactor: %v\n", err)
        os.Exit(1)
    }

    input, err := io.ReadAll(os.Stdin)
    if err != nil {
        fmt.Fprintf(os.Stderr, "[redact] ERROR: reading stdin: %v\n", err)
        os.Exit(1)
    }

    fmt.Print(r.Redact(string(input)))
}
```

### Provider layer wiring (STORY_04 provides the config fields)

In `internal/provider/native/*.go`, all six call sites of `domain.RedactSecrets(text)`
are replaced with `r.redactor.Redact(text)` where `r.redactor` is a `*domain.Redactor`
injected at construction time. Each provider receives the `Redactor` from the watcher's
startup sequence (see STORY_04).

If STORY_04 is not yet complete, `domain.RedactSecrets` continues to work as before —
this is the backwards-compatible fallback.

### Custom pattern replacement label

All custom patterns use the replacement `[REDACTED-CUSTOM]` regardless of which pattern
matched. This is intentional:
- It distinguishes custom-pattern redactions from built-in ones in logs
- It avoids exposing the pattern itself (the label could hint at what was matched)
- Operators who need pattern-specific labels can use named capture groups in their
  patterns if they choose — but the default replacement is `[REDACTED-CUSTOM]`

---

## Test Cases

Add to `internal/domain/redact_test.go`:

```go
// Redactor struct tests
func TestNew(t *testing.T) {
    t.Run("no extras", func(t *testing.T) {
        r, err := domain.New(nil)
        require.NoError(t, err)
        // built-in patterns still work
        assert.Equal(t, "password: [REDACTED]", r.Redact("password: hunter2"))
    })

    t.Run("valid extra pattern", func(t *testing.T) {
        r, err := domain.New([]string{`CORP-[0-9]{8}`})
        require.NoError(t, err)
        assert.Equal(t, "id: [REDACTED-CUSTOM]", r.Redact("id: CORP-12345678"))
        // built-ins still apply
        assert.Equal(t, "token: [REDACTED]", r.Redact("token: abc123"))
    })

    t.Run("invalid extra pattern returns error", func(t *testing.T) {
        _, err := domain.New([]string{`[invalid`})
        require.Error(t, err)
        assert.Contains(t, err.Error(), "invalid extra redact pattern")
    })

    t.Run("empty string pattern skipped", func(t *testing.T) {
        r, err := domain.New([]string{"", "  "})
        require.NoError(t, err)
        assert.NotNil(t, r)
    })

    t.Run("RedactSecrets shim unchanged", func(t *testing.T) {
        // Existing call sites still work.
        assert.Equal(t, "password: [REDACTED]", domain.RedactSecrets("password: hunter2"))
    })
}
```

Add to `cmd/redact/main_test.go`:

```go
t.Run("EXTRA_REDACT_PATTERNS applied", func(t *testing.T) {
    t.Setenv("EXTRA_REDACT_PATTERNS", `CORP-[0-9]{8}`)
    out := runRedact(t, "id: CORP-12345678 and token: abc")
    assert.Contains(t, out, "[REDACTED-CUSTOM]")
    assert.Contains(t, out, "[REDACTED]") // token pattern still fires
})

t.Run("invalid EXTRA_REDACT_PATTERNS skipped, no crash", func(t *testing.T) {
    t.Setenv("EXTRA_REDACT_PATTERNS", `[invalid,CORP-[0-9]{8}`)
    // Should not panic; CORP pattern still applied
    out := runRedact(t, "CORP-12345678")
    assert.Contains(t, out, "[REDACTED-CUSTOM]")
})
```

---

## Definition of Done

- [ ] `domain.New(extraPatterns []string) (*Redactor, error)` implemented
- [ ] `(*Redactor).Redact(text string) string` implemented
- [ ] `domain.RedactSecrets` preserved as a shim — all existing call sites compile
      and behave identically
- [ ] `cmd/redact/main.go` reads `EXTRA_REDACT_PATTERNS` and uses `domain.New`
- [ ] Invalid patterns in `EXTRA_REDACT_PATTERNS` warn to stderr but do not crash
- [ ] All new test cases pass
- [ ] All existing test cases pass (no regressions)
- [ ] `go test -timeout 30s -race ./internal/domain/... ./cmd/redact/...` passes
- [ ] `go vet ./...` passes
