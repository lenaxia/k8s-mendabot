package github_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	igithub "github.com/lenaxia/k8s-mechanic/internal/github"
)

// generateTestKey creates a small RSA key suitable for unit tests only.
func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	return key
}

// newMockTokenServer returns an httptest server that responds to the installation
// access_tokens endpoint. If statusCode != 201 it returns that code with no body.
// If token is empty and statusCode == 201 it returns `{"token":""}`.
func newMockTokenServer(t *testing.T, token string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusCreated {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	}))
}

func TestGitHubAppTokenProvider_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newMockTokenServer(t, "ghs_test_token", http.StatusCreated)
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "ghs_test_token" {
		t.Errorf("expected 'ghs_test_token', got %q", tok)
	}
}

func TestGitHubAppTokenProvider_CachesToken(t *testing.T) {
	t.Parallel()
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "cached_token"})
	}))
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	// First call — should hit the server.
	tok1, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	// Second call — must NOT hit the server.
	tok2, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if tok1 != tok2 {
		t.Errorf("tokens differ: %q vs %q", tok1, tok2)
	}
	if n := atomic.LoadInt32(&callCount); n != 1 {
		t.Errorf("expected exactly 1 server call, got %d", n)
	}
}

func TestGitHubAppTokenProvider_RefreshesOnExpiry(t *testing.T) {
	t.Parallel()
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "token_" + string(rune('A'+n-1))})
	}))
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	// Prime the cache.
	_, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Force expiry by setting ExpiresAt to the past.
	p.ForceExpiry()

	// Second call after expiry must hit the server again.
	tok2, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if n := atomic.LoadInt32(&callCount); n != 2 {
		t.Errorf("expected 2 server calls, got %d", n)
	}
	_ = tok2
}

func TestGitHubAppTokenProvider_Non201Error(t *testing.T) {
	t.Parallel()
	srv := newMockTokenServer(t, "", http.StatusForbidden)
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestGitHubAppTokenProvider_EmptyTokenInResponse(t *testing.T) {
	t.Parallel()
	srv := newMockTokenServer(t, "", http.StatusCreated)
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
}

func TestGitHubAppTokenProvider_MalformedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestGitHubAppTokenProvider_CancelledContext(t *testing.T) {
	t.Parallel()
	// Server that blocks until test is done — context cancellation should fire first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Token(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestGitHubAppTokenProvider_ConcurrentCallsSingleExchange(t *testing.T) {
	t.Parallel()
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Small sleep to make the race window wider.
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "concurrent_token"})
	}))
	defer srv.Close()

	p := &igithub.GitHubAppTokenProvider{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     generateTestKey(t),
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	}

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	tokens := make([]string, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tok, err := p.Token(context.Background())
			errs[idx] = err
			tokens[idx] = tok
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
	// Due to mutex, exactly 1 exchange should have fired.
	if n := atomic.LoadInt32(&callCount); n != 1 {
		t.Errorf("expected exactly 1 server call with concurrent goroutines, got %d", n)
	}
}
