package native

import (
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
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
	p := NewPodProvider(c)

	got := p.ProviderName()
	if got != "native" {
		t.Errorf("ProviderName() = %q, want %q", got, "native")
	}
}

// TestObjectType_IsPod verifies ObjectType() returns a *corev1.Pod.
func TestObjectType_IsPod(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

	obj := p.ObjectType()
	if _, ok := obj.(*corev1.Pod); !ok {
		t.Errorf("ObjectType() returned %T, want *corev1.Pod", obj)
	}
}

// TestHealthyRunningPod: running pod with no failure conditions → (nil, nil).
func TestHealthyRunningPod(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
	p := NewPodProvider(c)

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
// → finding with OOMKilled and container name in error text.
func TestCrashLoopBackOffOOMKilled(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
}

// TestCrashLoopBackOffGeneric: container waiting CrashLoopBackOff, last terminated "Error"
// → finding with container name.
func TestCrashLoopBackOffGeneric(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
}

// TestImagePullBackOff: container waiting ImagePullBackOff with non-empty Message
// → finding with the Message in error text.
func TestImagePullBackOff(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
}

// TestErrImagePull: container waiting ErrImagePull → finding with error text.
func TestErrImagePull(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
}

// TestCreateContainerConfigError: container waiting CreateContainerConfigError with message
// → finding with the message in error text.
func TestCreateContainerConfigError(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
}

// TestNonZeroExitCode: terminated container with exit code 137, Waiting == nil
// → finding with exit code in error text.
func TestNonZeroExitCode(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
}

// TestUnschedulablePending: pod pending with PodScheduled=Unschedulable
// → finding with scheduler message.
func TestUnschedulablePending(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
}

// TestNoOwnerRef: crashing pod with no ownerReferences → ParentObject == "Pod/<pod-name>".
func TestNoOwnerRef(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
	p := NewPodProvider(c)

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
	p := NewPodProvider(c)

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
	p := NewPodProvider(c)

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

// TestSourceRef_IsPodV1: SourceRef should identify the pod with APIVersion "v1".
func TestSourceRef_IsPodV1(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crash-pod",
			Namespace: "production",
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

	wantRef := domain.SourceRef{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "crash-pod",
		Namespace:  "production",
	}
	if finding.SourceRef != wantRef {
		t.Errorf("SourceRef = %+v, want %+v", finding.SourceRef, wantRef)
	}
}

// TestInitContainerCrashLoop: containerStatuses is empty/healthy but an init container
// is Waiting with reason "CrashLoopBackOff" → ExtractFinding returns a non-nil finding.
func TestInitContainerCrashLoop(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewPodProvider(c)

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
