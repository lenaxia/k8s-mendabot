package v1alpha1_test

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

// --- Phase constant tests ---

func TestPhaseConstants(t *testing.T) {
	tests := []struct {
		name  string
		phase v1alpha1.RemediationJobPhase
		want  string
	}{
		{"PhasePending", v1alpha1.PhasePending, "Pending"},
		{"PhaseDispatched", v1alpha1.PhaseDispatched, "Dispatched"},
		{"PhaseRunning", v1alpha1.PhaseRunning, "Running"},
		{"PhaseSucceeded", v1alpha1.PhaseSucceeded, "Succeeded"},
		{"PhaseFailed", v1alpha1.PhaseFailed, "Failed"},
		{"PhaseCancelled", v1alpha1.PhaseCancelled, "Cancelled"},
		{"PhasePermanentlyFailed", v1alpha1.PhasePermanentlyFailed, "PermanentlyFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.phase) != tt.want {
				t.Errorf("phase %q: got %q, want %q", tt.name, string(tt.phase), tt.want)
			}
		})
	}
}

func TestSourceTypeConstant(t *testing.T) {
	if v1alpha1.SourceTypeNative != "native" {
		t.Errorf("SourceTypeNative: got %q, want %q", v1alpha1.SourceTypeNative, "native")
	}
}

func TestConditionTypeConstants(t *testing.T) {
	if v1alpha1.ConditionJobDispatched != "JobDispatched" {
		t.Errorf("ConditionJobDispatched: got %q, want %q", v1alpha1.ConditionJobDispatched, "JobDispatched")
	}
	if v1alpha1.ConditionJobComplete != "JobComplete" {
		t.Errorf("ConditionJobComplete: got %q, want %q", v1alpha1.ConditionJobComplete, "JobComplete")
	}
	if v1alpha1.ConditionJobFailed != "JobFailed" {
		t.Errorf("ConditionJobFailed: got %q, want %q", v1alpha1.ConditionJobFailed, "JobFailed")
	}
	if v1alpha1.ConditionPermanentlyFailed != "PermanentlyFailed" {
		t.Errorf("ConditionPermanentlyFailed: got %q, want %q", v1alpha1.ConditionPermanentlyFailed, "PermanentlyFailed")
	}
}

// TestRemediationJobStatus_ZeroValue_HasEmptyPhase documents the Go language zero
// value of RemediationJobPhase. In production, RemediationJobReconciler immediately
// transitions Phase from "" to PhasePending on the first reconcile, so "" is never
// a stable observed state — it exists only in the brief window between object creation
// and the first reconcile loop.
func TestRemediationJobStatus_ZeroValue_HasEmptyPhase(t *testing.T) {
	var status v1alpha1.RemediationJobStatus
	if status.Phase != "" {
		t.Errorf("zero value Phase should be empty string, got %q", status.Phase)
	}
}

// --- DeepCopy tests ---

func newTestRemediationJob() *v1alpha1.RemediationJob {
	now := metav1.NewTime(time.Now())
	return &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-abc123def456",
			Namespace: "mendabot",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
			SourceType:  v1alpha1.SourceTypeNative,
			SinkType:    "github",
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "result-1",
				Namespace: "default",
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "my-pod",
				Namespace:    "default",
				ParentObject: "my-deployment",
				Errors:       `[{"text":"back-off restarting failed container"}]`,
				Details:      "The pod is crashing.",
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "kubernetes",
			AgentImage:         "ghcr.io/org/agent:latest",
			AgentSA:            "mendabot-agent",
		},
		Status: v1alpha1.RemediationJobStatus{
			Phase:        v1alpha1.PhasePending,
			JobRef:       "",
			DispatchedAt: &now,
			Conditions: []metav1.Condition{
				{Type: v1alpha1.ConditionJobDispatched, Status: metav1.ConditionTrue},
			},
		},
	}
}

func TestRemediationJob_DeepCopyObject_IndependentCopy(t *testing.T) {
	original := newTestRemediationJob()

	copied := original.DeepCopyObject()
	rjob, ok := copied.(*v1alpha1.RemediationJob)
	if !ok {
		t.Fatalf("DeepCopyObject did not return *RemediationJob")
	}

	rjob.Spec.Fingerprint = "changed"
	if original.Spec.Fingerprint == "changed" {
		t.Error("mutating copy Fingerprint affected original")
	}

	rjob.Spec.Finding.Kind = "Deployment"
	if original.Spec.Finding.Kind != "Pod" {
		t.Error("mutating copy Finding.Kind affected original")
	}

	rjob.Status.Phase = v1alpha1.PhaseSucceeded
	if original.Status.Phase != v1alpha1.PhasePending {
		t.Error("mutating copy Status.Phase affected original")
	}
}

func TestRemediationJob_DeepCopyInto_ConditionsIndependent(t *testing.T) {
	original := newTestRemediationJob()

	var dst v1alpha1.RemediationJob
	original.DeepCopyInto(&dst)

	dst.Status.Conditions[0].Type = "Changed"
	if original.Status.Conditions[0].Type != v1alpha1.ConditionJobDispatched {
		t.Error("mutating dst Conditions affected original")
	}
}

func TestRemediationJob_DeepCopyInto_NilConditions(t *testing.T) {
	original := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			Conditions: nil,
		},
	}
	var dst v1alpha1.RemediationJob
	original.DeepCopyInto(&dst)
	if dst.Status.Conditions != nil {
		t.Errorf("expected nil Conditions in copy, got %v", dst.Status.Conditions)
	}
}

func TestRemediationJob_DeepCopyInto_DispatchedAtIndependent(t *testing.T) {
	now := metav1.NewTime(time.Now())
	original := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			DispatchedAt: &now,
		},
	}
	var dst v1alpha1.RemediationJob
	original.DeepCopyInto(&dst)

	later := metav1.NewTime(time.Now().Add(time.Hour))
	*dst.Status.DispatchedAt = later
	if !original.Status.DispatchedAt.Equal(&now) {
		t.Error("mutating dst DispatchedAt value affected original: deep copy did not allocate independent *metav1.Time")
	}
}

func TestRemediationJob_DeepCopyInto_CompletedAtIndependent(t *testing.T) {
	now := metav1.NewTime(time.Now())
	original := &v1alpha1.RemediationJob{
		Status: v1alpha1.RemediationJobStatus{
			CompletedAt: &now,
		},
	}
	var dst v1alpha1.RemediationJob
	original.DeepCopyInto(&dst)

	later := metav1.NewTime(time.Now().Add(time.Hour))
	*dst.Status.CompletedAt = later
	if !original.Status.CompletedAt.Equal(&now) {
		t.Error("mutating dst CompletedAt value affected original: deep copy did not allocate independent *metav1.Time")
	}
}

func TestRemediationJob_DeepCopyObject_Nil(t *testing.T) {
	var rjob *v1alpha1.RemediationJob
	result := rjob.DeepCopyObject()
	if result != nil {
		t.Errorf("DeepCopyObject on nil *RemediationJob: expected nil, got %v", result)
	}
}

func TestRemediationJobList_DeepCopyObject_Nil(t *testing.T) {
	var list *v1alpha1.RemediationJobList
	result := list.DeepCopyObject()
	if result != nil {
		t.Errorf("DeepCopyObject on nil *RemediationJobList: expected nil, got %v", result)
	}
}

func TestRemediationJobList_DeepCopyObject_IndependentCopy(t *testing.T) {
	original := &v1alpha1.RemediationJobList{
		Items: []v1alpha1.RemediationJob{
			*newTestRemediationJob(),
		},
	}

	copied := original.DeepCopyObject()
	list, ok := copied.(*v1alpha1.RemediationJobList)
	if !ok {
		t.Fatalf("DeepCopyObject did not return *RemediationJobList")
	}

	list.Items[0].Spec.Fingerprint = "changed"
	if original.Items[0].Spec.Fingerprint == "changed" {
		t.Error("mutating copy Items[0].Spec affected original")
	}
}

func TestRemediationJobList_DeepCopyObject_EmptyItems(t *testing.T) {
	original := &v1alpha1.RemediationJobList{Items: nil}
	copied := original.DeepCopyObject().(*v1alpha1.RemediationJobList)
	if copied.Items != nil {
		t.Errorf("expected nil Items in copy, got %v", copied.Items)
	}
}

// --- Scheme registration ---

func TestAddToScheme_RegistersRemediationJobTypes(t *testing.T) {
	scheme := v1alpha1.NewScheme()

	gvks, _, err := scheme.ObjectKinds(&v1alpha1.RemediationJob{})
	if err != nil {
		t.Fatalf("RemediationJob not registered in scheme: %v", err)
	}
	found := false
	for _, gvk := range gvks {
		if gvk.Group == "remediation.mendabot.io" && gvk.Version == "v1alpha1" && gvk.Kind == "RemediationJob" {
			found = true
		}
	}
	if !found {
		t.Errorf("RemediationJob not registered under remediation.mendabot.io/v1alpha1, got %v", gvks)
	}

	gvks2, _, err := scheme.ObjectKinds(&v1alpha1.RemediationJobList{})
	if err != nil {
		t.Fatalf("RemediationJobList not registered: %v", err)
	}
	found2 := false
	for _, gvk := range gvks2 {
		if gvk.Group == "remediation.mendabot.io" && gvk.Kind == "RemediationJobList" {
			found2 = true
		}
	}
	if !found2 {
		t.Errorf("RemediationJobList not registered under remediation.mendabot.io/v1alpha1, got %v", gvks2)
	}
}

func TestPhasePermanentlyFailed_ConstantValue(t *testing.T) {
	if string(v1alpha1.PhasePermanentlyFailed) != "PermanentlyFailed" {
		t.Errorf("PhasePermanentlyFailed = %q, want %q",
			v1alpha1.PhasePermanentlyFailed, "PermanentlyFailed")
	}
}

func TestConditionPermanentlyFailed_ConstantValue(t *testing.T) {
	if v1alpha1.ConditionPermanentlyFailed != "PermanentlyFailed" {
		t.Errorf("ConditionPermanentlyFailed = %q, want %q",
			v1alpha1.ConditionPermanentlyFailed, "PermanentlyFailed")
	}
}

func TestDeepCopyInto_CopiesRetryCount(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int32
	}{
		{"zero", 0},
		{"one", 1},
		{"at-max", 3},
		{"over-max", 99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &v1alpha1.RemediationJob{}
			src.Status.RetryCount = tt.retryCount
			dst := &v1alpha1.RemediationJob{}
			src.DeepCopyInto(dst)
			if dst.Status.RetryCount != tt.retryCount {
				t.Errorf("DeepCopyInto: RetryCount = %d, want %d",
					dst.Status.RetryCount, tt.retryCount)
			}
		})
	}
}

func TestDeepCopyInto_CopiesMaxRetries(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int32
	}{
		{"default", 3},
		{"one", 1},
		{"ten", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &v1alpha1.RemediationJob{}
			src.Spec.MaxRetries = tt.maxRetries
			dst := &v1alpha1.RemediationJob{}
			src.DeepCopyInto(dst)
			if dst.Spec.MaxRetries != tt.maxRetries {
				t.Errorf("DeepCopyInto: MaxRetries = %d, want %d",
					dst.Spec.MaxRetries, tt.maxRetries)
			}
		})
	}
}

func TestRemediationJobSpec_MaxRetriesField(t *testing.T) {
	spec := v1alpha1.RemediationJobSpec{}
	if spec.MaxRetries != 0 {
		t.Errorf("zero-value MaxRetries = %d, want 0 (kubebuilder default applies at API server admission, not in Go)", spec.MaxRetries)
	}
}

func TestRemediationJobStatus_RetryCountField(t *testing.T) {
	status := v1alpha1.RemediationJobStatus{}
	if status.RetryCount != 0 {
		t.Errorf("zero-value RetryCount = %d, want 0", status.RetryCount)
	}
}

// TestNewScheme_RegistersRemediationGroupVersion verifies that NewScheme registers
// RemediationJob under remediation.mendabot.io/v1alpha1.
func TestNewScheme_RegistersRemediationGroupVersion(t *testing.T) {
	scheme := v1alpha1.NewScheme()

	// Verify RemediationJob is registered under remediation.mendabot.io/v1alpha1.
	rjobGVKs, _, err := scheme.ObjectKinds(&v1alpha1.RemediationJob{})
	if err != nil {
		t.Fatalf("RemediationJob not registered in full scheme: %v", err)
	}
	rjobFound := false
	for _, gvk := range rjobGVKs {
		if gvk.Group == "remediation.mendabot.io" && gvk.Version == "v1alpha1" {
			rjobFound = true
		}
	}
	if !rjobFound {
		t.Errorf("RemediationJob not registered under remediation.mendabot.io/v1alpha1, got %v", rjobGVKs)
	}
}
