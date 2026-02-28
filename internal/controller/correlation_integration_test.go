// pendingPeers calls r.List on RemediationJobs; covered by existing ClusterRole grant

package controller_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/config"
	"github.com/lenaxia/k8s-mechanic/internal/controller"
	"github.com/lenaxia/k8s-mechanic/internal/correlator"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
	"github.com/lenaxia/k8s-mechanic/internal/provider"
	"go.uber.org/zap"
)

// ctrlReqNS builds a reconcile request for a given name and namespace.
func ctrlReqNS(name, namespace string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}
}

// corrRunCounter is incremented each time a correlation test allocates a namespace.
// Using unique names per allocation ensures -count=N runs do not race with a
// terminating namespace from a prior run (Kubernetes namespace deletion is asynchronous
// and can take minutes; a terminating namespace rejects new object creation).
var corrRunCounter int64

// corrNS returns a unique namespace name for use in correlation tests.
// Each call returns "base-N" where N is a monotonically increasing counter,
// so successive -count=N invocations always get a fresh, never-seen namespace.
func corrNS(base string) string {
	return fmt.Sprintf("%s-%d", base, atomic.AddInt64(&corrRunCounter, 1))
}

// ensureNamespace creates a Kubernetes Namespace in the envtest cluster.
// Returns a cleanup func that deletes the namespace.
// Callers must use corrNS() to generate unique namespace names so this function
// never encounters an already-existing namespace.
func ensureNamespace(t *testing.T, ctx context.Context, c client.Client, name string) func() {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("create namespace %q: %v", name, err)
	}
	return func() { _ = c.Delete(ctx, ns) }
}

// cleanupJobsInNS registers a t.Cleanup that deletes all batch/v1 Jobs in the given
// namespace. Call this after any test that dispatches jobs into a custom namespace so
// that leftover Job objects do not pollute the shared envtest cluster for later tests.
//
// Ordering note: t.Cleanup runs in LIFO order. Register cleanupJobsInNS immediately
// after ensureNamespace so that jobs are swept before the rjob cleanup (which is
// registered later and therefore runs first). In envtest the GC controller is not
// running, so the rjob deletion does not cascade to owned jobs — cleanupJobsInNS must
// delete them explicitly. In a real cluster the GC would handle it, but the explicit
// delete is still safe (idempotent via ignored NotFound errors).
func cleanupJobsInNS(t *testing.T, ctx context.Context, c client.Client, namespace string) {
	t.Helper()
	t.Cleanup(func() {
		var jobs batchv1.JobList
		if err := c.List(ctx, &jobs, client.InNamespace(namespace)); err != nil {
			return
		}
		for i := range jobs.Items {
			deleteJob(ctx, c, &jobs.Items[i])
		}
	})
}

// newCorrelationRJob builds a RemediationJob with the given name, namespace, fingerprint, and finding.
func newCorrelationRJob(name, namespace, fp string, finding v1alpha1.FindingSpec) *v1alpha1.RemediationJob {
	return &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			// All mechanic-managed RemediationJobs carry this label; pendingPeers
			// uses it as a server-side filter to avoid full-namespace scans.
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mechanic-watcher",
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeNative,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "result-" + name,
				Namespace: namespace,
			},
			Finding:            finding,
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mechanic-agent:test",
			AgentSA:            "mechanic-agent",
		},
	}
}

// newCorrReconcilerWith builds a RemediationJobReconciler with the given rules and job builder.
// CorrelationWindowSeconds is set to 1 for all correlation integration tests.
func newCorrReconcilerWith(
	c client.Client,
	namespace string,
	rules []domain.CorrelationRule,
	jb *fakeJobBuilder,
) *controller.RemediationJobReconciler {
	cfg := config.Config{
		AgentNamespace:           namespace,
		MaxConcurrentJobs:        10,
		RemediationJobTTLSeconds: 604800,
		CorrelationWindowSeconds: 1,
	}
	var corr *correlator.Correlator
	if rules != nil {
		corr = &correlator.Correlator{Rules: rules}
	}
	return &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     c.Scheme(),
		Log:        zap.NewNop(),
		JobBuilder: jb,
		Cfg:        cfg,
		Correlator: corr,
	}
}

// dynamicFakeJobBuilder builds a fakeJobBuilder that always constructs a valid job
// for the given rjob by re-fetching it from the client immediately before building.
// This ensures the job has the correct UID-based selector even when the same builder
// is reused across multiple reconcile calls for different RemediationJobs.
type dynamicFakeJobBuilder struct {
	ctx                    context.Context
	c                      client.Client
	lastCorrelatedFindings []v1alpha1.FindingSpec
}

func (d *dynamicFakeJobBuilder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error) {
	d.lastCorrelatedFindings = correlatedFindings
	// Re-fetch to get the current UID, which must match the selector on the Job.
	var fetched v1alpha1.RemediationJob
	if err := d.c.Get(d.ctx, client.ObjectKeyFromObject(rjob), &fetched); err != nil {
		return nil, err
	}
	return newIntegrationJob(&fetched), nil
}

// TC-01: Single finding, no correlation — job dispatched without group label.
//
// Pattern: first reconcile returns window hold, sleep 1.1s, second reconcile dispatches.
// With no peers in the namespace, no group label is set.
func TestCorrelationIntegration_TC01_SingleFinding_NoCorrelation(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc01-single")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))
	cleanupJobsInNS(t, ctx, c, ns)

	const fp = "1111111122222222333333334444444411111111222222223333333344444444"
	rjob := newCorrelationRJob("tc01-rjob", ns, fp, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc01",
		Namespace:    ns,
		ParentObject: "deploy-tc01",
	})
	if err := c.Create(ctx, rjob); err != nil {
		t.Fatalf("create rjob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob) })

	// Re-fetch to get the UID assigned by envtest.
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob), rjob); err != nil {
		t.Fatalf("re-fetch rjob: %v", err)
	}

	jb := &fakeJobBuilder{returnJob: newIntegrationJob(rjob)}
	rules := []domain.CorrelationRule{correlator.SameNamespaceParentRule{}}
	rec := newCorrReconcilerWith(c, ns, rules, jb)

	// Initialise: "" → Pending.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob.Name, ns)); err != nil {
		t.Fatalf("init Reconcile: %v", err)
	}

	// First call after init: window not elapsed → expect RequeueAfter > 0.
	// Under heavy load (e.g. -count=3) the window may already have elapsed by the
	// time this reconcile runs; skip the hold assertion in that case and go straight
	// to the dispatch reconcile.
	result, err := rec.Reconcile(ctx, ctrlReqNS(rjob.Name, ns))
	if err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if result.RequeueAfter > 0 {
		// Window is still active — wait for it to elapse, then dispatch.
		time.Sleep(result.RequeueAfter + 100*time.Millisecond)
		if _, err = rec.Reconcile(ctx, ctrlReqNS(rjob.Name, ns)); err != nil {
			t.Fatalf("dispatch Reconcile: %v", err)
		}
	}
	// If RequeueAfter == 0 the window already elapsed and the job was dispatched
	// in the call above; fall through directly to the phase assertion.

	var updated v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob), &updated); err != nil {
		t.Fatalf("get updated rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseDispatched)
	}
	// No correlation group label must be set on a solo dispatch.
	if _, ok := updated.Labels[domain.CorrelationGroupIDLabel]; ok {
		t.Error("expected no correlation-group-id label on solo dispatch")
	}
	if _, ok := updated.Labels[domain.CorrelationGroupRoleLabel]; ok {
		t.Error("expected no correlation-role label on solo dispatch")
	}
}

// TC-02: SameNamespaceParent correlation — primary dispatches with group label and
// suppresses the peer directly.
//
// Both rjobs are initialised to Phase==Pending before the window elapses.
// Reconcile rjob1 (primary) after the window: rjob2 is still Phase==Pending → visible
// as a peer. SameNamespaceParentRule matches; rjob1 is selected as primary (older
// timestamp wins on ties). rjob1 calls suppressCorrelatedPeers, suppressing rjob2,
// then dispatches with the full set of correlated findings.
//
// rjob2 is now Suppressed and will not be dispatched independently.
func TestCorrelationIntegration_TC02_SameNamespaceParent(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc02-same-ns-parent")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))
	cleanupJobsInNS(t, ctx, c, ns)

	const fp1 = "aaaa1111bbbb2222cccc3333dddd4444aaaa1111bbbb2222cccc3333dddd4444"
	const fp2 = "bbbb2222cccc3333dddd4444eeee5555bbbb2222cccc3333dddd4444eeee5555"

	// Both jobs share the same namespace and a common ParentObject.
	rjob1 := newCorrelationRJob("tc02-rjob1", ns, fp1, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc02-a",
		Namespace:    ns,
		ParentObject: "my-app",
	})
	rjob2 := newCorrelationRJob("tc02-rjob2", ns, fp2, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc02-b",
		Namespace:    ns,
		ParentObject: "my-app",
	})
	if err := c.Create(ctx, rjob1); err != nil {
		t.Fatalf("create rjob1: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob1) })
	if err := c.Create(ctx, rjob2); err != nil {
		t.Fatalf("create rjob2: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob2) })

	// Re-fetch rjob1 to get UID.
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), rjob1); err != nil {
		t.Fatalf("re-fetch rjob1: %v", err)
	}

	// dynamicFakeJobBuilder re-fetches the rjob at Build() time to get the current UID,
	// ensuring the selector labels are correct regardless of which rjob is being dispatched.
	djb := &dynamicFakeJobBuilder{ctx: ctx, c: c}

	rules := []domain.CorrelationRule{correlator.SameNamespaceParentRule{}}
	rec := newCorrReconcilerWith(c, ns, rules, &fakeJobBuilder{})
	rec.JobBuilder = djb

	// Initialise rjob1: "" → Pending (returns Requeue:true, not RequeueAfter).
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns)); err != nil {
		t.Fatalf("init Reconcile on rjob1: %v", err)
	}
	// Initialise rjob2: "" → Pending so it is visible as a peer during rjob1's window-hold.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob2.Name, ns)); err != nil {
		t.Fatalf("init Reconcile on rjob2: %v", err)
	}

	// Second reconcile of rjob1 (now Pending): if the window has not yet elapsed, it must
	// return RequeueAfter > 0. Under heavy load the window may already have elapsed here;
	// guard so we don't produce a false negative.
	result1Hold, err := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns))
	if err != nil {
		t.Fatalf("window-hold Reconcile on rjob1: %v", err)
	}
	var updated1Hold v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated1Hold); err != nil {
		t.Fatalf("get rjob1 after window-hold reconcile: %v", err)
	}
	if updated1Hold.Status.Phase == v1alpha1.PhasePending && result1Hold.RequeueAfter == 0 {
		t.Error("TC02: rjob1 is still Pending but window-hold returned RequeueAfter=0 — expected window hold")
	}

	// Wait for window to elapse (window-hold behaviour is tested by TC01).
	time.Sleep(1100 * time.Millisecond)

	// Reconcile rjob1: window elapsed → correlator runs → rjob1 is primary
	// (rjob2 is still Phase==Pending and thus visible as a pending peer).
	// Primary calls suppressCorrelatedPeers → rjob2 becomes Suppressed, then dispatches.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns)); err != nil {
		t.Fatalf("dispatch Reconcile on rjob1: %v", err)
	}

	var updated1 v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated1); err != nil {
		t.Fatalf("get updated rjob1: %v", err)
	}
	if updated1.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("rjob1 phase = %q, want %q (primary must dispatch)", updated1.Status.Phase, v1alpha1.PhaseDispatched)
	}
	groupID, hasGroupID := updated1.Labels[domain.CorrelationGroupIDLabel]
	if !hasGroupID {
		t.Error("expected correlation-group-id label on primary rjob1")
	}
	if groupID == "" {
		t.Error("expected non-empty correlation-group-id on primary rjob1")
	}
	role, hasRole := updated1.Labels[domain.CorrelationGroupRoleLabel]
	if !hasRole {
		t.Error("expected correlation-role label on primary rjob1")
	}
	if role != domain.CorrelationRolePrimary {
		t.Errorf("rjob1 role = %q, want %q", role, domain.CorrelationRolePrimary)
	}

	// Assert that the primary received the peer's finding in correlatedFindings.
	if djb.lastCorrelatedFindings == nil {
		t.Error("TC02: expected non-nil lastCorrelatedFindings on primary dispatch")
	}
	if len(djb.lastCorrelatedFindings) < 1 {
		t.Errorf("TC02: expected lastCorrelatedFindings to have >= 1 entry, got %d", len(djb.lastCorrelatedFindings))
	}
	peerFindingFound := false
	for _, f := range djb.lastCorrelatedFindings {
		if f.Name == "pod-tc02-b" {
			peerFindingFound = true
			break
		}
	}
	if !peerFindingFound {
		t.Errorf("TC02: expected rjob2's finding (pod-tc02-b) in lastCorrelatedFindings, got %+v", djb.lastCorrelatedFindings)
	}

	// rjob2 must now be Suppressed — the primary called suppressCorrelatedPeers.
	var updated2 v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob2), &updated2); err != nil {
		t.Fatalf("get updated rjob2: %v", err)
	}
	if updated2.Status.Phase != v1alpha1.PhaseSuppressed {
		t.Errorf("rjob2 phase = %q, want %q (primary must have suppressed the peer)", updated2.Status.Phase, v1alpha1.PhaseSuppressed)
	}
	if updated2.Status.CorrelationGroupID == "" {
		t.Error("TC02: expected CorrelationGroupID to be set on suppressed rjob2")
	}
	if groupID != "" && updated2.Status.CorrelationGroupID != groupID {
		t.Errorf("TC02: peer CorrelationGroupID = %q, want primary's group label %q", updated2.Status.CorrelationGroupID, groupID)
	}
}

// TC-03: PVCPod correlation — PVC finding is primary, Pod finding is suppressed by the primary.
//
// Both jobs are initialised to Phase==Pending before the window elapses.
// Reconcile order: pod job first (window elapsed, PVC job still Phase==Pending → visible as peer).
// The PVCPodRule fires: PVC is the primary, Pod is a non-primary candidate.
// Non-primary candidate returns RequeueAfter:5s and stays Pending.
// Then PVC job reconciles as primary: Pod is still Pending → visible in peers.
// PVC calls suppressCorrelatedPeers (suppresses Pod), then dispatches with both findings.
func TestCorrelationIntegration_TC03_PVCPod(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc03-pvcpod")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))
	cleanupJobsInNS(t, ctx, c, ns)

	const pvcName = "my-pvc"
	const podName = "pod-tc03"
	const fpPVC = "cccc3333dddd4444eeee5555ffff6666cccc3333dddd4444eeee5555ffff6666"
	const fpPod = "dddd4444eeee5555ffff6666aaaa7777dddd4444eeee5555ffff6666aaaa7777"

	// Create the Pod in envtest with a volume referencing the PVC.
	// PVCPodRule calls client.Get on the Pod to inspect spec.volumes.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:latest"},
			},
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}
	if err := c.Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, pod) })

	// PVC RemediationJob (will be primary per PVCPodRule).
	rjobPVC := newCorrelationRJob("tc03-rjob-pvc", ns, fpPVC, v1alpha1.FindingSpec{
		Kind:      "PersistentVolumeClaim",
		Name:      pvcName,
		Namespace: ns,
	})
	// Pod RemediationJob (will be secondary — suppressed by the primary).
	rjobPod := newCorrelationRJob("tc03-rjob-pod", ns, fpPod, v1alpha1.FindingSpec{
		Kind:      "Pod",
		Name:      podName,
		Namespace: ns,
	})

	if err := c.Create(ctx, rjobPVC); err != nil {
		t.Fatalf("create rjobPVC: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjobPVC) })
	if err := c.Create(ctx, rjobPod); err != nil {
		t.Fatalf("create rjobPod: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjobPod) })

	// Re-fetch to get UIDs.
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjobPVC), rjobPVC); err != nil {
		t.Fatalf("re-fetch rjobPVC: %v", err)
	}

	djb := &dynamicFakeJobBuilder{ctx: ctx, c: c}
	rules := []domain.CorrelationRule{correlator.PVCPodRule{}}
	rec := newCorrReconcilerWith(c, ns, rules, &fakeJobBuilder{})
	rec.JobBuilder = djb

	// Initialise PVC job: "" → Pending.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjobPVC.Name, ns)); err != nil {
		t.Fatalf("init Reconcile on PVC job: %v", err)
	}
	// Initialise Pod job: "" → Pending so it is visible during PVC correlation.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjobPod.Name, ns)); err != nil {
		t.Fatalf("init Reconcile on Pod job: %v", err)
	}

	// Window-hold reconcile on PVC job: if the window has not yet elapsed, it must
	// return RequeueAfter > 0. Under heavy load the window may already have elapsed;
	// guard so we don't sleep a fixed 1100ms on slow machines AND avoid a false
	// negative on fast machines (same pattern as TC-02).
	resultPVCHold, err := rec.Reconcile(ctx, ctrlReqNS(rjobPVC.Name, ns))
	if err != nil {
		t.Fatalf("window-hold Reconcile on PVC job: %v", err)
	}
	var updatedPVCHold v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjobPVC), &updatedPVCHold); err != nil {
		t.Fatalf("get PVC rjob after window-hold reconcile: %v", err)
	}
	if updatedPVCHold.Status.Phase == v1alpha1.PhasePending && resultPVCHold.RequeueAfter == 0 {
		t.Error("TC03: PVC rjob is still Pending but window-hold returned RequeueAfter=0 — expected window hold")
	}
	if resultPVCHold.RequeueAfter > 0 {
		time.Sleep(resultPVCHold.RequeueAfter + 100*time.Millisecond)
	}

	// Reconcile Pod job FIRST (window elapsed, PVC job still Phase==Pending → visible as peer).
	// PVCPodRule: Pod candidate → PVC is primary → Pod is non-primary.
	// Non-primary must NOT self-suppress: returns RequeueAfter:5s and stays Pending.
	podResult, err := rec.Reconcile(ctx, ctrlReqNS(rjobPod.Name, ns))
	if err != nil {
		t.Fatalf("non-primary Reconcile on Pod job: %v", err)
	}
	if podResult.RequeueAfter != 5*time.Second {
		t.Errorf("TC03: Pod (non-primary) expected RequeueAfter=5s, got %v", podResult.RequeueAfter)
	}

	var updatedPodAfterFirst v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjobPod), &updatedPodAfterFirst); err != nil {
		t.Fatalf("get Pod rjob after first reconcile: %v", err)
	}
	if updatedPodAfterFirst.Status.Phase != v1alpha1.PhasePending {
		t.Errorf("TC03: Pod (non-primary) phase = %q, want %q after first reconcile", updatedPodAfterFirst.Status.Phase, v1alpha1.PhasePending)
	}

	// Reconcile PVC job: Pod is still Phase==Pending → visible as peer.
	// PVCPodRule matches; PVC is primary. suppressCorrelatedPeers suppresses Pod.
	// PVC dispatches with both findings (its own + Pod's).
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjobPVC.Name, ns)); err != nil {
		t.Fatalf("dispatch Reconcile on PVC job: %v", err)
	}

	var updatedPVC v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjobPVC), &updatedPVC); err != nil {
		t.Fatalf("get updated PVC rjob: %v", err)
	}
	if updatedPVC.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("PVC rjob phase = %q, want %q (PVC must dispatch as primary)", updatedPVC.Status.Phase, v1alpha1.PhaseDispatched)
	}

	// Pod must now be Suppressed — the PVC primary called suppressCorrelatedPeers.
	var updatedPod v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjobPod), &updatedPod); err != nil {
		t.Fatalf("get updated Pod rjob: %v", err)
	}
	if updatedPod.Status.Phase != v1alpha1.PhaseSuppressed {
		t.Errorf("Pod rjob phase = %q, want %q (Pod must be suppressed by PVC primary)", updatedPod.Status.Phase, v1alpha1.PhaseSuppressed)
	}

	// The PVC primary dispatched with correlated findings (Pod was still Pending when PVC reconciled).
	if djb.lastCorrelatedFindings == nil {
		t.Error("TC03: expected non-nil lastCorrelatedFindings on PVC primary dispatch (Pod peer was Pending)")
	}
	podFindingFound := false
	for _, f := range djb.lastCorrelatedFindings {
		if f.Name == podName {
			podFindingFound = true
			break
		}
	}
	if !podFindingFound {
		t.Errorf("TC03: expected Pod finding (%s) in lastCorrelatedFindings, got %+v", podName, djb.lastCorrelatedFindings)
	}
}

// TC-04: No correlation across namespaces — two jobs with identical ParentObject but
// different namespaces dispatch independently without any group label.
func TestCorrelationIntegration_TC04_NoCorrelationAcrossNamespaces(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns1 := corrNS("tc04-ns-alpha")
	ns2 := corrNS("tc04-ns-beta")
	t.Cleanup(ensureNamespace(t, ctx, c, ns1))
	t.Cleanup(ensureNamespace(t, ctx, c, ns2))
	cleanupJobsInNS(t, ctx, c, ns1)
	cleanupJobsInNS(t, ctx, c, ns2)

	const fp1 = "eeee5555ffff6666aaaa7777bbbb8888eeee5555ffff6666aaaa7777bbbb8888"
	const fp2 = "ffff6666aaaa7777bbbb8888cccc9999ffff6666aaaa7777bbbb8888cccc9999"

	// Both jobs share the same ParentObject but live in different namespaces.
	rjob1 := newCorrelationRJob("tc04-rjob-a", ns1, fp1, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc04-a",
		Namespace:    ns1,
		ParentObject: "same-parent",
	})
	rjob2 := newCorrelationRJob("tc04-rjob-b", ns2, fp2, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc04-b",
		Namespace:    ns2,
		ParentObject: "same-parent",
	})

	if err := c.Create(ctx, rjob1); err != nil {
		t.Fatalf("create rjob1: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob1) })
	if err := c.Create(ctx, rjob2); err != nil {
		t.Fatalf("create rjob2: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob2) })

	// Re-fetch to get UIDs.
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), rjob1); err != nil {
		t.Fatalf("re-fetch rjob1: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob2), rjob2); err != nil {
		t.Fatalf("re-fetch rjob2: %v", err)
	}

	rules := []domain.CorrelationRule{correlator.SameNamespaceParentRule{}}

	// Reconciler for ns1 — its AgentNamespace is ns1, so pendingPeers only lists ns1 jobs.
	jb1 := &fakeJobBuilder{returnJob: newIntegrationJob(rjob1)}
	rec1 := newCorrReconcilerWith(c, ns1, rules, jb1)

	// Reconciler for ns2 — its AgentNamespace is ns2.
	jb2 := &fakeJobBuilder{returnJob: newIntegrationJob(rjob2)}
	rec2 := newCorrReconcilerWith(c, ns2, rules, jb2)

	// Initialise both jobs: "" → Pending.
	if _, err := rec1.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns1)); err != nil {
		t.Fatalf("init Reconcile on rjob1: %v", err)
	}
	if _, err := rec2.Reconcile(ctx, ctrlReqNS(rjob2.Name, ns2)); err != nil {
		t.Fatalf("init Reconcile on rjob2: %v", err)
	}

	// Window-hold reconcile on rjob1 (in ns1): adaptive sleep to avoid flakiness under CI load.
	result1HoldNS, err1HoldNS := rec1.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns1))
	if err1HoldNS != nil {
		t.Fatalf("TC04: window-hold Reconcile on rjob1: %v", err1HoldNS)
	}
	var updated1HoldNS v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated1HoldNS); err != nil {
		t.Fatalf("TC04: get rjob1 after window-hold reconcile: %v", err)
	}
	if updated1HoldNS.Status.Phase == v1alpha1.PhasePending && result1HoldNS.RequeueAfter == 0 {
		t.Error("TC04: rjob1 is still Pending but window-hold returned RequeueAfter=0 — expected window hold")
	}
	if result1HoldNS.RequeueAfter > 0 {
		time.Sleep(result1HoldNS.RequeueAfter + 100*time.Millisecond)
	}

	// Reconcile each job: window elapsed, no peers in the same namespace → dispatch independently.
	if _, err := rec1.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns1)); err != nil {
		t.Fatalf("dispatch Reconcile on rjob1: %v", err)
	}
	if _, err := rec2.Reconcile(ctx, ctrlReqNS(rjob2.Name, ns2)); err != nil {
		t.Fatalf("dispatch Reconcile on rjob2: %v", err)
	}

	var updated1 v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated1); err != nil {
		t.Fatalf("get updated rjob1: %v", err)
	}
	if updated1.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("rjob1 phase = %q, want %q (must dispatch independently)", updated1.Status.Phase, v1alpha1.PhaseDispatched)
	}
	if _, ok := updated1.Labels[domain.CorrelationGroupIDLabel]; ok {
		t.Error("rjob1 must have no correlation-group-id label (different namespaces)")
	}

	var updated2 v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob2), &updated2); err != nil {
		t.Fatalf("get updated rjob2: %v", err)
	}
	if updated2.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("rjob2 phase = %q, want %q (must dispatch independently)", updated2.Status.Phase, v1alpha1.PhaseDispatched)
	}
	if _, ok := updated2.Labels[domain.CorrelationGroupIDLabel]; ok {
		t.Error("rjob2 must have no correlation-group-id label (different namespaces)")
	}
}

// TC-05: DISABLE_CORRELATION escape hatch — setting Correlator=nil on the reconciler
// causes a freshly-created job to dispatch immediately without any window hold.
func TestCorrelationIntegration_TC05_EscapeHatch_DisableCorrelation(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc05-escape")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))
	cleanupJobsInNS(t, ctx, c, ns)

	const fp1 = "9999aaaa8888bbbb7777cccc6666dddd9999aaaa8888bbbb7777cccc6666dddd"
	const fp2 = "8888bbbb7777cccc6666dddd5555eeee8888bbbb7777cccc6666dddd5555eeee"

	// Two correlated jobs (same ParentObject) exist. With Correlator==nil, neither should
	// be held in a correlation window — both must dispatch independently on their first
	// Pending→dispatch reconcile without any RequeueAfter.
	rjob1 := newCorrelationRJob("tc05-rjob1", ns, fp1, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc05-a",
		Namespace:    ns,
		ParentObject: "correlated-app",
	})
	rjob2 := newCorrelationRJob("tc05-rjob2", ns, fp2, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc05-b",
		Namespace:    ns,
		ParentObject: "correlated-app",
	})

	if err := c.Create(ctx, rjob1); err != nil {
		t.Fatalf("create rjob1: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob1) })
	if err := c.Create(ctx, rjob2); err != nil {
		t.Fatalf("create rjob2: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob2) })

	// Re-fetch rjob1 to get UID.
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), rjob1); err != nil {
		t.Fatalf("re-fetch rjob1: %v", err)
	}

	// rec.Correlator = nil is the escape hatch (DISABLE_CORRELATION=true).
	rec := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     c.Scheme(),
		Log:        zap.NewNop(),
		JobBuilder: &dynamicFakeJobBuilder{ctx: ctx, c: c},
		Cfg: config.Config{
			AgentNamespace:           ns,
			MaxConcurrentJobs:        10,
			RemediationJobTTLSeconds: 604800,
			CorrelationWindowSeconds: 1,
		},
		Correlator: nil,
	}

	// First Reconcile on rjob1: "" → Pending (initialisation step).
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns)); err != nil {
		t.Fatalf("init Reconcile with escape hatch: %v", err)
	}
	// Second Reconcile on rjob1 → immediate dispatch (Correlator==nil, no window hold), RequeueAfter == 0.
	result, err := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns))
	if err != nil {
		t.Fatalf("Reconcile with escape hatch: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected immediate dispatch (RequeueAfter=0) with escape hatch, got %v", result.RequeueAfter)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated); err != nil {
		t.Fatalf("get updated rjob1: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("rjob1 phase = %q, want %q (must dispatch immediately with escape hatch)", updated.Status.Phase, v1alpha1.PhaseDispatched)
	}
	if updated.Labels[domain.CorrelationGroupIDLabel] != "" {
		t.Errorf("rjob1: expected no correlation-group-id label with escape hatch, got %q", updated.Labels[domain.CorrelationGroupIDLabel])
	}

	// Re-fetch rjob2 to get UID.
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob2), rjob2); err != nil {
		t.Fatalf("re-fetch rjob2: %v", err)
	}
	// rjob2 must also dispatch immediately (no hold) with escape hatch.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob2.Name, ns)); err != nil {
		t.Fatalf("init Reconcile rjob2 with escape hatch: %v", err)
	}
	result2, err := rec.Reconcile(ctx, ctrlReqNS(rjob2.Name, ns))
	if err != nil {
		t.Fatalf("dispatch Reconcile rjob2 with escape hatch: %v", err)
	}
	if result2.RequeueAfter != 0 {
		t.Errorf("rjob2: expected immediate dispatch (RequeueAfter=0) with escape hatch, got %v", result2.RequeueAfter)
	}
	var updated2 v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob2), &updated2); err != nil {
		t.Fatalf("get updated rjob2: %v", err)
	}
	if updated2.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("rjob2 phase = %q, want %q (must dispatch immediately with escape hatch)", updated2.Status.Phase, v1alpha1.PhaseDispatched)
	}
}

// TestCorrelationIntegration_SourceProvider_SkipsSuppressed verifies that
// SourceProviderReconciler does not create a new RemediationJob when an existing
// one with the same fingerprint has Phase=Suppressed.
//
// Suppressed is handled by the existing default: case at provider.go:383; no code change needed.
func TestCorrelationIntegration_SourceProvider_SkipsSuppressed(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc06-suppressed-skip")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))

	// The finding we will reconcile.
	finding := &domain.Finding{
		Kind:         "Pod",
		Name:         "pod-tc06",
		Namespace:    ns,
		ParentObject: "deploy-tc06",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
	}

	// Pre-compute fingerprint so we can build the name that SourceProviderReconciler would use.
	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}

	// Create a Suppressed RemediationJob that shares the same fingerprint.
	existingRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mechanic-" + fp[:12],
			Namespace: ns,
			Labels: map[string]string{
				"remediation.mechanic.io/fingerprint": fp[:12],
				"app.kubernetes.io/managed-by":        "mechanic-watcher",
			},
			Annotations: map[string]string{
				"remediation.mechanic.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeNative,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "pod-tc06",
				Namespace: ns,
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         finding.Kind,
				Name:         finding.Name,
				Namespace:    finding.Namespace,
				ParentObject: finding.ParentObject,
				Errors:       finding.Errors,
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mechanic-agent:test",
			AgentSA:            "mechanic-agent",
		},
	}
	if err := c.Create(ctx, existingRJob); err != nil {
		t.Fatalf("create existing Suppressed rjob: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, existingRJob) })

	// Patch it to PhaseSuppressed.
	rjobCopy := existingRJob.DeepCopyObject().(*v1alpha1.RemediationJob)
	existingRJob.Status.Phase = v1alpha1.PhaseSuppressed
	if err := c.Status().Patch(ctx, existingRJob, client.MergeFrom(rjobCopy)); err != nil {
		t.Fatalf("patch status to Suppressed: %v", err)
	}

	// Create the Pod that the provider watches.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-tc06",
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx:latest"},
			},
		},
	}
	if err := c.Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, pod) })

	// Build a SourceProviderReconciler with the fake provider.
	p := &correlationFakeProvider{
		providerName: "native",
		finding:      finding,
	}
	rec := &provider.SourceProviderReconciler{
		Client: c,
		Scheme: c.Scheme(),
		Cfg: config.Config{
			AgentNamespace:           ns,
			MaxConcurrentJobs:        10,
			RemediationJobTTLSeconds: 604800,
			GitOpsRepo:               "org/repo",
			GitOpsManifestRoot:       "deploy",
			AgentImage:               "mechanic-agent:test",
			AgentSA:                  "mechanic-agent",
			// StabilisationWindow=0 disables the stabilisation hold so the provider
			// proceeds directly to the dedup check.
			StabilisationWindow: 0,
		},
		Log:      zap.NewNop(),
		Provider: p,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "pod-tc06", Namespace: ns}}
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Assert that no additional RemediationJob was created — only the pre-existing
	// Suppressed one should exist.
	var rjobList v1alpha1.RemediationJobList
	if err := c.List(ctx, &rjobList, client.InNamespace(ns)); err != nil {
		t.Fatalf("list RemediationJobs: %v", err)
	}
	count := 0
	for i := range rjobList.Items {
		if rjobList.Items[i].Spec.Fingerprint == fp {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 RemediationJob (the Suppressed one), got %d — Suppressed phase must be treated as non-Failed by SourceProviderReconciler", count)
	}
}

// correlationFakeProvider is a minimal SourceProvider for correlation integration tests.
// It watches corev1.Pod objects and returns the configured finding.
type correlationFakeProvider struct {
	providerName string
	finding      *domain.Finding
}

func (f *correlationFakeProvider) ProviderName() string      { return f.providerName }
func (f *correlationFakeProvider) ObjectType() client.Object { return &corev1.Pod{} }
func (f *correlationFakeProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if f.finding == nil {
		return nil, nil
	}
	result := *f.finding
	return &result, nil
}

var _ domain.SourceProvider = (*correlationFakeProvider)(nil)

// TC-02b: SameNamespaceParent suppression path — non-primary returns RequeueAfter:5s,
// then primary suppresses the peer and dispatches.
//
// rjob1 (Deployment) and rjob2 (Pod) share the same namespace and parent prefix.
// Deployment ranks higher than Pod in the hierarchy, so rjob1 is always selected
// as primary regardless of creation order or timestamp tie-breaking.
//
// Reconcile rjob2 (Pod) first while rjob1 is still Pending:
//   - SameNamespaceParentRule matches; rjob1 (Deployment, higher rank) is primary.
//   - rjob2 (Pod) is NOT the primary → returns RequeueAfter:5s, stays Pending.
//
// Then reconcile rjob1 (Deployment): rjob2 is still Pending → visible as peer.
// rjob1 is primary → calls suppressCorrelatedPeers (suppresses rjob2), then dispatches.
func TestCorrelationIntegration_TC02b_SecondaryIsSuppressed(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc02b-suppressed")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))
	cleanupJobsInNS(t, ctx, c, ns)

	const fp1 = "1a2b3c4d5e6f7a8b1a2b3c4d5e6f7a8b1a2b3c4d5e6f7a8b1a2b3c4d5e6f7a8b"
	const fp2 = "2b3c4d5e6f7a8b9c2b3c4d5e6f7a8b9c2b3c4d5e6f7a8b9c2b3c4d5e6f7a8b9c"

	// rjob1 is Deployment (rank 10), rjob2 is Pod (rank 1). Both share the same ParentObject.
	// Deployment always beats Pod in selectPrimary regardless of CreationTimestamp.
	rjob1 := newCorrelationRJob("tc02b-rjob1", ns, fp1, v1alpha1.FindingSpec{
		Kind:         "Deployment",
		Name:         "shared-app",
		Namespace:    ns,
		ParentObject: "shared-app",
	})
	rjob2 := newCorrelationRJob("tc02b-rjob2", ns, fp2, v1alpha1.FindingSpec{
		Kind:         "Pod",
		Name:         "pod-tc02b-b",
		Namespace:    ns,
		ParentObject: "shared-app",
	})

	if err := c.Create(ctx, rjob1); err != nil {
		t.Fatalf("create rjob1: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob1) })
	if err := c.Create(ctx, rjob2); err != nil {
		t.Fatalf("create rjob2: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob2) })

	djb := &dynamicFakeJobBuilder{ctx: ctx, c: c}
	rules := []domain.CorrelationRule{correlator.SameNamespaceParentRule{}}
	rec := newCorrReconcilerWith(c, ns, rules, &fakeJobBuilder{})
	rec.JobBuilder = djb

	// Initialise rjob1: "" → Pending.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns)); err != nil {
		t.Fatalf("init Reconcile on rjob1: %v", err)
	}
	// Initialise rjob2: "" → Pending.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob2.Name, ns)); err != nil {
		t.Fatalf("init Reconcile on rjob2: %v", err)
	}

	// Window-hold reconcile on rjob1: adaptive sleep to avoid flakiness under CI load.
	result1HoldB, err1HoldB := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns))
	if err1HoldB != nil {
		t.Fatalf("TC02b: window-hold Reconcile on rjob1: %v", err1HoldB)
	}
	var updated1HoldB v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated1HoldB); err != nil {
		t.Fatalf("TC02b: get rjob1 after window-hold reconcile: %v", err)
	}
	if updated1HoldB.Status.Phase == v1alpha1.PhasePending && result1HoldB.RequeueAfter == 0 {
		t.Error("TC02b: rjob1 is still Pending but window-hold returned RequeueAfter=0 — expected window hold")
	}
	if result1HoldB.RequeueAfter > 0 {
		time.Sleep(result1HoldB.RequeueAfter + 100*time.Millisecond)
	}

	// Reconcile rjob2 (Pod) first: rjob1 (Deployment) is still Pending → visible as peer.
	// SameNamespaceParentRule matches; Deployment (rjob1) outranks Pod (rjob2) → rjob1 is primary.
	// rjob2 (the candidate) is NOT primary → must return RequeueAfter:5s, stay Pending.
	result2, err := rec.Reconcile(ctx, ctrlReqNS(rjob2.Name, ns))
	if err != nil {
		t.Fatalf("reconcile rjob2: %v", err)
	}
	if result2.RequeueAfter != 5*time.Second {
		t.Errorf("expected RequeueAfter=5s for non-primary rjob2, got %+v", result2)
	}

	var updated2AfterFirst v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob2), &updated2AfterFirst); err != nil {
		t.Fatalf("get updated rjob2 after first reconcile: %v", err)
	}
	if updated2AfterFirst.Status.Phase != v1alpha1.PhasePending {
		t.Errorf("rjob2 phase after first reconcile = %q, want %q (non-primary must stay Pending)", updated2AfterFirst.Status.Phase, v1alpha1.PhasePending)
	}

	// Reconcile rjob1 (Deployment): rjob2 is still Pending → visible as peer.
	// rjob1 is primary → suppressCorrelatedPeers suppresses rjob2, then rjob1 dispatches.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns)); err != nil {
		t.Fatalf("reconcile rjob1: %v", err)
	}

	var updated1 v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updated1); err != nil {
		t.Fatalf("get updated rjob1: %v", err)
	}
	if updated1.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("rjob1 phase = %q, want %q (primary must dispatch)", updated1.Status.Phase, v1alpha1.PhaseDispatched)
	}

	// rjob2 must now be Suppressed by the primary.
	var updated2 v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob2), &updated2); err != nil {
		t.Fatalf("get updated rjob2 after primary reconcile: %v", err)
	}
	if updated2.Status.Phase != v1alpha1.PhaseSuppressed {
		t.Errorf("rjob2 phase = %q, want %q (primary must have suppressed the secondary)", updated2.Status.Phase, v1alpha1.PhaseSuppressed)
	}
	if updated2.Status.CorrelationGroupID == "" {
		t.Error("rjob2: expected CorrelationGroupID to be set on suppressed job")
	}
	primaryGroupID := updated1.Labels[domain.CorrelationGroupIDLabel]
	if primaryGroupID != "" && updated2.Status.CorrelationGroupID != primaryGroupID {
		t.Errorf("TC02b: peer CorrelationGroupID = %q, want primary's group label %q", updated2.Status.CorrelationGroupID, primaryGroupID)
	}
	role2, hasRole2 := updated2.Labels[domain.CorrelationGroupRoleLabel]
	if !hasRole2 {
		t.Error("rjob2: expected correlation-role label on suppressed job")
	}
	if role2 != domain.CorrelationRoleCorrelated {
		t.Errorf("rjob2 role = %q, want %q", role2, domain.CorrelationRoleCorrelated)
	}
	if updated2.Labels[domain.CorrelationGroupIDLabel] == "" {
		t.Errorf("rjob2: expected correlation-group-id label to be non-empty on suppressed job, got labels: %v", updated2.Labels)
	}
}

// TC-07: MultiPodSameNodeRule — three pods on the same node form a group.
//
// Three RemediationJob objects for pod findings all carry the same node annotation
// (domain.NodeNameAnnotation). With threshold=3, all three meet the threshold.
// The oldest job by CreationTimestamp is selected as primary (jobs created with a
// small sleep so timestamps differ).
//
// After the window elapses, reconciling the primary causes the two non-primary
// peers to be suppressed, and the primary dispatches with the two peer findings
// as correlatedFindings.
func TestCorrelationIntegration_TC07_MultiPodSameNode(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc07-multi-pod-node")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))
	cleanupJobsInNS(t, ctx, c, ns)

	const fp1 = "1111aaaa2222bbbb3333cccc4444dddd1111aaaa2222bbbb3333cccc4444dddd"
	const fp2 = "2222bbbb3333cccc4444dddd5555eeee2222bbbb3333cccc4444dddd5555eeee"
	const fp3 = "3333cccc4444dddd5555eeee6666ffff3333cccc4444dddd5555eeee6666ffff"
	const nodeName = "node-failing-tc07"

	makeNodePodRJob := func(name, fp, podName string) *v1alpha1.RemediationJob {
		rjob := newCorrelationRJob(name, ns, fp, v1alpha1.FindingSpec{
			Kind:      "Pod",
			Name:      podName,
			Namespace: ns,
		})
		if rjob.Annotations == nil {
			rjob.Annotations = map[string]string{}
		}
		rjob.Annotations[domain.NodeNameAnnotation] = nodeName
		return rjob
	}

	rjob1 := makeNodePodRJob("tc07-rjob1", fp1, "pod-tc07-a")
	rjob2 := makeNodePodRJob("tc07-rjob2", fp2, "pod-tc07-b")
	rjob3 := makeNodePodRJob("tc07-rjob3", fp3, "pod-tc07-c")

	if err := c.Create(ctx, rjob1); err != nil {
		t.Fatalf("create rjob1: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob1) })
	if err := c.Create(ctx, rjob2); err != nil {
		t.Fatalf("create rjob2: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob2) })
	if err := c.Create(ctx, rjob3); err != nil {
		t.Fatalf("create rjob3: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(ctx, rjob3) })

	// Re-fetch all jobs to get their server-assigned UIDs and CreationTimestamps.
	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2, rjob3} {
		if err := c.Get(ctx, client.ObjectKeyFromObject(rjob), rjob); err != nil {
			t.Fatalf("re-fetch %s: %v", rjob.Name, err)
		}
	}

	djb := &dynamicFakeJobBuilder{ctx: ctx, c: c}
	// Use threshold=3: exactly three pods on the same node triggers the rule.
	rules := []domain.CorrelationRule{
		correlator.MultiPodSameNodeRule{Threshold: 3},
	}
	rec := newCorrReconcilerWith(c, ns, rules, &fakeJobBuilder{})
	rec.JobBuilder = djb

	// Initialise all three jobs: "" → Pending.
	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2, rjob3} {
		if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob.Name, ns)); err != nil {
			t.Fatalf("init Reconcile on %s: %v", rjob.Name, err)
		}
	}

	// Window-hold reconcile on rjob1: returns RequeueAfter > 0 if window not yet elapsed.
	// Use adaptive sleep (same pattern as TC-02) to avoid flakiness under CI load.
	resultHold, errHold := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns))
	if errHold != nil {
		t.Fatalf("TC07: window-hold Reconcile on rjob1: %v", errHold)
	}
	var updatedHold v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updatedHold); err != nil {
		t.Fatalf("TC07: get rjob1 after window-hold reconcile: %v", err)
	}
	if updatedHold.Status.Phase == v1alpha1.PhasePending && resultHold.RequeueAfter == 0 {
		t.Error("TC07: rjob1 is still Pending but window-hold returned RequeueAfter=0 — expected window hold")
	}
	if resultHold.RequeueAfter > 0 {
		time.Sleep(resultHold.RequeueAfter + 100*time.Millisecond)
	}

	// Determine which job has the oldest CreationTimestamp (that will be the primary).
	// In envtest, jobs created in rapid succession may share the same second-level timestamp;
	// fall back to lexicographic Name ordering as a stable tiebreaker.
	primary := rjob1
	for _, rjob := range []*v1alpha1.RemediationJob{rjob2, rjob3} {
		pt := primary.CreationTimestamp
		rt := rjob.CreationTimestamp
		if rt.Before(&pt) || (rt.Equal(&pt) && rjob.Name < primary.Name) {
			primary = rjob
		}
	}

	// Reconcile a non-primary first: rule fires, candidate is not primary → RequeueAfter:5s.
	var nonPrimary *v1alpha1.RemediationJob
	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2, rjob3} {
		if rjob.Name != primary.Name {
			nonPrimary = rjob
			break
		}
	}
	npResult, err := rec.Reconcile(ctx, ctrlReqNS(nonPrimary.Name, ns))
	if err != nil {
		t.Fatalf("reconcile non-primary %s: %v", nonPrimary.Name, err)
	}
	if npResult.RequeueAfter != 5*time.Second {
		t.Errorf("TC07: non-primary %s expected RequeueAfter=5s, got %v", nonPrimary.Name, npResult.RequeueAfter)
	}

	// Non-primary must still be Pending (not self-suppressed).
	var npUpdated v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(nonPrimary), &npUpdated); err != nil {
		t.Fatalf("get non-primary after first reconcile: %v", err)
	}
	if npUpdated.Status.Phase != v1alpha1.PhasePending {
		t.Errorf("TC07: non-primary %s phase = %q after first reconcile, want Pending", nonPrimary.Name, npUpdated.Status.Phase)
	}

	// Reconcile the primary: all three jobs are still Pending → rule fires, primary dispatches.
	if _, err := rec.Reconcile(ctx, ctrlReqNS(primary.Name, ns)); err != nil {
		t.Fatalf("reconcile primary %s: %v", primary.Name, err)
	}

	var primaryUpdated v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(primary), &primaryUpdated); err != nil {
		t.Fatalf("get primary after dispatch: %v", err)
	}
	if primaryUpdated.Status.Phase != v1alpha1.PhaseDispatched {
		t.Errorf("TC07: primary %s phase = %q, want Dispatched", primary.Name, primaryUpdated.Status.Phase)
	}
	primaryGroupID := primaryUpdated.Labels[domain.CorrelationGroupIDLabel]
	if primaryGroupID == "" {
		t.Error("TC07: primary must have non-empty correlation-group-id label")
	}
	if primaryUpdated.Labels[domain.CorrelationGroupRoleLabel] != domain.CorrelationRolePrimary {
		t.Errorf("TC07: primary role = %q, want %q",
			primaryUpdated.Labels[domain.CorrelationGroupRoleLabel], domain.CorrelationRolePrimary)
	}

	// Both non-primary jobs must be Suppressed.
	suppressedCount := 0
	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2, rjob3} {
		if rjob.Name == primary.Name {
			continue
		}
		var updated v1alpha1.RemediationJob
		if err := c.Get(ctx, client.ObjectKeyFromObject(rjob), &updated); err != nil {
			t.Fatalf("TC07: get %s after primary dispatch: %v", rjob.Name, err)
		}
		if updated.Status.Phase != v1alpha1.PhaseSuppressed {
			t.Errorf("TC07: non-primary %s phase = %q, want Suppressed", rjob.Name, updated.Status.Phase)
		}
		if updated.Status.CorrelationGroupID != primaryGroupID {
			t.Errorf("TC07: non-primary %s CorrelationGroupID = %q, want %q",
				rjob.Name, updated.Status.CorrelationGroupID, primaryGroupID)
		}
		suppressedCount++
	}
	if suppressedCount != 2 {
		t.Errorf("TC07: expected 2 suppressed peers, got %d", suppressedCount)
	}

	// Primary must have dispatched with 2 correlated findings (the two suppressed peers).
	if len(djb.lastCorrelatedFindings) != 2 {
		t.Errorf("TC07: expected 2 correlated findings on primary dispatch, got %d: %+v",
			len(djb.lastCorrelatedFindings), djb.lastCorrelatedFindings)
	}
}

// TC-07b: MultiPodSameNodeRule — below threshold (2 pods, threshold=3) → no correlation.
func TestCorrelationIntegration_TC07b_MultiPodSameNode_BelowThreshold(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	c := newIntegrationClient(t)

	ns := corrNS("tc07b-below-thresh")
	t.Cleanup(ensureNamespace(t, ctx, c, ns))
	cleanupJobsInNS(t, ctx, c, ns)

	const fp1 = "4444dddd5555eeee6666ffff7777aaaa4444dddd5555eeee6666ffff7777aaaa"
	const fp2 = "5555eeee6666ffff7777aaaa8888bbbb5555eeee6666ffff7777aaaa8888bbbb"
	const nodeName = "node-tc07b"

	makeNodePodRJob := func(name, fp, podName string) *v1alpha1.RemediationJob {
		rjob := newCorrelationRJob(name, ns, fp, v1alpha1.FindingSpec{
			Kind:      "Pod",
			Name:      podName,
			Namespace: ns,
		})
		if rjob.Annotations == nil {
			rjob.Annotations = map[string]string{}
		}
		rjob.Annotations[domain.NodeNameAnnotation] = nodeName
		return rjob
	}

	rjob1 := makeNodePodRJob("tc07b-rjob1", fp1, "pod-tc07b-a")
	rjob2 := makeNodePodRJob("tc07b-rjob2", fp2, "pod-tc07b-b")

	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2} {
		if err := c.Create(ctx, rjob); err != nil {
			t.Fatalf("create %s: %v", rjob.Name, err)
		}
		t.Cleanup(func() { _ = c.Delete(ctx, rjob) })
		if err := c.Get(ctx, client.ObjectKeyFromObject(rjob), rjob); err != nil {
			t.Fatalf("re-fetch %s: %v", rjob.Name, err)
		}
	}

	djb := &dynamicFakeJobBuilder{ctx: ctx, c: c}
	// Threshold=3 but only 2 pods → rule must NOT fire.
	rules := []domain.CorrelationRule{
		correlator.MultiPodSameNodeRule{Threshold: 3},
	}
	rec := newCorrReconcilerWith(c, ns, rules, &fakeJobBuilder{})
	rec.JobBuilder = djb

	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2} {
		if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob.Name, ns)); err != nil {
			t.Fatalf("init Reconcile on %s: %v", rjob.Name, err)
		}
	}

	// Window-hold reconcile on rjob1: adaptive sleep to avoid flakiness under CI load.
	resultHold7b, errHold7b := rec.Reconcile(ctx, ctrlReqNS(rjob1.Name, ns))
	if errHold7b != nil {
		t.Fatalf("TC07b: window-hold Reconcile on rjob1: %v", errHold7b)
	}
	var updatedHold7b v1alpha1.RemediationJob
	if err := c.Get(ctx, client.ObjectKeyFromObject(rjob1), &updatedHold7b); err != nil {
		t.Fatalf("TC07b: get rjob1 after window-hold reconcile: %v", err)
	}
	if updatedHold7b.Status.Phase == v1alpha1.PhasePending && resultHold7b.RequeueAfter == 0 {
		t.Error("TC07b: rjob1 is still Pending but window-hold returned RequeueAfter=0 — expected window hold")
	}
	if resultHold7b.RequeueAfter > 0 {
		time.Sleep(resultHold7b.RequeueAfter + 100*time.Millisecond)
	}

	// Both jobs should dispatch independently (no correlation group label).
	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2} {
		if _, err := rec.Reconcile(ctx, ctrlReqNS(rjob.Name, ns)); err != nil {
			t.Fatalf("dispatch Reconcile on %s: %v", rjob.Name, err)
		}
	}

	for _, rjob := range []*v1alpha1.RemediationJob{rjob1, rjob2} {
		var updated v1alpha1.RemediationJob
		if err := c.Get(ctx, client.ObjectKeyFromObject(rjob), &updated); err != nil {
			t.Fatalf("get %s: %v", rjob.Name, err)
		}
		if updated.Status.Phase != v1alpha1.PhaseDispatched {
			t.Errorf("TC07b: %s phase = %q, want Dispatched (below threshold should dispatch independently)",
				rjob.Name, updated.Status.Phase)
		}
		if _, ok := updated.Labels[domain.CorrelationGroupIDLabel]; ok {
			t.Errorf("TC07b: %s must have no correlation-group-id label (below threshold)", rjob.Name)
		}
	}
}
