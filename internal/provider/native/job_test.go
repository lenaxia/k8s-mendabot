package native

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

// newExhaustedJob returns a Job in the backoff-exhausted state:
// failed > 0, active == 0, completionTime == nil.
func newExhaustedJob(name, namespace string, failedCount int32) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: batchv1.JobStatus{
			Failed: failedCount,
			Active: 0,
		},
	}
}

// TestJobProviderName_IsNative verifies ProviderName() returns "native".
func TestJobProviderName_IsNative(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestJobObjectType_IsJob verifies ObjectType() returns a *batchv1.Job.
func TestJobObjectType_IsJob(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	obj := p.ObjectType()
	if _, ok := obj.(*batchv1.Job); !ok {
		t.Errorf("ObjectType() returned %T, want *batchv1.Job", obj)
	}
}

// TestHealthyJob_ReturnsNil: active > 0, no failures → (nil, nil).
func TestHealthyJob_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "default"},
		Status: batchv1.JobStatus{
			Active: 1,
			Failed: 0,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for healthy job, got %+v", finding)
	}
}

// TestSucceededJob_ReturnsNil: completionTime set → (nil, nil).
func TestSucceededJob_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	now := metav1.NewTime(time.Now())
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "default"},
		Status: batchv1.JobStatus{
			Succeeded:      1,
			CompletionTime: &now,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for succeeded job (completionTime set), got %+v", finding)
	}
}

// TestFailedJobNoActive_Detected: failed > 0, active == 0, completionTime == nil → finding; severity = medium.
func TestFailedJobNoActive_Detected(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("my-job", "default", 3)

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for exhausted job, got nil")
	}
	if finding.Kind != "Job" {
		t.Errorf("finding.Kind = %q, want %q", finding.Kind, "Job")
	}
	if finding.Name != "my-job" {
		t.Errorf("finding.Name = %q, want %q", finding.Name, "my-job")
	}
	if finding.Namespace != "default" {
		t.Errorf("finding.Namespace = %q, want %q", finding.Namespace, "default")
	}
	assertErrorsJSON(t, finding.Errors)
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityMedium)
	}
}

// TestCronJobOwned_ReturnsNil: Job with ownerReference Kind=CronJob → (nil, nil), excluded.
func TestCronJobOwned_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("cronjob-run-1", "default", 2)
	job.OwnerReferences = []metav1.OwnerReference{
		ownerRef("CronJob", "my-cronjob", "cj-uid-1"),
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for CronJob-owned job, got %+v", finding)
	}
}

// TestFailedWithActiveStillRunning_ReturnsNil: failed > 0 but active > 0 (still retrying) → (nil, nil).
func TestFailedWithActiveStillRunning_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "default"},
		Status: batchv1.JobStatus{
			Failed: 2,
			Active: 1,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for job still retrying (active > 0), got %+v", finding)
	}
}

// TestCompletedSuccessfully_ReturnsNil: completionTime set (even with prior failures) → (nil, nil).
func TestCompletedSuccessfully_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	now := metav1.NewTime(time.Now())
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "default"},
		Status: batchv1.JobStatus{
			Failed:         1,
			Active:         0,
			Succeeded:      1,
			CompletionTime: &now,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for job with completionTime set, got %+v", finding)
	}
}

// TestZeroFailedZeroActive_ReturnsNil: failed=0, active=0, completionTime=nil — Job never ran → (nil, nil).
func TestZeroFailedZeroActive_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "default"},
		Status: batchv1.JobStatus{
			Failed: 0,
			Active: 0,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for job with zero failures, got %+v", finding)
	}
}

// TestSuspendedJob_ReturnsNil: status.conditions contains Suspended=True → (nil, nil).
func TestSuspendedJob_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("my-job", "default", 2)
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:   batchv1.JobSuspended,
			Status: corev1.ConditionTrue,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for suspended job, got %+v", finding)
	}
}

// TestJobWrongType_ReturnsError: passing a non-Job object → (nil, error).
func TestJobWrongType_ReturnsError(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "default"},
	}
	finding, err := p.ExtractFinding(pod)
	if err == nil {
		t.Fatal("expected error for wrong type, got nil")
	}
	if finding != nil {
		t.Errorf("expected nil finding on error, got %+v", finding)
	}
}

// TestJobFindingErrors_IsValidJSON: errors field must be a valid JSON array with ≥1 entry.
func TestJobFindingErrors_IsValidJSON(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("my-job", "default", 3)

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	var entries []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(finding.Errors), &entries); err != nil {
		t.Errorf("Errors field is not valid JSON: %v — value: %s", err, finding.Errors)
	}
	if len(entries) == 0 {
		t.Errorf("Errors JSON array is empty, expected at least one entry")
	}
}

// TestJobErrorText_IncludesFailureCount: error text includes the failed attempt count.
func TestJobErrorText_IncludesFailureCount(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("my-job", "default", 5)

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "5")
}

// TestJobStandaloneParentObject: exhausted Job with no ownerReferences →
// Finding.ParentObject == "Job/<name>".
func TestJobStandaloneParentObject(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("my-job", "default", 3)
	job.OwnerReferences = nil

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	want := "Job/my-job"
	if finding.ParentObject != want {
		t.Errorf("finding.ParentObject = %q, want %q", finding.ParentObject, want)
	}
}

// TestJobFailedWithConditionReason: exhausted Job with a Failed condition containing
// Reason and Message → finding error text contains both.
func TestJobFailedWithConditionReason(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("my-job", "default", 3)
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:    batchv1.JobFailed,
			Status:  corev1.ConditionTrue,
			Reason:  "BackoffLimitExceeded",
			Message: "Job has reached the specified backoff limit",
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "BackoffLimitExceeded")
	assertErrorTextContains(t, finding.Errors, "Job has reached the specified backoff limit")
}

// TestJobErrorText_Format: error text matches expected format "job <name>: failed (<X> attempts, 0 active)".
func TestJobErrorText_Format(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("my-job", "default", 4)

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	var entries []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(finding.Errors), &entries); err != nil {
		t.Fatalf("Errors is not valid JSON: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("Errors JSON array is empty")
	}

	wantSubstr := "job my-job: failed (4 attempts, 0 active)"
	if !strings.Contains(entries[0].Text, wantSubstr) {
		t.Errorf("error text %q does not contain %q", entries[0].Text, wantSubstr)
	}
}

// TestJobChainDepth_NonMechanicJob: non-mechanic failed job → ChainDepth = 0, finding != nil.
func TestJobChainDepth_NonMechanicJob(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("some-other-job", "default", 2)

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding for non-mechanic job")
	}
	if finding.ChainDepth != 0 {
		t.Errorf("ChainDepth = %d, want 0 for non-mechanic job", finding.ChainDepth)
	}
}

// TestJobChainDepth_MechanicOwnerDepth0: mechanic job, owner RJob has ChainDepth=0 → Finding.ChainDepth = 1.
func TestJobChainDepth_MechanicOwnerDepth0(t *testing.T) {
	s := newTestScheme()

	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-abc123",
			Namespace: "mechanic-system",
			UID:       "rjob-uid-1",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Finding: v1alpha1.FindingSpec{ChainDepth: 0},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(rjob).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("mechanic-agent-abc123456789", "mechanic-system", 2)
	job.Labels = map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"}
	job.OwnerReferences = []metav1.OwnerReference{
		ownerRef("RemediationJob", "rjob-abc123", "rjob-uid-1"),
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding for mechanic job")
	}
	if finding.ChainDepth != 1 {
		t.Errorf("ChainDepth = %d, want 1", finding.ChainDepth)
	}
}

// TestJobChainDepth_MechanicOwnerDepth1: mechanic job, owner RJob has ChainDepth=1 → Finding.ChainDepth = 2.
func TestJobChainDepth_MechanicOwnerDepth1(t *testing.T) {
	s := newTestScheme()

	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-abc123",
			Namespace: "mechanic-system",
			UID:       "rjob-uid-2",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Finding: v1alpha1.FindingSpec{ChainDepth: 1},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(rjob).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("mechanic-agent-abc123456789", "mechanic-system", 3)
	job.Labels = map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"}
	job.OwnerReferences = []metav1.OwnerReference{
		ownerRef("RemediationJob", "rjob-abc123", "rjob-uid-2"),
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding for mechanic job")
	}
	if finding.ChainDepth != 2 {
		t.Errorf("ChainDepth = %d, want 2", finding.ChainDepth)
	}
}

// TestJobChainDepth_MechanicOwnerNotFound: mechanic job, owner RJob not in cluster → ChainDepth = 1, finding != nil.
func TestJobChainDepth_MechanicOwnerNotFound(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("mechanic-agent-abc123456789", "mechanic-system", 2)
	job.Labels = map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"}
	job.OwnerReferences = []metav1.OwnerReference{
		ownerRef("RemediationJob", "rjob-missing", "rjob-uid-missing"),
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding when owner not found")
	}
	if finding.ChainDepth != 1 {
		t.Errorf("ChainDepth = %d, want 1 (default when owner not found)", finding.ChainDepth)
	}
}

// TestJobChainDepth_MechanicNoOwnerRef: mechanic job, no owner reference → ChainDepth = 1, finding != nil.
func TestJobChainDepth_MechanicNoOwnerRef(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("mechanic-agent-abc123456789", "mechanic-system", 2)
	job.Labels = map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"}
	job.OwnerReferences = nil

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding when mechanic job has no owner reference")
	}
	if finding.ChainDepth != 1 {
		t.Errorf("ChainDepth = %d, want 1 (default when no owner reference)", finding.ChainDepth)
	}
}

// TestJobChainDepth_MechanicMalformedOwnerRef: mechanic job with Kind=RemediationJob but
// empty Name → client.Get called with empty name returns NotFound → ChainDepth = 1 (safe fallback).
func TestJobChainDepth_MechanicMalformedOwnerRef(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("mechanic-agent-abc123456789", "mechanic-system", 2)
	job.Labels = map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"}
	// ownerReference has the right Kind but an empty Name — simulates a malformed ref.
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "remediation.mechanic.io/v1alpha1",
			Kind:       "RemediationJob",
			Name:       "", // empty — malformed
			UID:        "some-uid",
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding for malformed ownerRef mechanic job")
	}
	if finding.ChainDepth != 1 {
		t.Errorf("ChainDepth = %d, want 1 (safe fallback for malformed ownerRef)", finding.ChainDepth)
	}
}

// TestJobChainDepth_MechanicStillActive: mechanic job, Active=1, Failed=0 → (nil, nil).
func TestJobChainDepth_MechanicStillActive(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mechanic-agent-abc123456789",
			Namespace: "mechanic-system",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"},
		},
		Status: batchv1.JobStatus{
			Active: 1,
			Failed: 0,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for still-active mechanic job, got %+v", finding)
	}
}

// TestJobChainDepth_MechanicSucceeded: mechanic job with CompletionTime set → (nil, nil).
func TestJobChainDepth_MechanicSucceeded(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	now := metav1.NewTime(time.Now())
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mechanic-agent-abc123456789",
			Namespace: "mechanic-system",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"},
		},
		Status: batchv1.JobStatus{
			Succeeded:      1,
			CompletionTime: &now,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for succeeded mechanic job, got %+v", finding)
	}
}

// TestJobChainDepth_MechanicCronJobOwned: CronJob-owned mechanic job → (nil, nil).
func TestJobChainDepth_MechanicCronJobOwned(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("mechanic-agent-abc123456789", "mechanic-system", 2)
	job.Labels = map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"}
	job.OwnerReferences = []metav1.OwnerReference{
		ownerRef("CronJob", "my-cronjob", "cj-uid-1"),
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for CronJob-owned mechanic job, got %+v", finding)
	}
}

// TestJobChainDepth_MechanicSuspended: suspended mechanic job → (nil, nil).
func TestJobChainDepth_MechanicSuspended(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("mechanic-agent-abc123456789", "mechanic-system", 2)
	job.Labels = map[string]string{"app.kubernetes.io/managed-by": "mechanic-watcher"}
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:   batchv1.JobSuspended,
			Status: corev1.ConditionTrue,
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for suspended mechanic job, got %+v", finding)
	}
}

// TestJobAnnotationEnabled_False: exhausted job (BackoffLimitExceeded) with mechanic.io/enabled=false → (nil, nil).
// Uses an unhealthy object to prove the gate fires on an object that would otherwise produce
// a non-nil finding.
func TestJobAnnotationEnabled_False(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("ann-job", "default", 3)
	job.Annotations = map[string]string{
		domain.AnnotationEnabled: "false",
	}
	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when annotation enabled=false, got %+v", finding)
	}
}

// TestJobAnnotationSkipUntilFuture: exhausted job with mechanic.io/skip-until=2099-12-31 → (nil, nil).
func TestJobAnnotationSkipUntilFuture(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("skip-job", "default", 3)
	job.Annotations = map[string]string{
		domain.AnnotationSkipUntil: "2099-12-31",
	}
	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when skip-until is in the future, got %+v", finding)
	}
}

// TestJobConditionMessageRedacted: job failed condition message containing password=secret123
// → error text must NOT contain "secret123" and must contain "[REDACTED]".
func TestJobConditionMessageRedacted(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, testRedactor(t))

	job := newExhaustedJob("redact-job", "default", 3)
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:    batchv1.JobFailed,
			Status:  corev1.ConditionTrue,
			Reason:  "BackoffLimitExceeded",
			Message: "job failed: password=secret123 rejected",
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	if contains(finding.Errors, "secret123") {
		t.Errorf("error text should not contain raw secret value 'secret123': %s", finding.Errors)
	}
	assertErrorTextContains(t, finding.Errors, "[REDACTED]")
}
