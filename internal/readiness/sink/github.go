// Package sink provides readiness checkers for RemediationJob sink dependencies.
// Each checker validates that the credentials required for a specific sink type
// (GitHub App, GitLab token, etc.) are present and correctly formed before the
// watcher is permitted to create RemediationJob objects.
package sink

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	githubAppSecretName = "github-app" //nolint:gosec // G101 false positive: this is a Kubernetes Secret name, not a credential value
)

// requiredGitHubAppKeys are the keys that must be present and non-empty in the
// github-app Secret. Their values are not inspected — the agent validates the
// token exchange at runtime.
var requiredGitHubAppKeys = []string{"app-id", "installation-id", "private-key"}

// GitHubAppChecker validates that the github-app Kubernetes Secret exists in
// the agent namespace and contains all required keys with non-empty values.
//
// No network call is made — secret presence is the readiness contract here.
// The agent's init container performs the actual GitHub App token exchange and
// will fail with a clear error if the credentials are wrong.
type GitHubAppChecker struct {
	client    client.Client
	namespace string
}

// NewGitHubAppChecker returns a GitHubAppChecker that reads from the given namespace.
func NewGitHubAppChecker(c client.Client, namespace string) *GitHubAppChecker {
	return &GitHubAppChecker{client: c, namespace: namespace}
}

func (g *GitHubAppChecker) Name() string { return "sink/github-app" }

// Check reads the github-app Secret and asserts all required keys are present
// and non-empty. Returns a descriptive error for any missing or empty key.
func (g *GitHubAppChecker) Check(ctx context.Context) error {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: g.namespace, Name: githubAppSecretName}
	if err := g.client.Get(ctx, key, &secret); err != nil {
		return fmt.Errorf("sink/github-app: secret %q not found in namespace %q: %w",
			githubAppSecretName, g.namespace, err)
	}

	for _, k := range requiredGitHubAppKeys {
		v, ok := secret.Data[k]
		if !ok || len(v) == 0 {
			return fmt.Errorf("sink/github-app: secret %q is missing required key %q",
				githubAppSecretName, k)
		}
	}

	return nil
}
