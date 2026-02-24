package controller_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/controller"
	"go.uber.org/zap"
)

const testNamespace = "mendabot"

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := v1alpha1.NewScheme()
	if err := batchv1.AddToScheme(s); err != nil {
		t.Fatalf("batchv1.AddToScheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("clientgoscheme.AddToScheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := newTestScheme(t)
	return fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(objs...).
		Build()
}

func newReconciler(t *testing.T, c client.Client, jb *fakeJobBuilder, cfg config.Config) *controller.RemediationJobReconciler {
	t.Helper()
	s := newTestScheme(t)
	return &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        cfg,
	}
}

func defaultCfg() config.Config {
	return config.Config{
		AgentNamespace:           testNamespace,
		MaxConcurrentJobs:        2,
		RemediationJobTTLSeconds: 604800,
	}
}

func newRJob(name, fp string) *v1alpha1.RemediationJob {
	return &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			UID:       types.UID("uid-" + name),
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: fp,
		},
	}
}

func rjobReqFor(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: testNamespace}}
}

// TestRemediationJobReconciler_NotFound_ReturnsNil verifies NotFound → no error, no requeue.
func TestRemediationJobReconciler_NotFound_ReturnsNil(t *testing.T) {
	c := newFakeClient(t)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("nonexistent"))
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if result.RequeueAfter != 0 || result.Requeue {
		t.Errorf("expected zero Result, got %+v", result)
	}
	if len(jb.calls) != 0 {
		t.Error("expected no Build() calls for NotFound")
	}
}

// TestRemediationJobReconciler_BlankPhase_TransitionsToPending verifies that a
// freshly-created RemediationJob with Phase=="" is immediately transitioned to
// PhasePending and requeued, without creating any batch/v1 Job.
func TestRemediationJobReconciler_BlankPhase_TransitionsToPending(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-blank-phase", fp) // Phase is "" (zero value)
	c := newFakeClient(t, rjob)

	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-blank-phase"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue {
		t.Error("expected Requeue=true after blank-phase transition")
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected RequeueAfter=0 after blank-phase transition, got %v", result.RequeueAfter)
	}
	if len(jb.calls) != 0 {
		t.Error("expected no Build() calls during blank-phase transition")
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-blank-phase", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhasePending {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhasePending)
	}

	// No batch/v1 Job should have been created.
	var jobList batchv1.JobList
	if err := c.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Errorf("expected 0 jobs after blank-phase transition, got %d", len(jobList.Items))
	}
}

// TestRemediationJobReconciler_Pending_CreatesJob verifies Pending phase → job created,
// status patched to Dispatched.
func TestRemediationJobReconciler_Pending_CreatesJob(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	c := newFakeClient(t, rjob)

	job := defaultFakeJob(rjob)
	jb := &fakeJobBuilder{returnJob: job}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}

	// Job should exist
	var jobList batchv1.JobList
	if err := c.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("list jobs error: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobList.Items))
	}

	// Status should be Dispatched
	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-rjob", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseDispatched)
	}
	if updated.Status.JobRef == "" {
		t.Error("expected JobRef to be set")
	}
	if updated.Status.DispatchedAt == nil {
		t.Error("expected DispatchedAt to be set")
	}
}

// TestRemediationJobReconciler_MaxConcurrent_Requeues verifies at-limit → no job, requeue after 30s.
func TestRemediationJobReconciler_MaxConcurrent_Requeues(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhasePending // already transitioned from "" on prior reconcile

	// Create 2 active jobs (MaxConcurrentJobs=2)
	activeJob1 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "active-job-1",
			Namespace: testNamespace,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "mendabot-watcher"},
		},
		Status: batchv1.JobStatus{Active: 1},
	}
	activeJob2 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "active-job-2",
			Namespace: testNamespace,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "mendabot-watcher"},
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	c := newFakeClient(t, rjob, activeJob1, activeJob2)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 30*time.Second {
		t.Errorf("expected 30s requeue, got %v", result.RequeueAfter)
	}

	// No new job should be created
	var jobList batchv1.JobList
	if err := c.List(context.Background(), &jobList, client.InNamespace(testNamespace),
		client.MatchingLabels{"remediation.mendabot.io/remediation-job": "test-rjob"}); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Errorf("expected 0 new jobs, got %d", len(jobList.Items))
	}

	// Phase remains Pending — the reconciler returns early without changing it.
	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), client.ObjectKey{Name: "test-rjob", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhasePending {
		t.Errorf("expected Phase=%s, got %s", v1alpha1.PhasePending, updated.Status.Phase)
	}
}

// TestRemediationJobReconciler_JobExists_SyncsStatus verifies existing owned job → status synced.
func TestRemediationJobReconciler_JobExists_SyncsStatus(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhasePending

	existingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-rjob"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
		Status: batchv1.JobStatus{Active: 1},
	}

	c := newFakeClient(t, rjob, existingJob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-rjob", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseRunning {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseRunning)
	}
	if len(jb.calls) != 0 {
		t.Error("expected no Build() calls when job already exists")
	}
}

// TestRemediationJobReconciler_BuildError_ReturnsError verifies Build() error → reconciler error.
func TestRemediationJobReconciler_BuildError_ReturnsError(t *testing.T) {
	buildErr := fmt.Errorf("build failed: missing required field")
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	c := newFakeClient(t, rjob)

	jb := &fakeJobBuilder{returnErr: buildErr}
	r := newReconciler(t, c, jb, defaultCfg())

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err == nil {
		t.Error("expected error from Build() failure, got nil")
	}
}

// TestRemediationJobReconciler_Succeeded_TTLNotDue_Requeues verifies Succeeded + CompletedAt
// not yet past TTL → requeue.
func TestRemediationJobReconciler_Succeeded_TTLNotDue_Requeues(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	now := metav1.Now()
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhaseSucceeded
	rjob.Status.CompletedAt = &now

	c := newFakeClient(t, rjob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Error("expected non-zero RequeueAfter for TTL not yet due")
	}
	// RequeueAfter should be close to TTL (7 days)
	expectedTTL := time.Duration(defaultCfg().RemediationJobTTLSeconds) * time.Second
	if result.RequeueAfter > expectedTTL {
		t.Errorf("RequeueAfter %v exceeds TTL %v", result.RequeueAfter, expectedTTL)
	}
}

// TestRemediationJobReconciler_Failed_ReturnsNil verifies Failed phase → nil returned immediately.
func TestRemediationJobReconciler_Failed_ReturnsNil(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhaseFailed

	c := newFakeClient(t, rjob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Errorf("expected nil error for Failed phase, got %v", err)
	}
	if result.RequeueAfter != 0 || result.Requeue {
		t.Errorf("expected zero Result for Failed phase, got %+v", result)
	}
	if len(jb.calls) != 0 {
		t.Error("expected no Build() calls for Failed phase")
	}
}

// TestRemediationJobReconciler_Cancelled_ReturnsNil verifies Cancelled phase → nil returned immediately.
func TestRemediationJobReconciler_Cancelled_ReturnsNil(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhaseCancelled

	c := newFakeClient(t, rjob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Errorf("expected nil error for Cancelled phase, got %v", err)
	}
	if result.RequeueAfter != 0 || result.Requeue {
		t.Errorf("expected zero Result for Cancelled phase, got %+v", result)
	}
	if len(jb.calls) != 0 {
		t.Error("expected no Build() calls for Cancelled phase")
	}
}

// TestRemediationJobReconciler_OwnerRef verifies created job has ownerReference pointing to RJob.
func TestRemediationJobReconciler_OwnerRef(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	c := newFakeClient(t, rjob)

	job := defaultFakeJob(rjob)
	jb := &fakeJobBuilder{returnJob: job}
	r := newReconciler(t, c, jb, defaultCfg())

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var jobList batchv1.JobList
	if err := c.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobList.Items))
	}

	j := jobList.Items[0]
	if len(j.OwnerReferences) == 0 {
		t.Fatal("expected OwnerReferences to be set on job")
	}
	ref := j.OwnerReferences[0]
	if ref.Name != "test-rjob" {
		t.Errorf("ownerRef.Name = %q, want %q", ref.Name, "test-rjob")
	}
	if ref.Kind != "RemediationJob" {
		t.Errorf("ownerRef.Kind = %q, want %q", ref.Kind, "RemediationJob")
	}
}
