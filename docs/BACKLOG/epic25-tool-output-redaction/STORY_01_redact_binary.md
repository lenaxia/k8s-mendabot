# Story 01: `cmd/redact` Filter Binary

**Epic:** [epic25-tool-output-redaction](README.md)
**Priority:** Critical
**Status:** Complete

---

## User Story

As a **mendabot operator**, I want all tool call output from the LLM agent to be passed
through the same `RedactSecrets` function used at source, so that credentials appearing
in `kubectl`, `helm`, or other tool output are stripped before the LLM context is updated
and before any data leaves the cluster to an external LLM API.

---

## Background

`internal/domain.RedactSecrets` is the single source of truth for redaction patterns.
It is already used by all six native providers at source. The `cmd/redact` binary makes
the same function available as a Unix filter: reads stdin, writes redacted stdout.

By importing `internal/domain` directly, the binary and all wrappers share the exact
same compiled regex patterns. There is no way for the patterns to diverge between the
source-level redaction and the tool-output redaction.

---

## Acceptance Criteria

- [x] `cmd/redact/main.go` exists; reads all of stdin, applies `domain.RedactSecrets`,
      writes to stdout; exits 0 on success, exits 1 only on I/O error (stdin read failure
      or stdout write failure)
- [x] `cmd/redact/main_test.go` covers all cases listed in §Test Cases below (TDD)
- [x] `go test -timeout 30s -race ./cmd/redact/...` passes
- [x] `go build ./cmd/redact` succeeds
- [x] Binary is a standalone static binary (CGO_ENABLED=0) suitable for COPY in Dockerfile

---

## Technical Implementation

### New file: `cmd/redact/main.go`

`main()` is a one-liner that delegates to `run(r, w)`. `run` is the testable unit.
`os.Exit` must only appear in `main()`, never in `run()` — calling `os.Exit` in a test
kills the entire test process with no recovery.

```go
package main

import (
    "io"
    "os"

    "github.com/lenaxia/k8s-mendabot/internal/domain"
)

func run(r io.Reader, w io.Writer) error {
    input, err := io.ReadAll(r)
    if err != nil {
        return err
    }
    _, err = io.WriteString(w, domain.RedactSecrets(string(input)))
    return err
}

func main() {
    if err := run(os.Stdin, os.Stdout); err != nil {
        os.Exit(1)
    }
}
```

The binary:
- Reads all of stdin into memory (full buffering — required for multi-line PEM key patterns)
- Applies `domain.RedactSecrets` (all patterns including multi-line PEM)
- Writes redacted output to stdout
- Exits 0 on success, 1 only if stdin read fails or stdout write fails
- No trailing newline added or removed — output byte-for-byte matches input except for
  redacted tokens

### New file: `cmd/redact/main_test.go`

Tests call `run(strings.NewReader(input), &bytes.Buffer{})` directly — no exec, no
process spawning, no `os.Exit` risk. Table-driven.

---

## Test Cases

| Case | Input | Expected output |
|------|-------|----------------|
| Secret in kubectl output | `password: hunter2` | `password: [REDACTED]` |
| Bearer token in header | `Authorization: Bearer ghp_abc123...` | `Authorization: Bearer [REDACTED]` |
| GH token standalone (no bearer/token= prefix) | `ghs_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` | `[REDACTED-GH-TOKEN]` |
| GH Actions token standalone | `gha_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` | `[REDACTED-GH-TOKEN]` |
| PEM key block (multi-line) | `-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----` | `[REDACTED-PEM-KEY]` |
| Base64 secret value (≥40 chars) | `data: dGhpcyBpcyBhIHNlY3JldA==AAAAAAAAAAAAAAAAAAA` | `data: [REDACTED-BASE64]` |
| Base64 exactly 40 chars (boundary) | `data: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` | `data: [REDACTED-BASE64]` |
| Base64 exactly 39 chars (not redacted) | `data: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` | `data: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` |
| No secrets — passthrough | `pod app: CrashLoopBackOff` | `pod app: CrashLoopBackOff` |
| Empty input | `` | `` |
| Large input (>50KB) | 60KB of clean text | 60KB of clean text unchanged |
| Multiple patterns in one chunk | password= and token= on same line | both redacted |
| URL credentials | `postgres://user:pass@db:5432/mydb` | `postgres://[REDACTED]@db:5432/mydb` |

---

## Definition of Done

- [x] `cmd/redact/main.go` written **after** `cmd/redact/main_test.go` (TDD)
- [x] All test cases pass
- [x] `go test -timeout 30s -race ./cmd/redact/...` passes
- [x] `go build ./cmd/redact` succeeds
- [x] `go vet ./cmd/redact/...` reports no issues
