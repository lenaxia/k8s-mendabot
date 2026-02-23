package native

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// healthyNode builds a Node in a fully healthy state:
// NodeReady=True, MemoryPressure=False, DiskPressure=False, PIDPressure=False,
// NetworkUnavailable=False.
func healthyNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionTrue,
					Reason:  "KubeletReady",
					Message: "kubelet is posting ready status",
				},
				{
					Type:    corev1.NodeMemoryPressure,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletHasSufficientMemory",
					Message: "kubelet has sufficient memory available",
				},
				{
					Type:    corev1.NodeDiskPressure,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletHasNoDiskPressure",
					Message: "kubelet has no disk pressure",
				},
				{
					Type:    corev1.NodePIDPressure,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletHasSufficientPID",
					Message: "kubelet has sufficient PID available",
				},
			},
		},
	}
}

// TestNodeProvider_ProviderName verifies ProviderName() returns "native".
func TestNodeProvider_ProviderName(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestNodeProvider_ObjectType verifies ObjectType() returns a *corev1.Node.
func TestNodeProvider_ObjectType(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	obj := p.ObjectType()
	if _, ok := obj.(*corev1.Node); !ok {
		t.Errorf("ObjectType() returned %T, want *corev1.Node", obj)
	}
}

// TestNodeProvider_HealthyNode: all standard conditions healthy → (nil, nil).
func TestNodeProvider_HealthyNode(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-1")
	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for healthy node, got %+v", finding)
	}
}

// TestNodeProvider_NotReadyFalse: NodeReady=False → finding with condition error text.
func TestNodeProvider_NotReadyFalse(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-1")
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "KubeletNotReady",
		Message: "container runtime not ready",
	}

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for NotReady node, got nil")
	}
	if finding.Kind != "Node" {
		t.Errorf("finding.Kind = %q, want %q", finding.Kind, "Node")
	}
	if finding.Name != "node-1" {
		t.Errorf("finding.Name = %q, want %q", finding.Name, "node-1")
	}
	if finding.Namespace != "" {
		t.Errorf("finding.Namespace = %q, want empty (cluster-scoped)", finding.Namespace)
	}
	assertNodeErrorsJSON(t, finding.Errors)
	assertNodeErrorTextContains(t, finding.Errors, "Ready")
}

// TestNodeProvider_NotReadyUnknown: NodeReady=Unknown → finding returned.
func TestNodeProvider_NotReadyUnknown(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-2")
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionUnknown,
		Reason:  "NodeStatusUnknown",
		Message: "Kubelet stopped posting node status",
	}

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for Unknown-ready node, got nil")
	}
	assertNodeErrorsJSON(t, finding.Errors)
	assertNodeErrorTextContains(t, finding.Errors, "Ready")
	assertNodeErrorTextContains(t, finding.Errors, "Unknown")
}

// TestNodeProvider_MemoryPressure: NodeMemoryPressure=True → finding.
func TestNodeProvider_MemoryPressure(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-3")
	// Remove the healthy MemoryPressure=False condition added by healthyNode, then add True.
	node.Status.Conditions = filterNodeConditions(node.Status.Conditions, corev1.NodeMemoryPressure)
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:    corev1.NodeMemoryPressure,
		Status:  corev1.ConditionTrue,
		Reason:  "KubeletHasInsufficientMemory",
		Message: "kubelet has insufficient memory available",
	})

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for MemoryPressure node, got nil")
	}
	assertNodeErrorsJSON(t, finding.Errors)
	assertNodeErrorTextContains(t, finding.Errors, "MemoryPressure")
}

// TestNodeProvider_DiskPressure: NodeDiskPressure=True → finding.
func TestNodeProvider_DiskPressure(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-4")
	// Remove the healthy DiskPressure=False condition added by healthyNode, then add True.
	node.Status.Conditions = filterNodeConditions(node.Status.Conditions, corev1.NodeDiskPressure)
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:    corev1.NodeDiskPressure,
		Status:  corev1.ConditionTrue,
		Reason:  "KubeletHasDiskPressure",
		Message: "kubelet has disk pressure",
	})

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for DiskPressure node, got nil")
	}
	assertNodeErrorsJSON(t, finding.Errors)
	assertNodeErrorTextContains(t, finding.Errors, "DiskPressure")
}

// TestNodeProvider_PIDPressure: NodePIDPressure=True → finding.
func TestNodeProvider_PIDPressure(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-5")
	// Remove the healthy PIDPressure=False condition added by healthyNode, then add True.
	node.Status.Conditions = filterNodeConditions(node.Status.Conditions, corev1.NodePIDPressure)
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:    corev1.NodePIDPressure,
		Status:  corev1.ConditionTrue,
		Reason:  "KubeletHasInsufficientPID",
		Message: "kubelet has insufficient PID available",
	})

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for PIDPressure node, got nil")
	}
	assertNodeErrorsJSON(t, finding.Errors)
	assertNodeErrorTextContains(t, finding.Errors, "PIDPressure")
}

// TestNodeProvider_NetworkUnavailable: NodeNetworkUnavailable=True → finding.
func TestNodeProvider_NetworkUnavailable(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-6")
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:    corev1.NodeNetworkUnavailable,
		Status:  corev1.ConditionTrue,
		Reason:  "NoRouteCreated",
		Message: "No route was created for the node",
	})

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for NetworkUnavailable node, got nil")
	}
	assertNodeErrorsJSON(t, finding.Errors)
	assertNodeErrorTextContains(t, finding.Errors, "NetworkUnavailable")
}

// TestNodeProvider_EtcdIsVoterIgnored: EtcdIsVoter=True (k3s condition) → (nil, nil).
func TestNodeProvider_EtcdIsVoterIgnored(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-7")
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:    "EtcdIsVoter",
		Status:  corev1.ConditionTrue,
		Reason:  "MemberNotLearner",
		Message: "member is not a learner",
	})

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when EtcdIsVoter=True, got %+v", finding)
	}
}

// TestNodeProvider_MultipleConditions: NodeReady=False AND MemoryPressure=True
// → single finding with two error entries.
func TestNodeProvider_MultipleConditions(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-8")
	// Override Ready to False.
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "KubeletNotReady",
		Message: "container runtime not ready",
	}
	// Add MemoryPressure=True (remove the False one first).
	node.Status.Conditions = filterNodeConditions(node.Status.Conditions, corev1.NodeMemoryPressure)
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:    corev1.NodeMemoryPressure,
		Status:  corev1.ConditionTrue,
		Reason:  "KubeletHasInsufficientMemory",
		Message: "kubelet has insufficient memory available",
	})

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for multiple conditions, got nil")
	}

	var entries []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(finding.Errors), &entries); err != nil {
		t.Fatalf("Errors is not valid JSON: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 error entries, got %d: %s", len(entries), finding.Errors)
	}
}

// TestNodeProvider_WrongType: passing a non-Node object → (nil, error).
func TestNodeProvider_WrongType(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

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

// TestNodeProvider_ParentObject: node with no ownerReferences → ParentObject == "Node/<name>".
func TestNodeProvider_ParentObject(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("worker-01")
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "KubeletNotReady",
		Message: "kubelet not ready",
	}

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	want := "Node/worker-01"
	if finding.ParentObject != want {
		t.Errorf("ParentObject = %q, want %q", finding.ParentObject, want)
	}
}

// TestNodeProvider_SourceRef: finding SourceRef identifies the node correctly.
func TestNodeProvider_SourceRef(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("control-plane-01")
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "KubeletNotReady",
		Message: "kubelet not ready",
	}

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	if finding.SourceRef.APIVersion != "v1" {
		t.Errorf("SourceRef.APIVersion = %q, want %q", finding.SourceRef.APIVersion, "v1")
	}
	if finding.SourceRef.Kind != "Node" {
		t.Errorf("SourceRef.Kind = %q, want %q", finding.SourceRef.Kind, "Node")
	}
	if finding.SourceRef.Name != "control-plane-01" {
		t.Errorf("SourceRef.Name = %q, want %q", finding.SourceRef.Name, "control-plane-01")
	}
	if finding.SourceRef.Namespace != "" {
		t.Errorf("SourceRef.Namespace = %q, want empty (cluster-scoped)", finding.SourceRef.Namespace)
	}
}

// TestNodeProvider_ErrorTextFormat: error text includes condition type, status, reason, and message.
func TestNodeProvider_ErrorTextFormat(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-format")
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "KubeletNotReady",
		Message: "container runtime is not ready: runc is not installed",
	}

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}

	assertNodeErrorTextContains(t, finding.Errors, "node-format")
	assertNodeErrorTextContains(t, finding.Errors, "Ready")
	assertNodeErrorTextContains(t, finding.Errors, "KubeletNotReady")
	assertNodeErrorTextContains(t, finding.Errors, "container runtime is not ready")
}

// TestNodeProvider_FindingErrors_IsValidJSON: Errors field must be valid JSON array.
func TestNodeProvider_FindingErrors_IsValidJSON(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-json")
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "KubeletNotReady",
		Message: "not ready",
	}

	finding, err := p.ExtractFinding(node)
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
		t.Error("Errors JSON array is empty, expected at least one entry")
	}
}

// TestNodeProvider_NonStandardConditionTrue_Detected: a vendor/custom condition (GPUFailure=True)
// not in the standard switch must produce a finding containing "GPUFailure".
func TestNodeProvider_NonStandardConditionTrue_Detected(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("node-gpu")
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:    "GPUFailure",
		Status:  corev1.ConditionTrue,
		Reason:  "GPUMemoryFailure",
		Message: "GPU memory failure",
	})

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding for non-standard GPUFailure=True condition, got nil")
	}
	assertNodeErrorsJSON(t, finding.Errors)
	assertNodeErrorTextContains(t, finding.Errors, "GPUFailure")
}

// TestNodeProvider_ConditionMessageRedacted: node condition message containing password=secret123
// → error text must NOT contain "secret123" and must contain "[REDACTED]".
func TestNodeProvider_ConditionMessageRedacted(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewNodeProvider(c)

	node := healthyNode("redact-node")
	node.Status.Conditions[0] = corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionFalse,
		Reason:  "KubeletNotReady",
		Message: "kubelet failed: password=secret123 invalid",
	}

	finding, err := p.ExtractFinding(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertNodeErrorsJSON(t, finding.Errors)
	if contains(finding.Errors, "secret123") {
		t.Errorf("error text should not contain raw secret value 'secret123': %s", finding.Errors)
	}
	assertNodeErrorTextContains(t, finding.Errors, "[REDACTED]")
}

// assertNodeErrorsJSON verifies that the errors string is valid JSON with at least one entry.
func assertNodeErrorsJSON(t *testing.T, errors string) {
	t.Helper()
	var entries []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(errors), &entries); err != nil {
		t.Errorf("Errors is not valid JSON: %v — value: %s", err, errors)
	}
	if len(entries) == 0 {
		t.Error("Errors JSON array is empty")
	}
}

// assertNodeErrorTextContains checks that at least one entry in the errors JSON contains substr.
func assertNodeErrorTextContains(t *testing.T, errors, substr string) {
	t.Helper()
	var entries []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(errors), &entries); err != nil {
		t.Errorf("Errors is not valid JSON: %v", err)
		return
	}
	for _, e := range entries {
		if contains(e.Text, substr) {
			return
		}
	}
	t.Errorf("no error entry contains %q in: %s", substr, errors)
}

// filterNodeConditions returns conditions excluding all entries of the given type.
// Used in tests to replace a condition with a different status.
func filterNodeConditions(conditions []corev1.NodeCondition, excludeType corev1.NodeConditionType) []corev1.NodeCondition {
	result := make([]corev1.NodeCondition, 0, len(conditions))
	for _, c := range conditions {
		if c.Type != excludeType {
			result = append(result, c)
		}
	}
	return result
}
