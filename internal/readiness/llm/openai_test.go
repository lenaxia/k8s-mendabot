package llm_test

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mechanic/internal/readiness/llm"
)

const testNamespace = "mechanic"

func newFakeClient(objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

// validLLMSecret returns a correctly structured llm-credentials-<agentType>
// Secret using the new opaque config blob schema.
func validLLMSecret(agentType string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llm-credentials-" + agentType,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"provider-config": []byte(`{"model":"test","providers":{"openai":{"apiKey":"sk-test","baseURL":"https://api.openai.com/v1"}}}`),
		},
	}
}

// --- OpenAIChecker secret validation ---

func TestOpenAIChecker_Name(t *testing.T) {
	c := newFakeClient()
	checker := llm.NewOpenAIChecker(c, testNamespace, "opencode")
	if checker.Name() != "llm/openai" {
		t.Errorf("Name() = %q, want %q", checker.Name(), "llm/openai")
	}
}

func TestOpenAIChecker_FailsWhenSecretMissing(t *testing.T) {
	c := newFakeClient()
	checker := llm.NewOpenAIChecker(c, testNamespace, "opencode")

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error when secret is missing")
	}
	if !strings.Contains(err.Error(), "llm-credentials-opencode") {
		t.Errorf("error should mention secret name, got: %v", err)
	}
}

func TestOpenAIChecker_SecretNameDerivedFromAgentType(t *testing.T) {
	tests := []struct {
		agentType      string
		wantSecretName string
	}{
		{"opencode", "llm-credentials-opencode"},
		{"claude", "llm-credentials-claude"},
	}
	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			// Secret for the wrong agent type — checker should not find it.
			wrongSecret := validLLMSecret("other-agent")
			c := newFakeClient(wrongSecret)
			checker := llm.NewOpenAIChecker(c, testNamespace, tt.agentType)

			err := checker.Check(context.Background())
			if err == nil {
				t.Fatal("expected error: correct secret not present")
			}
			if !strings.Contains(err.Error(), tt.wantSecretName) {
				t.Errorf("error should mention %q, got: %v", tt.wantSecretName, err)
			}

			// Now add the correct secret — checker should pass.
			correctSecret := validLLMSecret(tt.agentType)
			c2 := newFakeClient(correctSecret)
			checker2 := llm.NewOpenAIChecker(c2, testNamespace, tt.agentType)
			if err := checker2.Check(context.Background()); err != nil {
				t.Errorf("expected no error with correct secret, got: %v", err)
			}
		})
	}
}

func TestOpenAIChecker_FailsWhenRequiredKeyMissing(t *testing.T) {
	for _, missingKey := range []string{"provider-config"} {
		t.Run("missing_"+missingKey, func(t *testing.T) {
			secret := validLLMSecret("opencode")
			delete(secret.Data, missingKey)
			c := newFakeClient(secret)
			checker := llm.NewOpenAIChecker(c, testNamespace, "opencode")

			err := checker.Check(context.Background())
			if err == nil {
				t.Fatalf("expected error when key %q is missing", missingKey)
			}
			if !strings.Contains(err.Error(), missingKey) {
				t.Errorf("error should mention missing key %q, got: %v", missingKey, err)
			}
		})
	}
}

func TestOpenAIChecker_FailsWhenKeyEmpty(t *testing.T) {
	for _, emptyKey := range []string{"provider-config"} {
		t.Run("empty_"+emptyKey, func(t *testing.T) {
			secret := validLLMSecret("opencode")
			secret.Data[emptyKey] = []byte("")
			c := newFakeClient(secret)
			checker := llm.NewOpenAIChecker(c, testNamespace, "opencode")

			err := checker.Check(context.Background())
			if err == nil {
				t.Fatalf("expected error when key %q is empty", emptyKey)
			}
		})
	}
}

func TestOpenAIChecker_PassesWithAllRequiredKeys(t *testing.T) {
	secret := validLLMSecret("opencode")
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace, "opencode")

	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("expected no error with all keys present, got: %v", err)
	}
}

// TestOpenAIChecker_PassesWithoutModelKey verifies that the readiness checker
// only requires "provider-config" and does NOT require the "model" key.
// The model is embedded inside the opaque provider-config blob; requiring a
// separate "model" key is dead weight that misleads operators.
// This test FAILS until requiredLLMKeys in openai.go drops "model".
func TestOpenAIChecker_PassesWithoutModelKey(t *testing.T) {
	secret := validLLMSecret("opencode")
	delete(secret.Data, "model")
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace, "opencode")

	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("checker should pass without 'model' key (model is in provider-config blob), got: %v", err)
	}
}

// --- BedrockChecker stub ---

func TestBedrockChecker_AlwaysReturnsNotImplemented(t *testing.T) {
	c := newFakeClient()
	checker := llm.NewBedrockChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("BedrockChecker should always return an error (not implemented)")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error should say 'not yet implemented', got: %v", err)
	}
}

func TestBedrockChecker_Name(t *testing.T) {
	c := newFakeClient()
	if llm.NewBedrockChecker(c, testNamespace).Name() != "llm/bedrock" {
		t.Error("BedrockChecker.Name() should return 'llm/bedrock'")
	}
}

// --- VertexChecker stub ---

func TestVertexChecker_AlwaysReturnsNotImplemented(t *testing.T) {
	c := newFakeClient()
	checker := llm.NewVertexChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("VertexChecker should always return an error (not implemented)")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error should say 'not yet implemented', got: %v", err)
	}
}

func TestVertexChecker_Name(t *testing.T) {
	c := newFakeClient()
	if llm.NewVertexChecker(c, testNamespace).Name() != "llm/vertex" {
		t.Error("VertexChecker.Name() should return 'llm/vertex'")
	}
}
