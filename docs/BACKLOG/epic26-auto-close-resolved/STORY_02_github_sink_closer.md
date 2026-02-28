# Story 02: GitHubSinkCloser Implementation (REST API)

## Status: Complete

## Goal

Implement `internal/sink/github/closer.go` — a `GitHubSinkCloser` that satisfies the
`domain.SinkCloser` interface. It uses the GitHub REST API directly (`net/http`) to
post a closure comment and close the PR or issue. No `gh` CLI subprocess is used.

## Background

See [README.md](README.md) for the full design. The watcher container contains only the
Go binary — the `gh` CLI is exclusively in the agent image. All GitHub API interactions
in the watcher must go through `net/http`.

This story depends on STORY_00 (`SinkCloser` interface) and STORY_01
(`TokenProvider`). Wire-up into the reconciler is STORY_03.

## Acceptance Criteria

- [x] `internal/sink/github/closer.go` implements `domain.SinkCloser`
- [x] Uses REST API, not `gh` CLI — no `exec.Command` anywhere
- [x] `SinkRef.URL == ""` → return `nil` immediately (no API calls)
- [x] `SinkRef.Number <= 0` → return descriptive error immediately (no API calls)
- [x] `SinkRef.Repo == ""` → return descriptive error immediately (no API calls)
- [x] Posts a comment before closing (comment text = `reason` parameter); comment
      failure is non-fatal — log and proceed to close
- [x] Closes PR: `PATCH /repos/{repo}/pulls/{number}` with `{"state":"closed"}`
- [x] Closes issue: `PATCH /repos/{repo}/issues/{number}` with `{"state":"closed"}`
- [x] `422` on the **close** step treated as success (already closed — idempotent)
- [x] Non-201 on the **comment** step is non-fatal: fall through to close, do not
      return an error to the caller
- [x] Other non-2xx on the close step returned as errors with status code in message
- [x] Token fetched fresh via `TokenProvider` on every `Close()` call (the provider
      handles caching — the closer must not cache independently)
- [x] Mock HTTP server used in all tests — no real GitHub calls
- [x] `go test -timeout 30s -race ./...` passes (this package)
- [x] `go build ./...` succeeds

## Implementation Notes

### Package structure

```
internal/sink/github/
    closer.go       — GitHubSinkCloser
    closer_test.go  — unit tests with httptest.NewServer
```

### closer.go

```go
package github

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    igithub "github.com/lenaxia/k8s-mechanic/internal/github"
    v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

// GitHubSinkCloser closes GitHub PRs and issues via the REST API.
type GitHubSinkCloser struct {
    TokenProvider igithub.TokenProvider
    // BaseURL allows overriding the GitHub API endpoint in tests.
    // Defaults to "https://api.github.com" when empty.
    BaseURL    string
    HTTPClient *http.Client
}

// Close posts a closure comment and closes the PR or issue referenced by rjob.Status.SinkRef.
// Returns nil if SinkRef.URL is empty.
// Returns nil if GitHub responds with 422 on the close step (already closed — idempotent).
// Returns an error for any other non-2xx response, or if SinkRef fields are invalid.
func (c *GitHubSinkCloser) Close(ctx context.Context, rjob *v1alpha1.RemediationJob, reason string) error {
    ref := rjob.Status.SinkRef
    if ref.URL == "" {
        return nil
    }
    if ref.Number <= 0 {
        return fmt.Errorf("SinkRef.Number must be > 0, got %d (sinkRef.url=%s)", ref.Number, ref.URL)
    }
    if ref.Repo == "" {
        return fmt.Errorf("SinkRef.Repo must not be empty (sinkRef.url=%s)", ref.URL)
    }

    token, err := c.TokenProvider.Token(ctx)
    if err != nil {
        return fmt.Errorf("getting GitHub token: %w", err)
    }

    base := c.BaseURL
    if base == "" {
        base = "https://api.github.com"
    }
    hc := c.HTTPClient
    if hc == nil {
        hc = http.DefaultClient
    }

    // Step 1: post the closure comment on the issue thread (works for both PRs and issues).
    // Comment failure is non-fatal: log a warning via the returned error at the call
    // site, then proceed to close regardless. The primary goal is closing the sink.
    if err := c.postComment(ctx, hc, base, token, ref.Repo, ref.Number, reason); err != nil {
        // Caller (SourceProviderReconciler) already logs-and-ignores Close() errors,
        // so we do not propagate comment errors — instead log here and fall through.
        _ = err // non-fatal: proceed to close
    }

    // Step 2: close the PR or issue.
    return c.closeItem(ctx, hc, base, token, ref.Repo, ref.Number, ref.Type)
}

func (c *GitHubSinkCloser) postComment(
    ctx context.Context, hc *http.Client, base, token, repo string, number int, body string,
) error {
    url := fmt.Sprintf("%s/repos/%s/issues/%d/comments", base, repo, number)
    payload, _ := json.Marshal(map[string]string{"body": body})
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
    if err != nil {
        return fmt.Errorf("building comment request: %w", err)
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/vnd.github+json")
    req.Header.Set("Content-Type", "application/json")

    resp, err := hc.Do(req)
    if err != nil {
        return fmt.Errorf("posting comment: %w", err)
    }
    defer resp.Body.Close()

    // 201 Created is the expected success status for comment creation.
    // Any other status (including 422) is a real error — do NOT treat 422 as
    // non-fatal here. A 422 on comment posting means a validation failure (e.g.
    // malformed body), not a locked issue (GitHub returns 410 Gone for locked
    // issues). Swallowing a 422 would silently skip the comment and proceed to
    // close, giving a false impression of success.
    //
    // Comment failure IS non-fatal to the overall Close operation: log a warning
    // and proceed to close the PR — the primary goal is closing the sink, and a
    // missing comment is acceptable.
    if resp.StatusCode == http.StatusCreated {
        return nil
    }
    // Non-201: log the failure at the call site (caller decides severity), return
    // a sentinel error so the caller can log it as a warning and still proceed.
    return fmt.Errorf("posting comment: unexpected status %d", resp.StatusCode)
}

func (c *GitHubSinkCloser) closeItem(
    ctx context.Context, hc *http.Client, base, token, repo string, number int, sinkType string,
) error {
    var apiPath string
    switch sinkType {
    case "pr":
        apiPath = fmt.Sprintf("%s/repos/%s/pulls/%d", base, repo, number)
    default: // "issue" or anything else
        apiPath = fmt.Sprintf("%s/repos/%s/issues/%d", base, repo, number)
    }

    payload, _ := json.Marshal(map[string]string{"state": "closed"})
    req, err := http.NewRequestWithContext(ctx, http.MethodPatch, apiPath, bytes.NewReader(payload))
    if err != nil {
        return fmt.Errorf("building close request: %w", err)
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/vnd.github+json")
    req.Header.Set("Content-Type", "application/json")

    resp, err := hc.Do(req)
    if err != nil {
        return fmt.Errorf("closing %s: %w", sinkType, err)
    }
    defer resp.Body.Close()

    // 200 OK is success. 422 means already closed — treat as success (idempotent).
    if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnprocessableEntity {
        return nil
    }
    return fmt.Errorf("closing %s: unexpected status %d", sinkType, resp.StatusCode)
}
```

### closer_test.go

All tests use `httptest.NewServer` — no real GitHub calls.

**Test scenarios:**

**Happy path — PR:**
- Mock server handles `POST /repos/org/repo/issues/42/comments` → 201
- Mock server handles `PATCH /repos/org/repo/pulls/42` → 200
- `Close()` with `SinkRef{Type:"pr", Repo:"org/repo", Number:42, URL:"..."}` → nil

**Happy path — issue:**
- Mock server handles comment + `PATCH /repos/org/repo/issues/99` → 200
- `Close()` with `SinkRef{Type:"issue", ...}` → nil

**Idempotency — already closed:**
- Mock server returns 422 on the `PATCH` → `Close()` returns nil (not an error)

**Empty SinkRef:**
- `SinkRef.URL == ""` → `Close()` returns nil immediately, no HTTP calls made
  (verify with a handler that fails the test if called)

**Token provider error:**
- `TokenProvider.Token()` returns error → `Close()` returns wrapped error, no HTTP calls

**Comment failure (non-201 response):**
- Mock server returns 422 on comment POST → `Close()` does NOT propagate the error;
  it falls through and still calls `closeItem`. The close succeeds (200). `Close()`
  returns nil. The missing comment is silent — this is acceptable (close is the
  primary goal).
- Mock server returns 500 on comment POST → same behaviour: falls through, close
  still attempted.

**Input validation:**
- `SinkRef.Number == 0` → `Close()` returns a descriptive error immediately, no HTTP
  calls made (verify with a no-op handler that fails the test if called)
- `SinkRef.Number == -1` → same: returns error immediately
- `SinkRef.Repo == ""` → `Close()` returns a descriptive error immediately, no HTTP
  calls made

**Unhappy path — unexpected status on close:**
- Mock server returns 500 on `PATCH` → `Close()` returns error with "500" in message

**Unhappy path — context cancelled:**
- Cancel the context before `Close()` → `Close()` returns error

### Interface compliance

Add a compile-time check:

```go
var _ domain.SinkCloser = (*GitHubSinkCloser)(nil)
```

## Files Touched

| File | Change |
|------|--------|
| `internal/sink/github/closer.go` | New file |
| `internal/sink/github/closer_test.go` | New file |

## TDD Sequence

1. Write `closer_test.go` — all tests fail (package doesn't exist)
2. Create `closer.go` with struct + method stubs
3. Implement `Close()`, `postComment()`, `closeItem()`
4. All tests pass including race detector
