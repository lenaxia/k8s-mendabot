package correlator

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// makeRJob is a helper to build a minimal RemediationJob for test use.
func makeRJob(name, ns, kind, resourceName, parentObject string) *v1alpha1.RemediationJob {
	return &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			UID:               types.UID(name + "-uid"),
			CreationTimestamp: metav1.Now(),
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint:        name + "-fingerprint123456",
			SourceType:         v1alpha1.SourceTypeNative,
			SinkType:           "github",
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "/",
			AgentImage:         "img:v1",
			AgentSA:            "agent-sa",
			SourceResultRef:    v1alpha1.ResultRef{Name: resourceName, Namespace: ns},
			Finding: v1alpha1.FindingSpec{
				Kind:         kind,
				Name:         resourceName,
				Namespace:    ns,
				ParentObject: parentObject,
			},
		},
	}
}

// makeRJobWithNode adds a node annotation to a RemediationJob.
func makeRJobWithNode(name, ns, kind, resourceName, parentObject, nodeName string) *v1alpha1.RemediationJob {
	rjob := makeRJob(name, ns, kind, resourceName, parentObject)
	rjob.Annotations = map[string]string{
		domain.NodeNameAnnotation: nodeName,
	}
	return rjob
}

// makeOlderRJob returns a RemediationJob with a creation timestamp older than makeRJob's.
func makeOlderRJob(name, ns, kind, resourceName, parentObject string) *v1alpha1.RemediationJob {
	rjob := makeRJob(name, ns, kind, resourceName, parentObject)
	rjob.CreationTimestamp = metav1.NewTime(time.Now().Add(-1 * time.Hour))
	return rjob
}

// ─── SameNamespaceParentRule ───────────────────────────────────────────────

func TestSameNamespaceParentRule_Match(t *testing.T) {
	// StatefulSet finding and PVC finding in same namespace with same parent prefix.
	// (Not Pod+Deployment from same provider — those would share fingerprint and be deduped.)
	sts := makeRJob("rjob-sts", "ns1", "StatefulSet", "my-app", "my-app")
	pvc := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-app-data", "my-app")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), sts, []*v1alpha1.RemediationJob{pvc}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected Matched=true for same namespace parent prefix")
	}
	if result.GroupID == "" {
		t.Error("expected non-empty GroupID on match")
	}
	// MatchedUIDs must contain both the candidate and the matched peer.
	if len(result.MatchedUIDs) != 2 {
		t.Errorf("expected MatchedUIDs to have 2 entries (candidate + peer), got %d: %v", len(result.MatchedUIDs), result.MatchedUIDs)
	}
	matchedSet := make(map[types.UID]bool)
	for _, uid := range result.MatchedUIDs {
		matchedSet[uid] = true
	}
	if !matchedSet[sts.UID] {
		t.Errorf("MatchedUIDs must contain candidate UID %s", sts.UID)
	}
	if !matchedSet[pvc.UID] {
		t.Errorf("MatchedUIDs must contain peer UID %s", pvc.UID)
	}
}

func TestSameNamespaceParentRule_DifferentNamespace_NoMatch(t *testing.T) {
	sts := makeRJob("rjob-sts", "ns1", "StatefulSet", "my-app", "my-app")
	pvc := makeRJob("rjob-pvc", "ns2", "PersistentVolumeClaim", "my-app-data", "my-app")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), sts, []*v1alpha1.RemediationJob{pvc}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false for different namespaces")
	}
}

func TestSameNamespaceParentRule_DifferentParent_NoMatch(t *testing.T) {
	sts := makeRJob("rjob-sts", "ns1", "StatefulSet", "my-app", "my-app")
	pvc := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "other-pvc", "other-app")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), sts, []*v1alpha1.RemediationJob{pvc}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false for different parents")
	}
}

func TestSameNamespaceParentRule_EmptyPeers_NoMatch(t *testing.T) {
	sts := makeRJob("rjob-sts", "ns1", "StatefulSet", "my-app", "my-app")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), sts, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false for empty peers")
	}
}

func TestSameNamespaceParentRule_PrimarySelection_OlderIsHigherHierarchy(t *testing.T) {
	// Both are StatefulSet kind: older one should be primary.
	older := makeOlderRJob("rjob-old", "ns1", "StatefulSet", "my-app", "my-app")
	newer := makeRJob("rjob-new", "ns1", "StatefulSet", "my-app-2", "my-app")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), newer, []*v1alpha1.RemediationJob{older}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected Matched=true")
	}
	if result.PrimaryUID != older.UID {
		t.Errorf("expected primary to be older rjob UID=%s, got %s", older.UID, result.PrimaryUID)
	}
}

func TestSameNamespaceParentRule_PrimarySelection_DeploymentOverPod(t *testing.T) {
	// Deployment ranks higher than Pod in hierarchy.
	deploy := makeRJob("rjob-deploy", "ns1", "Deployment", "my-app", "my-app")
	pod := makeRJob("rjob-pod", "ns1", "Pod", "my-app-abc", "my-app")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), pod, []*v1alpha1.RemediationJob{deploy}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected Matched=true")
	}
	if result.PrimaryUID != deploy.UID {
		t.Errorf("expected Deployment to be primary, got UID=%s", result.PrimaryUID)
	}
}

// ─── PVCPodRule ────────────────────────────────────────────────────────────

func buildFakeClientWithPod(pod *corev1.Pod) client.Client {
	scheme := v1alpha1.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
}

func TestPVCPodRule_CandidatePod_PeerPVC_Match(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "my-pvc",
						},
					},
				},
			},
		},
	}
	cl := buildFakeClientWithPod(pod)

	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")
	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), podRJob, []*v1alpha1.RemediationJob{pvcRJob}, cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected Matched=true when pod references PVC")
	}
	if result.PrimaryUID != pvcRJob.UID {
		t.Errorf("expected PVC to be primary, got %s", result.PrimaryUID)
	}
	// MatchedUIDs must contain both the Pod candidate and the PVC peer.
	if len(result.MatchedUIDs) != 2 {
		t.Errorf("expected MatchedUIDs to have 2 entries, got %d: %v", len(result.MatchedUIDs), result.MatchedUIDs)
	}
	matchedSet := make(map[types.UID]bool)
	for _, uid := range result.MatchedUIDs {
		matchedSet[uid] = true
	}
	if !matchedSet[podRJob.UID] {
		t.Errorf("MatchedUIDs must contain Pod UID %s", podRJob.UID)
	}
	if !matchedSet[pvcRJob.UID] {
		t.Errorf("MatchedUIDs must contain PVC UID %s", pvcRJob.UID)
	}
}

func TestPVCPodRule_CandidatePVC_PeerPod_Match(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "my-pvc",
						},
					},
				},
			},
		},
	}
	cl := buildFakeClientWithPod(pod)

	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")
	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), pvcRJob, []*v1alpha1.RemediationJob{podRJob}, cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected Matched=true (reverse orientation: candidate=PVC, peer=Pod)")
	}
	if result.PrimaryUID != pvcRJob.UID {
		t.Errorf("expected PVC to be primary, got %s", result.PrimaryUID)
	}
	// MatchedUIDs must contain both the PVC candidate and the Pod peer.
	if len(result.MatchedUIDs) != 2 {
		t.Errorf("expected MatchedUIDs to have 2 entries, got %d: %v", len(result.MatchedUIDs), result.MatchedUIDs)
	}
	matchedSet2 := make(map[types.UID]bool)
	for _, uid := range result.MatchedUIDs {
		matchedSet2[uid] = true
	}
	if !matchedSet2[pvcRJob.UID] {
		t.Errorf("MatchedUIDs must contain PVC UID %s", pvcRJob.UID)
	}
	if !matchedSet2[podRJob.UID] {
		t.Errorf("MatchedUIDs must contain Pod UID %s", podRJob.UID)
	}
}

func TestPVCPodRule_NoMatchingVolume_NoMatch(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "different-pvc",
						},
					},
				},
			},
		},
	}
	cl := buildFakeClientWithPod(pod)

	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")
	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), podRJob, []*v1alpha1.RemediationJob{pvcRJob}, cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pod does not reference the PVC")
	}
}

func TestPVCPodRule_NeitherKind_NoMatch(t *testing.T) {
	sts := makeRJob("rjob-sts", "ns1", "StatefulSet", "my-app", "my-app")
	deploy := makeRJob("rjob-deploy", "ns1", "Deployment", "my-deploy", "my-deploy")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), sts, []*v1alpha1.RemediationJob{deploy}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when neither candidate nor peers are Pod or PVC")
	}
}

func TestPVCPodRule_PodGone_NoMatch(t *testing.T) {
	// Pod not in the fake client (simulates pod gone after finding was created).
	scheme := v1alpha1.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")
	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), podRJob, []*v1alpha1.RemediationJob{pvcRJob}, cl)
	if err != nil {
		t.Fatalf("unexpected error (pod gone must be non-fatal): %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pod is gone")
	}
}

func TestPVCPodRule_DifferentNamespace_NoMatch(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "my-pvc",
						},
					},
				},
			},
		},
	}
	cl := buildFakeClientWithPod(pod)

	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")
	pvcRJob := makeRJob("rjob-pvc", "ns2", "PersistentVolumeClaim", "my-pvc", "my-pvc") // different ns

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), podRJob, []*v1alpha1.RemediationJob{pvcRJob}, cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when PVC is in a different namespace")
	}
}

// TestSameNamespaceParentRule_NoSpuriousPrefixMatch verifies that two findings whose
// parent names share a string prefix but NOT a dash-separated token boundary are NOT
// correlated. Example: "app" must not match "application".
func TestSameNamespaceParentRule_NoSpuriousPrefixMatch(t *testing.T) {
	a := makeRJob("rjob-a", "ns1", "Deployment", "app", "app")
	b := makeRJob("rjob-b", "ns1", "Deployment", "application", "application")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), a, []*v1alpha1.RemediationJob{b}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false: 'app' must not spuriously match 'application' (no dash separator)")
	}
}

// TestSameNamespaceParentRule_KnownFalsePositive_DashSiblings documents the known
// limitation of isParentPrefix: two sibling components that share a dash-separated
// naming prefix (e.g. "cert-manager" and "cert-manager-cainjector") WILL be correlated
// because "cert-manager" is a dash-separated prefix of "cert-manager-cainjector".
//
// This is an intentional trade-off: the rule is designed for cross-provider correlation
// where the parent name IS a full deployment identifier. In environments where sibling
// apps share a common prefix, this rule may produce false-positive correlations.
// Operators should disable this rule (or replace it with a stricter one) in such cases.
func TestSameNamespaceParentRule_KnownFalsePositive_DashSiblings(t *testing.T) {
	a := makeRJob("rjob-a", "ns1", "Deployment", "cert-manager", "cert-manager")
	b := makeRJob("rjob-b", "ns1", "Deployment", "cert-manager-cainjector", "cert-manager-cainjector")

	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), a, []*v1alpha1.RemediationJob{b}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// This WILL match — it is the documented false-positive. The test exists to
	// ensure future changes to isParentPrefix do not silently change this behaviour
	// without a deliberate decision.
	if !result.Matched {
		t.Error("known false-positive: 'cert-manager' should match 'cert-manager-cainjector' " +
			"(dash-separated prefix). If this fails, isParentPrefix was changed — " +
			"update this test AND the isParentPrefix comment to document the new behaviour.")
	}
}

// ─── MultiPodSameNodeRule ──────────────────────────────────────────────────

func TestMultiPodSameNodeRule_AtThreshold_Match(t *testing.T) {
	// 3 pods on node-abc => at threshold of 3 => match
	pod1 := makeRJobWithNode("rjob-pod1", "ns1", "Pod", "pod1", "pod1", "node-abc")
	pod2 := makeRJobWithNode("rjob-pod2", "ns1", "Pod", "pod2", "pod2", "node-abc")
	pod3 := makeRJobWithNode("rjob-pod3", "ns1", "Pod", "pod3", "pod3", "node-abc")

	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2, pod3}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected Matched=true when pod count >= threshold")
	}
	// MatchedUIDs must contain all 3 matched pods.
	if len(result.MatchedUIDs) != 3 {
		t.Errorf("expected MatchedUIDs to have 3 entries, got %d: %v", len(result.MatchedUIDs), result.MatchedUIDs)
	}
	matchedNodeSet := make(map[types.UID]bool)
	for _, uid := range result.MatchedUIDs {
		matchedNodeSet[uid] = true
	}
	for _, pod := range []*v1alpha1.RemediationJob{pod1, pod2, pod3} {
		if !matchedNodeSet[pod.UID] {
			t.Errorf("MatchedUIDs must contain pod UID %s", pod.UID)
		}
	}
}

func TestMultiPodSameNodeRule_BelowThreshold_NoMatch(t *testing.T) {
	// 2 pods on node-abc with threshold=3 => no match
	pod1 := makeRJobWithNode("rjob-pod1", "ns1", "Pod", "pod1", "pod1", "node-abc")
	pod2 := makeRJobWithNode("rjob-pod2", "ns1", "Pod", "pod2", "pod2", "node-abc")

	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pod count < threshold")
	}
}

func TestMultiPodSameNodeRule_ThresholdMinusOne_NoMatch(t *testing.T) {
	// Exactly threshold-1 pods.
	pod1 := makeRJobWithNode("rjob-pod1", "ns1", "Pod", "pod1", "pod1", "node-abc")
	pod2 := makeRJobWithNode("rjob-pod2", "ns1", "Pod", "pod2", "pod2", "node-abc")
	// 2 total, threshold=3 => 2 < 3 => no match
	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false at threshold-1")
	}
}

func TestMultiPodSameNodeRule_NoNodeAnnotation_NoMatch(t *testing.T) {
	// Pods with no node annotation (pending/unschedulable) should not match.
	pod1 := makeRJob("rjob-pod1", "ns1", "Pod", "pod1", "pod1")
	pod2 := makeRJob("rjob-pod2", "ns1", "Pod", "pod2", "pod2")
	pod3 := makeRJob("rjob-pod3", "ns1", "Pod", "pod3", "pod3")

	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2, pod3}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pods have no node annotation (pending/unscheduled)")
	}
}

func TestMultiPodSameNodeRule_DifferentNodes_NoMatch(t *testing.T) {
	// Pods on different nodes.
	pod1 := makeRJobWithNode("rjob-pod1", "ns1", "Pod", "pod1", "pod1", "node-abc")
	pod2 := makeRJobWithNode("rjob-pod2", "ns1", "Pod", "pod2", "pod2", "node-def")
	pod3 := makeRJobWithNode("rjob-pod3", "ns1", "Pod", "pod3", "pod3", "node-ghi")

	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2, pod3}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pods are on different nodes")
	}
}

func TestMultiPodSameNodeRule_NonPodKinds_ExcludedFromCount(t *testing.T) {
	// Non-pod findings should not be counted even if they have the node annotation.
	pod1 := makeRJobWithNode("rjob-pod1", "ns1", "Pod", "pod1", "pod1", "node-abc")
	pod2 := makeRJobWithNode("rjob-pod2", "ns1", "Pod", "pod2", "pod2", "node-abc")
	sts := makeRJobWithNode("rjob-sts", "ns1", "StatefulSet", "my-app", "my-app", "node-abc")

	// sts is not a pod, so only 2 pods counted => no match at threshold=3
	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2, sts}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when non-pod findings do not reach threshold")
	}
}

func TestSameNamespaceParentRule_Name(t *testing.T) {
	r := SameNamespaceParentRule{}
	if r.Name() == "" {
		t.Error("Name() must not be empty")
	}
}

func TestPVCPodRule_Name(t *testing.T) {
	r := PVCPodRule{}
	if r.Name() == "" {
		t.Error("Name() must not be empty")
	}
}

func TestMultiPodSameNodeRule_Name(t *testing.T) {
	r := MultiPodSameNodeRule{Threshold: 3}
	if r.Name() == "" {
		t.Error("Name() must not be empty")
	}
}

func TestSameNamespaceParentRule_EmptyParent_NoMatch(t *testing.T) {
	a := makeRJob("rjob-a", "ns1", "StatefulSet", "my-app", "")
	b := makeRJob("rjob-b", "ns1", "Deployment", "other-app", "")
	rule := SameNamespaceParentRule{}
	result, err := rule.Evaluate(context.Background(), a, []*v1alpha1.RemediationJob{b}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when ParentObject is empty")
	}
}

func TestMultiPodSameNodeRule_ZeroThreshold_ReturnsError(t *testing.T) {
	// With Threshold=0, the rule must return an error — no silent default.
	pod1 := makeRJobWithNode("rjob-pod1", "ns1", "Pod", "pod1", "pod1", "node-abc")
	pod2 := makeRJobWithNode("rjob-pod2", "ns1", "Pod", "pod2", "pod2", "node-abc")
	rule := MultiPodSameNodeRule{} // Threshold=0
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2}, nil)
	if err == nil {
		t.Fatal("expected error for Threshold=0, got nil")
	}
	if result.Matched {
		t.Error("expected Matched=false when Threshold=0 returns an error")
	}
}

func TestMultiPodSameNodeRule_PrimaryUID_IsOldestPodJob(t *testing.T) {
	// Oldest pod by CreationTimestamp must be selected as PrimaryUID.
	older := makeRJobWithNode("rjob-old", "ns1", "Pod", "pod-old", "pod-old", "node-abc")
	older.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	middle := makeRJobWithNode("rjob-mid", "ns1", "Pod", "pod-mid", "pod-mid", "node-abc")
	middle.CreationTimestamp = metav1.NewTime(time.Now().Add(-1 * time.Hour))

	newest := makeRJobWithNode("rjob-new", "ns1", "Pod", "pod-new", "pod-new", "node-abc")
	newest.CreationTimestamp = metav1.NewTime(time.Now())

	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), newest, []*v1alpha1.RemediationJob{older, middle}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected Matched=true with 3 pods at threshold")
	}
	if result.PrimaryUID == "" {
		t.Fatal("expected non-empty PrimaryUID on match")
	}
	if result.PrimaryUID != older.UID {
		t.Errorf("expected PrimaryUID=%s (oldest pod), got %s", older.UID, result.PrimaryUID)
	}
}

func TestPVCPodRule_CandidatePVC_PeerPod_NoVolumeMatch(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "different-pvc",
						},
					},
				},
			},
		},
	}
	cl := buildFakeClientWithPod(pod)

	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")
	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), pvcRJob, []*v1alpha1.RemediationJob{podRJob}, cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pod does not mount the candidate PVC (reverse orientation)")
	}
}

// TestMultiPodSameNodeRule_TwoNodesBothAtThreshold_DeterministicWinner verifies that when
// two nodes each have >= threshold pods, the rule always selects the same (lexicographically
// first) node as the winner regardless of map iteration order.
func TestMultiPodSameNodeRule_TwoNodesBothAtThreshold_DeterministicWinner(t *testing.T) {
	// 3 pods on node-aaa, 3 pods on node-zzz, threshold=3.
	// node-aaa is lexicographically first so all matched UIDs must be from node-aaa.
	aaa1 := makeRJobWithNode("rjob-aaa1", "ns1", "Pod", "pod-aaa1", "pod-aaa1", "node-aaa")
	aaa2 := makeRJobWithNode("rjob-aaa2", "ns1", "Pod", "pod-aaa2", "pod-aaa2", "node-aaa")
	aaa3 := makeRJobWithNode("rjob-aaa3", "ns1", "Pod", "pod-aaa3", "pod-aaa3", "node-aaa")
	zzz1 := makeRJobWithNode("rjob-zzz1", "ns1", "Pod", "pod-zzz1", "pod-zzz1", "node-zzz")
	zzz2 := makeRJobWithNode("rjob-zzz2", "ns1", "Pod", "pod-zzz2", "pod-zzz2", "node-zzz")
	zzz3 := makeRJobWithNode("rjob-zzz3", "ns1", "Pod", "pod-zzz3", "pod-zzz3", "node-zzz")

	nodeAAAUIDs := map[types.UID]bool{
		aaa1.UID: true,
		aaa2.UID: true,
		aaa3.UID: true,
	}

	rule := MultiPodSameNodeRule{Threshold: 3}
	peers := []*v1alpha1.RemediationJob{aaa2, aaa3, zzz1, zzz2, zzz3}

	for i := 0; i < 5; i++ {
		t.Run("run", func(t *testing.T) {
			result, err := rule.Evaluate(context.Background(), aaa1, peers, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Matched {
				t.Fatal("expected Matched=true")
			}
			// Must always select node-aaa (lexicographically first).
			if len(result.MatchedUIDs) != 3 {
				t.Errorf("expected 3 MatchedUIDs (all from node-aaa), got %d: %v", len(result.MatchedUIDs), result.MatchedUIDs)
			}
			for _, uid := range result.MatchedUIDs {
				if !nodeAAAUIDs[uid] {
					t.Errorf("MatchedUIDs contains UID %s which is not from node-aaa", uid)
				}
			}
		})
	}
}

// TestMultiPodSameNodeRule_MultipleNodes_OnlyThresholdNodeMatches verifies that when
// 3 pods are on node-abc (meets threshold=3) and 2 pods are on node-def (below threshold),
// only node-abc's pods appear in the match and MatchedUIDs.
func TestMultiPodSameNodeRule_MultipleNodes_OnlyThresholdNodeMatches(t *testing.T) {
	pod1 := makeRJobWithNode("rjob-pod1", "ns1", "Pod", "pod1", "pod1", "node-abc")
	pod2 := makeRJobWithNode("rjob-pod2", "ns1", "Pod", "pod2", "pod2", "node-abc")
	pod3 := makeRJobWithNode("rjob-pod3", "ns1", "Pod", "pod3", "pod3", "node-abc")
	pod4 := makeRJobWithNode("rjob-pod4", "ns1", "Pod", "pod4", "pod4", "node-def")
	pod5 := makeRJobWithNode("rjob-pod5", "ns1", "Pod", "pod5", "pod5", "node-def")

	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2, pod3, pod4, pod5}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected Matched=true: node-abc has 3 pods at threshold=3")
	}

	// PrimaryUID must be one of the node-abc pods.
	nodeABCUIDs := map[types.UID]bool{
		pod1.UID: true,
		pod2.UID: true,
		pod3.UID: true,
	}
	if !nodeABCUIDs[result.PrimaryUID] {
		t.Errorf("expected PrimaryUID to be one of node-abc pods, got %s", result.PrimaryUID)
	}

	// MatchedUIDs must contain exactly the 3 node-abc pods.
	if len(result.MatchedUIDs) != 3 {
		t.Errorf("expected MatchedUIDs to have exactly 3 entries, got %d: %v", len(result.MatchedUIDs), result.MatchedUIDs)
	}
	for _, uid := range result.MatchedUIDs {
		if !nodeABCUIDs[uid] {
			t.Errorf("MatchedUIDs must not contain node-def pod UID %s", uid)
		}
	}
	// node-def pods must NOT be in MatchedUIDs.
	nodeDefUIDs := map[types.UID]bool{
		pod4.UID: true,
		pod5.UID: true,
	}
	for _, uid := range result.MatchedUIDs {
		if nodeDefUIDs[uid] {
			t.Errorf("MatchedUIDs must not contain node-def pod UID %s", uid)
		}
	}
}

// TestMultiPodSameNodeRule_PrimaryUID_Tiebreaker_LexicographicName verifies that when
// two pod RemediationJobs share the same CreationTimestamp, the one with the
// lexicographically smallest Name is selected as primary.
func TestMultiPodSameNodeRule_PrimaryUID_Tiebreaker_LexicographicName(t *testing.T) {
	ts := metav1.NewTime(time.Now().Add(-1 * time.Hour))

	// "rjob-aaa" is lexicographically before "rjob-zzz" and both have the same timestamp.
	aaa := makeRJobWithNode("rjob-aaa", "ns1", "Pod", "pod-aaa", "pod-aaa", "node-abc")
	aaa.CreationTimestamp = ts

	zzz := makeRJobWithNode("rjob-zzz", "ns1", "Pod", "pod-zzz", "pod-zzz", "node-abc")
	zzz.CreationTimestamp = ts

	mid := makeRJobWithNode("rjob-mmm", "ns1", "Pod", "pod-mmm", "pod-mmm", "node-abc")
	mid.CreationTimestamp = ts

	rule := MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), zzz, []*v1alpha1.RemediationJob{aaa, mid}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected Matched=true with 3 pods at threshold")
	}
	if result.PrimaryUID != aaa.UID {
		t.Errorf("expected PrimaryUID=%s (lexicographically smallest name 'rjob-aaa'), got %s", aaa.UID, result.PrimaryUID)
	}
}

// TestPVCPodRule_CandidatePod_ClientGetGenericError_PropagatesError verifies that
// when candidate=Pod and client.Get returns a generic (non-NotFound) error, the
// rule propagates the error so the controller can requeue. Only NotFound is
// treated as a non-fatal miss (pod gone).
func TestPVCPodRule_CandidatePod_ClientGetGenericError_PropagatesError(t *testing.T) {
	scheme := v1alpha1.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	genericErr := errors.New("internal server error: etcd timeout")
	cl := interceptor.NewClient(baseClient, interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return genericErr
		},
	})

	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")
	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), podRJob, []*v1alpha1.RemediationJob{pvcRJob}, cl)
	if err == nil {
		t.Fatal("expected error to be propagated for generic client.Get failure, got nil")
	}
	if result.Matched {
		t.Error("expected Matched=false when client.Get returns a generic error")
	}
}

// TestPVCPodRule_CandidatePod_ClientGetNotFound_NoError verifies that when
// candidate=Pod and client.Get returns NotFound, the rule treats it as a
// non-fatal miss (pod gone) and returns Matched=false, nil.
func TestPVCPodRule_CandidatePod_ClientGetNotFound_NoError(t *testing.T) {
	scheme := v1alpha1.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	// Build a client with no Pod objects — Get will return NotFound.
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")
	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), podRJob, []*v1alpha1.RemediationJob{pvcRJob}, baseClient)
	if err != nil {
		t.Fatalf("expected nil error for NotFound (pod gone), got: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pod is not found")
	}
}

// TestPVCPodRule_CandidatePVC_PeerPod_ClientGetGenericError_PropagatesError verifies
// that when candidate=PVC and client.Get returns a generic (non-NotFound) error for
// a pod peer, the rule propagates the error so the controller can requeue.
func TestPVCPodRule_CandidatePVC_PeerPod_ClientGetGenericError_PropagatesError(t *testing.T) {
	scheme := v1alpha1.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	genericErr := errors.New("internal server error: etcd timeout")
	cl := interceptor.NewClient(baseClient, interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return genericErr
		},
	})

	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")
	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), pvcRJob, []*v1alpha1.RemediationJob{podRJob}, cl)
	if err == nil {
		t.Fatal("expected error to be propagated for generic client.Get failure on pod peer, got nil")
	}
	if result.Matched {
		t.Error("expected Matched=false when client.Get returns a generic error")
	}
}

// TestPVCPodRule_CandidatePVC_PeerPod_ClientGetNotFound_NoError verifies that when
// candidate=PVC and client.Get returns NotFound for a pod peer, the peer is skipped
// (non-fatal miss) and the rule returns Matched=false, nil.
func TestPVCPodRule_CandidatePVC_PeerPod_ClientGetNotFound_NoError(t *testing.T) {
	scheme := v1alpha1.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	// No Pod objects in the fake client → Get returns NotFound.
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	pvcRJob := makeRJob("rjob-pvc", "ns1", "PersistentVolumeClaim", "my-pvc", "my-pvc")
	podRJob := makeRJob("rjob-pod", "ns1", "Pod", "my-pod", "my-pod")

	rule := PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), pvcRJob, []*v1alpha1.RemediationJob{podRJob}, baseClient)
	if err != nil {
		t.Fatalf("expected nil error for NotFound pod peer, got: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pod peer is not found")
	}
}
