package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	sinkhub "github.com/lenaxia/k8s-mendabot/internal/sink/github"
)

// Compile-time interface check.
var _ domain.SinkCloser = (*sinkhub.GitHubSinkCloser)(nil)

// staticTokenProvider is a test double for TokenProvider.
type staticTokenProvider struct {
	token string
	err   error
}

func (s *staticTokenProvider) Token(_ context.Context) (string, error) {
	return s.token, s.err
}

// routeHandler stores per-path handlers for the mock server.
type routeHandler struct {
	handlers map[string]http.HandlerFunc
}

func (r *routeHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	key := req.Method + " " + req.URL.Path
	if h, ok := r.handlers[key]; ok {
		h(w, req)
		return
	}
	http.Error(w, fmt.Sprintf("unexpected request: %s %s", req.Method, req.URL.Path), http.StatusInternalServerError)
}

func newCloser(t *testing.T, token string, mux *routeHandler) (*sinkhub.GitHubSinkCloser, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	closer := &sinkhub.GitHubSinkCloser{
		TokenProvider: &staticTokenProvider{token: token},
		BaseURL:       srv.URL,
		HTTPClient:    srv.Client(),
	}
	return closer, srv
}

func TestGitHubSinkCloser_HappyPath_PR(t *testing.T) {
	t.Parallel()
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/42": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"open"}`))
		},
		"POST /repos/org/repo/issues/42/comments": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
		"PATCH /repos/org/repo/pulls/42": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["state"] != "closed" {
				http.Error(w, "bad state", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{
				Type:   "pr",
				URL:    "https://github.com/org/repo/pull/42",
				Number: 42,
				Repo:   "org/repo",
			},
		},
	}
	if err := closer.Close(context.Background(), rjob, "resolved"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitHubSinkCloser_HappyPath_Issue(t *testing.T) {
	t.Parallel()
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/99": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"open"}`))
		},
		"POST /repos/org/repo/issues/99/comments": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
		"PATCH /repos/org/repo/issues/99": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{
				Type:   "issue",
				URL:    "https://github.com/org/repo/issues/99",
				Number: 99,
				Repo:   "org/repo",
			},
		},
	}
	if err := closer.Close(context.Background(), rjob, "resolved"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitHubSinkCloser_Idempotent_422OnClose(t *testing.T) {
	t.Parallel()
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/42": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"open"}`))
		},
		"POST /repos/org/repo/issues/42/comments": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
		"PATCH /repos/org/repo/pulls/42": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity) // already closed
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	if err := closer.Close(context.Background(), rjob, "reason"); err != nil {
		t.Fatalf("expected nil for 422 on close (idempotent), got: %v", err)
	}
}

func TestGitHubSinkCloser_EmptySinkRef_NoHTTPCalls(t *testing.T) {
	t.Parallel()
	called := false
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"POST /any": func(w http.ResponseWriter, r *http.Request) { called = true },
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{} // zero SinkRef
	if err := closer.Close(context.Background(), rjob, "reason"); err != nil {
		t.Fatalf("expected nil for empty SinkRef, got: %v", err)
	}
	if called {
		t.Error("HTTP call made when SinkRef.URL is empty")
	}
}

func TestGitHubSinkCloser_TokenProviderError(t *testing.T) {
	t.Parallel()
	httpCalled := false
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"POST /any": func(w http.ResponseWriter, r *http.Request) { httpCalled = true },
	}}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	closer := &sinkhub.GitHubSinkCloser{
		TokenProvider: &staticTokenProvider{err: fmt.Errorf("auth failed")},
		BaseURL:       srv.URL,
		HTTPClient:    srv.Client(),
	}
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	err := closer.Close(context.Background(), rjob, "reason")
	if err == nil {
		t.Fatal("expected error when token provider fails")
	}
	if httpCalled {
		t.Error("HTTP call made despite token provider error")
	}
}

func TestGitHubSinkCloser_CommentFailure_StillCloses(t *testing.T) {
	t.Parallel()
	closeCalled := false
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/42": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"open"}`))
		},
		"POST /repos/org/repo/issues/42/comments": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity) // comment fails
		},
		"PATCH /repos/org/repo/pulls/42": func(w http.ResponseWriter, r *http.Request) {
			closeCalled = true
			w.WriteHeader(http.StatusOK)
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	if err := closer.Close(context.Background(), rjob, "reason"); err != nil {
		t.Fatalf("Close should succeed even if comment fails, got: %v", err)
	}
	if !closeCalled {
		t.Error("PATCH (close) was not called despite comment failure")
	}
}

func TestGitHubSinkCloser_Comment500_StillCloses(t *testing.T) {
	t.Parallel()
	closeCalled := false
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/42": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"open"}`))
		},
		"POST /repos/org/repo/issues/42/comments": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
		"PATCH /repos/org/repo/pulls/42": func(w http.ResponseWriter, r *http.Request) {
			closeCalled = true
			w.WriteHeader(http.StatusOK)
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	if err := closer.Close(context.Background(), rjob, "reason"); err != nil {
		t.Fatalf("expected nil despite comment 500, got: %v", err)
	}
	if !closeCalled {
		t.Error("PATCH not called")
	}
}

func TestGitHubSinkCloser_InvalidNumber_Zero(t *testing.T) {
	t.Parallel()
	httpCalled := false
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"ANY": func(w http.ResponseWriter, r *http.Request) { httpCalled = true },
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "https://github.com/org/repo/pull/0", Number: 0, Repo: "org/repo"},
		},
	}
	err := closer.Close(context.Background(), rjob, "reason")
	if err == nil {
		t.Fatal("expected error for Number=0")
	}
	if httpCalled {
		t.Error("HTTP call made despite invalid Number")
	}
}

func TestGitHubSinkCloser_InvalidNumber_Negative(t *testing.T) {
	t.Parallel()
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: -1, Repo: "org/repo"},
		},
	}
	if err := closer.Close(context.Background(), rjob, "reason"); err == nil {
		t.Fatal("expected error for Number=-1")
	}
}

func TestGitHubSinkCloser_InvalidRepo_Empty(t *testing.T) {
	t.Parallel()
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: ""},
		},
	}
	if err := closer.Close(context.Background(), rjob, "reason"); err == nil {
		t.Fatal("expected error for empty Repo")
	}
}

func TestGitHubSinkCloser_UnexpectedStatusOnClose(t *testing.T) {
	t.Parallel()
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/42": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"open"}`))
		},
		"POST /repos/org/repo/issues/42/comments": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
		"PATCH /repos/org/repo/pulls/42": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	err := closer.Close(context.Background(), rjob, "reason")
	if err == nil {
		t.Fatal("expected error for 500 on close")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected '500' in error message, got: %v", err)
	}
}

func TestGitHubSinkCloser_CancelledContext(t *testing.T) {
	t.Parallel()
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{}}
	closer, _ := newCloser(t, "tok", mux)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before call

	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	err := closer.Close(ctx, rjob, "reason")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestGitHubSinkCloser_AlreadyClosed_NoCommentNoClose verifies that Close is
// fully idempotent when the GitHub API reports the item is already closed:
// neither a comment nor a PATCH call is made.
func TestGitHubSinkCloser_AlreadyClosed_NoCommentNoClose(t *testing.T) {
	t.Parallel()
	commentCalled := false
	closeCalled := false
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/42": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"closed"}`))
		},
		"POST /repos/org/repo/issues/42/comments": func(w http.ResponseWriter, r *http.Request) {
			commentCalled = true
			w.WriteHeader(http.StatusCreated)
		},
		"PATCH /repos/org/repo/pulls/42": func(w http.ResponseWriter, r *http.Request) {
			closeCalled = true
			w.WriteHeader(http.StatusOK)
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	if err := closer.Close(context.Background(), rjob, "reason"); err != nil {
		t.Fatalf("expected nil for already-closed item, got: %v", err)
	}
	if commentCalled {
		t.Error("comment was posted on an already-closed PR — duplicate comment bug")
	}
	if closeCalled {
		t.Error("PATCH was called on an already-closed PR — wasted API call")
	}
}

// TestGitHubSinkCloser_IsClosedError_FallsThrough verifies that a failure in
// the isClosed GET call is treated as non-fatal: Close proceeds to post the
// comment and close the item anyway.
func TestGitHubSinkCloser_IsClosedError_FallsThrough(t *testing.T) {
	t.Parallel()
	closeCalled := false
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/issues/42": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError) // isClosed fails
		},
		"POST /repos/org/repo/issues/42/comments": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
		"PATCH /repos/org/repo/pulls/42": func(w http.ResponseWriter, r *http.Request) {
			closeCalled = true
			w.WriteHeader(http.StatusOK)
		},
	}}
	closer, _ := newCloser(t, "tok", mux)
	rjob := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			SinkRef: v1alpha1.SinkRef{Type: "pr", URL: "u", Number: 42, Repo: "org/repo"},
		},
	}
	if err := closer.Close(context.Background(), rjob, "reason"); err != nil {
		t.Fatalf("expected nil when isClosed GET fails (non-fatal), got: %v", err)
	}
	if !closeCalled {
		t.Error("PATCH was not called despite isClosed error — Close should fall through")
	}
}
