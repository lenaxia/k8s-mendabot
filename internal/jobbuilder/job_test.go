package jobbuilder

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

func TestNew_EmptyAgentNamespace_ReturnsError(t *testing.T) {
	_, err := New(Config{AgentNamespace: ""})
	if err == nil {
		t.Fatal("expected error when AgentNamespace is empty, got nil")
	}
}

func TestNew_ValidConfig_Succeeds(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mendabot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil Builder")
	}
}

func TestBuild_PanicsNotImplemented(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mendabot"})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic, got none")
		}
	}()

	_, _ = b.Build(&v1alpha1.RemediationJob{})
}

// Ensure the return type is correct — Build returns (*batchv1.Job, error).
var _ func(*v1alpha1.RemediationJob) (*batchv1.Job, error) = (*Builder)(nil).Build
