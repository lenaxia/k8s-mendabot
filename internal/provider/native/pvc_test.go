package native

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

// makePVC builds a PVC with the given name, namespace and phase.
func makePVC(name, namespace string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
}

// makeEvent builds a corev1.Event for the given PVC and reason/message.
func makeEvent(pvcName, pvcNamespace, reason, message string) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName + "-" + reason,
			Namespace: pvcNamespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "PersistentVolumeClaim",
			Name:      pvcName,
			Namespace: pvcNamespace,
		},
		Reason:  reason,
		Message: message,
	}
}

// TestPVCProviderName_IsNative verifies ProviderName() returns "native".
func TestPVCProviderName_IsNative(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPVCProvider(c, testRedactor(t))

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestPVCObjectType_IsPVC verifies ObjectType() returns a *corev1.PersistentVolumeClaim.
func TestPVCObjectType_IsPVC(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPVCProvider(c, testRedactor(t))

	obj := p.ObjectType()
	if _, ok := obj.(*corev1.PersistentVolumeClaim); !ok {
		t.Errorf("ObjectType() returned %T, want *corev1.PersistentVolumeClaim", obj)
	}
}

// TestBoundPVC_ReturnsNil: Phase=Bound → (nil, nil).
func TestBoundPVC_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPVCProvider(c, testRedactor(t))

	pvc := makePVC("my-pvc", "default", corev1.ClaimBound)
	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for Bound PVC, got %+v", finding)
	}
}

// TestPendingPVC_NoEvents_ReturnsNil: Phase=Pending, no events → (nil, nil).
func TestPendingPVC_NoEvents_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPVCProvider(c, testRedactor(t))

	pvc := makePVC("my-pvc", "default", corev1.ClaimPending)
	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for Pending PVC with no events, got %+v", finding)
	}
}

// TestPendingPVC_WithProvisioningFailed_ReturnsFinding:
// Phase=Pending, ProvisioningFailed event → finding with event message as error text; severity = high.
func TestPendingPVC_WithProvisioningFailed_ReturnsFinding(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("my-pvc", "default", corev1.ClaimPending)
	event := makeEvent("my-pvc", "default", "ProvisioningFailed", "no storage class found")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for Pending PVC with ProvisioningFailed event, got nil")
	}
	if finding.Kind != "PersistentVolumeClaim" {
		t.Errorf("finding.Kind = %q, want %q", finding.Kind, "PersistentVolumeClaim")
	}
	if finding.Name != "my-pvc" {
		t.Errorf("finding.Name = %q, want %q", finding.Name, "my-pvc")
	}
	if finding.Namespace != "default" {
		t.Errorf("finding.Namespace = %q, want %q", finding.Namespace, "default")
	}
	wantParent := "PersistentVolumeClaim/my-pvc"
	if finding.ParentObject != wantParent {
		t.Errorf("finding.ParentObject = %q, want %q", finding.ParentObject, wantParent)
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "no storage class found")
	if finding.Severity != domain.SeverityHigh {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityHigh)
	}
}

// TestPendingPVC_WithOtherEvent_ReturnsNil:
// Phase=Pending, event reason is "Provisioning" (not "ProvisioningFailed") → (nil, nil).
func TestPendingPVC_WithOtherEvent_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("my-pvc", "default", corev1.ClaimPending)
	event := makeEvent("my-pvc", "default", "Provisioning", "provisioning started")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for Pending PVC with non-ProvisioningFailed event, got %+v", finding)
	}
}

// TestPVCWrongType_ReturnsError: pass a Pod → (nil, error).
func TestPVCWrongType_ReturnsError(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPVCProvider(c, testRedactor(t))

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

// TestPVCFindingErrors_IsValidJSON: Errors field must be valid JSON with at least one entry.
func TestPVCFindingErrors_IsValidJSON(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("my-pvc", "default", corev1.ClaimPending)
	event := makeEvent("my-pvc", "default", "ProvisioningFailed", "storageclass not found")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
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

// TestPVCErrorText_IncludesEventMessage: error text must contain the ProvisioningFailed event message.
func TestPVCErrorText_IncludesEventMessage(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("my-pvc", "default", corev1.ClaimPending)
	event := makeEvent("my-pvc", "default", "ProvisioningFailed", "rbd: create volume failed: volume already exists")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorTextContains(t, finding.Errors, "rbd: create volume failed: volume already exists")
}

// TestPVCBoundWithStaleEvents_ReturnsNil: Phase=Bound even when stale ProvisioningFailed events exist → (nil, nil).
func TestPVCBoundWithStaleEvents_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("my-pvc", "default", corev1.ClaimBound)
	event := makeEvent("my-pvc", "default", "ProvisioningFailed", "old failure message")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for Bound PVC (phase check before event lookup), got %+v", finding)
	}
}

// TestPVCEventMessageRedacted: ProvisioningFailed event message containing password=secret123
// → error text must NOT contain "secret123" and must contain "[REDACTED]".
func TestPVCEventMessageRedacted(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("my-pvc", "default", corev1.ClaimPending)
	event := makeEvent("my-pvc", "default", "ProvisioningFailed", "provision failed: password=secret123 rejected")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
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

// TestPVCAnnotationEnabled_False: Pending PVC with ProvisioningFailed event and mechanic.io/enabled=false → (nil, nil).
// Uses an unhealthy object to prove the gate fires on an object that would otherwise produce
// a non-nil finding.
func TestPVCAnnotationEnabled_False(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("ann-pvc", "default", corev1.ClaimPending)
	pvc.Annotations = map[string]string{
		domain.AnnotationEnabled: "false",
	}
	event := makeEvent("ann-pvc", "default", "ProvisioningFailed", "no storage class found")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when annotation enabled=false, got %+v", finding)
	}
}

// TestPVCAnnotationSkipUntilFuture: Pending PVC with ProvisioningFailed event and mechanic.io/skip-until=2099-12-31 → (nil, nil).
func TestPVCAnnotationSkipUntilFuture(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("skip-pvc", "default", corev1.ClaimPending)
	pvc.Annotations = map[string]string{
		domain.AnnotationSkipUntil: "2099-12-31",
	}
	event := makeEvent("skip-pvc", "default", "ProvisioningFailed", "no storage class found")

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when skip-until is in the future, got %+v", finding)
	}
}

// TestPVCEventForDifferentKind_ReturnsNil: Pending PVC, but event's involvedObject.kind
// is "Pod" (not PVC) with matching name → must not produce a false finding.
func TestPVCEventForDifferentKind_ReturnsNil(t *testing.T) {
	s := newTestScheme()
	pvc := makePVC("my-pvc", "default", corev1.ClaimPending)
	// Event for a Pod that happens to share the name "my-pvc"
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pvc-pod-event",
			Namespace: "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "my-pvc",
			Namespace: "default",
		},
		Reason:  "ProvisioningFailed",
		Message: "this is a pod event, not a pvc event",
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc, event).Build()
	p := NewPVCProvider(c, testRedactor(t))

	finding, err := p.ExtractFinding(pvc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding: event was for a Pod, not a PVC, got %+v", finding)
	}
}
