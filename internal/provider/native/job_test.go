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

	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
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

// ptr returns a pointer to any value.
func ptr[T any](v T) *T { return &v }

// newTestConfig returns a minimal config for testing.
func newTestConfig() config.Config {
	return config.Config{
		GitOpsRepo:         "test/repo",
		GitOpsManifestRoot: "manifests",
		AgentImage:         "test/image:latest",
		AgentNamespace:     "default",
		AgentSA:            "default",
	}
}

// TestJobProviderName_IsNative verifies ProviderName() returns "native".
func TestJobProviderName_IsNative(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestJobObjectType_IsJob verifies ObjectType() returns a *batchv1.Job.
func TestJobObjectType_IsJob(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	obj := p.ObjectType()
	if _, ok := obj.(*batchv1.Job); !ok {
		t.Errorf("ObjectType() returned %T, want *batchv1.Job", obj)
	}
}

// TestHealthyJob_ReturnsNil: active > 0, no failures → (nil, nil).
func TestHealthyJob_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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

// TestFailedJobNoActive_Detected: failed > 0, active == 0, completionTime == nil → finding.
func TestFailedJobNoActive_Detected(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

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
}

// TestCronJobOwned_ReturnsNil: Job with ownerReference Kind=CronJob → (nil, nil), excluded.
func TestCronJobOwned_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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

// TestJobSourceRef_IsBatchV1: SourceRef identifies the job with APIVersion "batch/v1".
func TestJobSourceRef_IsBatchV1(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("my-job", "production", 3)

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	wantRef := domain.SourceRef{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       "my-job",
		Namespace:  "production",
	}
	if finding.SourceRef != wantRef {
		t.Errorf("SourceRef = %+v, want %+v", finding.SourceRef, wantRef)
	}
}

// TestJobStandaloneParentObject: exhausted Job with no ownerReferences →
// Finding.ParentObject == "Job/<name>".
func TestJobStandaloneParentObject(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
	p := NewJobProvider(c, newTestConfig())

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
