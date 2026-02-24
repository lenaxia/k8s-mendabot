package llm

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BedrockChecker is the readiness checker for AWS Bedrock LLM endpoints.
// It validates the bedrock-credentials Secret and probes the Bedrock API.
//
// This checker is a stub — it is not yet implemented. It is present as an
// extension point so that LLM_PROVIDER=bedrock is recognised at startup and
// fails loudly rather than silently falling through.
type BedrockChecker struct {
	client    client.Client
	namespace string
}

// NewBedrockChecker returns a BedrockChecker that reads secrets from namespace.
func NewBedrockChecker(c client.Client, namespace string) *BedrockChecker {
	return &BedrockChecker{client: c, namespace: namespace}
}

func (b *BedrockChecker) Name() string { return "llm/bedrock" }

// Check is not yet implemented. It always returns an error directing the
// operator to track the implementation issue.
func (b *BedrockChecker) Check(_ context.Context) error {
	return errors.New("llm/bedrock: checker not yet implemented; " +
		"set LLM_PROVIDER=openai or leave LLM_PROVIDER unset to disable the LLM check")
}
