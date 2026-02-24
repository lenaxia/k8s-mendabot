package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FindingFingerprint computes the deduplication key for a Finding.
// It is a pure function — the same input always produces the same output.
//
// Algorithm:
//  1. Parse f.Errors (pre-serialised JSON) into []struct{ Text string }.
//     An empty string or "[]" are treated identically (zero texts).
//  2. Extract the Text field from each element and sort the resulting slice.
//  3. Build a payload struct containing Namespace, Kind, ParentObject, and
//     the sorted texts.
//  4. JSON-encode the payload with SetEscapeHTML(false) to avoid mangling
//     "<", ">", and "&" characters inside error texts.
//  5. Return the lowercase hex SHA256 of the encoded bytes (always 64 chars).
//
// Returns an error only if f.Errors is non-empty and not valid JSON, or if
// json.Encode fails (extremely unlikely in practice).
func FindingFingerprint(f *Finding) (string, error) {
	var failures []struct {
		Text string `json:"text"`
	}
	if f.Errors != "" {
		if err := json.Unmarshal([]byte(f.Errors), &failures); err != nil {
			return "", fmt.Errorf("FindingFingerprint: malformed errors JSON: %w", err)
		}
	}

	texts := make([]string, 0, len(failures))
	for _, fv := range failures {
		texts = append(texts, fv.Text)
	}
	sort.Strings(texts)

	payload := struct {
		Namespace    string   `json:"namespace"`
		Kind         string   `json:"kind"`
		ParentObject string   `json:"parentObject"`
		ErrorTexts   []string `json:"errorTexts"`
	}{
		Namespace:    f.Namespace,
		Kind:         f.Kind,
		ParentObject: f.ParentObject,
		ErrorTexts:   texts,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "", fmt.Errorf("FindingFingerprint: json.Encode failed: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(buf.Bytes())), nil
}

// SourceProvider is implemented by any component that watches an external resource
// and can produce a normalised Finding from it.
//
// The SourceProvider does NOT create RemediationJob objects directly — that is the
// responsibility of SourceProviderReconciler, which calls ExtractFinding() and owns
// the creation logic. Fingerprinting is performed by domain.FindingFingerprint, which
// is a pure function and not provider-specific.
type SourceProvider interface {
	// ProviderName returns a stable, lowercase identifier for this provider.
	// Used as the value of RemediationJobSpec.SourceType (e.g. "native", "prometheus").
	// Must be unique across all registered providers.
	ProviderName() string

	// ObjectType returns a pointer to the runtime.Object type this provider watches.
	// Used by SourceProviderReconciler to register the correct informer.
	ObjectType() client.Object

	// ExtractFinding converts a watched object into a Finding.
	// Returns (nil, nil) if the object should be skipped (e.g. no errors present).
	// Returns (nil, err) for transient errors that should trigger a requeue.
	ExtractFinding(obj client.Object) (*Finding, error)
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

	// Details is a human-readable explanation of the finding.
	// May be empty.
	Details string

	// SourceRef identifies the native object that produced this Finding.
	SourceRef SourceRef
}

// SourceRef is a back-reference to the native object that produced a Finding.
type SourceRef struct {
	// APIVersion of the native object (e.g. "v1", "apps/v1").
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
