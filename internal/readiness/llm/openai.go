// Package llm provides readiness checkers for LLM backend dependencies.
// Each checker validates that the required Kubernetes Secret exists with the
// expected keys. Secret names and key names are derived from the agentType
// (e.g. "llm-credentials-opencode" for agentType="opencode").
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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// requiredLLMKeys are the keys that must be present and non-empty in the
// llm-credentials-<agentType> Secret.
//
//   - provider-config: opaque JSON blob passed directly to the agent runner.
//     The model, endpoint URL, and all provider-specific config are embedded
//     inside this blob. mechanic never interprets its contents.
var requiredLLMKeys = []string{"provider-config"}

// OpenAIChecker validates the per-agent-type LLM credentials Secret.
// The secret name is "llm-credentials-<agentType>" and must contain the key
// listed in requiredLLMKeys. Endpoint reachability is not probed because
// the endpoint URL is embedded inside the opaque provider-config blob and
// cannot be extracted without interpreting provider-specific JSON schemas.
type OpenAIChecker struct {
	client     client.Client
	namespace  string
	secretName string
}

// NewOpenAIChecker returns an OpenAIChecker that reads the secret
// "llm-credentials-<agentType>" from namespace.
func NewOpenAIChecker(c client.Client, namespace, agentType string) *OpenAIChecker {
	return &OpenAIChecker{
		client:     c,
		namespace:  namespace,
		secretName: "llm-credentials-" + agentType,
	}
}

func (o *OpenAIChecker) Name() string { return "llm/openai" }

// Check validates that the llm-credentials-<agentType> Secret exists and
// contains all required keys with non-empty values.
func (o *OpenAIChecker) Check(ctx context.Context) error {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: o.namespace, Name: o.secretName}
	if err := o.client.Get(ctx, key, &secret); err != nil {
		return fmt.Errorf("llm/openai: secret %q not found in namespace %q: %w",
			o.secretName, o.namespace, err)
	}

	for _, k := range requiredLLMKeys {
		v, ok := secret.Data[k]
		if !ok || len(v) == 0 {
			return fmt.Errorf("llm/openai: secret %q is missing required key %q",
				o.secretName, k)
		}
	}

	return nil
}
