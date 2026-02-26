package controller_test

import (
	"context"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/controller"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newDryRunRJobWithJob(
	rjobName, fp string,
	rjobPhase v1alpha1.RemediationJobPhase,
	jobAnnotations map[string]string,
) (*v1alpha1.RemediationJob, *batchv1.Job) {
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rjobName,
			Namespace: testNamespace,
			UID:       types.UID("uid-" + rjobName),
		},
		Spec: v1alpha1.RemediationJobSpec{Fingerprint: fp},
	}
	rjob.Status.Phase = rjobPhase

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "mendabot-agent-" + fp[:12],
			Namespace:   testNamespace,
			Labels:      map[string]string{"remediation.mendabot.io/remediation-job": rjobName},
			Annotations: jobAnnotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "remediation.mendabot.io/v1alpha1",
					Kind:       "RemediationJob",
					Name:       rjobName,
					UID:        types.UID("uid-" + rjobName),
					Controller: ptr(true),
				},
			},
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	return rjob, job
}

// newDryRunCM creates the ConfigMap that emit_dry_run_report() would write.
func newDryRunCM(cmName, namespace, report, patch string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"report": report,
			"patch":  patch,
		},
	}
}

// ---------------------------------------------------------------------------
// DryRunCMName unit tests
// ---------------------------------------------------------------------------

// TestDryRunCMName verifies the exported naming function used by both the
// controller (production) and tests. Having a single implementation eliminates
// the silent-divergence risk identified in the audit.
func TestDryRunCMName(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint string
		want        string
	}{
		{
			name:        "longer than 12 chars — truncated",
			fingerprint: "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345",
			want:        "mendabot-dryrun-abcdefghijkl",
		},
		{
			name:        "exactly 12 chars — boundary, no truncation",
			fingerprint: "abcdefghijkl",
			want:        "mendabot-dryrun-abcdefghijkl",
		},
		{
			name:        "shorter than 12 chars — used as-is (jobbuilder rejects this, but function should not panic)",
			fingerprint: "short",
			want:        "mendabot-dryrun-short",
		},
		{
			name:        "different 12-char prefix",
			fingerprint: "aabbccddeeff00112233445566778899",
			want:        "mendabot-dryrun-aabbccddeeff",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := controller.DryRunCMName(tt.fingerprint)
			if got != tt.want {
				t.Errorf("DryRunCMName(%q) = %q, want %q", tt.fingerprint, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Reconciler integration tests
// ---------------------------------------------------------------------------

// TestReconcile_DryRunSucceeded_ReportAndPatchStored verifies the happy path:
// ConfigMap present with report+patch → both appear in status.message; CM deleted.
func TestReconcile_DryRunSucceeded_ReportAndPatchStored(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-full", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	cmName := controller.DryRunCMName(fp)
	cm := newDryRunCM(cmName, testNamespace,
		"## Root Cause\nImagePullBackOff — image not found.",
		"diff --git a/foo.yaml b/foo.yaml\n--- a/foo.yaml\n+++ b/foo.yaml\n@@ -1 +1 @@\n-old\n+new",
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-full", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-full", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseSucceeded {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseSucceeded)
	}
	if !strings.Contains(updated.Status.Message, "ImagePullBackOff") {
		t.Errorf("Message = %q — want report content", updated.Status.Message)
	}
	if !strings.Contains(updated.Status.Message, "PROPOSED PATCH") {
		t.Errorf("Message = %q — want patch section", updated.Status.Message)
	}
	if !strings.Contains(updated.Status.Message, "+new") {
		t.Errorf("Message = %q — want patch content", updated.Status.Message)
	}

	// ConfigMap must be deleted after reading.
	var remaining corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{Name: cmName, Namespace: testNamespace}, &remaining)
	if err == nil {
		t.Error("expected ConfigMap to be deleted after reading, but it still exists")
	}
}

// TestReconcile_DryRunSucceeded_ReportOnlyNoPatch verifies that when the patch
// key is empty, the PROPOSED PATCH section is omitted from status.message.
func TestReconcile_DryRunSucceeded_ReportOnlyNoPatch(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-nopatch", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	cm := newDryRunCM(controller.DryRunCMName(fp), testNamespace,
		"## Root Cause\nThe image tag was wrong.",
		"", // no patch
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-nopatch", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-nopatch", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.Contains(updated.Status.Message, "image tag was wrong") {
		t.Errorf("Message = %q — want report content", updated.Status.Message)
	}
	if strings.Contains(updated.Status.Message, "PROPOSED PATCH") {
		t.Errorf("Message = %q — must NOT contain patch section when patch is empty", updated.Status.Message)
	}
}

// TestReconcile_DryRunSucceeded_CMNotFound verifies that when the ConfigMap is
// absent (agent crashed before writing it), Message starts with
// "dry-run report unavailable".
func TestReconcile_DryRunSucceeded_CMNotFound(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-nocm", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	// No ConfigMap created.
	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-nocm", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-nocm", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.HasPrefix(updated.Status.Message, "dry-run report unavailable") {
		t.Errorf("Message = %q, want prefix \"dry-run report unavailable\"", updated.Status.Message)
	}
}

// TestReconcile_NoDryRun_MessageNotPopulated verifies that a succeeded Job
// without the dry-run annotation leaves Message empty.
func TestReconcile_NoDryRun_MessageNotPopulated(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-nodryrun", fp, v1alpha1.PhaseDispatched,
		nil, // no annotations
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-nodryrun", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-nodryrun", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Message != "" {
		t.Errorf("Message = %q, want empty string for non-dry-run succeeded job", updated.Status.Message)
	}
}

// TestReconcile_DryRunSucceeded_MessageAlreadySet verifies that when
// rjob.Status.Message is already set, the reconciler does not overwrite it
// and does not touch the ConfigMap.
func TestReconcile_DryRunSucceeded_MessageAlreadySet(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-alreadyset", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)
	rjob.Status.Message = "existing report"

	// A CM exists with different content — if idempotency guard fires, CM is
	// NOT read and Message stays "existing report".
	cmName := controller.DryRunCMName(fp)
	cm := newDryRunCM(cmName, testNamespace, "NEW CONTENT", "")

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-alreadyset", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-alreadyset", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Message != "existing report" {
		t.Errorf("Message = %q, want \"existing report\" — idempotency guard must prevent overwrite", updated.Status.Message)
	}
	if updated.Status.Phase != v1alpha1.PhaseSucceeded {
		t.Errorf("Phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseSucceeded)
	}

	// CM must NOT have been deleted (guard fired before fetchDryRunReport).
	var remaining corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Name: cmName, Namespace: testNamespace}, &remaining); err != nil {
		t.Error("expected ConfigMap to still exist when idempotency guard fired, but it was deleted")
	}
}

// TestReconcile_DryRunSucceeded_CMNameDerivedFromFingerprint verifies that the
// controller constructs the ConfigMap name as "mendabot-dryrun-<fp[:12]>", which
// must match what emit_dry_run_report() writes. Tests the exact-12-char boundary.
func TestReconcile_DryRunSucceeded_CMNameDerivedFromFingerprint(t *testing.T) {
	// Use a fingerprint whose first 12 chars are distinctive.
	const fp = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-cmname", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	expectedCMName := controller.DryRunCMName(fp) // "mendabot-dryrun-aabbccddeeff"
	cm := newDryRunCM(expectedCMName, testNamespace, "root cause found", "")

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-cmname", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-cmname", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.Contains(updated.Status.Message, "root cause found") {
		t.Errorf("Message = %q — expected report content; CM name derivation may be wrong", updated.Status.Message)
	}

	// CM was consumed (deleted).
	var remaining corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{Name: expectedCMName, Namespace: testNamespace}, &remaining)
	if err == nil {
		t.Error("expected ConfigMap to be deleted after reading")
	}
}

// TestReconcile_DryRunSucceeded_WrongNamespaceCMNotFound verifies that a
// ConfigMap written in the wrong namespace is not found by the controller,
// confirming that AGENT_NAMESPACE and r.Cfg.AgentNamespace must agree.
func TestReconcile_DryRunSucceeded_WrongNamespaceCMNotFound(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-wrongns", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	// CM written to a different namespace than AgentNamespace.
	cmName := controller.DryRunCMName(fp)
	cm := newDryRunCM(cmName, "wrong-namespace", "should not be found", "")

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(), // AgentNamespace = testNamespace, not "wrong-namespace"
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-wrongns", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-wrongns", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	// Controller should report CM not found — it looked in the wrong namespace.
	if !strings.HasPrefix(updated.Status.Message, "dry-run report unavailable") {
		t.Errorf("Message = %q — expected \"dry-run report unavailable\" when CM is in wrong namespace", updated.Status.Message)
	}
	if strings.Contains(updated.Status.Message, "should not be found") {
		t.Errorf("Message = %q — CM from wrong namespace must not be read", updated.Status.Message)
	}
}

// TestReconcile_DryRunSucceeded_CMDeletedAfterRead verifies the controller
// performs a best-effort delete after reading.
func TestReconcile_DryRunSucceeded_CMDeletedAfterRead(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-cmdelete", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	cmName := controller.DryRunCMName(fp)
	cm := newDryRunCM(cmName, testNamespace, "some report", "some patch")

	// An unrelated CM in the same namespace must not be touched.
	unrelated := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated-cm", Namespace: testNamespace},
		Data:       map[string]string{"key": "value"},
	}

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm, unrelated).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-cmdelete", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dry-run CM must be gone.
	var remaining corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{Name: cmName, Namespace: testNamespace}, &remaining)
	if err == nil {
		t.Errorf("ConfigMap %q still exists after reconcile — expected deletion", cmName)
	}

	// Unrelated CM must still exist.
	var unrelatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Name: "unrelated-cm", Namespace: testNamespace}, &unrelatedCM); err != nil {
		t.Error("unrelated ConfigMap was unexpectedly deleted")
	}
}
