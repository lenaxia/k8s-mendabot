package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	igithub "github.com/lenaxia/k8s-mendabot/internal/github"
)

// Compile-time interface check.
var _ domain.SinkCloser = (*GitHubSinkCloser)(nil)

// GitHubSinkCloser closes GitHub PRs and issues via the REST API.
// No gh CLI subprocess is used — the watcher container does not contain it.
type GitHubSinkCloser struct {
	TokenProvider igithub.TokenProvider
	// BaseURL allows overriding the GitHub API endpoint in tests.
	// Defaults to "https://api.github.com" when empty.
	BaseURL    string
	HTTPClient *http.Client
}

// Close posts a closure comment and closes the PR or issue referenced by
// rjob.Status.SinkRef.
//
// Returns nil if SinkRef.URL is empty (no sink to close).
// Returns nil if the item is already closed (fully idempotent — no duplicate comment).
// Returns nil if GitHub responds with 422 on the close step (already closed).
// Returns an error for any other non-2xx response, or if SinkRef fields are invalid.
//
// Comment failure is non-fatal: the comment error is discarded and Close proceeds to
// close the item. The primary goal is closing the sink; a missing comment is acceptable.
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

	// Check whether the item is already closed before doing anything.
	// This makes Close fully idempotent: repeated calls on an already-closed
	// PR/issue produce no side-effects (no duplicate comment, no wasted API call).
	already, err := c.isClosed(ctx, hc, base, token, ref.Repo, ref.Number)
	if err != nil {
		// Non-fatal: if we can't determine state, proceed optimistically.
		already = false
	}
	if already {
		return nil
	}

	// Step 1: post the closure comment. Failure is non-fatal — fall through.
	_ = c.postComment(ctx, hc, base, token, ref.Repo, ref.Number, reason)

	// Step 2: close the PR or issue.
	return c.closeItem(ctx, hc, base, token, ref.Repo, ref.Number, ref.Type)
}

// isClosed returns true if the GitHub issue/PR is already in "closed" state.
// It uses the issues API (works for both PRs and plain issues).
// Errors are returned to the caller, which treats them as non-fatal.
func (c *GitHubSinkCloser) isClosed(
	ctx context.Context, hc *http.Client, base, token, repo string, number int,
) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/issues/%d", base, repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("building state request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := hc.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetching issue state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return false, fmt.Errorf("fetching issue state: unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("decoding issue state: %w", err)
	}
	return payload.State == "closed", nil
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

	if resp.StatusCode == http.StatusCreated {
		return nil
	}
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
