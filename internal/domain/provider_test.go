package domain_test

import (
	"testing"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// TestFinding_ZeroValue verifies the zero value of Finding is safe (no panic, zero values).
func TestFinding_ZeroValue(t *testing.T) {
	var f domain.Finding
	if f.Kind != "" {
		t.Errorf("zero value Finding.Kind should be empty, got %q", f.Kind)
	}
	if f.Errors != "" {
		t.Errorf("zero value Finding.Errors should be empty, got %q", f.Errors)
	}
}

// TestSourceRef_ZeroValue verifies the zero value of SourceRef is safe.
func TestSourceRef_ZeroValue(t *testing.T) {
	var ref domain.SourceRef
	if ref.APIVersion != "" {
		t.Errorf("zero value SourceRef.APIVersion should be empty, got %q", ref.APIVersion)
	}
}

// TestFinding_EmptyParentObject verifies that a Finding with an empty ParentObject
// is safe to construct and access — no panic, zero value behaviour.
func TestFinding_EmptyParentObject(t *testing.T) {
	f := domain.Finding{
		Kind:         "Pod",
		Name:         "orphan-pod",
		Namespace:    "default",
		ParentObject: "",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
	}
	if f.ParentObject != "" {
		t.Errorf("expected empty ParentObject, got %q", f.ParentObject)
	}
}

// TestFinding_EmptyErrors verifies that a Finding with an empty Errors string
// is safe to construct and access — no panic, zero value behaviour.
func TestFinding_EmptyErrors(t *testing.T) {
	f := domain.Finding{
		Kind:      "Deployment",
		Name:      "my-deploy",
		Namespace: "default",
		Errors:    "",
	}
	if f.Errors != "" {
		t.Errorf("expected empty Errors, got %q", f.Errors)
	}
}

// TestSinkConfig_FieldsExist verifies the SinkConfig struct can be constructed with all fields.
func TestSinkConfig_FieldsExist(t *testing.T) {
	cfg := domain.SinkConfig{
		Type: "github",
		AdditionalEnv: map[string]string{
			"GITHUB_HOST": "github.example.com",
		},
	}
	if cfg.Type != "github" {
		t.Errorf("SinkConfig.Type: got %q, want %q", cfg.Type, "github")
	}
	if cfg.AdditionalEnv["GITHUB_HOST"] != "github.example.com" {
		t.Errorf("SinkConfig.AdditionalEnv[GITHUB_HOST]: got %q, want %q",
			cfg.AdditionalEnv["GITHUB_HOST"], "github.example.com")
	}
}

// TestSinkConfig_ZeroValue verifies the zero value of SinkConfig is safe.
func TestSinkConfig_ZeroValue(t *testing.T) {
	var cfg domain.SinkConfig
	if cfg.Type != "" {
		t.Errorf("zero value SinkConfig.Type should be empty, got %q", cfg.Type)
	}
	if cfg.AdditionalEnv != nil {
		t.Errorf("zero value SinkConfig.AdditionalEnv should be nil, got %v", cfg.AdditionalEnv)
	}
}
