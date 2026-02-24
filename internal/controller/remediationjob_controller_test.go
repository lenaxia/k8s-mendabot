package controller_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
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

// TestRemediationJobReconciler_PhaseFailed_IncrementsRetryCount verifies that
// when the owned batch/v1 Job transitions to Failed, RetryCount is incremented
// exactly once.
func TestRemediationJobReconciler_PhaseFailed_IncrementsRetryCount(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-retry-count", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	rjob.Spec.MaxRetries = 3

	backoffLimit := int32(1)
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-retry-count"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: &backoffLimit},
		Status: batchv1.JobStatus{Failed: backoffLimit + 1},
	}

	c := newFakeClient(t, rjob, failedJob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-retry-count"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-retry-count", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", updated.Status.RetryCount)
	}
	if updated.Status.Phase != v1alpha1.PhaseFailed {
		t.Errorf("Phase = %q, want %q (below cap)", updated.Status.Phase, v1alpha1.PhaseFailed)
	}
}

// TestRemediationJobReconciler_PhaseFailed_AtCap_PermanentlyFails verifies that
// when RetryCount reaches MaxRetries, phase transitions to PermanentlyFailed.
func TestRemediationJobReconciler_PhaseFailed_AtCap_PermanentlyFails(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-perm-fail", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	rjob.Spec.MaxRetries = 3
	rjob.Status.RetryCount = 2 // one more failure will hit the cap

	backoffLimit := int32(1)
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-perm-fail"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: &backoffLimit},
		Status: batchv1.JobStatus{Failed: backoffLimit + 1},
	}

	c := newFakeClient(t, rjob, failedJob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-perm-fail"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-perm-fail", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.RetryCount != 3 {
		t.Errorf("RetryCount = %d, want 3", updated.Status.RetryCount)
	}
	if updated.Status.Phase != v1alpha1.PhasePermanentlyFailed {
		t.Errorf("Phase = %q, want %q", updated.Status.Phase, v1alpha1.PhasePermanentlyFailed)
	}
	found := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == v1alpha1.ConditionPermanentlyFailed && cond.Status == metav1.ConditionTrue {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ConditionPermanentlyFailed=True, not found in conditions")
	}
}

// TestRemediationJobReconciler_RetryCount_Idempotent verifies that re-reconciling
// an already-Failed rjob does NOT increment RetryCount again.
func TestRemediationJobReconciler_RetryCount_Idempotent(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-retry-idem", fp)
	// Already in Failed phase with RetryCount=1 — simulates second reconcile
	rjob.Status.Phase = v1alpha1.PhaseFailed
	rjob.Status.RetryCount = 1
	rjob.Spec.MaxRetries = 3

	c := newFakeClient(t, rjob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-retry-idem"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-retry-idem", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	// Phase is already Failed → short-circuit, no change
	if updated.Status.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1 (idempotent — must not re-increment on already-Failed rjob)", updated.Status.RetryCount)
	}
}

// TestRemediationJobReconciler_PermanentlyFailed_ReturnsNil verifies
// PermanentlyFailed phase → returns immediately, no dispatch.
func TestRemediationJobReconciler_PermanentlyFailed_ReturnsNil(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-perm-noop", fp)
	rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed

	c := newFakeClient(t, rjob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-perm-noop"))
	if err != nil {
		t.Errorf("expected nil error for PermanentlyFailed phase, got %v", err)
	}
	if result.RequeueAfter != 0 || result.Requeue {
		t.Errorf("expected zero Result for PermanentlyFailed phase, got %+v", result)
	}
	if len(jb.calls) != 0 {
		t.Error("expected no Build() calls for PermanentlyFailed phase")
	}
}

// TestRemediationJobReconciler_TerminalPhases_NoBuild verifies all three
// terminal-no-dispatch phases return immediately without calling Build().
func TestRemediationJobReconciler_TerminalPhases_NoBuild(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	tests := []struct {
		phase v1alpha1.RemediationJobPhase
	}{
		{v1alpha1.PhaseFailed},
		{v1alpha1.PhaseCancelled},
		{v1alpha1.PhasePermanentlyFailed},
	}
	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			rjob := newRJob("test-terminal-"+string(tt.phase), fp)
			rjob.Status.Phase = tt.phase

			c := newFakeClient(t, rjob)
			jb := &fakeJobBuilder{}
			r := newReconciler(t, c, jb, defaultCfg())

			result, err := r.Reconcile(context.Background(),
				rjobReqFor("test-terminal-"+string(tt.phase)))
			if err != nil {
				t.Errorf("phase %q: unexpected error: %v", tt.phase, err)
			}
			if result.RequeueAfter != 0 || result.Requeue {
				t.Errorf("phase %q: expected zero Result, got %+v", tt.phase, result)
			}
			if len(jb.calls) != 0 {
				t.Errorf("phase %q: expected no Build() calls", tt.phase)
			}
		})
	}
}

// TestRemediationJobReconciler_PhaseFailed_ZeroMaxRetries_UsesDefault verifies that when
// Spec.MaxRetries == 0 (zero value / unset), the reconciler falls back to the default of 3.
// With RetryCount=2 and one more failure, RetryCount becomes 3 which equals the effective
// maxRetries (3), so the phase must transition to PermanentlyFailed.
func TestRemediationJobReconciler_PhaseFailed_ZeroMaxRetries_UsesDefault(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-zero-maxretries", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	rjob.Spec.MaxRetries = 0   // zero value — should fall back to default of 3
	rjob.Status.RetryCount = 2 // one more failure pushes it to 3, hitting the fallback cap

	backoffLimit := int32(1)
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-zero-maxretries"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: &backoffLimit},
		Status: batchv1.JobStatus{Failed: backoffLimit + 1},
	}

	c := newFakeClient(t, rjob, failedJob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-zero-maxretries"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-zero-maxretries", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	// RetryCount should be incremented to 3
	if updated.Status.RetryCount != 3 {
		t.Errorf("RetryCount = %d, want 3", updated.Status.RetryCount)
	}
	// With fallback maxRetries=3 and RetryCount=3, phase must be PermanentlyFailed
	if updated.Status.Phase != v1alpha1.PhasePermanentlyFailed {
		t.Errorf("Phase = %q, want %q — fallback MaxRetries=3 not applied", updated.Status.Phase, v1alpha1.PhasePermanentlyFailed)
	}
	// The PermanentlyFailed condition must be set
	found := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == v1alpha1.ConditionPermanentlyFailed && cond.Status == metav1.ConditionTrue {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ConditionPermanentlyFailed=True, not found in conditions")
	}
}

// TestRemediationJobReconciler_PhaseSucceeded_NilCompletedAt_SetsCompletedAt verifies that
// when a RemediationJob is already in PhaseSucceeded but CompletedAt is nil (e.g. status
// patch for CompletedAt was lost, or an external actor set Phase=Succeeded without the
// timestamp), the reconciler sets CompletedAt to now and requeues so the TTL path can run
// normally on the next reconcile.
func TestRemediationJobReconciler_PhaseSucceeded_NilCompletedAt_SetsCompletedAt(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-succeeded-nil-cat", fp)
	rjob.Status.Phase = v1alpha1.PhaseSucceeded
	// CompletedAt is intentionally nil — this is the bug scenario

	before := time.Now()

	c := newFakeClient(t, rjob)
	jb := &fakeJobBuilder{}
	r := newReconciler(t, c, jb, defaultCfg())

	result, err := r.Reconcile(context.Background(), rjobReqFor("test-succeeded-nil-cat"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must requeue so the TTL path runs on the next reconcile
	if result.RequeueAfter == 0 && !result.Requeue {
		t.Error("expected a requeue (RequeueAfter > 0 or Requeue=true) when CompletedAt was nil")
	}

	// CompletedAt must now be set in the persisted status
	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-succeeded-nil-cat", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set after safety-net reconcile, got nil")
	}
	after := time.Now()
	// Allow a 2-second window to account for metav1.Now() stripping the monotonic
	// clock and any sub-second rounding.
	if updated.Status.CompletedAt.Time.Before(before.Add(-2*time.Second)) || updated.Status.CompletedAt.Time.After(after.Add(2*time.Second)) {
		t.Errorf("CompletedAt %v is outside expected window [%v, %v]",
			updated.Status.CompletedAt.Time, before, after)
	}

	// No batch/v1 Job should be created
	var jobList batchv1.JobList
	if err := c.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Errorf("expected 0 jobs after safety-net reconcile, got %d", len(jobList.Items))
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

func drainEvents(ch <-chan string) []string {
	var out []string
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

// TestReconcile_EmitsEvent_JobDispatched verifies that a JobDispatched event is emitted
// when a PhasePending rjob has no existing job and dispatch succeeds.
func TestReconcile_EmitsEvent_JobDispatched(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	c := newFakeClient(t, rjob)

	job := defaultFakeJob(rjob)
	jb := &fakeJobBuilder{returnJob: job}

	fakeRecorder := record.NewFakeRecorder(10)
	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     newTestScheme(t),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        defaultCfg(),
		Recorder:   fakeRecorder,
	}

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := drainEvents(fakeRecorder.Events)
	var found bool
	for _, e := range events {
		if strings.Contains(e, "JobDispatched") && strings.Contains(e, string(corev1.EventTypeNormal)) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Normal JobDispatched event, got: %v", events)
	}
}

// TestReconcile_EmitsEvent_JobSucceeded_WithPR verifies that a JobSucceeded event containing
// the PR URL is emitted when the owned Job has Succeeded>0 and PRRef is set.
func TestReconcile_EmitsEvent_JobSucceeded_WithPR(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob-pr", fp)
	rjob.Status.Phase = v1alpha1.PhaseDispatched
	rjob.Status.PRRef = "https://github.com/example/repo/pull/42"

	succeededJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-rjob-pr"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
		Status: batchv1.JobStatus{Succeeded: 1},
	}

	c := newFakeClient(t, rjob, succeededJob)
	jb := &fakeJobBuilder{}

	fakeRecorder := record.NewFakeRecorder(10)
	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     newTestScheme(t),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        defaultCfg(),
		Recorder:   fakeRecorder,
	}

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob-pr"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := drainEvents(fakeRecorder.Events)
	var found bool
	for _, e := range events {
		if strings.Contains(e, "JobSucceeded") && strings.Contains(e, "https://github.com/example/repo/pull/42") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected JobSucceeded event with PR URL, got: %v", events)
	}
}

// TestReconcile_EmitsEvent_JobSucceeded_NoPR verifies that a JobSucceeded event without
// a PR URL is emitted when the owned Job has Succeeded>0 and PRRef is empty.
func TestReconcile_EmitsEvent_JobSucceeded_NoPR(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob-nopr", fp)
	rjob.Status.Phase = v1alpha1.PhaseDispatched

	succeededJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-rjob-nopr"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
		Status: batchv1.JobStatus{Succeeded: 1},
	}

	c := newFakeClient(t, rjob, succeededJob)
	jb := &fakeJobBuilder{}

	fakeRecorder := record.NewFakeRecorder(10)
	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     newTestScheme(t),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        defaultCfg(),
		Recorder:   fakeRecorder,
	}

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob-nopr"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := drainEvents(fakeRecorder.Events)
	var found bool
	for _, e := range events {
		if strings.Contains(e, "JobSucceeded") && strings.Contains(e, "Agent Job completed") && !strings.Contains(e, "PR:") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected JobSucceeded event without PR URL, got: %v", events)
	}
}

// TestReconcile_EmitsEvent_JobFailed verifies that a Warning JobFailed event is emitted
// when the owned Job has Failed >= BackoffLimit+1.
func TestReconcile_EmitsEvent_JobFailed(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob-failed", fp)
	rjob.Status.Phase = v1alpha1.PhaseDispatched

	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-rjob-failed"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
		Status: batchv1.JobStatus{Failed: 2},
	}

	c := newFakeClient(t, rjob, failedJob)
	jb := &fakeJobBuilder{}

	fakeRecorder := record.NewFakeRecorder(10)
	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     newTestScheme(t),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        defaultCfg(),
		Recorder:   fakeRecorder,
	}

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob-failed"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := drainEvents(fakeRecorder.Events)
	var found bool
	for _, e := range events {
		if strings.Contains(e, "JobFailed") && strings.Contains(e, string(corev1.EventTypeWarning)) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Warning JobFailed event, got: %v", events)
	}
}

// TestReconcile_EmitsEvent_JobPermanentlyFailed verifies that a Warning JobPermanentlyFailed
// event is emitted (not JobFailed) when the owned Job fails and RetryCount reaches MaxRetries.
func TestReconcile_EmitsEvent_JobPermanentlyFailed(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob-perm-failed", fp)
	rjob.Status.Phase = v1alpha1.PhaseDispatched
	rjob.Spec.MaxRetries = 3
	rjob.Status.RetryCount = 2 // one more failure → RetryCount becomes 3 == MaxRetries → PermanentlyFailed

	backoffLimit := int32(1)
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/remediation-job": "test-rjob-perm-failed"},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: &backoffLimit},
		Status: batchv1.JobStatus{Failed: 2},
	}

	c := newFakeClient(t, rjob, failedJob)
	jb := &fakeJobBuilder{}

	fakeRecorder := record.NewFakeRecorder(10)
	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     newTestScheme(t),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        defaultCfg(),
		Recorder:   fakeRecorder,
	}

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob-perm-failed"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := drainEvents(fakeRecorder.Events)
	var foundPerm bool
	var foundFailed bool
	for _, e := range events {
		if strings.Contains(e, "JobPermanentlyFailed") {
			foundPerm = true
		}
		if strings.Contains(e, "JobFailed") && !strings.Contains(e, "JobPermanentlyFailed") {
			foundFailed = true
		}
	}
	if !foundPerm {
		t.Errorf("expected JobPermanentlyFailed event, got: %v", events)
	}
	if foundFailed {
		t.Errorf("expected no plain JobFailed event on permanently-failed path, got: %v", events)
	}
}

// TestReconcile_NilRecorder_NoPanic verifies that a nil Recorder does not panic
// when a PhasePending rjob is dispatched.
func TestReconcile_NilRecorder_NoPanic(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob-nil-rec", fp)
	rjob.Status.Phase = v1alpha1.PhasePending
	c := newFakeClient(t, rjob)

	job := defaultFakeJob(rjob)
	jb := &fakeJobBuilder{returnJob: job}

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     newTestScheme(t),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        defaultCfg(),
		// Recorder intentionally not set — zero value nil
	}

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob-nil-rec"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the job was still created (dispatch succeeded despite nil Recorder)
	var jobList batchv1.JobList
	if err := c.List(context.Background(), &jobList, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Errorf("expected 1 job created, got %d", len(jobList.Items))
	}
}

// TestReconcile_EmitsEvent_JobDispatched_AlreadyExists verifies that a Normal JobDispatched
// event is emitted even when dispatch hits the AlreadyExists path (a batch/v1 Job with the
// expected name was already present before Reconcile ran).
func TestReconcile_EmitsEvent_JobDispatched_AlreadyExists(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob := newRJob("test-rjob-already-exists", fp)
	rjob.Status.Phase = v1alpha1.PhasePending

	// Pre-create the Job with the same name that JobBuilder would produce, but WITHOUT
	// the rjob ownership label — this means the List query in the reconciler won't find
	// it, so the controller proceeds to dispatch(), which calls r.Create() and gets
	// AlreadyExists because the name conflicts.
	preExistingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: testNamespace,
			// intentionally no "remediation.mendabot.io/remediation-job" label
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			},
		},
		Spec: batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
	}

	c := newFakeClient(t, rjob, preExistingJob)

	job := defaultFakeJob(rjob)
	jb := &fakeJobBuilder{returnJob: job}

	fakeRecorder := record.NewFakeRecorder(10)
	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     newTestScheme(t),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        defaultCfg(),
		Recorder:   fakeRecorder,
	}

	_, err := r.Reconcile(context.Background(), rjobReqFor("test-rjob-already-exists"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := drainEvents(fakeRecorder.Events)
	var foundDispatched bool
	var foundNormal bool
	for _, e := range events {
		if strings.Contains(e, "JobDispatched") {
			foundDispatched = true
			if strings.Contains(e, string(corev1.EventTypeNormal)) {
				foundNormal = true
			}
		}
	}
	if !foundDispatched {
		t.Errorf("expected JobDispatched event on AlreadyExists path, got: %v", events)
	}
	if !foundNormal {
		t.Errorf("expected Normal event type for JobDispatched on AlreadyExists path, got: %v", events)
	}
}
