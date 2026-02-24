package llm

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VertexChecker is the readiness checker for GCP Vertex AI LLM endpoints.
// It validates the vertex-credentials Secret and probes the Vertex AI API.
//
// This checker is a stub — it is not yet implemented. It is present as an
// extension point so that LLM_PROVIDER=vertex is recognised at startup and
// fails loudly rather than silently falling through.
type VertexChecker struct {
	client    client.Client
	namespace string
}

// NewVertexChecker returns a VertexChecker that reads secrets from namespace.
func NewVertexChecker(c client.Client, namespace string) *VertexChecker {
	return &VertexChecker{client: c, namespace: namespace}
}

func (v *VertexChecker) Name() string { return "llm/vertex" }

// Check is not yet implemented. It always returns an error directing the
// operator to track the implementation issue.
func (v *VertexChecker) Check(_ context.Context) error {
	return errors.New("llm/vertex: checker not yet implemented; " +
		"set LLM_PROVIDER=openai or leave LLM_PROVIDER unset to disable the LLM check")
}
