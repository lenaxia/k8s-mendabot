package llm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mendabot/internal/readiness/llm"
)

const testNamespace = "mendabot"

func newFakeClient(objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func validLLMSecret(baseURL string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llm-credentials",
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"api-key":         []byte("sk-test-key"),
			"base-url":        []byte(baseURL),
			"model":           []byte("gpt-4o"),
			"kube-api-server": []byte("https://kubernetes.default.svc"),
		},
	}
}

// --- OpenAIChecker secret validation ---

func TestOpenAIChecker_Name(t *testing.T) {
	c := newFakeClient()
	checker := llm.NewOpenAIChecker(c, testNamespace)
	if checker.Name() != "llm/openai" {
		t.Errorf("Name() = %q, want %q", checker.Name(), "llm/openai")
	}
}

func TestOpenAIChecker_FailsWhenSecretMissing(t *testing.T) {
	c := newFakeClient()
	checker := llm.NewOpenAIChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error when secret is missing")
	}
	if !strings.Contains(err.Error(), "llm-credentials") {
		t.Errorf("error should mention secret name, got: %v", err)
	}
}

func TestOpenAIChecker_FailsWhenKeyMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Only the three LLM endpoint keys are validated; kube-api-server is not
	// an LLM credential and is not the responsibility of this checker.
	for _, missingKey := range []string{"api-key", "base-url", "model"} {
		t.Run("missing_"+missingKey, func(t *testing.T) {
			secret := validLLMSecret(srv.URL)
			delete(secret.Data, missingKey)
			c := newFakeClient(secret)
			checker := llm.NewOpenAIChecker(c, testNamespace)

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

// --- OpenAIChecker /models probe ---

func TestOpenAIChecker_PassesOnHTTP200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing or malformed Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	secret := validLLMSecret(srv.URL)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("expected no error on 200, got: %v", err)
	}
}

func TestOpenAIChecker_PassesOnHTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	secret := validLLMSecret(srv.URL)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("expected no error on 404 (endpoint not implemented), got: %v", err)
	}
}

func TestOpenAIChecker_PassesOnHTTP405(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	secret := validLLMSecret(srv.URL)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("expected no error on 405 (endpoint not implemented), got: %v", err)
	}
}

func TestOpenAIChecker_FailsOnHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	secret := validLLMSecret(srv.URL)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

func TestOpenAIChecker_FailsOn401WithCredentialMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	secret := validLLMSecret(srv.URL)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error on 401")
	}
	// Error must mention the credential secret so the operator knows what to fix.
	if !strings.Contains(err.Error(), "api-key") {
		t.Errorf("401 error should mention api-key, got: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("401 error should mention status code, got: %v", err)
	}
}

func TestOpenAIChecker_FailsOn403WithCredentialMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	secret := validLLMSecret(srv.URL)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("403 error should mention status code, got: %v", err)
	}
}

func TestOpenAIChecker_FailsOnUnreachableEndpoint(t *testing.T) {
	// Use a closed server to simulate a connection-refused error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	secret := validLLMSecret(url)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error when endpoint is unreachable")
	}
}

func TestOpenAIChecker_RespectsContextCancellation(t *testing.T) {
	// Slow server that holds the connection open long enough for the context
	// to be cancelled before a response is returned.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	secret := validLLMSecret(srv.URL)
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	err := checker.Check(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	// The error must be a context error, not a generic probe failure string.
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context error (DeadlineExceeded or Canceled), got: %v", err)
	}
}

func TestOpenAIChecker_StripsTrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// base-url with trailing slash — should still probe /models not //models
	secret := validLLMSecret(srv.URL + "/")
	c := newFakeClient(secret)
	checker := llm.NewOpenAIChecker(c, testNamespace)

	if err := checker.Check(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/models" {
		t.Errorf("probe path = %q, want %q", gotPath, "/models")
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
