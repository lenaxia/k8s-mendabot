package provider_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/provider"
)

const integrationNamespace = "default"

func integrationProviderCfg() config.Config {
	return config.Config{
		AgentNamespace:           integrationNamespace,
		MaxConcurrentJobs:        10,
		RemediationJobTTLSeconds: 604800,
		GitOpsRepo:               "org/repo",
		GitOpsManifestRoot:       "deploy",
		AgentImage:               "mendabot-agent:test",
		AgentSA:                  "mendabot-agent",
	}
}

// integrationFakeProvider is a fakeSourceProvider backed by corev1.Pod for integration tests.
// It watches Pod objects and returns a configured finding.
type integrationFakeProvider struct {
	name    string
	finding *domain.Finding
}

func (f *integrationFakeProvider) ProviderName() string      { return f.name }
func (f *integrationFakeProvider) ObjectType() client.Object { return &corev1.Pod{} }
func (f *integrationFakeProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, nil
	}
	if f.finding == nil {
		return nil, nil
	}
	// Return a copy with name/namespace filled in from the pod.
	result := *f.finding
	result.Name = pod.Name
	result.Namespace = pod.Namespace
	return &result, nil
}

var _ domain.SourceProvider = (*integrationFakeProvider)(nil)

func newIntegrationSourceReconciler(p *integrationFakeProvider) *provider.SourceProviderReconciler {
	return &provider.SourceProviderReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Cfg:      integrationProviderCfg(),
		Provider: p,
	}
}

func integrationEventually(t *testing.T, condition func() bool, timeout, interval time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatal("condition not met within timeout")
}

func newTestPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:latest"},
			},
		},
	}
}

func integrationReq(name, namespace string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}
}

// TestIntegration_CreateRemediationJob verifies: provider with finding →
// RemediationJob created with correct fields.
func TestIntegration_CreateRemediationJob(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	pod := newTestPod("pod-creates-rjob", integrationNamespace)
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatalf("create Pod: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         pod.Name,
		Namespace:    integrationNamespace,
		ParentObject: "my-deployment",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
		Details:      "test finding details",
	}
	p := &integrationFakeProvider{name: "native", finding: finding}

	rec := newIntegrationSourceReconciler(p)
	req := integrationReq(pod.Name, integrationNamespace)
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	integrationEventually(t, func() bool {
		if err := k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace)); err != nil {
			return false
		}
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == pod.Name &&
				rjobList.Items[i].Spec.SourceType == "native" {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	var found *v1alpha1.RemediationJob
	for i := range rjobList.Items {
		if rjobList.Items[i].Spec.SourceResultRef.Name == pod.Name {
			found = &rjobList.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected RemediationJob to be created")
	}
	if found.Spec.SourceType != "native" {
		t.Errorf("sourceType = %q, want %q", found.Spec.SourceType, "native")
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, found) })
}

// TestIntegration_DuplicateFingerprint_Skips verifies: same fingerprint on second
// reconcile → no second RemediationJob created.
func TestIntegration_DuplicateFingerprint_Skips(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	pod := newTestPod("pod-dedup", integrationNamespace)
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatalf("create Pod: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         pod.Name,
		Namespace:    integrationNamespace,
		ParentObject: "my-deployment",
		Errors:       `[{"text":"ImagePullBackOff"}]`,
		Details:      "test finding details",
	}
	p := &integrationFakeProvider{name: "native", finding: finding}
	rec := newIntegrationSourceReconciler(p)
	req := integrationReq(pod.Name, integrationNamespace)

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	integrationEventually(t, func() bool {
		_ = k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace))
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == pod.Name {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	t.Cleanup(func() {
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == pod.Name {
				_ = k8sClient.Delete(ctx, &rjobList.Items[i])
			}
		}
	})

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	var rjobList2 v1alpha1.RemediationJobList
	if err := k8sClient.List(ctx, &rjobList2, client.InNamespace(integrationNamespace)); err != nil {
		t.Fatalf("list RemediationJobs: %v", err)
	}
	count := 0
	for i := range rjobList2.Items {
		if rjobList2.Items[i].Spec.SourceResultRef.Name == pod.Name {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 RemediationJob for duplicate fingerprint, got %d", count)
	}
}

// TestIntegration_FailedPhase_ReDispatches verifies: existing Failed RemediationJob
// → deleted and new one created.
func TestIntegration_FailedPhase_ReDispatches(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	pod := newTestPod("pod-failed-redispatch", integrationNamespace)
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatalf("create Pod: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         pod.Name,
		Namespace:    integrationNamespace,
		ParentObject: "my-deployment",
		Errors:       `[{"text":"OOMKilled"}]`,
		Details:      "test finding details",
	}
	p := &integrationFakeProvider{name: "native", finding: finding}
	rec := newIntegrationSourceReconciler(p)
	req := integrationReq(pod.Name, integrationNamespace)

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	integrationEventually(t, func() bool {
		_ = k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace))
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == pod.Name {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	var firstRJob *v1alpha1.RemediationJob
	for i := range rjobList.Items {
		if rjobList.Items[i].Spec.SourceResultRef.Name == pod.Name {
			firstRJob = &rjobList.Items[i]
			break
		}
	}
	if firstRJob == nil {
		t.Fatal("expected first RemediationJob to be created")
	}

	rjobCopy := firstRJob.DeepCopyObject().(*v1alpha1.RemediationJob)
	firstRJob.Status.Phase = v1alpha1.PhaseFailed
	if err := k8sClient.Status().Patch(ctx, firstRJob, client.MergeFrom(rjobCopy)); err != nil {
		t.Fatalf("patch status to Failed: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, firstRJob) })

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("second Reconcile (after Failed): %v", err)
	}

	var rjobList2 v1alpha1.RemediationJobList
	integrationEventually(t, func() bool {
		if err := k8sClient.List(ctx, &rjobList2, client.InNamespace(integrationNamespace)); err != nil {
			return false
		}
		for i := range rjobList2.Items {
			if rjobList2.Items[i].Spec.SourceResultRef.Name == pod.Name &&
				rjobList2.Items[i].Status.Phase != v1alpha1.PhaseFailed {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	var newRJob *v1alpha1.RemediationJob
	for i := range rjobList2.Items {
		if rjobList2.Items[i].Spec.SourceResultRef.Name == pod.Name &&
			rjobList2.Items[i].Status.Phase != v1alpha1.PhaseFailed {
			newRJob = &rjobList2.Items[i]
			t.Cleanup(func() { _ = k8sClient.Delete(ctx, newRJob) })
			break
		}
	}
	if newRJob == nil {
		t.Error("expected a new non-Failed RemediationJob after re-dispatch")
	}
}

// TestIntegration_NoErrors_Skipped verifies: nil finding → no RemediationJob created.
func TestIntegration_NoErrors_Skipped(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	pod := newTestPod("pod-no-errors", integrationNamespace)
	if err := k8sClient.Create(ctx, pod); err != nil {
		t.Fatalf("create Pod: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pod) })

	// Provider returns nil finding (no errors detected).
	p := &integrationFakeProvider{name: "native", finding: nil}
	rec := newIntegrationSourceReconciler(p)
	req := integrationReq(pod.Name, integrationNamespace)

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	if err := k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace)); err != nil {
		t.Fatalf("list RemediationJobs: %v", err)
	}
	for i := range rjobList.Items {
		if rjobList.Items[i].Spec.SourceResultRef.Name == pod.Name {
			t.Errorf("expected no RemediationJob for nil-finding pod, found %q", rjobList.Items[i].Name)
		}
	}
}

// TestIntegration_ResultDeleted_CancelsPending verifies: source not found →
// Pending RemediationJob deleted.
func TestIntegration_ResultDeleted_CancelsPending(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	fp := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: integrationNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/fingerprint": fp[:12],
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeNative,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "pod-deleted-pending",
				Namespace: integrationNamespace,
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-abc",
				Namespace:    integrationNamespace,
				ParentObject: "my-deploy",
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
		},
	}
	if err := k8sClient.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, rjob) })

	// Patch phase to Pending to simulate a job that has already been through its first
	// reconcile. A job with Phase=="" (created but not yet reconciled) is also cancellable;
	// that case is covered by TestIntegration_ResultDeleted_CancelsBlankPhase below.
	rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	rjob.Status.Phase = v1alpha1.PhasePending
	if err := k8sClient.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
		t.Fatalf("patch phase to Pending: %v", err)
	}

	// No Pod exists — source is deleted.
	p := &integrationFakeProvider{name: "native", finding: nil}
	rec := newIntegrationSourceReconciler(p)
	req := integrationReq("pod-deleted-pending", integrationNamespace)

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile (NotFound): %v", err)
	}

	integrationEventually(t, func() bool {
		var fetched v1alpha1.RemediationJob
		err := k8sClient.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationNamespace}, &fetched)
		return err != nil
	}, 5*time.Second, 100*time.Millisecond)
}

// TestIntegration_ResultDeleted_CancelsDispatched verifies: source not found →
// Dispatched RemediationJob deleted.
func TestIntegration_ResultDeleted_CancelsDispatched(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	fp := "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: integrationNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/fingerprint": fp[:12],
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeNative,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "pod-deleted-dispatched",
				Namespace: integrationNamespace,
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-def",
				Namespace:    integrationNamespace,
				ParentObject: "my-deploy",
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
		},
	}
	if err := k8sClient.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, rjob) })

	// Patch the RemediationJob to Dispatched phase.
	rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	rjob.Status.Phase = v1alpha1.PhaseDispatched
	if err := k8sClient.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
		t.Fatalf("patch status to Dispatched: %v", err)
	}

	// No Pod exists — source is deleted.
	p := &integrationFakeProvider{name: "native", finding: nil}
	rec := newIntegrationSourceReconciler(p)
	req := integrationReq("pod-deleted-dispatched", integrationNamespace)

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile (NotFound): %v", err)
	}

	integrationEventually(t, func() bool {
		var fetched v1alpha1.RemediationJob
		err := k8sClient.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationNamespace}, &fetched)
		return err != nil
	}, 5*time.Second, 100*time.Millisecond)
}

// TestIntegration_ResultDeleted_CancelsBlankPhase verifies that a RemediationJob with
// Phase=="" (created by SourceProviderReconciler but not yet touched by
// RemediationJobReconciler) is cancelled and deleted when its source is deleted before
// the first controller reconcile fires. This is the race window between Create() and
// the first ""-→Pending reconcile.
func TestIntegration_ResultDeleted_CancelsBlankPhase(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	fp := "deadbeefcafe0011223344556677889900aabbccddeeff0011223344556677"
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: integrationNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/fingerprint": fp[:12],
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeNative,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "pod-blank-phase",
				Namespace: integrationNamespace,
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-blank",
				Namespace:    integrationNamespace,
				ParentObject: "my-deploy",
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
		},
		// Status is NOT patched — Phase remains "" (Go zero value), simulating
		// a job that exists in etcd but has not yet been touched by the controller.
	}
	if err := k8sClient.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, rjob) })

	// Source pod does not exist (was deleted before the controller first reconciled).
	p := &integrationFakeProvider{name: "native", finding: nil}
	rec := newIntegrationSourceReconciler(p)
	req := integrationReq("pod-blank-phase", integrationNamespace)

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile (NotFound): %v", err)
	}

	// The RemediationJob must be cancelled (Phase patched to Cancelled) and deleted.
	integrationEventually(t, func() bool {
		var fetched v1alpha1.RemediationJob
		err := k8sClient.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationNamespace}, &fetched)
		return err != nil // deleted
	}, 5*time.Second, 100*time.Millisecond)
}
