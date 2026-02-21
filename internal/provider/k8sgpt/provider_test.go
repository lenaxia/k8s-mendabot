package k8sgpt

import (
	"testing"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: K8sGPTProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*K8sGPTProvider)(nil)

func TestProviderName_Panics(t *testing.T) {
	p := &K8sGPTProvider{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, got none")
		}
	}()
	_ = p.ProviderName()
}

func TestObjectType_Panics(t *testing.T) {
	p := &K8sGPTProvider{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, got none")
		}
	}()
	_ = p.ObjectType()
}

func TestExtractFinding_Panics(t *testing.T) {
	p := &K8sGPTProvider{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, got none")
		}
	}()
	_, _ = p.ExtractFinding(nil)
}

func TestFingerprint_Panics(t *testing.T) {
	p := &K8sGPTProvider{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, got none")
		}
	}()
	_ = p.Fingerprint(&domain.Finding{})
}
