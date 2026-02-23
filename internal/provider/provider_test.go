package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/metrics"
	"github.com/lenaxia/k8s-mendabot/internal/provider"
)

// fakeSourceProvider is a controllable domain.SourceProvider for unit tests.
type fakeSourceProvider struct {
	name       string
	objectType client.Object
	finding    *domain.Finding
	findErr    error
}

func (f *fakeSourceProvider) ProviderName() string      { return f.name }
func (f *fakeSourceProvider) ObjectType() client.Object { return f.objectType }
func (f *fakeSourceProvider) ExtractFinding(_ client.Object) (*domain.Finding, error) {
	return f.finding, f.findErr
}

var _ domain.SourceProvider = (*fakeSourceProvider)(nil)

const agentNamespace = "mendabot"

func newTestScheme() *runtime.Scheme {
	s := v1alpha1.NewScheme()
	// Add client-go scheme so that core types (ConfigMap used as watched object) are registered.
	_ = clientgoscheme.AddToScheme(s)
	return s
}

func newTestClient(objs ...client.Object) client.Client {
	s := newTestScheme()
	return fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(objs...).
		Build()
}

func newTestReconciler(p *fakeSourceProvider, c client.Client) *provider.SourceProviderReconciler {
	return &provider.SourceProviderReconciler{
		Client:   c,
		Scheme:   newTestScheme(),
		Cfg:      config.Config{AgentNamespace: agentNamespace},
		Provider: p,
	}
}

// makeWatchedObject creates a ConfigMap as a generic watched object for unit tests.
// The reconciler logic does not depend on the type — only ExtractFinding does.
func makeWatchedObject(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func reqFor(name, namespace string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}
}

// TestSourceProviderReconciler_CallsExtractFinding verifies ExtractFinding is invoked.
func TestSourceProviderReconciler_CallsExtractFinding(t *testing.T) {
	called := false
	p := &fakeSourceProvider{
		name:       "fake",
		objectType: &corev1.ConfigMap{},
		findErr:    nil,
	}
	p.finding = nil // nil finding → skip, but still calls ExtractFinding

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)

	trackingProvider := &trackingFakeProvider{inner: p}
	r := &provider.SourceProviderReconciler{
		Client:   c,
		Scheme:   newTestScheme(),
		Cfg:      config.Config{AgentNamespace: agentNamespace},
		Provider: trackingProvider,
	}

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !trackingProvider.extractCalled {
		t.Error("expected ExtractFinding to be called")
	}
	_ = called
}

type trackingFakeProvider struct {
	inner         *fakeSourceProvider
	extractCalled bool
}

func (t *trackingFakeProvider) ProviderName() string      { return t.inner.name }
func (t *trackingFakeProvider) ObjectType() client.Object { return t.inner.objectType }
func (t *trackingFakeProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	t.extractCalled = true
	return t.inner.ExtractFinding(obj)
}

var _ domain.SourceProvider = (*trackingFakeProvider)(nil)

// TestSourceProviderReconciler_SkipsOnNilFinding verifies no RemediationJob is created when
// ExtractFinding returns nil, nil.
func TestSourceProviderReconciler_SkipsOnNilFinding(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "fake",
		objectType: &corev1.ConfigMap{},
		finding:    nil,
		findErr:    nil,
	}
	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_CreatesRemediationJob verifies a RemediationJob is created
// with correct fields for a valid finding. The fingerprint is computed by domain.FindingFingerprint.
func TestSourceProviderReconciler_CreatesRemediationJob(t *testing.T) {
	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         "pod-abc",
		Namespace:    "default",
		ParentObject: "my-deploy",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
		Details:      "Pod is crash looping",
	}
	expectedFP, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("computing expected fingerprint: %v", err)
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	r := newTestReconciler(p, c)

	_, err = r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 RemediationJob, got %d", len(list.Items))
	}

	rjob := list.Items[0]
	expectedName := "mendabot-" + expectedFP[:12]
	if rjob.Name != expectedName {
		t.Errorf("name = %q, want %q", rjob.Name, expectedName)
	}
	if rjob.Spec.Fingerprint != expectedFP {
		t.Errorf("fingerprint = %q, want %q", rjob.Spec.Fingerprint, expectedFP)
	}
	if rjob.Spec.SourceType != "native" {
		t.Errorf("sourceType = %q, want %q", rjob.Spec.SourceType, "native")
	}
	if rjob.Spec.SourceResultRef.Name != "r1" {
		t.Errorf("sourceResultRef.Name = %q, want %q", rjob.Spec.SourceResultRef.Name, "r1")
	}
	if rjob.Spec.SourceResultRef.Namespace != "default" {
		t.Errorf("sourceResultRef.Namespace = %q, want %q", rjob.Spec.SourceResultRef.Namespace, "default")
	}
	if rjob.Labels["remediation.mendabot.io/fingerprint"] != expectedFP[:12] {
		t.Errorf("fingerprint label = %q, want %q", rjob.Labels["remediation.mendabot.io/fingerprint"], expectedFP[:12])
	}
	if rjob.Annotations["remediation.mendabot.io/fingerprint-full"] != expectedFP {
		t.Errorf("fingerprint-full annotation = %q, want %q", rjob.Annotations["remediation.mendabot.io/fingerprint-full"], expectedFP)
	}
	if rjob.Spec.Finding.Kind != "Pod" {
		t.Errorf("finding.kind = %q, want %q", rjob.Spec.Finding.Kind, "Pod")
	}
}

// TestSourceProviderReconciler_SkipsDuplicateFingerprint verifies no second RemediationJob is
// created when a non-Failed one with the same fingerprint already exists.
func TestSourceProviderReconciler_SkipsDuplicateFingerprint(t *testing.T) {
	finding := &domain.Finding{
		Kind: "Pod", Namespace: "default", ParentObject: "my-deploy",
		Errors: `[{"text":"error"}]`,
	}
	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("computing fingerprint: %v", err)
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}

	existing := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: agentNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/fingerprint": fp[:12]},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: fp,
			SourceType:  "native",
		},
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj, existing)
	r := newTestReconciler(p, c)

	_, err = r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected exactly 1 RemediationJob (existing), got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_ReDispatchesFailedRemediationJob verifies a new RemediationJob
// is created when the existing one has phase Failed. The Failed one is deleted first, then a
// new one with the standard name is created.
func TestSourceProviderReconciler_ReDispatchesFailedRemediationJob(t *testing.T) {
	finding := &domain.Finding{
		Kind: "Pod", Namespace: "default", ParentObject: "my-deploy",
		Errors: `[{"text":"error"}]`,
	}
	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("computing fingerprint: %v", err)
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}

	// Existing Failed RemediationJob with same fingerprint and standard name.
	// The reconciler should delete it and create a new one with the same name.
	failedRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: agentNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/fingerprint": fp[:12]},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint:        fp,
			SourceType:         "native",
			SinkType:           "github",
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
			SourceResultRef:    v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod",
				Namespace:    "default",
				ParentObject: "my-deploy",
			},
		},
		Status: v1alpha1.RemediationJobStatus{
			Phase: v1alpha1.PhaseFailed,
		},
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj, failedRJob)
	r := newTestReconciler(p, c)

	_, err = r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	// Failed one deleted, new one created — net result is 1.
	if len(list.Items) != 1 {
		t.Errorf("expected 1 RemediationJob (failed deleted, new created), got %d", len(list.Items))
	}
	if list.Items[0].Status.Phase == v1alpha1.PhaseFailed {
		t.Error("expected new RemediationJob not to be in Failed phase")
	}
}

// TestSourceProviderReconciler_NotFound_DeletesPendingRJobs verifies that when the watched
// object is not found, any Pending/Dispatched RemediationJobs for that source ref are deleted.
func TestSourceProviderReconciler_NotFound_DeletesPendingRJobs(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
	}

	pendingRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-pending",
			Namespace: agentNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceResultRef: v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
		},
		Status: v1alpha1.RemediationJobStatus{Phase: v1alpha1.PhasePending},
	}

	// No watched object — it's been deleted
	c := newTestClient(pendingRJob)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs after source deleted, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_NotFound_DeletesDispatchedRJobs verifies Dispatched jobs are
// also cancelled when the source object is deleted.
func TestSourceProviderReconciler_NotFound_DeletesDispatchedRJobs(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
	}

	dispatchedRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-dispatched",
			Namespace: agentNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceResultRef: v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
		},
		Status: v1alpha1.RemediationJobStatus{Phase: v1alpha1.PhaseDispatched},
	}

	c := newTestClient(dispatchedRJob)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs after source deleted, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_NotFound_DeletesRunningRJobs verifies Running jobs are
// also cancelled when the source object is deleted.
func TestSourceProviderReconciler_NotFound_DeletesRunningRJobs(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
	}

	runningRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-running",
			Namespace: agentNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceResultRef: v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
		},
		Status: v1alpha1.RemediationJobStatus{Phase: v1alpha1.PhaseRunning},
	}

	c := newTestClient(runningRJob)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs after source deleted, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_FingerprintError_ReturnsError verifies that a malformed
// Errors JSON in the finding causes domain.FindingFingerprint to return an error which
// is propagated as a reconciler error.
func TestSourceProviderReconciler_FingerprintError_ReturnsError(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding: &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "my-deploy",
			Errors: "not-json",
		},
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err == nil {
		t.Error("expected error from malformed Errors JSON, got nil")
	}
}

// --- Stabilisation window tests ---

func newTestReconcilerWithWindow(p *fakeSourceProvider, c client.Client, window time.Duration) *provider.SourceProviderReconciler {
	return &provider.SourceProviderReconciler{
		Client: c,
		Scheme: newTestScheme(),
		Cfg: config.Config{
			AgentNamespace:      agentNamespace,
			StabilisationWindow: window,
		},
		Provider: p,
	}
}

func makeFinding() *domain.Finding {
	return &domain.Finding{
		Kind:         "Pod",
		Name:         "pod-abc",
		Namespace:    "default",
		ParentObject: "my-deploy",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
	}
}

// TestStabilisationWindow_WindowZeroImmediate verifies that StabilisationWindow==0 bypasses
// the firstSeen map entirely and creates a RemediationJob immediately.
func TestStabilisationWindow_WindowZeroImmediate(t *testing.T) {
	finding := makeFinding()
	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}
	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	r := newTestReconcilerWithWindow(p, c, 0)

	result, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no RequeueAfter for window=0, got %v", result.RequeueAfter)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 RemediationJob created immediately, got %d", len(list.Items))
	}
}

// TestStabilisationWindow_WindowNotElapsed verifies that on first sight of a finding with
// a non-zero window, the reconciler returns RequeueAfter > 0 and does not create a RemediationJob.
func TestStabilisationWindow_WindowNotElapsed(t *testing.T) {
	finding := makeFinding()
	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}
	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	window := 2 * time.Minute
	r := newTestReconcilerWithWindow(p, c, window)

	result, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Errorf("expected RequeueAfter > 0 on first sight, got %v", result.RequeueAfter)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs before window elapses, got %d", len(list.Items))
	}
}

// TestStabilisationWindow_WindowElapsed verifies that when the window has already elapsed
// (firstSeen entry is old enough), the reconciler proceeds to create a RemediationJob.
func TestStabilisationWindow_WindowElapsed(t *testing.T) {
	finding := makeFinding()
	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("computing fingerprint: %v", err)
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}
	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	window := 2 * time.Minute
	r := newTestReconcilerWithWindow(p, c, window)

	// Pre-populate firstSeen with a timestamp 3 minutes in the past so the window is elapsed.
	r.SetFirstSeenForTest(fp, time.Now().Add(-3*time.Minute))

	result, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no RequeueAfter after window elapsed, got %v", result.RequeueAfter)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 RemediationJob after window elapsed, got %d", len(list.Items))
	}
}

// TestStabilisationWindow_SecondSightWithinWindow verifies that when the window has not elapsed
// yet (entry in firstSeen is recent), the reconciler returns a RequeueAfter equal to the
// remaining time, and no RemediationJob is created.
func TestStabilisationWindow_SecondSightWithinWindow(t *testing.T) {
	finding := makeFinding()
	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("computing fingerprint: %v", err)
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}
	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	window := 2 * time.Minute
	r := newTestReconcilerWithWindow(p, c, window)

	// Pre-populate firstSeen with a timestamp 30 seconds ago (within the 2-minute window).
	elapsed := 30 * time.Second
	r.SetFirstSeenForTest(fp, time.Now().Add(-elapsed))

	result, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Remaining time should be approximately window - elapsed = 90s.
	// Allow for a few seconds of test execution time.
	minExpected := window - elapsed - 2*time.Second
	if result.RequeueAfter < minExpected {
		t.Errorf("RequeueAfter = %v, want >= %v (remaining window time)", result.RequeueAfter, minExpected)
	}
	if result.RequeueAfter >= window {
		t.Errorf("RequeueAfter = %v, want < %v (full window)", result.RequeueAfter, window)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs while window not elapsed, got %d", len(list.Items))
	}
}

// TestStabilisationWindow_FindingClearsResetsWindow verifies that when ExtractFinding returns
// nil (finding cleared), the firstSeen entry is evicted. A subsequent finding restarts the window.
func TestStabilisationWindow_FindingClearsResetsWindow(t *testing.T) {
	finding := makeFinding()
	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("computing fingerprint: %v", err)
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}
	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	window := 2 * time.Minute
	r := newTestReconcilerWithWindow(p, c, window)

	// Pre-populate firstSeen as if we already recorded a first sight.
	r.FirstSeen()[fp] = time.Now().Add(-30 * time.Second)

	// Now simulate the finding clearing: set provider to return nil.
	p.finding = nil
	_, err = r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error on nil-finding reconcile: %v", err)
	}

	// The firstSeen entry should have been evicted.
	if _, exists := r.FirstSeen()[fp]; exists {
		t.Error("expected firstSeen entry to be evicted after finding cleared")
	}

	// Restore the finding — subsequent reconcile should restart the window (not proceed to create).
	p.finding = finding
	result, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error on re-finding reconcile: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Errorf("expected window to restart after finding cleared, got RequeueAfter=%v", result.RequeueAfter)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs after window reset, got %d", len(list.Items))
	}
}

// TestStabilisationWindow_NotFoundClearsMap verifies that when the watched object is
// deleted (not-found path), the firstSeen map is cleared entirely.
func TestStabilisationWindow_NotFoundClearsMap(t *testing.T) {
	finding := makeFinding()
	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("computing fingerprint: %v", err)
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
	}
	// No watched object in the client — it has been deleted.
	c := newTestClient()
	window := 2 * time.Minute
	r := newTestReconcilerWithWindow(p, c, window)

	// Pre-populate firstSeen with an entry.
	r.FirstSeen()[fp] = time.Now()
	r.FirstSeen()["other-fp"] = time.Now()

	_, err = r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.FirstSeen()) != 0 {
		t.Errorf("expected firstSeen to be cleared on not-found, got %d entries", len(r.FirstSeen()))
	}
}

// TestReconcile_SelfRemediation_NoDoubleCountAttempt verifies that creating a self-remediation
// RemediationJob does NOT increment selfRemediationAttemptsTotal in the provider.
// The attempt is recorded only once: when the job completes (in remediationjob_controller.go).
func TestReconcile_SelfRemediation_NoDoubleCountAttempt(t *testing.T) {
	metrics.ResetMetrics()
	t.Cleanup(metrics.ResetMetrics)

	finding := &domain.Finding{
		Kind:              "Job",
		Name:              "mendabot-agent-abc123",
		Namespace:         "mendabot",
		ParentObject:      "Job/mendabot-agent-abc123",
		Errors:            `[{"text":"job failed"}]`,
		IsSelfRemediation: true,
		ChainDepth:        1,
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}

	obj := makeWatchedObject("r1", "mendabot")
	c := newTestClient(obj)
	r := &provider.SourceProviderReconciler{
		Client:   c,
		Scheme:   newTestScheme(),
		Cfg:      config.Config{AgentNamespace: agentNamespace, SelfRemediationMaxDepth: 3},
		Provider: p,
	}

	_, err := r.Reconcile(context.Background(), reqFor("r1", "mendabot"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no self-remediation attempt was recorded at dispatch time.
	// Both success=true and success=false counters must be zero.
	successCount := testutil.ToFloat64(
		metrics.SelfRemediationAttemptsTotal().WithLabelValues("native", "mendabot", "true"),
	)
	failureCount := testutil.ToFloat64(
		metrics.SelfRemediationAttemptsTotal().WithLabelValues("native", "mendabot", "false"),
	)
	if successCount != 0 {
		t.Errorf("selfRemediationAttemptsTotal{success=true} = %v, want 0 (no pre-counting at dispatch)", successCount)
	}
	if failureCount != 0 {
		t.Errorf("selfRemediationAttemptsTotal{success=false} = %v, want 0 (no pre-counting at dispatch)", failureCount)
	}
}

// TestReconcile_MaxDepthExceeded_MetricFires verifies that when a self-remediation finding
// has ChainDepth >= SelfRemediationMaxDepth, the mendabot_max_depth_exceeded_total counter fires.
func TestReconcile_MaxDepthExceeded_MetricFires(t *testing.T) {
	metrics.ResetMetrics()
	t.Cleanup(metrics.ResetMetrics)

	// ChainDepth == SelfRemediationMaxDepth: should fire the metric
	finding := &domain.Finding{
		Kind:              "Job",
		Name:              "mendabot-agent-deep",
		Namespace:         "mendabot",
		ParentObject:      "Job/mendabot-agent-deep",
		Errors:            `[{"text":"job failed at depth 2"}]`,
		IsSelfRemediation: true,
		ChainDepth:        2,
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}

	obj := makeWatchedObject("r1", "mendabot")
	c := newTestClient(obj)
	r := &provider.SourceProviderReconciler{
		Client:   c,
		Scheme:   newTestScheme(),
		Cfg:      config.Config{AgentNamespace: agentNamespace, SelfRemediationMaxDepth: 2},
		Provider: p,
	}

	_, err := r.Reconcile(context.Background(), reqFor("r1", "mendabot"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// mendabot_max_depth_exceeded_total must have incremented at least once.
	count := testutil.ToFloat64(
		metrics.MaxDepthExceededTotal().WithLabelValues("native", "mendabot", "2"),
	)
	if count < 1 {
		t.Errorf("mendabot_max_depth_exceeded_total{depth=2} = %v, want >= 1", count)
	}
}

// TestReconcile_DisableCascadeCheck_Bypasses verifies that DisableCascadeCheck=true
// lets a self-remediation finding through without cascade suppression.
func TestReconcile_DisableCascadeCheck_Bypasses(t *testing.T) {
	metrics.ResetMetrics()
	t.Cleanup(metrics.ResetMetrics)

	finding := &domain.Finding{
		Kind:              "Job",
		Name:              "mendabot-agent-abc",
		Namespace:         "mendabot",
		ParentObject:      "Job/mendabot-agent-abc",
		Errors:            `[{"text":"job failed"}]`,
		IsSelfRemediation: true,
		ChainDepth:        1,
	}

	p := &fakeSourceProvider{
		name:       "native",
		objectType: &corev1.ConfigMap{},
		finding:    finding,
	}

	obj := makeWatchedObject("r1", "mendabot")
	c := newTestClient(obj)
	r := &provider.SourceProviderReconciler{
		Client: c,
		Scheme: newTestScheme(),
		Cfg: config.Config{
			AgentNamespace:          agentNamespace,
			SelfRemediationMaxDepth: 3,
			DisableCascadeCheck:     true,
		},
		Provider: p,
	}

	_, err := r.Reconcile(context.Background(), reqFor("r1", "mendabot"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A RemediationJob should have been created (cascade suppression did not fire).
	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 RemediationJob (DisableCascadeCheck bypasses suppression), got %d", len(list.Items))
	}

	// infrastructure_cascade suppression counter must be zero.
	suppressCount := testutil.ToFloat64(
		metrics.CascadeSuppressionsTotal().WithLabelValues("native", "mendabot", "infrastructure_cascade"),
	)
	if suppressCount != 0 {
		t.Errorf("cascadeSuppressionsTotal{infrastructure_cascade} = %v, want 0 (DisableCascadeCheck=true)", suppressCount)
	}
}
