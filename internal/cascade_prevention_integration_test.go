package internal_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/circuitbreaker"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/provider"
	"github.com/lenaxia/k8s-mendabot/internal/provider/native"
)

// TestFullCascadePreventionIntegration tests the complete cascade prevention system
// including circuit breaker, chain depth tracking, and concurrent reconciliation
func TestFullCascadePreventionIntegration(t *testing.T) {
	// Create scheme with all required types
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1alpha1.AddRemediationToScheme(s)

	// Configuration with cascade prevention enabled
	cfg := config.Config{
		GitOpsRepo:              "test/repo",
		GitOpsManifestRoot:      "manifests",
		AgentImage:              "test/image:latest",
		AgentNamespace:          "mendabot",
		AgentSA:                 "default",
		SelfRemediationMaxDepth: 3,
		SelfRemediationCooldown: 5 * time.Minute,
		StabilisationWindow:     2 * time.Minute,
	}

	// Step 1: Setup initial state
	t.Log("Step 1: Setup initial state with circuit breaker")

	// Create circuit breaker ConfigMap
	cbConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      circuitbreaker.ConfigMapName,
			Namespace: "mendabot",
		},
		Data: map[string]string{
			circuitbreaker.LastSelfRemediationKey: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		},
	}

	// Create initial failed job
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-job-failed",
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Failed: 3,
			Active: 0,
		},
	}

	// Create client with initial objects
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cbConfigMap, failedJob).
		Build()

	// Step 2: Test circuit breaker allows first remediation
	t.Log("Step 2: Test circuit breaker allows remediation")

	cb := circuitbreaker.New(c, "mendabot", cfg.SelfRemediationCooldown)
	allowed, remaining, err := cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("circuit breaker ShouldAllow failed: %v", err)
	}
	if !allowed {
		t.Errorf("circuit breaker should allow (last remediation 10 minutes ago, cooldown 5 minutes)")
	}
	if remaining != 0 {
		t.Errorf("remaining time should be 0 when allowed, got %v", remaining)
	}

	// Step 3: Create Job provider and extract finding
	t.Log("Step 3: Extract finding from failed job")

	jobProvider := native.NewJobProvider(c, cfg)
	finding, err := jobProvider.ExtractFinding(failedJob)
	if err != nil {
		t.Fatalf("ExtractFinding failed: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for failed job")
	}
	if finding.IsSelfRemediation {
		t.Error("initial job finding should not be self-remediation")
	}
	if finding.ChainDepth != 0 {
		t.Errorf("initial chain depth should be 0, got %d", finding.ChainDepth)
	}

	// Step 4: Simulate RemediationJob creation
	t.Log("Step 4: Simulate RemediationJob creation")

	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("FindingFingerprint failed: %v", err)
	}

	remediationJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: cfg.AgentNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/fingerprint": fp[:12],
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint:     fp,
			SourceType:      "native",
			ChainDepth:      finding.ChainDepth,
			SourceResultRef: v1alpha1.ResultRef{Name: failedJob.Name, Namespace: failedJob.Namespace},
			Finding: v1alpha1.FindingSpec{
				Kind:         finding.Kind,
				Name:         finding.Name,
				Namespace:    finding.Namespace,
				ParentObject: finding.ParentObject,
			},
		},
	}

	// Add RemediationJob to client
	c = fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cbConfigMap, failedJob, remediationJob).
		Build()

	// Step 5: Simulate mendabot agent job failure (self-remediation)
	t.Log("Step 5: Simulate mendabot agent job failure (chain depth 1)")

	mendabotJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-abc123",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               remediationJob.Name,
					UID:                remediationJob.UID,
					Controller:         boolPtr(true),
					BlockOwnerDeletion: boolPtr(true),
				},
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Active: 0,
		},
	}

	// Update client with mendabot job
	c = fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cbConfigMap, failedJob, remediationJob, mendabotJob).
		Build()

	jobProvider = native.NewJobProvider(c, cfg)
	mendabotFinding, err := jobProvider.ExtractFinding(mendabotJob)
	if err != nil {
		t.Fatalf("ExtractFinding mendabot job failed: %v", err)
	}
	if mendabotFinding == nil {
		t.Fatal("expected finding for mendabot agent job")
	}
	if !mendabotFinding.IsSelfRemediation {
		t.Error("mendabot job finding should be self-remediation")
	}
	if mendabotFinding.ChainDepth != 1 {
		t.Errorf("mendabot job chain depth should be 1, got %d", mendabotFinding.ChainDepth)
	}

	// Step 6: Test circuit breaker blocks immediate second remediation
	t.Log("Step 6: Test circuit breaker blocks immediate second remediation")

	// Update circuit breaker ConfigMap with recent timestamp
	recentTime := time.Now().Add(-1 * time.Minute)
	cbConfigMap.Data[circuitbreaker.LastSelfRemediationKey] = recentTime.Format(time.RFC3339)

	c = fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cbConfigMap, failedJob, remediationJob, mendabotJob).
		Build()

	cb = circuitbreaker.New(c, "mendabot", cfg.SelfRemediationCooldown)
	allowed, remaining, err = cb.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("circuit breaker ShouldAllow failed: %v", err)
	}
	if allowed {
		t.Error("circuit breaker should block (last remediation 1 minute ago, cooldown 5 minutes)")
	}
	if remaining <= 0 {
		t.Errorf("remaining time should be > 0 when blocked, got %v", remaining)
	}
	if remaining > cfg.SelfRemediationCooldown {
		t.Errorf("remaining time %v should not exceed cooldown %v", remaining, cfg.SelfRemediationCooldown)
	}

	// Step 7: Test chain depth limit enforcement
	t.Log("Step 7: Test chain depth limit enforcement")

	// Create a RemediationJob at max depth
	// Note: In reality, RemediationJobs are in AgentNamespace, but agent jobs reference them
	// from their own namespace. The job provider should handle this.
	// For the test, we'll put both in the same namespace for simplicity.
	maxDepthRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remediation-max-depth",
			Namespace: "default", // Same namespace as job
			UID:       "uid-max-depth",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: cfg.SelfRemediationMaxDepth, // Depth = 3 (max)
		},
	}

	c = fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cbConfigMap, failedJob, mendabotJob, maxDepthRJob).
		Build()

	// Test extraction at max depth - should exceed limit
	maxDepthJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-max-depth",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               "remediation-max-depth",
					UID:                "uid-max-depth",
					Controller:         boolPtr(true),
					BlockOwnerDeletion: boolPtr(true),
				},
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Active: 0,
		},
	}

	jobProvider = native.NewJobProvider(c, cfg)
	maxDepthFinding, err := jobProvider.ExtractFinding(maxDepthJob)
	if err != nil {
		t.Fatalf("ExtractFinding max depth job failed: %v", err)
	}
	// The RemediationJob has chain depth = cfg.SelfRemediationMaxDepth (3)
	// When extracted, it becomes 3 + 1 = 4, which exceeds max depth 3
	// So should return nil
	if maxDepthFinding != nil {
		t.Errorf("expected nil finding when chain depth exceeds max (%d), got finding with depth %d",
			cfg.SelfRemediationMaxDepth, maxDepthFinding.ChainDepth)
	}

	// Test extraction at max depth - 1 (should succeed)
	belowMaxDepthRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remediation-below-max",
			Namespace: "default", // Same namespace as job
			UID:       "uid-below-max",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: cfg.SelfRemediationMaxDepth - 1, // Depth = 2 (max-1)
		},
	}

	c = fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cbConfigMap, failedJob, mendabotJob, belowMaxDepthRJob).
		Build()

	belowMaxDepthJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-below-max",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               "remediation-below-max",
					UID:                "uid-below-max",
					Controller:         boolPtr(true),
					BlockOwnerDeletion: boolPtr(true),
				},
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Active: 0,
		},
	}

	jobProvider = native.NewJobProvider(c, cfg)
	belowMaxDepthFinding, err := jobProvider.ExtractFinding(belowMaxDepthJob)
	if err != nil {
		t.Fatalf("ExtractFinding below max depth job failed: %v", err)
	}
	if belowMaxDepthFinding == nil {
		t.Fatal("expected finding when chain depth is below max")
	}
	// Should be (max-1) + 1 = max
	if belowMaxDepthFinding.ChainDepth != cfg.SelfRemediationMaxDepth {
		t.Errorf("ChainDepth = %d, want %d (max depth)",
			belowMaxDepthFinding.ChainDepth, cfg.SelfRemediationMaxDepth)
	}

	// Step 8: Test concurrent reconciliation with stabilization window
	t.Log("Step 8: Test concurrent reconciliation")

	// Create provider reconciler with stabilization window
	sourceProvider := &fakeSourceProvider{
		name:       "native",
		objectType: &batchv1.Job{},
		finding:    finding,
	}

	reconciler := &provider.SourceProviderReconciler{
		Client:   c,
		Scheme:   s,
		Cfg:      cfg,
		Provider: sourceProvider,
	}

	// Simulate concurrent reconciliation requests
	const numConcurrent = 5
	results := make(chan ctrl.Result, numConcurrent)
	errors := make(chan error, numConcurrent)

	var wg sync.WaitGroup
	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      failedJob.Name,
					Namespace: failedJob.Namespace,
				},
			}
			result, err := reconciler.Reconcile(context.Background(), req)
			errors <- err
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Errorf("concurrent reconcile error: %v", err)
		}
	}

	// Count RemediationJobs created
	var rjobList v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &rjobList, client.InNamespace(cfg.AgentNamespace)); err != nil {
		t.Fatalf("list RemediationJobs failed: %v", err)
	}

	// With stabilization window, no new RemediationJobs should be created immediately
	// (existing one should prevent duplicates)
	// We have cbConfigMap, failedJob, mendabotJob, belowMaxDepthRJob = 4 objects
	// But only 1 RemediationJob (belowMaxDepthRJob)
	expectedRemediationJobs := 1
	if len(rjobList.Items) > expectedRemediationJobs {
		t.Errorf("expected no new RemediationJobs from concurrent reconciliation, got %d total (expected %d)",
			len(rjobList.Items), expectedRemediationJobs)
	}

	t.Log("Integration test completed successfully")
}

// TestCircuitBreakerPersistenceAcrossRestarts tests circuit breaker state persistence
func TestCircuitBreakerPersistenceAcrossRestarts(t *testing.T) {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)

	// Create ConfigMap with circuit breaker state
	timestamp := time.Now().Add(-3 * time.Minute)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      circuitbreaker.ConfigMapName,
			Namespace: "mendabot",
		},
		Data: map[string]string{
			circuitbreaker.LastSelfRemediationKey: timestamp.Format(time.RFC3339),
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(configMap).Build()
	cooldown := 5 * time.Minute

	// First controller instance
	cb1 := circuitbreaker.New(c, "mendabot", cooldown)
	allowed1, remaining1, err := cb1.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("first instance ShouldAllow failed: %v", err)
	}

	// Second controller instance (simulating restart)
	cb2 := circuitbreaker.New(c, "mendabot", cooldown)
	allowed2, remaining2, err := cb2.ShouldAllow(context.Background())
	if err != nil {
		t.Fatalf("second instance ShouldAllow failed: %v", err)
	}

	// Both instances should see the same state
	if allowed1 != allowed2 {
		t.Errorf("allowed mismatch: first=%v, second=%v", allowed1, allowed2)
	}
	// Allow for tiny differences due to time passing between calls
	if (remaining1 - remaining2).Abs() > time.Millisecond {
		t.Errorf("remaining mismatch: first=%v, second=%v (diff: %v)", remaining1, remaining2, remaining1-remaining2)
	}

	// With 3 minutes elapsed and 5 minute cooldown, should not be allowed
	if allowed1 {
		t.Error("should not be allowed (3 minutes elapsed, 5 minute cooldown)")
	}
	if remaining1 <= 0 {
		t.Errorf("remaining should be > 0, got %v", remaining1)
	}
	if remaining1 > 2*time.Minute+10*time.Second || remaining1 < 2*time.Minute-10*time.Second {
		t.Errorf("remaining should be ~2 minutes, got %v", remaining1)
	}
}

// TestConcurrentChainDepthTracking tests race-free chain depth tracking
func TestConcurrentChainDepthTracking(t *testing.T) {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1alpha1.AddRemediationToScheme(s)

	cfg := config.Config{
		SelfRemediationMaxDepth: 5,
	}

	// Create parent RemediationJob
	parentRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: "default",
			UID:       "parent-uid",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: 2,
		},
	}

	const numGoroutines = 20
	results := make(chan *domain.Finding, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Deep-copy parentRJob to avoid a data race: fake.NewClientBuilder().WithObjects()
			// calls obj.SetResourceVersion("999") on the passed object during Build(), which
			// mutates the shared pointer concurrently across goroutines.
			localParent := parentRJob.DeepCopyObject().(client.Object)
			// Each goroutine gets its own client
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(localParent).Build()
			p := native.NewJobProvider(c, cfg)

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("mendabot-agent-%d", id),
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "mendabot-watcher",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "remediation.mendabot.io/v1alpha1",
							Kind:               "RemediationJob",
							Name:               "parent",
							UID:                "parent-uid",
							Controller:         boolPtr(true),
							BlockOwnerDeletion: boolPtr(true),
						},
					},
				},
				Status: batchv1.JobStatus{
					Failed: 1,
					Active: 0,
				},
			}

			finding, err := p.ExtractFinding(job)
			if err != nil {
				t.Errorf("goroutine %d: ExtractFinding failed: %v", id, err)
				results <- nil
				return
			}
			results <- finding
		}(i)
	}

	wg.Wait()
	close(results)

	// Collect and verify all findings
	var findings []*domain.Finding
	for finding := range results {
		if finding != nil {
			findings = append(findings, finding)
		}
	}

	if len(findings) == 0 {
		t.Fatal("no findings produced")
	}

	// All findings should have the same chain depth
	expectedDepth := findings[0].ChainDepth
	for i, f := range findings {
		if f.ChainDepth != expectedDepth {
			t.Errorf("finding %d: ChainDepth = %d, expected %d", i, f.ChainDepth, expectedDepth)
		}
	}

	// Chain depth should be 3 (2 from parent + 1)
	if expectedDepth != 3 {
		t.Errorf("ChainDepth = %d, want 3", expectedDepth)
	}
}

// Helper types and functions

type fakeSourceProvider struct {
	name       string
	objectType client.Object
	finding    *domain.Finding
	findErr    error
}

func (f *fakeSourceProvider) ProviderName() string      { return f.name }
func (f *fakeSourceProvider) ObjectType() client.Object { return f.objectType }
func (f *fakeSourceProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	return f.finding, f.findErr
}

func boolPtr(b bool) *bool {
	return &b
}
