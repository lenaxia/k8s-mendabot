package native

import (
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// TestStatefulSetProviderName_IsNative verifies ProviderName() returns "native".
func TestStatefulSetProviderName_IsNative(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestStatefulSetObjectType_IsStatefulSet verifies ObjectType() returns a *appsv1.StatefulSet.
func TestStatefulSetObjectType_IsStatefulSet(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	obj := p.ObjectType()
	if _, ok := obj.(*appsv1.StatefulSet); !ok {
		t.Errorf("ObjectType() returned %T, want *appsv1.StatefulSet", obj)
	}
}

// TestHealthyStatefulSet_ReturnsNil: spec.replicas=3, ready=3, no Available=False → (nil, nil).
func TestHealthyStatefulSet_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           3,
			ReadyReplicas:      3,
			Conditions: []appsv1.StatefulSetCondition{
				{
					Type:   "Available",
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for healthy statefulset, got %+v", finding)
	}
}

// TestReplicasMismatch_NotScaling: spec=3, ready=1, generation==observedGeneration → finding;
// 3/2=1 (integer division), 1 < 1 is false → medium severity.
func TestReplicasMismatch_NotScaling(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 2,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      1,
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for replica mismatch (not scaling), got nil")
	}
	if finding.Kind != "StatefulSet" {
		t.Errorf("finding.Kind = %q, want %q", finding.Kind, "StatefulSet")
	}
	if finding.Name != "my-sts" {
		t.Errorf("finding.Name = %q, want %q", finding.Name, "my-sts")
	}
	if finding.Namespace != "default" {
		t.Errorf("finding.Namespace = %q, want %q", finding.Namespace, "default")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "1")
	assertErrorTextContains(t, finding.Errors, "3")
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q (1 of 3: 3/2=1, not less than half)", finding.Severity, domain.SeverityMedium)
	}
}

// TestReplicasMismatch_Scaling_ReturnsNil: spec=3, ready=1, generation!=observedGeneration → (nil, nil).
func TestReplicasMismatch_Scaling_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 3,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      1,
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding while scaling (generation mismatch), got %+v", finding)
	}
}

// TestAvailableFalse_Detected: Available=False → finding even if replicas match; severity = medium.
func TestAvailableFalse_Detected(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      3,
			Conditions: []appsv1.StatefulSetCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "StatefulSet does not have minimum availability.",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for Available=False condition, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "MinimumReplicasUnavailable")
	assertErrorTextContains(t, finding.Errors, "StatefulSet does not have minimum availability.")
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityMedium)
	}
}

// TestNoAvailableCondition_ReturnsNil: no Available condition at all, replicas match → (nil, nil).
func TestNoAvailableCondition_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      3,
			Conditions:         nil,
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding (no conditions, healthy replicas), got %+v", finding)
	}
}

// TestNilReplicas_OneReplica_Healthy: spec.replicas=nil (implies 1), ready=1 → (nil, nil).
func TestNilReplicas_OneReplica_Healthy(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: nil,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      1,
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for nil replicas (defaults to 1) with 1 ready, got %+v", finding)
	}
}

// TestStatefulSetAvailableFalseMessageRedacted: Available=False condition message containing
// password=secret123 → error text must NOT contain "secret123" and must contain "[REDACTED]".
func TestStatefulSetAvailableFalseMessageRedacted(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "redact-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      3,
			Conditions: []appsv1.StatefulSetCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "connection failed: password=secret123 wrong",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(sts)
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

// TestStatefulSetWrongType_ReturnsError: passing a non-StatefulSet object → (nil, error).
func TestStatefulSetWrongType_ReturnsError(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

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

// TestStatefulSetFindingErrors_IsValidJSON: errors field is valid JSON array with ≥1 entry.
func TestStatefulSetFindingErrors_IsValidJSON(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      0,
		},
	}

	finding, err := p.ExtractFinding(sts)
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

// TestStatefulSetParentObject_IsSelf: StatefulSet with no ownerReferences →
// ParentObject == "StatefulSet/<name>".
func TestStatefulSetParentObject_IsSelf(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-sts",
			Namespace:       "default",
			Generation:      1,
			OwnerReferences: nil,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      0,
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	want := "StatefulSet/my-sts"
	if finding.ParentObject != want {
		t.Errorf("finding.ParentObject = %q, want %q", finding.ParentObject, want)
	}
}

// TestStatefulSetBothConditions_TwoEntries: replica mismatch AND Available=False both present
// → Errors JSON contains two entries.
func TestStatefulSetBothConditions_TwoEntries(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      1,
			Conditions: []appsv1.StatefulSetCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "StatefulSet does not have minimum availability.",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(sts)
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

// TestStatefulSetAnnotationEnabled_False: degraded statefulset (ReadyReplicas=0, Replicas=3)
// with mendabot.io/enabled=false → (nil, nil).
// Uses an unhealthy object to prove the gate fires on an object that would otherwise produce
// a non-nil finding.

// TestStatefulSetSeverity_Critical: spec=3, readyReplicas=0, generation==observedGeneration → critical.
func TestStatefulSetSeverity_Critical(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "crit-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{Replicas: int32Ptr(3)},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      0,
		},
	}

	finding, err := p.ExtractFinding(sts)
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

// TestStatefulSetSeverity_High: spec=4, readyReplicas=1 (< 4/2=2, less than half) → high.
func TestStatefulSetSeverity_High(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "high-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{Replicas: int32Ptr(4)},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      1,
		},
	}

	finding, err := p.ExtractFinding(sts)
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

// TestStatefulSetSeverity_Medium: spec=4, readyReplicas=3 (>= half, but < spec) → medium.
func TestStatefulSetSeverity_Medium(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "med-sts",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{Replicas: int32Ptr(4)},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      3,
		},
	}

	finding, err := p.ExtractFinding(sts)
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
func TestStatefulSetAnnotationEnabled_False(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ann-sts",
			Namespace:  "default",
			Generation: 1,
			Annotations: map[string]string{
				domain.AnnotationEnabled: "false",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           3,
			ReadyReplicas:      0,
		},
	}
	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when annotation enabled=false, got %+v", finding)
	}
}

// TestStatefulSetAnnotationSkipUntilFuture: degraded statefulset with mendabot.io/skip-until=2099-12-31 → (nil, nil).
func TestStatefulSetAnnotationSkipUntilFuture(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "skip-sts",
			Namespace:  "default",
			Generation: 1,
			Annotations: map[string]string{
				domain.AnnotationSkipUntil: "2099-12-31",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      0,
		},
	}
	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when skip-until is in the future, got %+v", finding)
	}
}

// TestAvailableFalse_DuringScaling: Available=False present even while scaling
// (generation != observedGeneration) → finding still returned for Available=False.
func TestAvailableFalse_DuringScaling(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewStatefulSetProvider(c)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-sts",
			Namespace:  "default",
			Generation: 3,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      1,
			Conditions: []appsv1.StatefulSetCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasUnavailable",
					Message: "StatefulSet does not have minimum availability.",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(sts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for Available=False even during scaling, got nil")
	}
	assertErrorsJSON(t, finding.Errors)

	var entries []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(finding.Errors), &entries); err != nil {
		t.Fatalf("Errors is not valid JSON: %v", err)
	}
	// Only 1 entry: replica mismatch suppressed (scaling), Available=False reported
	if len(entries) != 1 {
		t.Errorf("expected 1 error entry (Available=False only, replicas suppressed during scaling), got %d: %s", len(entries), finding.Errors)
	}
}
