# Story 01: GitHub App Token Provider in Watcher

## Status: Complete

## Goal

Implement `internal/github/token.go` — a `TokenProvider` that exchanges a GitHub App
private key for a short-lived installation token and caches the result for 55 minutes.
This is the authentication substrate used by `GitHubSinkCloser` (STORY_02) and
eventually by epic27.

## Background

The agent container already has `get-github-app-token.sh` which performs this exchange
at job startup. The watcher needs the same capability in-process so it can make GitHub
REST API calls (auto-close PRs) without shelling out to a subprocess.

The GitHub App token exchange flow:
1. Mint a JWT signed with the App private key (RS256, 10-minute expiry)
2. `POST /app/installations/{installation_id}/access_tokens` with the JWT as Bearer →
   returns `{"token": "ghs_...", "expires_at": "..."}` (1 hour validity)
3. Cache the token for 55 minutes; re-exchange before expiry

## Acceptance Criteria

- [x] `internal/github/token.go` defines `TokenProvider` interface and
      `GitHubAppTokenProvider` struct
- [x] `GitHubAppTokenProvider` reads App ID, Installation ID, and PEM private key from
      configurable fields (not hard-coded env var reads — those live in `main.go`)
- [x] Token is cached in-memory for 55 minutes; re-exchanged automatically when stale
- [x] JWT signing uses `RS256` with `golang-jwt/jwt`
- [x] Mock HTTP server used in tests — no real GitHub calls
- [x] `go test -timeout 30s -race ./...` passes (this package)
- [x] `go build ./...` succeeds

## Implementation Notes

### Package structure

```
internal/github/
    token.go       — TokenProvider interface + GitHubAppTokenProvider
    token_test.go  — unit tests with httptest.NewServer mock
```

No other files in this package yet (the closer lives in `internal/sink/github/`).

### token.go

```go
package github

import (
    "context"
    "crypto/rsa"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

const tokenCacheTTL = 55 * time.Minute

// TokenProvider returns a valid GitHub installation token.
// Implementations are safe for concurrent use.
type TokenProvider interface {
    Token(ctx context.Context) (string, error)
}

// GitHubAppTokenProvider exchanges a GitHub App private key for installation tokens.
// Tokens are cached for 55 minutes and re-exchanged automatically.
type GitHubAppTokenProvider struct {
    AppID          int64
    InstallationID int64
    PrivateKey     *rsa.PrivateKey
    // BaseURL allows overriding the GitHub API endpoint in tests.
    // Defaults to "https://api.github.com" when empty.
    BaseURL    string
    HTTPClient *http.Client

    mu         sync.Mutex
    cachedToken string
    expiresAt   time.Time
}

// Token returns a valid installation token, using the cache when possible.
func (p *GitHubAppTokenProvider) Token(ctx context.Context) (string, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.cachedToken != "" && time.Now().Before(p.expiresAt) {
        return p.cachedToken, nil
    }
    tok, err := p.exchange(ctx)
    if err != nil {
        return "", err
    }
    p.cachedToken = tok
    p.expiresAt = time.Now().Add(tokenCacheTTL)
    return tok, nil
}

func (p *GitHubAppTokenProvider) exchange(ctx context.Context) (string, error) {
    // 1. Mint a JWT valid for 10 minutes.
    now := time.Now()
    claims := jwt.RegisteredClaims{
        IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // 60s clock skew buffer
        ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
        Issuer:    fmt.Sprintf("%d", p.AppID),
    }
    token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
    signed, err := token.SignedString(p.PrivateKey)
    if err != nil {
        return "", fmt.Errorf("signing JWT: %w", err)
    }

    // 2. Exchange JWT for installation token.
    base := p.BaseURL
    if base == "" {
        base = "https://api.github.com"
    }
    url := fmt.Sprintf("%s/app/installations/%d/access_tokens", base, p.InstallationID)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
    if err != nil {
        return "", fmt.Errorf("building token request: %w", err)
    }
    req.Header.Set("Authorization", "Bearer "+signed)
    req.Header.Set("Accept", "application/vnd.github+json")

    hc := p.HTTPClient
    if hc == nil {
        hc = http.DefaultClient
    }
    resp, err := hc.Do(req)
    if err != nil {
        return "", fmt.Errorf("token exchange request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return "", fmt.Errorf("token exchange: unexpected status %d", resp.StatusCode)
    }

    var body struct {
        Token string `json:"token"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        return "", fmt.Errorf("decoding token response: %w", err)
    }
    if body.Token == "" {
        return "", fmt.Errorf("token exchange: empty token in response")
    }
    return body.Token, nil
}
```

### token_test.go

Test scenarios (table-driven where possible):

**Happy path:**
- `Token()` returns the token from the mock server on first call
- `Token()` returns the cached token on the second call (mock server must NOT be called
  a second time — verify with a call counter)
- Token is re-exchanged after cache expiry (set `expiresAt` to `time.Now().Add(-1s)`
  to simulate stale cache, then call `Token()` again)

**Unhappy paths:**
- Mock server returns non-201 → `Token()` returns error with status code in message
- Mock server returns `{"token":""}` → `Token()` returns error
- Mock server returns malformed JSON → `Token()` returns error
- `http.NewRequestWithContext` with cancelled context → `Token()` returns error

**Concurrency:**
- Multiple goroutines call `Token()` concurrently; only one exchange should fire
  (use `-race` to detect data races)

### Dependencies

Add to `go.mod`:
```
github.com/golang-jwt/jwt/v5
```

Run `go mod tidy` after adding.

## Files Touched

| File | Change |
|------|--------|
| `internal/github/token.go` | New file |
| `internal/github/token_test.go` | New file |
| `go.mod` / `go.sum` | Add `github.com/golang-jwt/jwt/v5` |

## TDD Sequence

1. Write `token_test.go` — all tests fail (package doesn't exist)
2. Create `internal/github/token.go` with `TokenProvider` interface and
   `GitHubAppTokenProvider` skeleton
3. Implement `exchange()` and `Token()` with cache
4. All tests pass including race detector
