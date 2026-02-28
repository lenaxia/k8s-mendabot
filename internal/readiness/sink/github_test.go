package sink_test

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

	"github.com/lenaxia/k8s-mechanic/internal/readiness/sink"
)

const testNamespace = "mechanic"

func newFakeClient(objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func validGitHubAppSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "github-app",
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"app-id":          []byte("123456"),
			"installation-id": []byte("78901234"),
			"private-key":     []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIE..."),
		},
	}
}

func TestGitHubAppChecker_PassesWithValidSecret(t *testing.T) {
	secret := validGitHubAppSecret()
	c := newFakeClient(secret)
	checker := sink.NewGitHubAppChecker(c, testNamespace)

	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("expected no error with valid secret, got: %v", err)
	}
}

func TestGitHubAppChecker_FailsWhenSecretMissing(t *testing.T) {
	c := newFakeClient()
	checker := sink.NewGitHubAppChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error when secret is missing")
	}
	if !strings.Contains(err.Error(), "github-app") {
		t.Errorf("error should mention secret name, got: %v", err)
	}
}

func TestGitHubAppChecker_FailsWhenKeyMissing(t *testing.T) {
	for _, missingKey := range []string{"app-id", "installation-id", "private-key"} {
		t.Run("missing_"+missingKey, func(t *testing.T) {
			secret := validGitHubAppSecret()
			delete(secret.Data, missingKey)
			c := newFakeClient(secret)
			checker := sink.NewGitHubAppChecker(c, testNamespace)

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

func TestGitHubAppChecker_FailsWhenKeyEmpty(t *testing.T) {
	secret := validGitHubAppSecret()
	secret.Data["private-key"] = []byte{}
	c := newFakeClient(secret)
	checker := sink.NewGitHubAppChecker(c, testNamespace)

	err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("expected error when key value is empty")
	}
	if !strings.Contains(err.Error(), "private-key") {
		t.Errorf("error should mention empty key name, got: %v", err)
	}
}

func TestGitHubAppChecker_Name(t *testing.T) {
	c := newFakeClient()
	checker := sink.NewGitHubAppChecker(c, testNamespace)
	if checker.Name() != "sink/github-app" {
		t.Errorf("Name() = %q, want %q", checker.Name(), "sink/github-app")
	}
}
