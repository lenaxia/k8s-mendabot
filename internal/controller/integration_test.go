package controller_test

import (
	"context"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/controller"
	"github.com/lenaxia/k8s-mendabot/internal/testutil"
	"go.uber.org/zap"
)

// newIntegrationClient returns a client that can operate on both v1alpha1 CRDs and
// batch/v1 Jobs. The suite_test.go k8sClient only includes v1alpha1 types, so we
// build a superset scheme here and create a new client for envtest integration tests.
func newIntegrationClient(t *testing.T) client.Client {
	t.Helper()
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	s := v1alpha1.NewScheme()
	if err := batchv1.AddToScheme(s); err != nil {
		t.Fatalf("batchv1.AddToScheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("clientgoscheme.AddToScheme: %v", err)
	}
	c, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

const integrationCtrlNamespace = "default"

func integrationControllerCfg() config.Config {
	return config.Config{
		AgentNamespace:           integrationCtrlNamespace,
		MaxConcurrentJobs:        10,
		RemediationJobTTLSeconds: 604800,
	}
}

func newCtrlReconciler(c client.Client, jb *fakeJobBuilder) (*controller.RemediationJobReconciler, *record.FakeRecorder) {
	rec := record.NewFakeRecorder(32)
	return &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     c.Scheme(),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        integrationControllerCfg(),
		Recorder:   rec,
	}, rec
}

func waitFor(t *testing.T, condition func() bool, timeout, interval time.Duration) {
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

// waitForGone polls until c.Get returns an error for obj, indicating the object has
// been removed (or is no longer accessible) in the API server. Use this after a Delete
// call when a subsequent Create of an object with the same name must not race with the
// deletion — batch/v1 Jobs have finalizers and remain in a terminating state after Delete
// is called until Kubernetes finishes cleanup.
func waitForGone(t *testing.T, ctx context.Context, c client.Client, obj client.Object, timeout time.Duration) {
	t.Helper()
	key := client.ObjectKeyFromObject(obj)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tmp := obj.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, key, tmp); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("waitForGone: %s/%s still present after %v; proceeding anyway", obj.GetNamespace(), obj.GetName(), timeout)
}

// deleteJob strips finalizers from a batch/v1 Job then deletes it. envtest does not
// run the batch controller, so Jobs with the batch.kubernetes.io/job-tracking finalizer
// would otherwise remain in a terminating state forever, causing the next test run to
// fail with "object is being deleted" when it tries to create a job with the same name.
// Callers must use this instead of c.Delete directly for all batch/v1 Job deletions.
func deleteJob(ctx context.Context, c client.Client, job *batchv1.Job) {
	existing := &batchv1.Job{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(job), existing); err != nil {
		return // already gone
	}
	if len(existing.Finalizers) > 0 {
		existing.Finalizers = nil
		_ = c.Update(ctx, existing)
	}
	_ = c.Delete(ctx, existing)
}

func newIntegrationRJob(name, fp string) *v1alpha1.RemediationJob {
	return &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: integrationCtrlNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeNative,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "result-test",
				Namespace: integrationCtrlNamespace,
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-test",
				Namespace:    integrationCtrlNamespace,
				ParentObject: "my-deploy",
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
		},
	}
}

func ctrlReq(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: integrationCtrlNamespace}}
}

// minimalPodTemplateSpec returns a minimal pod template spec accepted by Kubernetes.
func minimalPodTemplateSpec() corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "agent",
					Image: "mendabot-agent:test",
				},
			},
		},
	}
}

func newIntegrationJob(rjob *v1alpha1.RemediationJob) *batchv1.Job {
	ns := rjob.Namespace
	if ns == "" {
		ns = integrationCtrlNamespace
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + rjob.Spec.Fingerprint[:12],
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "remediation.mendabot.io/v1alpha1",
					Kind:       "RemediationJob",
					Name:       rjob.Name,
					UID:        rjob.UID,
				},
			},
			Labels: map[string]string{
				"remediation.mendabot.io/remediation-job": rjob.Name,
				"app.kubernetes.io/managed-by":            "mendabot-watcher",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr(int32(1)),
			Template:     minimalPodTemplateSpec(),
		},
	}
}

// TestRemediationJobReconciler_CreatesJob verifies: Pending RemediationJob → Job created
// and status updated to Dispatched.
func TestRemediationJobReconciler_CreatesJob(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	// Pre-test: batch/v1 Jobs have finalizers and remain terminating after Delete. Wait
	// until the deterministic job name from any prior run is fully gone so the reconciler
	// does not find a stale terminating job and skip dispatch.
	staleJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "mendabot-agent-aaaa0000bbbb", Namespace: integrationCtrlNamespace}}
	deleteJob(ctx, c, staleJob)
	waitForGone(t, ctx, c, staleJob, 10*time.Second)

	const fp = "aaaa0000bbbb1111cccc2222dddd3333aaaa0000bbbb1111cccc2222dddd3333"
	rjob := newIntegrationRJob("rjob-creates-job", fp)
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, rjob); err != nil {
		t.Fatalf("re-fetch rjob: %v", err)
	}

	job := newIntegrationJob(rjob)
	jb := &fakeJobBuilder{returnJob: job}
	rec, fakeRec := newCtrlReconciler(c, jb)

	// First call: "" → Pending (initialisation step).
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-creates-job")); err != nil {
		t.Fatalf("first Reconcile (init): %v", err)
	}
	// Verify Phase was set to Pending before proceeding — this pins the intermediate
	// state so a regression that removes the init guard cannot go undetected.
	var afterInit v1alpha1.RemediationJob
	if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, &afterInit); err != nil {
		t.Fatalf("get rjob after init: %v", err)
	}
	if afterInit.Status.Phase != v1alpha1.PhasePending {
		t.Errorf("phase after init = %q, want %q", afterInit.Status.Phase, v1alpha1.PhasePending)
	}
	// Second call: Pending → Dispatched + Job created.
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-creates-job")); err != nil {
		t.Fatalf("second Reconcile (dispatch): %v", err)
	}

	var jobList batchv1.JobList
	waitFor(t, func() bool {
		if err := c.List(ctx, &jobList, client.InNamespace(integrationCtrlNamespace),
			client.MatchingLabels{"remediation.mendabot.io/remediation-job": rjob.Name}); err != nil {
			return false
		}
		return len(jobList.Items) == 1
	}, 5*time.Second, 100*time.Millisecond)

	t.Cleanup(func() {
		for i := range jobList.Items {
			deleteJob(ctx, c, &jobList.Items[i])
		}
	})

	var updated v1alpha1.RemediationJob
	if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, &updated); err != nil {
		t.Fatalf("get updated rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseDispatched)
	}
	if updated.Status.JobRef == "" {
		t.Error("expected JobRef to be set")
	}
	if updated.Status.DispatchedAt == nil {
		t.Error("expected DispatchedAt to be set after dispatch")
	}

	events := testutil.DrainEvents(fakeRec)
	var foundDispatched bool
	for _, e := range events {
		if strings.Contains(e, "JobDispatched") {
			foundDispatched = true
			break
		}
	}
	if !foundDispatched {
		t.Errorf("expected JobDispatched event, got: %v", events)
	}
}

// TestRemediationJobReconciler_SyncsStatus_Running verifies: Job active → phase = Running.
func TestRemediationJobReconciler_SyncsStatus_Running(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	// Pre-test: wait for any stale job from a prior run to be fully gone.
	staleJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "mendabot-agent-bbbb1111cccc", Namespace: integrationCtrlNamespace}}
	deleteJob(ctx, c, staleJob)
	waitForGone(t, ctx, c, staleJob, 10*time.Second)

	const fp = "bbbb1111cccc2222dddd3333eeee4444bbbb1111cccc2222dddd3333eeee4444"
	rjob := newIntegrationRJob("rjob-syncs-running", fp)
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	existingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: integrationCtrlNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/remediation-job": rjob.Name,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1)), Template: minimalPodTemplateSpec()},
		Status: batchv1.JobStatus{Active: 1},
	}
	if err := c.Create(ctx, existingJob); err != nil {
		t.Fatalf("create Job: %v", err)
	}
	t.Cleanup(func() { deleteJob(ctx, c, existingJob) })

	existingJob.Status.Active = 1
	if err := c.Status().Update(ctx, existingJob); err != nil {
		t.Fatalf("update Job status: %v", err)
	}

	jb := &fakeJobBuilder{}
	rec, _ := newCtrlReconciler(c, jb)

	// First call: "" → Pending (initialisation step).
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-syncs-running")); err != nil {
		t.Fatalf("first Reconcile (init): %v", err)
	}
	// Second call: Pending, Job already exists → syncs status.
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-syncs-running")); err != nil {
		t.Fatalf("second Reconcile (sync): %v", err)
	}

	var updated v1alpha1.RemediationJob
	waitFor(t, func() bool {
		if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, &updated); err != nil {
			return false
		}
		return updated.Status.Phase == v1alpha1.PhaseRunning
	}, 5*time.Second, 100*time.Millisecond)
}

// TestRemediationJobReconciler_SyncsStatus_Succeeded verifies: Job succeeded → phase = Succeeded.
func TestRemediationJobReconciler_SyncsStatus_Succeeded(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	// Pre-test: wait for any stale job from a prior run to be fully gone.
	staleJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "mendabot-agent-cccc2222dddd", Namespace: integrationCtrlNamespace}}
	deleteJob(ctx, c, staleJob)
	waitForGone(t, ctx, c, staleJob, 10*time.Second)

	const fp = "cccc2222dddd3333eeee4444ffff5555cccc2222dddd3333eeee4444ffff5555"
	rjob := newIntegrationRJob("rjob-syncs-succeeded", fp)
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	existingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: integrationCtrlNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/remediation-job": rjob.Name,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1)), Template: minimalPodTemplateSpec()},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	if err := c.Create(ctx, existingJob); err != nil {
		t.Fatalf("create Job: %v", err)
	}
	t.Cleanup(func() { deleteJob(ctx, c, existingJob) })

	existingJob.Status.Succeeded = 1
	if err := c.Status().Update(ctx, existingJob); err != nil {
		t.Fatalf("update Job status: %v", err)
	}

	jb := &fakeJobBuilder{}
	rec, fakeRec := newCtrlReconciler(c, jb)

	// First call: "" → Pending (initialisation step).
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-syncs-succeeded")); err != nil {
		t.Fatalf("first Reconcile (init): %v", err)
	}
	// Second call: Pending, Job already exists → syncs status.
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-syncs-succeeded")); err != nil {
		t.Fatalf("second Reconcile (sync): %v", err)
	}

	var updated v1alpha1.RemediationJob
	waitFor(t, func() bool {
		if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, &updated); err != nil {
			return false
		}
		return updated.Status.Phase == v1alpha1.PhaseSucceeded
	}, 5*time.Second, 100*time.Millisecond)

	events := testutil.DrainEvents(fakeRec)
	var foundSucceeded bool
	for _, e := range events {
		if strings.Contains(e, "JobSucceeded") {
			foundSucceeded = true
			break
		}
	}
	if !foundSucceeded {
		t.Errorf("expected JobSucceeded event, got: %v", events)
	}
}

// TestRemediationJobReconciler_SyncsStatus_Failed verifies: Job failed → phase = Failed.
func TestRemediationJobReconciler_SyncsStatus_Failed(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	// Pre-test: wait for any stale job from a prior run to be fully gone.
	staleJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "mendabot-agent-dddd3333eeee", Namespace: integrationCtrlNamespace}}
	deleteJob(ctx, c, staleJob)
	waitForGone(t, ctx, c, staleJob, 10*time.Second)

	const fp = "dddd3333eeee4444ffff5555aaaa6666dddd3333eeee4444ffff5555aaaa6666"
	backoffLimit := int32(1)
	rjob := newIntegrationRJob("rjob-syncs-failed", fp)
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	existingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: integrationCtrlNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/remediation-job": rjob.Name,
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: &backoffLimit, Template: minimalPodTemplateSpec()},
		Status: batchv1.JobStatus{Failed: backoffLimit + 1},
	}
	if err := c.Create(ctx, existingJob); err != nil {
		t.Fatalf("create Job: %v", err)
	}
	t.Cleanup(func() { deleteJob(ctx, c, existingJob) })

	existingJob.Status.Failed = backoffLimit + 1
	if err := c.Status().Update(ctx, existingJob); err != nil {
		t.Fatalf("update Job status: %v", err)
	}

	jb := &fakeJobBuilder{}
	rec, fakeRec := newCtrlReconciler(c, jb)

	// First call: "" → Pending (initialisation step).
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-syncs-failed")); err != nil {
		t.Fatalf("first Reconcile (init): %v", err)
	}
	// Second call: Pending, Job already exists → syncs status.
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-syncs-failed")); err != nil {
		t.Fatalf("second Reconcile (sync): %v", err)
	}

	var updated v1alpha1.RemediationJob
	waitFor(t, func() bool {
		if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, &updated); err != nil {
			return false
		}
		return updated.Status.Phase == v1alpha1.PhaseFailed
	}, 5*time.Second, 100*time.Millisecond)

	events := testutil.DrainEvents(fakeRec)
	var foundFailed bool
	for _, e := range events {
		if strings.Contains(e, "JobFailed") {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Errorf("expected JobFailed event, got: %v", events)
	}
}

// TestRemediationJobReconciler_MaxConcurrentJobs_Requeues verifies: At limit → requeues,
// no new Job created.
func TestRemediationJobReconciler_MaxConcurrentJobs_Requeues(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	// Pre-test: wait for any stale blocker job from a prior run to be fully gone.
	staleJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "active-job-concurrent", Namespace: integrationCtrlNamespace}}
	deleteJob(ctx, c, staleJob)
	waitForGone(t, ctx, c, staleJob, 10*time.Second)

	const fp = "eeee4444ffff5555aaaa6666bbbb7777eeee4444ffff5555aaaa6666bbbb7777"
	rjob := newIntegrationRJob("rjob-max-concurrent", fp)
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	activeJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "active-job-concurrent",
			Namespace: integrationCtrlNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			},
		},
		Spec: batchv1.JobSpec{Template: minimalPodTemplateSpec()},
	}
	if err := c.Create(ctx, activeJob); err != nil {
		t.Fatalf("create active Job: %v", err)
	}
	t.Cleanup(func() { deleteJob(ctx, c, activeJob) })

	activeJob.Status.Active = 1
	if err := c.Status().Update(ctx, activeJob); err != nil {
		t.Fatalf("update active job status: %v", err)
	}
	// Re-fetch to confirm Status.Active persisted. The active-count logic in the
	// controller has two branches: Active>0 OR (Succeeded==0 && CompletionTime==nil).
	// We explicitly verify the Active>0 branch is exercised so a regression in that
	// branch is not masked by the second branch.
	var fetchedActiveJob batchv1.Job
	if err := c.Get(ctx, client.ObjectKeyFromObject(activeJob), &fetchedActiveJob); err != nil {
		t.Fatalf("re-fetch active job: %v", err)
	}
	if fetchedActiveJob.Status.Active != 1 {
		t.Fatalf("expected activeJob.Status.Active=1 after update, got %d — test precondition not met", fetchedActiveJob.Status.Active)
	}

	limitCfg := integrationControllerCfg()
	limitCfg.MaxConcurrentJobs = 1
	jb := &fakeJobBuilder{}
	rec := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     c.Scheme(),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        limitCfg,
	}

	result, err := rec.Reconcile(ctx, ctrlReq("rjob-max-concurrent"))
	if err != nil {
		t.Fatalf("first Reconcile (init): %v", err)
	}
	// First call: "" → Pending; Requeue=true, RequeueAfter=0.
	if !result.Requeue {
		t.Errorf("expected Requeue=true on blank-phase init, got %+v", result)
	}

	// Second call: Pending, at limit → RequeueAfter=30s.
	result, err = rec.Reconcile(ctx, ctrlReq("rjob-max-concurrent"))
	if err != nil {
		t.Fatalf("second Reconcile (at limit): %v", err)
	}
	if result.RequeueAfter != 30*time.Second {
		t.Errorf("RequeueAfter = %v, want 30s", result.RequeueAfter)
	}

	var jobList batchv1.JobList
	if err := c.List(ctx, &jobList, client.InNamespace(integrationCtrlNamespace),
		client.MatchingLabels{"remediation.mendabot.io/remediation-job": rjob.Name}); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Errorf("expected 0 new jobs, got %d", len(jobList.Items))
	}

	// Phase must be Pending — set by the blank-phase init on the first reconcile.
	var updated v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKey{Name: rjob.Name, Namespace: rjob.Namespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhasePending {
		t.Errorf("expected Phase=%s, got %s", v1alpha1.PhasePending, updated.Status.Phase)
	}
}

// TestRemediationJobReconciler_OwnerReference verifies: Created Job has ownerRef →
// RemediationJob.
func TestRemediationJobReconciler_OwnerReference(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	// Pre-test: wait for any stale job from a prior run to be fully gone so the
	// waitFor loop below does not find a job with an old OwnerReference UID.
	staleJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "mendabot-agent-ffff5555aaaa", Namespace: integrationCtrlNamespace}}
	deleteJob(ctx, c, staleJob)
	waitForGone(t, ctx, c, staleJob, 10*time.Second)

	const fp = "ffff5555aaaa6666bbbb7777cccc8888ffff5555aaaa6666bbbb7777cccc8888"
	rjob := newIntegrationRJob("rjob-ownerref", fp)
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, rjob); err != nil {
		t.Fatalf("re-fetch rjob (need UID): %v", err)
	}

	job := newIntegrationJob(rjob)
	jb := &fakeJobBuilder{returnJob: job}
	rec, _ := newCtrlReconciler(c, jb)

	// First call: "" → Pending (initialisation step).
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-ownerref")); err != nil {
		t.Fatalf("first Reconcile (init): %v", err)
	}
	// Second call: Pending → Dispatched + Job created.
	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-ownerref")); err != nil {
		t.Fatalf("second Reconcile (dispatch): %v", err)
	}

	var jobList batchv1.JobList
	waitFor(t, func() bool {
		if err := c.List(ctx, &jobList, client.InNamespace(integrationCtrlNamespace),
			client.MatchingLabels{"remediation.mendabot.io/remediation-job": rjob.Name}); err != nil {
			return false
		}
		return len(jobList.Items) == 1
	}, 5*time.Second, 100*time.Millisecond)

	t.Cleanup(func() {
		for i := range jobList.Items {
			deleteJob(ctx, c, &jobList.Items[i])
		}
	})

	j := jobList.Items[0]
	if len(j.OwnerReferences) == 0 {
		t.Fatal("expected OwnerReferences to be set on job")
	}
	ref := j.OwnerReferences[0]
	if ref.Name != rjob.Name {
		t.Errorf("ownerRef.Name = %q, want %q", ref.Name, rjob.Name)
	}
	if ref.Kind != "RemediationJob" {
		t.Errorf("ownerRef.Kind = %q, want %q", ref.Kind, "RemediationJob")
	}
	if ref.UID != rjob.UID {
		t.Errorf("ownerRef.UID = %q, want %q", ref.UID, rjob.UID)
	}
}

// TestRemediationJobReconciler_Terminal_NoOp verifies: Succeeded/Failed phase → no action
// (no new Job created, correct return behaviour).
func TestRemediationJobReconciler_Terminal_NoOp(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	cases := []struct {
		name               string
		rjobName           string
		phase              v1alpha1.RemediationJobPhase
		fp                 string
		setCompletedAt     bool
		expectRequeueAfter bool
	}{
		{
			name:     "Failed_NoRequeue",
			rjobName: "rjob-terminal-failed",
			phase:    v1alpha1.PhaseFailed,
			fp:       "1111aaaa2222bbbb3333cccc4444dddd1111aaaa2222bbbb3333cccc4444dddd",
		},
		{
			name:     "Succeeded_NoCompletedAt_NoRequeue",
			rjobName: "rjob-terminal-succeeded",
			phase:    v1alpha1.PhaseSucceeded,
			fp:       "2222bbbb3333cccc4444dddd5555eeee2222bbbb3333cccc4444dddd5555eeee",
		},
		{
			name:               "Succeeded_CompletedAt_TTLNotDue_Requeues",
			rjobName:           "rjob-terminal-succeeded-ttl",
			phase:              v1alpha1.PhaseSucceeded,
			fp:                 "3333cccc4444dddd5555eeee6666ffff3333cccc4444dddd5555eeee6666ffff",
			setCompletedAt:     true,
			expectRequeueAfter: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newIntegrationClient(t)

			cfg := integrationControllerCfg()
			cfg.RemediationJobTTLSeconds = 86400

			rjob := newIntegrationRJob(tc.rjobName, tc.fp)
			if err := c.Create(ctx, rjob); err != nil {
				t.Fatalf("create RemediationJob: %v", err)
			}
			t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjob.Status.Phase = tc.phase
			if tc.setCompletedAt {
				now := metav1.Now()
				rjob.Status.CompletedAt = &now
			}
			if err := c.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
				t.Fatalf("patch status: %v", err)
			}

			jb := &fakeJobBuilder{}
			rec := &controller.RemediationJobReconciler{
				Client:     c,
				Scheme:     c.Scheme(),
				Log:        zap.NewNop(),
				JobBuilder: jb,
				Cfg:        cfg,
			}

			result, err := rec.Reconcile(ctx, ctrlReq(tc.rjobName))
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}

			if len(jb.calls) != 0 {
				t.Errorf("expected no Build() calls for terminal phase %q, got %d", tc.phase, len(jb.calls))
			}

			if tc.phase == v1alpha1.PhaseFailed {
				if result.RequeueAfter != 0 || result.Requeue {
					t.Errorf("Failed phase: expected zero Result, got %+v", result)
				}
			}

			if tc.expectRequeueAfter {
				if result.RequeueAfter == 0 {
					t.Errorf("expected RequeueAfter > 0 for Succeeded+CompletedAt (TTL not yet due), got 0")
				}
			}

			var jobList batchv1.JobList
			if err := c.List(ctx, &jobList, client.InNamespace(integrationCtrlNamespace),
				client.MatchingLabels{"remediation.mendabot.io/remediation-job": tc.rjobName}); err != nil {
				t.Fatalf("list jobs: %v", err)
			}
			if len(jobList.Items) != 0 {
				t.Errorf("expected no jobs for terminal phase %q, got %d", tc.phase, len(jobList.Items))
			}
		})
	}
}

// TestRemediationJobReconciler_PermanentlyFailed verifies that when RetryCount >= MaxRetries
// the reconciler transitions the rjob to PhasePermanentlyFailed.
func TestRemediationJobReconciler_PermanentlyFailed(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	const fp = "eeee5555ffff6666aaaa7777bbbb8888eeee5555ffff6666aaaa7777bbbb8888"

	// Pre-test: wait for any stale job from a prior run to be fully gone.
	staleJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "mendabot-agent-" + fp[:12], Namespace: integrationCtrlNamespace}}
	deleteJob(ctx, c, staleJob)
	waitForGone(t, ctx, c, staleJob, 10*time.Second)

	rjob := newIntegrationRJob("rjob-perm-failed", fp)
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	// Set initial status: Phase=Dispatched, MaxRetries=2, RetryCount=1 — one more failure will hit the cap.
	rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	rjob.Status.Phase = v1alpha1.PhaseDispatched
	rjob.Status.RetryCount = 1
	if err := c.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
		t.Fatalf("patch status: %v", err)
	}

	// Re-fetch to apply MaxRetries via spec — patch spec directly.
	if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, rjob); err != nil {
		t.Fatalf("re-fetch rjob: %v", err)
	}
	rjobSpecCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	rjob.Spec.MaxRetries = 2
	if err := c.Patch(ctx, rjob, client.MergeFrom(rjobSpecCopy)); err != nil {
		t.Fatalf("patch spec MaxRetries: %v", err)
	}

	// Re-fetch to get the latest resourceVersion after spec patch.
	if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, rjob); err != nil {
		t.Fatalf("re-fetch rjob after spec patch: %v", err)
	}

	// Create an owned batch/v1 Job in failed state: BackoffLimit=1, Failed=2.
	backoffLimit := int32(1)
	existingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-" + fp[:12],
			Namespace: integrationCtrlNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/remediation-job": rjob.Name,
			},
		},
		Spec: batchv1.JobSpec{BackoffLimit: &backoffLimit, Template: minimalPodTemplateSpec()},
	}
	if err := c.Create(ctx, existingJob); err != nil {
		t.Fatalf("create Job: %v", err)
	}
	t.Cleanup(func() { deleteJob(ctx, c, existingJob) })

	existingJob.Status.Failed = backoffLimit + 1
	if err := c.Status().Update(ctx, existingJob); err != nil {
		t.Fatalf("update Job status: %v", err)
	}

	jb := &fakeJobBuilder{}
	rec, fakeRec := newCtrlReconciler(c, jb)

	if _, err := rec.Reconcile(ctx, ctrlReq("rjob-perm-failed")); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var updated v1alpha1.RemediationJob
	waitFor(t, func() bool {
		if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, &updated); err != nil {
			return false
		}
		return updated.Status.Phase == v1alpha1.PhasePermanentlyFailed
	}, 5*time.Second, 100*time.Millisecond)

	if updated.Status.Phase != v1alpha1.PhasePermanentlyFailed {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhasePermanentlyFailed)
	}

	events := testutil.DrainEvents(fakeRec)
	var foundPermFailed bool
	for _, e := range events {
		if strings.Contains(e, "JobPermanentlyFailed") {
			foundPermFailed = true
			break
		}
	}
	if !foundPermFailed {
		t.Errorf("expected JobPermanentlyFailed event, got: %v", events)
	}
}

// TestRemediationJobChainDepthRoundTrip verifies that FindingSpec.ChainDepth survives
// a full create/read round-trip through the envtest API server, confirming that the
// testdata CRD YAML includes chainDepth in the finding.properties schema.
func TestRemediationJobChainDepthRoundTrip(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chain-depth-roundtrip",
			Namespace: integrationCtrlNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "roundtrip-fp",
			SourceType:  "native",
			Finding: v1alpha1.FindingSpec{
				Kind:       "Job",
				Name:       "mendabot-agent-abc",
				Namespace:  integrationCtrlNamespace,
				ChainDepth: 2,
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
		},
	}

	// Pre-test cleanup: delete any stale object from a previous run.
	_ = c.Delete(ctx, &v1alpha1.RemediationJob{ObjectMeta: metav1.ObjectMeta{
		Name:      rjob.Name,
		Namespace: integrationCtrlNamespace,
	}})

	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("Create RemediationJob: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Delete(ctx, &v1alpha1.RemediationJob{ObjectMeta: metav1.ObjectMeta{
			Name:      rjob.Name,
			Namespace: integrationCtrlNamespace,
		}})
	})

	var got v1alpha1.RemediationJob
	if err := c.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationCtrlNamespace}, &got); err != nil {
		t.Fatalf("Get RemediationJob: %v", err)
	}

	if got.Spec.Finding.ChainDepth != 2 {
		t.Errorf("ChainDepth round-trip: got %d, want 2", got.Spec.Finding.ChainDepth)
	}
}
