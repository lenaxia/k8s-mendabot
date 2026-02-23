# Story 01: Secret Value Redaction in Finding.Errors

**Epic:** [epic12-security-review](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot operator**, I want credentials and other sensitive values to be stripped
from error text before they are stored in `RemediationJob.Spec.Finding.Errors` or injected
into the agent Job environment, so that a pod failure message containing a database URL
or API key does not expose that credential to anyone who can read the Job spec or logs.

---

## Background

All six native providers construct `Finding.Errors` from Kubernetes status fields that
may contain credentials. For example:

- `pod.go:buildWaitingText()` (line 141) includes `cs.State.Waiting.Message` verbatim.
  A container that fails to start because `DATABASE_URL` is wrong may log the full URL
  including password in `State.Waiting.Message`.
- `node.go:buildNodeConditionText()` (line 116) includes `cond.Message` verbatim.
- `job.go` (line 113) includes `cond.Message` verbatim.

`Finding.Errors` flows to `RemediationJob.Spec.Finding.Errors` in
`SourceProviderReconciler.Reconcile()` (`internal/provider/provider.go` line 307), then to
the `FINDING_ERRORS` env var in the agent Job via `jobbuilder/job.go` line 122.
Anyone with `kubectl describe job -n mendabot` access can read the raw env var value.

The comment in `domain.Finding` already acknowledges this:
```go
// Errors is a pre-serialised, redacted JSON string of error descriptions.
// Sensitive fields must be stripped by the provider before populating this field.
```
No provider currently implements this requirement.

---

## Acceptance Criteria

- [ ] `internal/domain/redact.go` contains `RedactSecrets(text string) string`
- [ ] `RedactSecrets` applies the patterns defined in §Technical Implementation
- [ ] Each native provider calls `RedactSecrets` on every error text string before
      appending to its `errors` slice (before `json.Marshal`)
- [ ] `internal/domain/redact_test.go` covers: URL credentials, `password=`, `token=`,
      `secret=`, `api-key=`, base64 strings ≥ 40 chars, and non-matching clean text
- [ ] All tests pass: `go test -timeout 30s -race ./internal/domain/...`
- [ ] Redaction is documented as best-effort in a code comment — no false guarantee

---

## Technical Implementation

### New file: `internal/domain/redact.go`

```go
package domain

import "regexp"

// redactPatterns is the ordered list of patterns applied by RedactSecrets.
// Each pattern is applied in sequence; the result of one is the input to the next.
var redactPatterns = []struct {
    re          *regexp.Regexp
    replacement string
}{
    // URL credentials: scheme://user:pass@host
    {regexp.MustCompile(`(?i)://[^:@\s]+:[^@\s]+@`), `://[REDACTED]@`},
    // password=value or password: value
    {regexp.MustCompile(`(?i)(password\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
    // token=value or token: value
    {regexp.MustCompile(`(?i)(token\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
    // secret=value or secret: value
    {regexp.MustCompile(`(?i)(secret\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
    // api-key=value or api_key=value or apikey=value
    {regexp.MustCompile(`(?i)(api[_-]?key\s*[=:]\s*)\S+`), `${1}[REDACTED]`},
    // base64-looking strings >= 40 chars (likely encoded credentials or tokens)
    {regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`), `[REDACTED-BASE64]`},
}

// RedactSecrets applies a set of heuristic patterns to strip credential-like values
// from error text before it is stored in Finding.Errors.
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
```

### Changes to native providers

In each of the six providers, every place an error text string is produced must be
wrapped in `domain.RedactSecrets(...)` before appending:

**`internal/provider/native/pod.go`**

`buildWaitingText` (line 141) and `buildCrashLoopText` (line 128) return the text; the
caller appends it. Apply redaction at the append site:

```go
// Before:
errors = append(errors, errorEntry{Text: buildWaitingText(cs)})

// After:
errors = append(errors, errorEntry{Text: domain.RedactSecrets(buildWaitingText(cs))})
```

And for the terminated exit-code case (line 82):
```go
// Before:
text := fmt.Sprintf("container %s: terminated with exit code %d", cs.Name, cs.State.Terminated.ExitCode)

// After — exit code is numeric, but message can contain credentials:
msg := cs.State.Terminated.Message
if msg != "" {
    msg = ": " + domain.RedactSecrets(msg)
}
text := fmt.Sprintf("container %s: terminated with exit code %d%s", cs.Name, cs.State.Terminated.ExitCode, msg)
```

Unschedulable message (line 94):
```go
text := fmt.Sprintf("pod %s: %s: %s", cond.Reason, domain.RedactSecrets(cond.Message))
```

**`internal/provider/native/node.go`**

`buildNodeConditionText` (line 116) includes `cond.Message`:
```go
func buildNodeConditionText(nodeName string, cond corev1.NodeCondition) string {
    return fmt.Sprintf("node %s has condition %s (%s): %s",
        nodeName, cond.Type, cond.Reason, domain.RedactSecrets(cond.Message))
}
```

**`internal/provider/native/job.go`**

`cond.Message` at line 113:
```go
condText := fmt.Sprintf("job %s: %s: %s", job.Name, cond.Reason, domain.RedactSecrets(cond.Message))
```

**`internal/provider/native/deployment.go`, `statefulset.go`, `pvc.go`**

Apply the same pattern: any `cond.Message` or `.Message` field used in an error text
string must be wrapped in `domain.RedactSecrets(...)`.

### Test file: `internal/domain/redact_test.go`

Table-driven tests covering every pattern, plus non-matching clean text:

```go
func TestRedactSecrets(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {
            name:  "URL credentials",
            input: "failed to connect to postgres://myuser:s3cr3tpass@db.example.com:5432/mydb",
            want:  "failed to connect to postgres://[REDACTED]@db.example.com:5432/mydb",
        },
        {
            name:  "password= assignment",
            input: "DATABASE_PASSWORD=hunter2 caused startup failure",
            want:  "DATABASE_PASSWORD=[REDACTED] caused startup failure",
        },
        {
            name:  "token= assignment",
            input: "GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz012345 rejected",
            want:  "GITHUB_TOKEN=[REDACTED] rejected",
        },
        {
            name:  "api-key= assignment",
            input: "api-key=sk-abc123xyz456longkeyvalue00000000",
            want:  "api-key=[REDACTED]",
        },
        {
            name:  "base64 string >= 40 chars",
            input: "value: c2VjcmV0dmFsdWV0aGF0aXNsb25nZW5vdWdodG9iZXJlZGFjdGVk",
            want:  "value: [REDACTED-BASE64]",
        },
        {
            name:  "base64 string < 40 chars not redacted",
            input: "short: YWJjZGVmZ2g=",
            want:  "short: YWJjZGVmZ2g=",
        },
        {
            name:  "clean text unchanged",
            input: "container app: CrashLoopBackOff (last exit: OOMKilled)",
            want:  "container app: CrashLoopBackOff (last exit: OOMKilled)",
        },
        {
            name:  "empty string",
            input: "",
            want:  "",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := domain.RedactSecrets(tt.input)
            if got != tt.want {
                t.Errorf("RedactSecrets(%q) = %q, want %q", tt.input, got, tt.want)
            }
        })
    }
}
```

---

## Tasks

- [ ] Write `TestRedactSecrets` in `internal/domain/redact_test.go` (TDD — run first, must fail)
- [ ] Implement `internal/domain/redact.go` with `RedactSecrets`
- [ ] Run tests — must pass
- [ ] Update `pod.go`: redact in `buildWaitingText` caller, `buildCrashLoopText` caller,
      terminated message, and unschedulable message
- [ ] Update `node.go`: redact `cond.Message` in `buildNodeConditionText`
- [ ] Update `job.go`: redact `cond.Message` at line 113
- [ ] Update `deployment.go`, `statefulset.go`, `pvc.go`: redact all `.Message` fields
      used in error text
- [ ] Run full suite: `go test -timeout 30s -race ./...`

---

## Dependencies

**Depends on:** epic09-native-provider (the six providers this story modifies)
**Blocks:** STORY_06 (pentest cannot validate this until it is implemented)

---

## Definition of Done

- [ ] `go test -timeout 30s -race ./internal/domain/...` passes
- [ ] `go test -timeout 30s -race ./internal/provider/native/...` passes
- [ ] `go vet ./...` clean
- [ ] `RedactSecrets` is documented as best-effort (not a guarantee)
- [ ] No provider constructs an error text string from a `.Message` field without
      wrapping in `domain.RedactSecrets`
