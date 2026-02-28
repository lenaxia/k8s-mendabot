package native

import (
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

// int32Ptr is a test helper that returns a pointer to an int32.
func int32Ptr(v int32) *int32 { return &v }

// TestDeploymentProviderName_IsNative verifies ProviderName() returns "native".
func TestDeploymentProviderName_IsNative(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestDeploymentObjectType_IsDeployment verifies ObjectType() returns a *appsv1.Deployment.
func TestDeploymentObjectType_IsDeployment(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	obj := p.ObjectType()
	if _, ok := obj.(*appsv1.Deployment); !ok {
		t.Errorf("ObjectType() returned %T, want *appsv1.Deployment", obj)
	}
}

// TestHealthyDeployment: spec.replicas=3, status.readyReplicas=3, no Available=False → (nil, nil).
func TestHealthyDeployment(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 3,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for healthy deployment, got %+v", finding)
	}
}

// TestDegradedDeployment: spec.replicas=3, status.replicas=3, status.readyReplicas=1
// → Finding with mismatch error text; readyReplicas=1, spec/2=1, 1 < 1 is false → medium severity.
func TestDegradedDeployment(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 1,
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for degraded deployment, got nil")
	}
	if finding.Kind != "Deployment" {
		t.Errorf("finding.Kind = %q, want %q", finding.Kind, "Deployment")
	}
	if finding.Name != "my-deploy" {
		t.Errorf("finding.Name = %q, want %q", finding.Name, "my-deploy")
	}
	if finding.Namespace != "default" {
		t.Errorf("finding.Namespace = %q, want %q", finding.Namespace, "default")
	}
	assertErrorsJSON(t, finding.Errors)
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q (1 of 3: 3/2=1, not less than half)", finding.Severity, domain.SeverityMedium)
	}
}

// TestZeroReadyReplicas: spec.replicas=2, status.replicas=2, status.readyReplicas=0 → Finding; severity = critical.
func TestZeroReadyReplicas(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zero-ready",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      2,
			ReadyReplicas: 0,
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for zero-ready deployment, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	if finding.Severity != domain.SeverityCritical {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityCritical)
	}
}

// TestScalingDownTransient: spec.replicas=2, status.replicas=3, status.readyReplicas=2
// → (nil, nil) — scaling transient, not a failure.
func TestScalingDownTransient(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scaling-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 2,
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for scaling-down transient, got %+v", finding)
	}
}

// TestAvailableConditionFalse: spec.replicas=3, status.readyReplicas=3,
// condition Available=False with non-empty Reason and Message → Finding returned; severity = medium.
func TestAvailableConditionFalse(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "avail-false-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 3,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "Deployment does not have minimum availability.",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for Available=False condition, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityMedium)
	}
}

// TestErrorTextIncludesReason: when Available=False, error text must include the Reason
// and Message from the condition.
func TestErrorTextIncludesReason(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "avail-false-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 3,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "Deployment does not have minimum availability.",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	assertErrorTextContains(t, finding.Errors, "MinimumReplicasUnavailable")
	assertErrorTextContains(t, finding.Errors, "Deployment does not have minimum availability.")
}

// TestDeploymentAvailableFalseMessageRedacted: Available=False condition message containing
// password=secret123 → error text must NOT contain "secret123" and must contain "[REDACTED]".
func TestDeploymentAvailableFalseMessageRedacted(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redact-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 3,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "failed auth: password=secret123 rejected",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(deploy)
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

// TestDeploymentWrongType: passing a Pod → (nil, error).
func TestDeploymentWrongType(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

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

// TestErrorTextContent: degraded deployment → error text contains both spec.replicas
// and status.readyReplicas values.
func TestErrorTextContent(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 0,
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	// Error text must contain both spec.replicas (3) and readyReplicas (0).
	assertErrorTextContains(t, finding.Errors, "3")
	assertErrorTextContains(t, finding.Errors, "0")
}

// TestDeploymentFindingErrors_IsValidJSON: Errors field must be valid JSON with at least one entry.
func TestDeploymentFindingErrors_IsValidJSON(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 1,
		},
	}

	finding, err := p.ExtractFinding(deploy)
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

// TestDeploymentParentObject_IsSelf: Deployment with no ownerReferences →
// ParentObject == "Deployment/<name>".
func TestDeploymentParentObject_IsSelf(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-deploy",
			Namespace:       "default",
			OwnerReferences: nil,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 1,
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	want := "Deployment/my-deploy"
	if finding.ParentObject != want {
		t.Errorf("finding.ParentObject = %q, want %q", finding.ParentObject, want)
	}
}

// TestDeploymentProvider_AvailableFalse_WhileScalingDown: spec.replicas=2, status.replicas=3,
// status.readyReplicas=2 (scale-down transient), AND Available=False → finding returned.
// Available=False must be reported even when the replica mismatch check is skipped.
func TestDeploymentProvider_AvailableFalse_WhileScalingDown(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scaling-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3, // status.replicas > spec.replicas → scale-down transient
			ReadyReplicas: 2,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "Deployment does not have minimum availability.",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding: Available=False must be reported even during scale-down transient, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "Available")
}

// → Errors JSON contains two entries.
func TestBothConditions_TwoEntries(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "Deployment does not have minimum availability.",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(deploy)
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
	if len(entries) != 2 {
		t.Errorf("expected 2 error entries (replica mismatch + Available=False), got %d: %s", len(entries), finding.Errors)
	}
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("expected severity medium, got %q", finding.Severity)
	}
}

// TestDeploymentAnnotationEnabled_False: degraded deployment (ReadyReplicas=0, Replicas=3)
// with mechanic.io/enabled=false → (nil, nil).
// Uses an unhealthy object to prove the gate fires on an object that would otherwise produce
// a non-nil finding.

// TestDeploymentSeverity_Critical: spec=4, readyReplicas=0 → critical.
func TestDeploymentSeverity_Critical(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "crit-deploy", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(4)},
		Status:     appsv1.DeploymentStatus{Replicas: 4, ReadyReplicas: 0},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	if finding.Severity != domain.SeverityCritical {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityCritical)
	}
}

// TestDeploymentSeverity_High: spec=4, readyReplicas=1 (< 4/2=2, so less than half) → high.
func TestDeploymentSeverity_High(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "high-deploy", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(4)},
		Status:     appsv1.DeploymentStatus{Replicas: 4, ReadyReplicas: 1},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	if finding.Severity != domain.SeverityHigh {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityHigh)
	}
}

// TestDeploymentSeverity_Medium: spec=4, readyReplicas=3 (3 >= 4/2=2, but 3 < 4) → medium.
func TestDeploymentSeverity_Medium(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "med-deploy", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(4)},
		Status:     appsv1.DeploymentStatus{Replicas: 4, ReadyReplicas: 3},
	}

	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityMedium)
	}
}

func TestDeploymentAnnotationEnabled_False(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ann-deploy",
			Namespace: "default",
			Annotations: map[string]string{
				domain.AnnotationEnabled: "false",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 0,
		},
	}
	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when annotation enabled=false, got %+v", finding)
	}
}

// TestDeploymentAnnotationSkipUntilFuture: degraded deployment with mechanic.io/skip-until=2099-12-31 → (nil, nil).
func TestDeploymentAnnotationSkipUntilFuture(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewDeploymentProvider(c)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "skip-deploy",
			Namespace: "default",
			Annotations: map[string]string{
				domain.AnnotationSkipUntil: "2099-12-31",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 0,
		},
	}
	finding, err := p.ExtractFinding(deploy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when skip-until is in the future, got %+v", finding)
	}
}
