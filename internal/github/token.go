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

	mu          sync.Mutex
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

// ForceExpiry expires the cached token immediately, forcing a re-exchange on the
// next Token() call. Used only in tests.
func (p *GitHubAppTokenProvider) ForceExpiry() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expiresAt = time.Now().Add(-time.Second)
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
