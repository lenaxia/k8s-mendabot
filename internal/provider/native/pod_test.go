package native

import (
	"encoding/json"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

// runningPod returns a healthy running pod with all containers ready.
func runningPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(name + "-uid"),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
}

// TestProviderName_IsNative verifies ProviderName() returns "native".
func TestProviderName_IsNative(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestObjectType_IsPod verifies ObjectType() returns a *corev1.Pod.
func TestObjectType_IsPod(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	obj := p.ObjectType()
	if _, ok := obj.(*corev1.Pod); !ok {
		t.Errorf("ObjectType() returned %T, want *corev1.Pod", obj)
	}
}

// TestHealthyRunningPod: running pod with no failure conditions → (nil, nil).
func TestHealthyRunningPod(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := runningPod("my-pod", "default")
	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for healthy pod, got %+v", finding)
	}
}

// TestWrongType: passing a non-Pod object → (nil, error).
func TestWrongType(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
	}
	finding, err := p.ExtractFinding(deploy)
	if err == nil {
		t.Fatal("expected error for wrong type, got nil")
	}
	if finding != nil {
		t.Errorf("expected nil finding on error, got %+v", finding)
	}
}

// TestCrashLoopBackOffOOMKilled: container waiting CrashLoopBackOff with last terminated OOMKilled
// → finding with OOMKilled and container name in error text; restart count ≤ 5 → high severity.
func TestCrashLoopBackOffOOMKilled(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crash-pod",
			Namespace: "default",
			UID:       "crash-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "my-app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	if finding.Kind != "Pod" {
		t.Errorf("finding.Kind = %q, want %q", finding.Kind, "Pod")
	}
	if finding.Name != "crash-pod" {
		t.Errorf("finding.Name = %q, want %q", finding.Name, "crash-pod")
	}
	if finding.Namespace != "default" {
		t.Errorf("finding.Namespace = %q, want %q", finding.Namespace, "default")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "OOMKilled")
	assertErrorTextContains(t, finding.Errors, "my-app")
	if finding.Severity != domain.SeverityHigh {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityHigh)
	}
}

// TestCrashLoopBackOffGeneric: container waiting CrashLoopBackOff, last terminated "Error"
// restart count ≤ 5 → high severity.
func TestCrashLoopBackOffGeneric(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crash-pod",
			Namespace: "default",
			UID:       "crash-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "my-app",
					RestartCount: 4,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 1,
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "my-app")
	if finding.Severity != domain.SeverityHigh {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityHigh)
	}
}

// TestImagePullBackOff: container waiting ImagePullBackOff with non-empty Message
// → finding with the Message in error text; severity = high.
func TestImagePullBackOff(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-pod",
			Namespace: "default",
			UID:       "pull-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ImagePullBackOff",
							Message: "Back-off pulling image \"registry.io/my-app:latest\"",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "ImagePullBackOff")
	assertErrorTextContains(t, finding.Errors, "Back-off pulling image")
	if finding.Severity != domain.SeverityHigh {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityHigh)
	}
}

// TestErrImagePull: container waiting ErrImagePull → finding with error text; severity = high.
func TestErrImagePull(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-pod",
			Namespace: "default",
			UID:       "pull-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "ErrImagePull",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "ErrImagePull")
	if finding.Severity != domain.SeverityHigh {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityHigh)
	}
}

// TestCreateContainerConfigError: container waiting CreateContainerConfigError with message
// → finding with the message in error text; severity = medium (other waiting reason).
func TestCreateContainerConfigError(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-pod",
			Namespace: "default",
			UID:       "config-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CreateContainerConfigError",
							Message: "secret \"my-secret\" not found",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "CreateContainerConfigError")
	assertErrorTextContains(t, finding.Errors, "my-secret")
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityMedium)
	}
}

// TestNonZeroExitCode: terminated container with exit code 137, Waiting == nil
// → finding with exit code in error text; severity = medium (non-zero exit, not OOMKilled as waiting).
func TestNonZeroExitCode(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "term-pod",
			Namespace: "default",
			UID:       "term-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "Error",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "137")
	if finding.Severity != domain.SeverityMedium {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityMedium)
	}
}

// TestUnschedulablePending: pod pending with PodScheduled=Unschedulable
// → finding with scheduler message; severity = high.
func TestUnschedulablePending(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod",
			Namespace: "default",
			UID:       "pending-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/3 nodes are available: insufficient memory",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "Unschedulable")
	assertErrorTextContains(t, finding.Errors, "insufficient memory")
	if finding.Severity != domain.SeverityHigh {
		t.Errorf("finding.Severity = %q, want %q", finding.Severity, domain.SeverityHigh)
	}
}

// TestNoOwnerRef: crashing pod with no ownerReferences → ParentObject == "Pod/<pod-name>".
func TestNoOwnerRef(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "orphan-pod",
			Namespace:       "default",
			UID:             "orphan-pod-uid",
			OwnerReferences: nil,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	want := "Pod/orphan-pod"
	if finding.ParentObject != want {
		t.Errorf("finding.ParentObject = %q, want %q", finding.ParentObject, want)
	}
}

// TestWithDeploymentParent: crashing pod Pod → ReplicaSet → Deployment
// → ParentObject == "Deployment/my-app".
func TestWithDeploymentParent(t *testing.T) {
	s := newTestScheme()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
			UID:       "deploy-uid-1",
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app-7d9f8b",
			Namespace: "default",
			UID:       "rs-uid-1",
			OwnerReferences: []metav1.OwnerReference{
				ownerRef("Deployment", "my-app", "deploy-uid-1"),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(deploy, rs).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app-7d9f8b-abc12",
			Namespace: "default",
			UID:       "pod-uid-1",
			OwnerReferences: []metav1.OwnerReference{
				ownerRef("ReplicaSet", "my-app-7d9f8b", "rs-uid-1"),
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	want := "Deployment/my-app"
	if finding.ParentObject != want {
		t.Errorf("finding.ParentObject = %q, want %q", finding.ParentObject, want)
	}
}

// TestMultipleContainerFailures: two containers both failing → Errors has two entries.
func TestMultipleContainerFailures(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-pod",
			Namespace: "default",
			UID:       "multi-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app-a",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
				{
					Name: "app-b",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "ImagePullBackOff",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
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
		t.Errorf("expected 2 error entries, got %d: %s", len(entries), finding.Errors)
	}
}

// TestFindingErrors_IsValidJSON: errors field must be a valid JSON array.
func TestFindingErrors_IsValidJSON(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crash-pod",
			Namespace: "default",
			UID:       "crash-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
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

// TestInitContainerCrashLoop: containerStatuses is empty/healthy but an init container
// is Waiting with reason "CrashLoopBackOff" → ExtractFinding returns a non-nil finding.
func TestInitContainerCrashLoop(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init-crash-pod",
			Namespace: "default",
			UID:       "init-crash-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			// No failing main containers.
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: false,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "PodInitializing",
						},
					},
				},
			},
			// Init container is crash-looping.
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "init-setup",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 1,
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding for init container CrashLoopBackOff, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	assertErrorTextContains(t, finding.Errors, "init-setup")
	assertErrorTextContains(t, finding.Errors, "CrashLoopBackOff")
}

// TestWaitingMessageRedacted: container waiting with a message containing password=secret123
// → error text must NOT contain "secret123" and must contain "[REDACTED]".
func TestWaitingMessageRedacted(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redact-pod",
			Namespace: "default",
			UID:       "redact-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CreateContainerConfigError",
							Message: "failed to start: password=secret123 not accepted",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
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

// TestWaitingMessageTruncated: container waiting with a Waiting.Message exceeding
// maxWaitingMessage (1024 bytes) → error text must be truncated and contain "...[truncated]".
// The message uses spaces so it is not matched by the base64 redaction pattern
// (which requires 40+ consecutive alphanumeric chars), ensuring truncation fires.
func TestWaitingMessageTruncated(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	longMsg := strings.Repeat("container startup error occurred. ", 40) // 1360 chars, spaces break base64

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trunc-pod",
			Namespace: "default",
			UID:       "trunc-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CreateContainerConfigError",
							Message: longMsg,
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	if contains(finding.Errors, longMsg) {
		t.Errorf("error text should not contain the full 1360-char message (expected truncation): %s", finding.Errors)
	}
	assertErrorTextContains(t, finding.Errors, "...[truncated]")
}

// TestTerminatedMessageRedacted: container terminated non-zero with a message containing password=secret123
// → error text must NOT contain "secret123" and must contain "[REDACTED]".
func TestTerminatedMessageRedacted(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "term-redact-pod",
			Namespace: "default",
			UID:       "term-redact-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Message:  "connection failed: password=secret123",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
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

// TestUnschedulableMessageRedacted: unschedulable condition message containing token=abc123
// → error text must NOT contain "abc123" and must contain "[REDACTED]".
func TestUnschedulableMessageRedacted(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sched-redact-pod",
			Namespace: "default",
			UID:       "sched-redact-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/3 nodes available: token=supersecrettoken123",
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	assertErrorsJSON(t, finding.Errors)
	if contains(finding.Errors, "supersecrettoken123") {
		t.Errorf("error text should not contain raw secret value 'supersecrettoken123': %s", finding.Errors)
	}
	assertErrorTextContains(t, finding.Errors, "[REDACTED]")
}

// TestPodAnnotationEnabled_False: crashing pod with mechanic.io/enabled=false → (nil, nil).
// Uses an unhealthy CrashLoopBackOff pod to prove the gate fires on an object that would
// otherwise produce a non-nil finding.
func TestPodAnnotationEnabled_False(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ann-pod",
			Namespace: "default",
			UID:       "ann-pod-uid",
			Annotations: map[string]string{
				domain.AnnotationEnabled: "false",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}
	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when annotation enabled=false, got %+v", finding)
	}
}

// TestPodAnnotationSkipUntilFuture: crashing pod with mechanic.io/skip-until=2099-12-31 → (nil, nil).
func TestPodAnnotationSkipUntilFuture(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "skip-pod",
			Namespace: "default",
			UID:       "skip-pod-uid",
			Annotations: map[string]string{
				domain.AnnotationSkipUntil: "2099-12-31",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}
	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding when skip-until is in the future, got %+v", finding)
	}
}

// TestCrashLoopBackOff_Critical: CrashLoopBackOff with restart count > 5 → severity = critical.
func TestCrashLoopBackOff_Critical(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "critical-pod",
			Namespace: "default",
			UID:       "critical-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "my-app",
					RestartCount: 10,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
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

// TestCrashLoopBackOff_HighAtBoundary: CrashLoopBackOff with restart count exactly 5 → severity = high (not critical).
func TestCrashLoopBackOff_HighAtBoundary(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "boundary-pod",
			Namespace: "default",
			UID:       "boundary-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "my-app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
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

// TestOOMKilledTerminated_High: container terminated with OOMKilled (not waiting/CrashLoop)
// → severity = high (terminated OOMKilled maps to medium via non-zero exit, but we need to
// clarify: OOMKilled is only high if it's the Terminated.Reason on a non-waiting container.
// Per story: "Any container in OOMKilled (terminated reason) → high".
// This covers Terminated state where Reason is OOMKilled and Waiting is nil.
func TestOOMKilledTerminated_High(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oom-term-pod",
			Namespace: "default",
			UID:       "oom-term-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "my-app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
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

// TestCrashLoopBackOff_CriticalWins_OverHigh: two containers: one CrashLoopBackOff > 5 restarts
// (critical), one ImagePullBackOff (high) → severity = critical (highest wins).
func TestCrashLoopBackOff_CriticalWins_OverHigh(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c, testRedactor(t))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-sev-pod",
			Namespace: "default",
			UID:       "multi-sev-pod-uid",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app-a",
					RestartCount: 10,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
				{
					Name: "app-b",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "ImagePullBackOff",
						},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected finding, got nil")
	}
	if finding.Severity != domain.SeverityCritical {
		t.Errorf("finding.Severity = %q, want %q (critical should win over high)", finding.Severity, domain.SeverityCritical)
	}
}

// TestComputePodSeverity_CrashLoopBackOff_OOMKilled_LastTermination: computePodSeverity for a pod
// in CrashLoopBackOff (State.Waiting) with RestartCount=3 and LastTerminationState.Terminated.Reason="OOMKilled"
// → severity = high. This verifies the OOMKilled check in the CrashLoopBackOff branch, which is
// otherwise unreachable via cs.State.Terminated (since State.Waiting is set, not State.Terminated).
func TestComputePodSeverity_CrashLoopBackOff_OOMKilled_LastTermination(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "my-app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	got := computePodSeverity(pod)
	if got != domain.SeverityHigh {
		t.Errorf("computePodSeverity() = %q, want %q", got, domain.SeverityHigh)
	}
}

// assertErrorsJSON verifies that the errors string is valid JSON with at least one entry.
func assertErrorsJSON(t *testing.T, errors string) {
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

// assertErrorTextContains checks that at least one entry in the errors JSON contains substr.
func assertErrorTextContains(t *testing.T, errors, substr string) {
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

// contains is a simple string contains helper to avoid importing strings in tests.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
