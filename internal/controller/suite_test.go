// To install envtest binaries:
//
//	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
//	setup-envtest use --bin-dir /usr/local/kubebuilder/bin
//	export KUBEBUILDER_ASSETS=$(setup-envtest use -p path)
package controller_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg *rest.Config
	// k8sClient is created with the v1alpha1-only scheme and is used exclusively
	// by TestSuite_StartsAndStops to verify the envtest environment starts correctly.
	// All correlation integration tests use newIntegrationClient() instead, which
	// builds a client with the full scheme (v1alpha1 + batchv1 + corev1).
	k8sClient  client.Client
	testEnv    *envtest.Environment
	suiteReady bool
)

func TestMain(m *testing.M) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		out, err := exec.Command("setup-envtest", "use", "-p", "path").Output()
		if err == nil {
			assets = strings.TrimSpace(string(out))
			os.Setenv("KUBEBUILDER_ASSETS", assets)
		}
	}

	if assets != "" {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{"../../testdata/crds"},
		}
		var err error
		cfg, err = testEnv.Start()
		if err == nil {
			scheme := v1alpha1.NewScheme()
			k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
			if err == nil {
				suiteReady = true
			} else {
				fmt.Fprintf(os.Stderr, "failed to create k8s client: %v\n", err)
				_ = testEnv.Stop()
				testEnv = nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "failed to start envtest: %v\n", err)
		}
	}

	code := m.Run()

	if testEnv != nil {
		_ = testEnv.Stop()
	}

	os.Exit(code)
}

func TestSuite_StartsAndStops(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	var rjobList v1alpha1.RemediationJobList
	if err := k8sClient.List(ctx, &rjobList); err != nil {
		t.Fatalf("expected no error listing RemediationJobList, got: %v", err)
	}
}
