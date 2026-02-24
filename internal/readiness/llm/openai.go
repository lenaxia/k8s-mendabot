// Package llm provides readiness checkers for LLM backend dependencies.
// Each checker validates both that the required Kubernetes Secret exists with
// the expected keys and that the LLM endpoint is reachable and responsive.
//
// Checkers are selected by the LLM_PROVIDER config field:
//
//	LLM_PROVIDER=openai   → OpenAIChecker
//	LLM_PROVIDER=bedrock  → reserved (not yet implemented; rejected at startup)
//	LLM_PROVIDER=vertex   → reserved (not yet implemented; rejected at startup)
//	(unset)               → NopChecker (gate disabled)
package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	llmCredentialsSecretName = "llm-credentials"
	modelsProbeTimeout       = 10 * time.Second
)

// requiredOpenAIKeys are the keys that must be present and non-empty in the
// llm-credentials Secret for the OpenAI-compatible provider.
// Only the keys directly needed to reach and authenticate the LLM endpoint
// are validated here. Other keys co-located in the secret (e.g. kube-api-server)
// belong to different concerns and are not the responsibility of this checker.
var requiredOpenAIKeys = []string{"api-key", "base-url", "model"}

// OpenAIChecker validates the llm-credentials Secret for an OpenAI-compatible
// endpoint and confirms the endpoint is reachable by probing GET <base-url>/models.
//
// Probe semantics:
//   - 2xx → healthy
//   - 404/405 → healthy (server is up, /models endpoint not implemented)
//   - 401/403 → credential error (secret key is wrong); fails with a clear message
//   - 5xx, timeout, network error → unhealthy
//   - context cancelled/deadline exceeded → propagated as-is (not a readiness failure)
type OpenAIChecker struct {
	client     client.Client
	namespace  string
	httpClient *http.Client
}

// NewOpenAIChecker returns an OpenAIChecker that reads secrets from namespace.
func NewOpenAIChecker(c client.Client, namespace string) *OpenAIChecker {
	return &OpenAIChecker{
		client:    c,
		namespace: namespace,
		httpClient: &http.Client{
			Timeout: modelsProbeTimeout,
		},
	}
}

func (o *OpenAIChecker) Name() string { return "llm/openai" }

// Check validates the llm-credentials Secret then probes GET <base-url>/models.
func (o *OpenAIChecker) Check(ctx context.Context) error {
	apiKey, baseURL, err := o.readCredentials(ctx)
	if err != nil {
		return err
	}
	return o.probeModels(ctx, baseURL, apiKey)
}

// readCredentials reads and validates the llm-credentials Secret, returning
// the api-key and base-url values on success.
func (o *OpenAIChecker) readCredentials(ctx context.Context) (apiKey, baseURL string, err error) {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: o.namespace, Name: llmCredentialsSecretName}
	if err := o.client.Get(ctx, key, &secret); err != nil {
		return "", "", fmt.Errorf("llm/openai: secret %q not found in namespace %q: %w",
			llmCredentialsSecretName, o.namespace, err)
	}

	for _, k := range requiredOpenAIKeys {
		v, ok := secret.Data[k]
		if !ok || len(v) == 0 {
			return "", "", fmt.Errorf("llm/openai: secret %q is missing required key %q",
				llmCredentialsSecretName, k)
		}
	}

	return string(secret.Data["api-key"]), string(secret.Data["base-url"]), nil
}

// probeModels sends GET <baseURL>/models and returns an error if the endpoint
// is not reachable or returns a server error.
func (o *OpenAIChecker) probeModels(ctx context.Context, baseURL, apiKey string) error {
	url := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("llm/openai: failed to build probe request for %q: %w", url, err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		// Propagate context cancellation/deadline cleanly rather than wrapping
		// it as an LLM probe failure (e.g. during controller shutdown).
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("llm/openai: probe GET %q failed: %w", url, err)
	}
	// Always drain and close the body so the underlying TCP connection is
	// returned to the pool, even when we do not read the response body.
	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()              //nolint:errcheck
	}()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// Healthy.
		return nil
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		// Server is up but /models is not implemented — treat as healthy.
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// Server is reachable but the API key is invalid.
		return fmt.Errorf("llm/openai: probe GET %q returned %d — check the api-key value in secret %q",
			url, resp.StatusCode, llmCredentialsSecretName)
	default:
		return fmt.Errorf("llm/openai: probe GET %q returned unexpected status %d",
			url, resp.StatusCode)
	}
}
