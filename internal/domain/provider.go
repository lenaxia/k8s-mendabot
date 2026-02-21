package domain

import "sigs.k8s.io/controller-runtime/pkg/client"

// SourceProvider is implemented by any component that watches an external resource
// and can produce a normalised Finding from it.
//
// The SourceProvider does NOT create RemediationJob objects directly — that is the
// responsibility of SourceProviderReconciler, which calls ExtractFinding() and owns
// the creation logic.
type SourceProvider interface {
	// ProviderName returns a stable, lowercase identifier for this provider.
	// Used as the value of RemediationJobSpec.SourceType (e.g. "k8sgpt", "prometheus").
	// Must be unique across all registered providers.
	ProviderName() string

	// ObjectType returns a pointer to the runtime.Object type this provider watches.
	// Used by SourceProviderReconciler to register the correct informer.
	ObjectType() client.Object

	// ExtractFinding converts a watched object into a Finding.
	// Returns (nil, nil) if the object should be skipped (e.g. no errors present).
	// Returns (nil, err) for transient errors that should trigger a requeue.
	ExtractFinding(obj client.Object) (*Finding, error)

	// Fingerprint computes the deduplication key for the given Finding.
	// Must be deterministic: same logical finding always produces the same fingerprint.
	Fingerprint(f *Finding) string
}

// Finding is the provider-agnostic representation of a cluster problem.
// All source providers must map their native type to this struct.
// The RemediationJob spec is populated directly from this struct.
type Finding struct {
	// Kind is the Kubernetes resource kind affected (e.g. "Pod", "Deployment").
	Kind string

	// Name is the plain resource name (no namespace prefix).
	Name string

	// Namespace is the namespace of the affected resource.
	Namespace string

	// ParentObject is the logical owner (e.g. "my-deployment" for a crashing pod).
	// Used as the deduplication anchor. If there is no meaningful parent, use Name.
	ParentObject string

	// Errors is a pre-serialised, redacted JSON string of error descriptions.
	// Format: [{"text":"..."},{"text":"..."}]
	// Sensitive fields must be stripped by the provider before populating this field.
	Errors string

	// Details is a human-readable explanation of the finding (e.g. k8sgpt LLM analysis).
	// May be empty.
	Details string

	// SourceRef identifies the native object that produced this Finding.
	SourceRef SourceRef
}

// SourceRef is a back-reference to the native object that produced a Finding.
type SourceRef struct {
	// APIVersion of the native object (e.g. "core.k8sgpt.ai/v1alpha1").
	APIVersion string

	// Kind of the native object (e.g. "Result").
	Kind string

	// Name of the native object.
	Name string

	// Namespace of the native object.
	Namespace string
}

// SinkConfig holds the configuration passed to the agent for a specific sink.
// Populated from watcher env vars and mounted Secrets; injected as Job env vars.
type SinkConfig struct {
	// Type identifies the sink implementation (e.g. "github", "gitlab", "gitea").
	// Injected as SINK_TYPE env var into the agent Job.
	Type string

	// AdditionalEnv holds sink-specific env vars (e.g. GITLAB_HOST for a GitLab sink).
	// These are injected alongside the standard FINDING_* vars.
	AdditionalEnv map[string]string
}
