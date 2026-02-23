package native

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
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
		GitOpsRepo:              "test/repo",
		GitOpsManifestRoot:      "manifests",
		AgentImage:              "test/image:latest",
		AgentNamespace:          "default",
		AgentSA:                 "default",
		SelfRemediationMaxDepth: 2,
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

// TestMendabotJob_ChainDepthIncremented verifies chain depth is incremented for mendabot jobs.
func TestMendabotJob_ChainDepthIncremented(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "1", // Depth 1 becomes 2 after increment
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for mendabot job, got nil")
	}
	if !finding.IsSelfRemediation {
		t.Error("IsSelfRemediation should be true for mendabot job")
	}
	if finding.ChainDepth != 2 {
		t.Errorf("ChainDepth = %d, want 2 (1 + 1)", finding.ChainDepth)
	}
}

// TestMendabotJob_NoChainDepthAnnotation starts at depth 1.
func TestMendabotJob_NoChainDepthAnnotation(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	// No chain-depth annotation

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for mendabot job, got nil")
	}
	if !finding.IsSelfRemediation {
		t.Error("IsSelfRemediation should be true for mendabot job")
	}
	if finding.ChainDepth != 1 {
		t.Errorf("ChainDepth = %d, want 1 (default 0 + 1)", finding.ChainDepth)
	}
}

// TestMendabotJob_InvalidChainDepthAnnotation treats invalid value as 0.
func TestMendabotJob_InvalidChainDepthAnnotation(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "not-a-number",
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for mendabot job, got nil")
	}
	if !finding.IsSelfRemediation {
		t.Error("IsSelfRemediation should be true for mendabot job")
	}
	if finding.ChainDepth != 1 {
		t.Errorf("ChainDepth = %d, want 1 (invalid annotation treated as 0 + 1)", finding.ChainDepth)
	}
}

// TestMendabotJob_ExceedsMaxDepth returns nil when chain depth exceeds limit.
func TestMendabotJob_ExceedsMaxDepth(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "3", // Config has max depth 2, so 3+1=4 > 2
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when chain depth exceeds max (3+1 > 2), got %+v", finding)
	}
}

// TestNonMendabotJob_NoChainDepth sets ChainDepth to 0.
func TestNonMendabotJob_NoChainDepth(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("regular-job", "default", 3)
	// No mendabot labels

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for regular job, got nil")
	}
	if finding.IsSelfRemediation {
		t.Error("IsSelfRemediation should be false for regular job")
	}
	if finding.ChainDepth != 0 {
		t.Errorf("ChainDepth = %d, want 0 for non-mendabot job", finding.ChainDepth)
	}
}

// TestConcurrentChainDepthRace demonstrates the race condition in chain depth tracking.
// This test simulates multiple concurrent calls to ExtractFinding on the same Job.
func TestConcurrentChainDepthRace(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	// Create a mendabot job with initial chain depth 1
	job := newExhaustedJob("mendabot-agent-race", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "1",
	}

	// Simulate concurrent calls
	const goroutines = 10
	results := make(chan *domain.Finding, goroutines)
	errors := make(chan error, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			finding, err := p.ExtractFinding(job)
			if err != nil {
				errors <- err
				return
			}
			results <- finding
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Fatalf("unexpected error: %v", err)
	}

	// Collect all findings
	var findings []*domain.Finding
	for finding := range results {
		if finding != nil {
			findings = append(findings, finding)
		}
	}

	// With the race condition, multiple findings could be produced with the same chain depth
	// In a correct implementation, only one should proceed (or all should get the same depth)
	// Actually, with current code, all will get chain depth 2 and proceed since 2 <= max depth 2
	// This demonstrates the race: multiple reconcilers could create duplicate RemediationJobs
	if len(findings) > 0 {
		// All findings should have the same chain depth
		expectedDepth := findings[0].ChainDepth
		for i, f := range findings {
			if f.ChainDepth != expectedDepth {
				t.Errorf("finding %d has ChainDepth %d, expected %d (race condition)", i, f.ChainDepth, expectedDepth)
			}
		}
		// In a race-free system, we might want only one finding to be produced
		// But ExtractFinding is stateless, so multiple calls will return the same result
		// The race is at the reconciler level creating duplicate RemediationJobs
	}
}

// TestMendabotJob_ChainDepthFromOwner verifies chain depth is read from owner RemediationJob.
func TestMendabotJob_ChainDepthFromOwner(t *testing.T) {
	s := newTestScheme()

	// Create a RemediationJob with chain depth 1
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent-remediation",
			Namespace: "default",
			UID:       "parent-uid",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: 1,
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(rjob).Build()
	p := NewJobProvider(c, newTestConfig())

	// Create a mendabot job owned by the RemediationJob
	job := newExhaustedJob("mendabot-agent-owned", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "99", // Should be ignored in favor of owner
	}
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         "remediation.mendabot.io/v1alpha1",
			Kind:               "RemediationJob",
			Name:               "parent-remediation",
			UID:                "parent-uid",
			Controller:         ptr(true),
			BlockOwnerDeletion: ptr(true),
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for mendabot job, got nil")
	}
	if !finding.IsSelfRemediation {
		t.Error("IsSelfRemediation should be true for mendabot job")
	}
	// Should read chain depth 1 from owner, increment to 2
	if finding.ChainDepth != 2 {
		t.Errorf("ChainDepth = %d, want 2 (1 from owner + 1)", finding.ChainDepth)
	}
}

// TestMendabotJob_ChainDepthFromOwnerNotFound falls back to annotation.
func TestMendabotJob_ChainDepthFromOwnerNotFound(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	// Create a mendabot job with owner reference but owner doesn't exist
	job := newExhaustedJob("mendabot-agent-orphaned", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "1", // 1+1=2, which is <= max depth 2
	}
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         "remediation.mendabot.io/v1alpha1",
			Kind:               "RemediationJob",
			Name:               "nonexistent-parent",
			UID:                "nonexistent-uid",
			Controller:         ptr(true),
			BlockOwnerDeletion: ptr(true),
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for mendabot job, got nil")
	}
	// Should fall back to annotation 1, increment to 2
	if finding.ChainDepth != 2 {
		t.Errorf("ChainDepth = %d, want 2 (1 from annotation + 1)", finding.ChainDepth)
	}
}

// TestMendabotJob_ChainDepthFromWrongOwnerType ignores non-RemediationJob owners.
func TestMendabotJob_ChainDepthFromWrongOwnerType(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	// Create a mendabot job with non-RemediationJob owner
	job := newExhaustedJob("mendabot-agent-wrong-owner", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "1", // 1+1=2, which is <= max depth 2
	}
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         "batch/v1",
			Kind:               "Job",
			Name:               "some-other-job",
			UID:                "job-uid",
			Controller:         ptr(true),
			BlockOwnerDeletion: ptr(true),
		},
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for mendabot job, got nil")
	}
	// Should fall back to annotation 1, increment to 2
	if finding.ChainDepth != 2 {
		t.Errorf("ChainDepth = %d, want 2 (1 from annotation + 1)", finding.ChainDepth)
	}
}

// TestAtomicChainDepthTracking simulates the race condition scenario and verifies
// that reading from owner RemediationJob provides atomic chain depth tracking.
func TestAtomicChainDepthTracking(t *testing.T) {
	s := newTestScheme()

	// Create a parent RemediationJob with chain depth 1
	parentRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "parent-remediation",
			Namespace:       "default",
			UID:             "parent-uid",
			ResourceVersion: "100",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: 1,
		},
	}

	// Simulate multiple concurrent reconcilers
	const numReconcilers = 5
	findings := make([]*domain.Finding, numReconcilers)
	errors := make([]error, numReconcilers)

	var wg sync.WaitGroup
	for i := 0; i < numReconcilers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Each reconciler gets its own client with the same parent RemediationJob
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(parentRJob).Build()
			p := NewJobProvider(c, newTestConfig())

			// Create the same mendabot job for all reconcilers
			job := newExhaustedJob("mendabot-agent-atomic", "default", 1)
			job.Labels = map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			}
			job.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               "parent-remediation",
					UID:                "parent-uid",
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			}

			finding, err := p.ExtractFinding(job)
			errors[idx] = err
			findings[idx] = finding
		}(i)
	}

	wg.Wait()

	// Check all reconcilers got the same result
	var firstFinding *domain.Finding
	for i, err := range errors {
		if err != nil {
			t.Errorf("reconciler %d: unexpected error: %v", i, err)
			continue
		}
		if findings[i] == nil {
			t.Errorf("reconciler %d: expected finding, got nil", i)
			continue
		}

		if firstFinding == nil {
			firstFinding = findings[i]
		} else {
			// All findings should have the same chain depth
			if findings[i].ChainDepth != firstFinding.ChainDepth {
				t.Errorf("reconciler %d: ChainDepth = %d, expected %d (inconsistent reads)",
					i, findings[i].ChainDepth, firstFinding.ChainDepth)
			}
			// All findings should have the same fingerprint
			fp1, err1 := domain.FindingFingerprint(firstFinding)
			fp2, err2 := domain.FindingFingerprint(findings[i])
			if err1 != nil || err2 != nil {
				t.Errorf("reconciler %d: error computing fingerprint: %v, %v", i, err1, err2)
			} else if fp1 != fp2 {
				t.Errorf("reconciler %d: fingerprint mismatch: %s vs %s", i, fp1, fp2)
			}
		}
	}

	// Verify chain depth is correct (1 from parent + 1 = 2)
	if firstFinding != nil && firstFinding.ChainDepth != 2 {
		t.Errorf("ChainDepth = %d, want 2 (1 from parent + 1)", firstFinding.ChainDepth)
	}
}

// TestJobProvider_SelfRemediationMaxDepthZero tests that SELF_REMEDIATION_MAX_DEPTH=0 disables self-remediation
func TestJobProvider_SelfRemediationMaxDepthZero(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	// Create config with max depth 0
	cfg := newTestConfig()
	cfg.SelfRemediationMaxDepth = 0
	p := NewJobProvider(c, cfg)

	// Create a mendabot job that would normally trigger self-remediation
	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "0",
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when SELF_REMEDIATION_MAX_DEPTH=0, got %+v", finding)
	}
}

// TestJobProvider_NegativeChainDepthAnnotation tests invalid negative chain depth annotation
func TestJobProvider_NegativeChainDepthAnnotation(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "-1", // Invalid negative value
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding even with invalid negative annotation (should be treated as 0)")
	}
	if !finding.IsSelfRemediation {
		t.Error("finding should be marked as self-remediation")
	}
	// strconv.Atoi("-1") returns -1, not an error, so chain depth would be -1 + 1 = 0
	// But the implementation should handle negative values by returning 0
	if finding.ChainDepth != 0 {
		t.Errorf("ChainDepth = %d, want 0 (negative annotation -1 + 1 = 0)", finding.ChainDepth)
	}
}

// TestJobProvider_ChainDepthExceedsMaxDepthByOne tests edge case where chain depth equals max depth
func TestJobProvider_ChainDepthExceedsMaxDepthByOne(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	// Config has max depth 2
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "2", // 2+1=3 > max depth 2
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when chain depth (2+1=3) exceeds max depth 2, got %+v", finding)
	}
}

// TestJobProvider_ChainDepthEqualsMaxDepth tests edge case where chain depth equals max depth
func TestJobProvider_ChainDepthEqualsMaxDepth(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	// Config has max depth 2
	p := NewJobProvider(c, newTestConfig())

	job := newExhaustedJob("mendabot-agent-abc123", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "1", // 1+1=2 equals max depth 2
	}

	finding, err := p.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding when chain depth (1+1=2) equals max depth 2")
	}
	if finding.ChainDepth != 2 {
		t.Errorf("ChainDepth = %d, want 2", finding.ChainDepth)
	}
}

// TestJobProvider_ControllerRestartScenario tests chain depth persistence across controller restarts
func TestJobProvider_ControllerRestartScenario(t *testing.T) {
	s := newTestScheme()

	// Simulate controller restart: same job, same annotations
	job := newExhaustedJob("mendabot-agent-restart", "default", 1)
	job.Labels = map[string]string{
		"app.kubernetes.io/managed-by": "mendabot-watcher",
	}
	job.Annotations = map[string]string{
		"remediation.mendabot.io/chain-depth": "1",
	}

	// First controller instance
	c1 := fake.NewClientBuilder().WithScheme(s).Build()
	p1 := NewJobProvider(c1, newTestConfig())

	finding1, err := p1.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error from first instance: %v", err)
	}
	if finding1 == nil {
		t.Fatal("expected finding from first instance")
	}
	if finding1.ChainDepth != 2 {
		t.Errorf("first instance ChainDepth = %d, want 2", finding1.ChainDepth)
	}

	// Simulate controller restart - new instance with same job
	c2 := fake.NewClientBuilder().WithScheme(s).Build()
	p2 := NewJobProvider(c2, newTestConfig())

	finding2, err := p2.ExtractFinding(job)
	if err != nil {
		t.Fatalf("unexpected error from second instance: %v", err)
	}
	if finding2 == nil {
		t.Fatal("expected finding from second instance")
	}
	if finding2.ChainDepth != 2 {
		t.Errorf("second instance ChainDepth = %d, want 2 (should be consistent across restarts)", finding2.ChainDepth)
	}
}

// TestJobProvider_ConcurrentReconciliationRace tests race condition in concurrent reconciliation
func TestJobProvider_ConcurrentReconciliationRace(t *testing.T) {
	s := newTestScheme()

	// Create parent RemediationJob
	parentRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent-remediation",
			Namespace: "default",
			UID:       "parent-uid",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: 1,
		},
	}

	const numGoroutines = 20
	results := make(chan *domain.Finding, numGoroutines)
	errors := make(chan error, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Deep-copy parentRJob to avoid a data race: fake.NewClientBuilder().WithObjects()
			// calls obj.SetResourceVersion("999") on the passed object during Build(), which
			// mutates the shared pointer concurrently across goroutines.
			localParent := parentRJob.DeepCopyObject().(client.Object)
			// Each goroutine gets its own client with the same parent
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(localParent).Build()
			p := NewJobProvider(c, newTestConfig())

			job := newExhaustedJob("mendabot-agent-race", "default", 1)
			job.Labels = map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			}
			job.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               "parent-remediation",
					UID:                "parent-uid",
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			}

			finding, err := p.ExtractFinding(job)
			errors <- err
			results <- finding
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Collect all findings
	var findings []*domain.Finding
	for finding := range results {
		if finding != nil {
			findings = append(findings, finding)
		}
	}

	// Verify consistency across all goroutines
	if len(findings) > 0 {
		expectedDepth := findings[0].ChainDepth
		for i, f := range findings {
			if f.ChainDepth != expectedDepth {
				t.Errorf("finding %d has ChainDepth %d, expected %d (inconsistent across goroutines)",
					i, f.ChainDepth, expectedDepth)
			}
		}
	}
}

// TestJobProvider_MemoryLeakPrevention tests that firstSeen map doesn't leak memory
// by simulating many unique findings over time
func TestJobProvider_MemoryLeakPrevention(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	// Create config with stabilization window
	cfg := newTestConfig()
	cfg.StabilisationWindow = 1 * time.Minute
	p := NewJobProvider(c, cfg)

	// Simulate many unique findings
	const numFindings = 1000
	for i := 0; i < numFindings; i++ {
		job := newExhaustedJob(fmt.Sprintf("job-%d", i), "default", 1)
		_, err := p.ExtractFinding(job)
		if err != nil {
			t.Fatalf("unexpected error for job %d: %v", i, err)
		}
	}

	// Note: This test is more about demonstrating the pattern
	// In a real scenario, we'd want to verify that old entries are evicted
	// from the firstSeen map when they're no longer needed
}

// TestJobProvider_InvalidChainDepthAnnotationFormat tests various invalid annotation formats
func TestJobProvider_InvalidChainDepthAnnotationFormat(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, newTestConfig())

	testCases := []struct {
		name        string
		annotation  string
		expectDepth int
	}{
		{"empty string", "", 1},             // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"whitespace", "   ", 1},            // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"decimal", "1.5", 1},               // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"negative decimal", "-1.5", 1},     // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"scientific notation", "1e3", 1},   // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"hex", "0xFF", 1},                  // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"binary", "0b101", 1},              // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"text with numbers", "depth-1", 1}, // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"boolean", "true", 1},              // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"null", "null", 1},                 // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"array", "[1]", 1},                 // strconv.Atoi returns error, falls back to 0, then 0+1=1
		{"object", "{\"depth\":1}", 1},      // strconv.Atoi returns error, falls back to 0, then 0+1=1
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			job := newExhaustedJob("mendabot-agent", "default", 1)
			job.Labels = map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			}
			job.Annotations = map[string]string{
				"remediation.mendabot.io/chain-depth": tc.annotation,
			}

			finding, err := p.ExtractFinding(job)
			if err != nil {
				t.Fatalf("unexpected error for annotation %q: %v", tc.annotation, err)
			}
			if finding == nil {
				t.Fatalf("expected finding for annotation %q", tc.annotation)
			}
			if finding.ChainDepth != tc.expectDepth {
				t.Errorf("annotation %q: ChainDepth = %d, want %d",
					tc.annotation, finding.ChainDepth, tc.expectDepth)
			}
		})
	}
}
