package controller_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	fakerest "k8s.io/client-go/rest/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/controller"
)

// ---------------------------------------------------------------------------
// Fake kubernetes.Interface for dry-run log-stream tests
// ---------------------------------------------------------------------------

// fakeLogKubeClient is a kubernetes.Interface that delegates to fake.Clientset
// but overrides CoreV1().Pods().GetLogs() to return configurable log content.
type fakeLogKubeClient struct {
	*kubefake.Clientset
	logContent string
	logErr     error
}

func newFakeLogKubeClient(logContent string, logErr error) *fakeLogKubeClient {
	return &fakeLogKubeClient{
		Clientset:  kubefake.NewClientset(),
		logContent: logContent,
		logErr:     logErr,
	}
}

func (f *fakeLogKubeClient) CoreV1() corev1client.CoreV1Interface {
	return &fakeCoreV1{
		CoreV1Interface: f.Clientset.CoreV1(),
		logContent:      f.logContent,
		logErr:          f.logErr,
	}
}

// fakeCoreV1 wraps CoreV1Interface and overrides Pods() to inject log behaviour.
type fakeCoreV1 struct {
	corev1client.CoreV1Interface
	logContent string
	logErr     error
}

func (f *fakeCoreV1) Pods(namespace string) corev1client.PodInterface {
	return &fakePodClient{
		PodInterface: f.CoreV1Interface.Pods(namespace),
		namespace:    namespace,
		logContent:   f.logContent,
		logErr:       f.logErr,
	}
}

// fakePodClient wraps PodInterface and overrides GetLogs.
type fakePodClient struct {
	corev1client.PodInterface
	namespace  string
	logContent string
	logErr     error
}

func (f *fakePodClient) GetLogs(name string, opts *corev1.PodLogOptions) *restclient.Request {
	if f.logErr != nil {
		fakeClient := &fakerest.RESTClient{
			Client: fakerest.CreateHTTPClient(func(_ *http.Request) (*http.Response, error) {
				return nil, f.logErr
			}),
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
			GroupVersion:         corev1.SchemeGroupVersion,
			VersionedAPIPath:     fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log", f.namespace, name),
		}
		return fakeClient.Request()
	}
	fakeClient := &fakerest.RESTClient{
		Client: fakerest.CreateHTTPClient(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(f.logContent)),
			}, nil
		}),
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         corev1.SchemeGroupVersion,
		VersionedAPIPath:     fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log", f.namespace, name),
	}
	return fakeClient.Request()
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

const dryRunSentinel = "=== DRY_RUN INVESTIGATION REPORT ==="

func newDryRunRJobWithJob(
	rjobName, fp string,
	rjobPhase v1alpha1.RemediationJobPhase,
	jobAnnotations map[string]string,
) (*v1alpha1.RemediationJob, *batchv1.Job) {
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rjobName,
			Namespace: testNamespace,
			UID:       types.UID("uid-" + rjobName),
		},
		Spec: v1alpha1.RemediationJobSpec{Fingerprint: fp},
	}
	rjob.Status.Phase = rjobPhase

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "mendabot-agent-" + fp[:12],
			Namespace:   testNamespace,
			Labels:      map[string]string{"remediation.mendabot.io/remediation-job": rjobName},
			Annotations: jobAnnotations,
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	return rjob, job
}

func newSucceededPod(podName, namespace, jobName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"batch.kubernetes.io/job-name": jobName},
		},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestFetchDryRunReport_NilKubeClient verifies that when KubeClient is nil,
// the reconciler does not panic and sets Message to the "not configured" string.
// We verify this by testing through the reconcile loop with KubeClient = nil
// on a dry-run-annotated succeeded job.
func TestFetchDryRunReport_NilKubeClient(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-nilclient", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
		KubeClient: nil, // nil — triggers the "not configured" fallback
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-nilclient", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-nilclient", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	want := "dry-run report unavailable: KubeClient not configured"
	if updated.Status.Message != want {
		t.Errorf("Message = %q, want %q", updated.Status.Message, want)
	}
}

// TestReconcile_DryRunSucceeded_ReportStored verifies the full dry-run report
// extraction: sentinel present → Message contains post-sentinel text.
func TestReconcile_DryRunSucceeded_ReportStored(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-stored", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	pod := newSucceededPod("test-pod-stored", testNamespace, job.Name)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, pod).
		Build()

	logContent := dryRunSentinel + "\n## Root Cause\nImagePullBackOff — image not found."
	kubeClient := newFakeLogKubeClient(logContent, nil)

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
		KubeClient: kubeClient,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-stored", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-stored", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseSucceeded {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseSucceeded)
	}
	if !strings.Contains(updated.Status.Message, "ImagePullBackOff") {
		t.Errorf("Message = %q — want it to contain post-sentinel report text", updated.Status.Message)
	}
	if strings.Contains(updated.Status.Message, dryRunSentinel) {
		t.Errorf("Message = %q — must NOT contain the sentinel line itself", updated.Status.Message)
	}
}

// TestReconcile_DryRunSucceeded_ReportTruncated verifies that log content
// exceeding maxReportBytes is truncated (message length ≤ 10,000 bytes).
func TestReconcile_DryRunSucceeded_ReportTruncated(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-trunc", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	pod := newSucceededPod("test-pod-trunc", testNamespace, job.Name)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, pod).
		Build()

	// Preamble (9 bytes) + sentinel (37 bytes) + newline (1 byte) = 47 bytes prefix,
	// followed by 11,000 bytes of 'x'. The LimitReader caps the entire stream at
	// 10,000 bytes, so only ~9,953 'x' bytes are read — confirming truncation.
	bigLog := "preamble\n" + dryRunSentinel + "\n" + strings.Repeat("x", 11_000)
	kubeClient := newFakeLogKubeClient(bigLog, nil)

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
		KubeClient: kubeClient,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-trunc", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-trunc", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if len(updated.Status.Message) > 10_000 {
		t.Errorf("Message length = %d, want ≤ 10,000", len(updated.Status.Message))
	}
	// Confirm truncation actually occurred: not all 11,000 'x' bytes should be present.
	if strings.Count(updated.Status.Message, "x") >= 11_000 {
		t.Errorf("message was not truncated: contains all 11,000 bytes of post-sentinel content (%d x-bytes found)", strings.Count(updated.Status.Message, "x"))
	}
}

// TestReconcile_DryRunSucceeded_SentinelAbsent verifies that when no sentinel
// is present, Message starts with "(sentinel not found".
func TestReconcile_DryRunSucceeded_SentinelAbsent(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-nosentinel", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	pod := newSucceededPod("test-pod-nosentinel", testNamespace, job.Name)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, pod).
		Build()

	kubeClient := newFakeLogKubeClient("agent output without any sentinel line", nil)

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
		KubeClient: kubeClient,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-nosentinel", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-nosentinel", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.HasPrefix(updated.Status.Message, "(sentinel not found") {
		t.Errorf("Message = %q, want prefix \"(sentinel not found\"", updated.Status.Message)
	}
}

// TestReconcile_DryRunSucceeded_NoPodFound verifies that when no succeeded pod
// exists, Message starts with "dry-run report unavailable".
func TestReconcile_DryRunSucceeded_NoPodFound(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-nopod", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	// No pod objects — pod list will be empty.
	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job).
		Build()

	kubeClient := newFakeLogKubeClient("irrelevant", nil)

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
		KubeClient: kubeClient,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-nopod", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-nopod", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.HasPrefix(updated.Status.Message, "dry-run report unavailable") {
		t.Errorf("Message = %q, want prefix \"dry-run report unavailable\"", updated.Status.Message)
	}
}

// TestReconcile_NoDryRun_MessageNotPopulated verifies that a succeeded Job
// without the dry-run annotation leaves Message empty.
func TestReconcile_NoDryRun_MessageNotPopulated(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-nodryrun", fp, v1alpha1.PhaseDispatched,
		nil, // no annotations
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job).
		Build()

	kubeClient := newFakeLogKubeClient("should not be reached", nil)

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
		KubeClient: kubeClient,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-nodryrun", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-nodryrun", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Message != "" {
		t.Errorf("Message = %q, want empty string for non-dry-run succeeded job", updated.Status.Message)
	}
}

// Verify our fake implements the interface (compile-time check).
var _ interface {
	CoreV1() corev1client.CoreV1Interface
} = (*fakeLogKubeClient)(nil)
