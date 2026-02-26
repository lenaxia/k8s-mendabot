package correlator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// kindHierarchy ranks Kubernetes kinds by ownership level.
// Higher value = higher in the hierarchy = preferred as primary.
var kindHierarchy = map[string]int{
	"Deployment":  10,
	"StatefulSet": 9,
	"DaemonSet":   8,
	"Job":         7,
	"ReplicaSet":  6,
	"Pod":         1,
}

// kindRank returns the hierarchy rank for a kind (0 for unknown kinds).
func kindRank(kind string) int {
	return kindHierarchy[kind]
}

// selectPrimary picks the primary RemediationJob from the candidate and a set of matched peers.
// Rules:
//  1. Higher kind hierarchy rank wins (Deployment > StatefulSet > Pod > others).
//  2. On a tie, the oldest CreationTimestamp wins.
//  3. On a further tie (equal timestamps), the lexicographically smallest Name wins.
func selectPrimary(candidate *v1alpha1.RemediationJob, matched []*v1alpha1.RemediationJob) *v1alpha1.RemediationJob {
	all := make([]*v1alpha1.RemediationJob, 0, len(matched)+1)
	all = append(all, candidate)
	all = append(all, matched...)

	primary := all[0]
	for _, p := range all[1:] {
		pr := kindRank(p.Spec.Finding.Kind)
		cr := kindRank(primary.Spec.Finding.Kind)
		if pr > cr {
			primary = p
			continue
		}
		if pr == cr && p.CreationTimestamp.Before(&primary.CreationTimestamp) {
			primary = p
			continue
		}
		if pr == cr && p.CreationTimestamp.Equal(&primary.CreationTimestamp) && p.Name < primary.Name {
			primary = p
		}
	}
	return primary
}

// ─── SameNamespaceParentRule ──────────────────────────────────────────────

// isParentPrefix reports whether b is a parent-token prefix of a.
// It returns true when a == b (exact match) or when a starts with b followed by "-"
// (b is a proper dash-separated prefix of a, e.g. "my-app" is a parent of "my-app-7d9f-xyz").
// Using strings.HasPrefix(a, b) alone over-matches: "app" would match "application".
// Note: this still matches when two sibling apps share a dash-separated naming prefix
// (e.g. "cert-manager" matches "cert-manager-cainjector"). That is a known limitation —
// this rule is designed for cross-provider scenarios and is most reliable when parent names
// are full deployment identifiers, not partial shared prefixes.
func isParentPrefix(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"-")
}

// SameNamespaceParentRule matches RemediationJob objects that share a namespace
// and whose ParentObject names have a prefix relationship. This rule is designed for
// cross-provider correlation: same-provider findings for the same parent are
// fingerprint-deduplicated by SourceProviderReconciler before reaching the correlator,
// so this rule fires primarily in multi-provider deployments.
//
// It does NOT match Pod + Deployment from the same native provider — those share a
// fingerprint and are deduplicated by SourceProviderReconciler before reaching the correlator.
type SameNamespaceParentRule struct{}

func (r SameNamespaceParentRule) Name() string { return "SameNamespaceParent" }

func (r SameNamespaceParentRule) Evaluate(
	ctx context.Context,
	candidate *v1alpha1.RemediationJob,
	peers []*v1alpha1.RemediationJob,
	_ client.Client,
) (domain.CorrelationResult, error) {
	cNS := candidate.Spec.Finding.Namespace
	cParent := candidate.Spec.Finding.ParentObject

	var matched []*v1alpha1.RemediationJob
	for _, p := range peers {
		if p.UID == candidate.UID {
			continue
		}
		if p.Spec.Finding.Namespace != cNS {
			continue
		}
		pParent := p.Spec.Finding.ParentObject
		if cParent == "" || pParent == "" {
			continue
		}
		if isParentPrefix(cParent, pParent) || isParentPrefix(pParent, cParent) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		return domain.CorrelationResult{}, nil
	}
	primary := selectPrimary(candidate, matched)
	matchedUIDs := make([]types.UID, 0, len(matched)+1)
	matchedUIDs = append(matchedUIDs, candidate.UID)
	for _, p := range matched {
		matchedUIDs = append(matchedUIDs, p.UID)
	}
	return domain.CorrelationResult{
		Matched:     true,
		GroupID:     domain.NewCorrelationGroupID(),
		PrimaryUID:  primary.UID,
		Reason:      "same-namespace-parent-prefix",
		MatchedUIDs: matchedUIDs,
	}, nil
}

// ─── PVCPodRule ────────────────────────────────────────────────────────────

// PVCPodRule matches a PVC finding and a Pod finding in the same namespace when
// the pod's volume list references the PVC. The PVC is always the primary — it
// is the root cause.
//
// This is the only rule that requires a live API call (client.Get on the Pod) to
// inspect spec.volumes. If the pod is gone, the rule returns Matched=false, nil.
type PVCPodRule struct{}

func (r PVCPodRule) Name() string { return "PVCPod" }

func (r PVCPodRule) Evaluate(
	ctx context.Context,
	candidate *v1alpha1.RemediationJob,
	peers []*v1alpha1.RemediationJob,
	c client.Client,
) (domain.CorrelationResult, error) {
	candidateKind := candidate.Spec.Finding.Kind

	switch candidateKind {
	case "Pod":
		return r.evaluatePodCandidate(ctx, candidate, peers, c)
	case "PersistentVolumeClaim":
		return r.evaluatePVCCandidate(ctx, candidate, peers, c)
	default:
		return domain.CorrelationResult{}, nil
	}
}

// evaluatePodCandidate handles candidate=Pod, looking for PVC peers.
// All PVC peers whose claim names appear in the pod's volume list are accumulated
// into a single group — a pod may mount multiple PVCs, each with its own finding.
func (r PVCPodRule) evaluatePodCandidate(
	ctx context.Context,
	candidate *v1alpha1.RemediationJob,
	peers []*v1alpha1.RemediationJob,
	c client.Client,
) (domain.CorrelationResult, error) {
	ns := candidate.Spec.Finding.Namespace
	podName := candidate.Spec.Finding.Name

	var pvcPeers []*v1alpha1.RemediationJob
	for _, p := range peers {
		if p.Spec.Finding.Kind == "PersistentVolumeClaim" && p.Spec.Finding.Namespace == ns {
			pvcPeers = append(pvcPeers, p)
		}
	}
	if len(pvcPeers) == 0 {
		return domain.CorrelationResult{}, nil
	}

	var pod corev1.Pod
	if err := c.Get(ctx, types.NamespacedName{Name: podName, Namespace: ns}, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Pod has been deleted — no match for this candidate.
			return domain.CorrelationResult{}, nil
		}
		// Transient error (API server unavailable, RBAC, etc.) — propagate so
		// the controller can requeue rather than silently skipping correlation.
		return domain.CorrelationResult{}, err
	}

	claimNames := podClaimNames(&pod)

	// Accumulate ALL PVC peers whose claim is mounted by this pod.
	// Do NOT early-return on first match: a pod may mount multiple PVCs,
	// each with its own finding that must be suppressed as a group.
	var matchedPVCs []*v1alpha1.RemediationJob
	for _, pvcPeer := range pvcPeers {
		if _, ok := claimNames[pvcPeer.Spec.Finding.Name]; ok {
			matchedPVCs = append(matchedPVCs, pvcPeer)
		}
	}
	if len(matchedPVCs) == 0 {
		return domain.CorrelationResult{}, nil
	}

	// Use the oldest matched PVC as primary (PVCs are root cause over Pod).
	// selectPrimary would pick Pod over PVC due to kindHierarchy ranking, so we
	// explicitly designate the oldest PVC as primary here.
	// Sort matchedPVCs by (CreationTimestamp, Name) for deterministic selection
	// regardless of API server list ordering.
	sort.Slice(matchedPVCs, func(i, j int) bool {
		ti := matchedPVCs[i].CreationTimestamp
		tj := matchedPVCs[j].CreationTimestamp
		if ti.Equal(&tj) {
			return matchedPVCs[i].Name < matchedPVCs[j].Name
		}
		return ti.Before(&tj)
	})
	primary := matchedPVCs[0]

	matchedUIDs := make([]types.UID, 0, len(matchedPVCs)+1)
	matchedUIDs = append(matchedUIDs, candidate.UID)
	for _, pvc := range matchedPVCs {
		matchedUIDs = append(matchedUIDs, pvc.UID)
	}
	return domain.CorrelationResult{
		Matched:     true,
		GroupID:     domain.NewCorrelationGroupID(),
		PrimaryUID:  primary.UID,
		Reason:      "pvc-pod-volume-reference",
		MatchedUIDs: matchedUIDs,
	}, nil
}

// evaluatePVCCandidate handles candidate=PVC, looking for Pod peers.
// All pods that mount this PVC are accumulated into a single group —
// multiple pods may reference the same PVC, each with its own finding.
func (r PVCPodRule) evaluatePVCCandidate(
	ctx context.Context,
	candidate *v1alpha1.RemediationJob,
	peers []*v1alpha1.RemediationJob,
	c client.Client,
) (domain.CorrelationResult, error) {
	ns := candidate.Spec.Finding.Namespace
	pvcName := candidate.Spec.Finding.Name

	// Accumulate ALL pod peers that mount this PVC.
	// Do NOT early-return on first match: multiple pods may mount the same PVC,
	// each with its own finding that must be suppressed in the same group.
	var matchedPods []*v1alpha1.RemediationJob
	for _, p := range peers {
		if p.Spec.Finding.Kind != "Pod" || p.Spec.Finding.Namespace != ns {
			continue
		}

		var pod corev1.Pod
		if err := c.Get(ctx, types.NamespacedName{Name: p.Spec.Finding.Name, Namespace: ns}, &pod); err != nil {
			if apierrors.IsNotFound(err) {
				// Pod has been deleted — non-fatal, skip this peer.
				continue
			}
			// Transient error — propagate so the controller can requeue.
			return domain.CorrelationResult{}, err
		}

		claimNames := podClaimNames(&pod)
		if _, ok := claimNames[pvcName]; ok {
			matchedPods = append(matchedPods, p)
		}
	}
	if len(matchedPods) == 0 {
		return domain.CorrelationResult{}, nil
	}

	matchedUIDs := make([]types.UID, 0, len(matchedPods)+1)
	matchedUIDs = append(matchedUIDs, candidate.UID)
	for _, pod := range matchedPods {
		matchedUIDs = append(matchedUIDs, pod.UID)
	}
	return domain.CorrelationResult{
		Matched:     true,
		GroupID:     domain.NewCorrelationGroupID(),
		PrimaryUID:  candidate.UID,
		Reason:      "pvc-pod-volume-reference",
		MatchedUIDs: matchedUIDs,
	}, nil
}

// podClaimNames returns a set of PVC claim names referenced by a pod's volumes.
func podClaimNames(pod *corev1.Pod) map[string]struct{} {
	names := make(map[string]struct{})
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			names[v.PersistentVolumeClaim.ClaimName] = struct{}{}
		}
	}
	return names
}

// ─── MultiPodSameNodeRule ──────────────────────────────────────────────────

// MultiPodSameNodeRule matches when >= Threshold pod findings all ran on the same
// node. The node name is read from the domain.NodeNameAnnotation annotation on the
// RemediationJob (written by SourceProviderReconciler from finding.NodeName).
//
// Known limitation — single group per first qualifying node: when multiple nodes
// each independently meet the threshold, this rule produces a correlation group only
// for the lexicographically first qualifying node. Pod findings on the remaining
// qualifying nodes are NOT correlated by this rule; they will be evaluated by
// subsequent rules (SameNamespaceParentRule, PVCPodRule) and, if no rule matches,
// will dispatch independently. This is an intentional design trade-off to keep the
// rule's output a single deterministic CorrelationResult per evaluation. Future work
// could extend the rule to emit one group per qualifying node by restructuring
// CorrelationRule to return []CorrelationResult.
//
// Known limitation — scheduled pods only: pods in Pending/Unschedulable state have
// no spec.nodeName and therefore no annotation; they are excluded from the count.
// This rule only fires for pods that were scheduled to a node and then started crashing.
type MultiPodSameNodeRule struct {
	Threshold int
}

func (r MultiPodSameNodeRule) Name() string { return "MultiPodSameNode" }

func (r MultiPodSameNodeRule) Evaluate(
	ctx context.Context,
	candidate *v1alpha1.RemediationJob,
	peers []*v1alpha1.RemediationJob,
	_ client.Client,
) (domain.CorrelationResult, error) {
	// Guard first — before any work — so a misconfigured threshold fails fast.
	// buildCorrelator() in cmd/watcher/main.go validates this at startup; a zero
	// value here indicates a test or wiring bug.
	if r.Threshold <= 0 {
		return domain.CorrelationResult{}, fmt.Errorf("MultiPodSameNodeRule: Threshold must be > 0, got %d", r.Threshold)
	}

	all := make([]*v1alpha1.RemediationJob, 0, len(peers)+1)
	all = append(all, candidate)
	all = append(all, peers...)

	// Count pod findings by node.
	nodeCount := make(map[string]int)
	for _, rjob := range all {
		if rjob.Spec.Finding.Kind != "Pod" {
			continue
		}
		nodeName := rjob.Annotations[domain.NodeNameAnnotation]
		if nodeName == "" {
			continue
		}
		nodeCount[nodeName]++
	}

	// Collect qualifying node names and sort them so the winner is always deterministic.
	var qualifyingNodes []string
	for nodeName, count := range nodeCount {
		if count >= r.Threshold {
			qualifyingNodes = append(qualifyingNodes, nodeName)
		}
	}
	if len(qualifyingNodes) == 0 {
		return domain.CorrelationResult{}, nil
	}
	sort.Strings(qualifyingNodes)
	nodeName := qualifyingNodes[0]

	// Collect all pod jobs on the winning node and select the oldest as primary.
	var nodePods []*v1alpha1.RemediationJob
	for _, rjob := range all {
		if rjob.Spec.Finding.Kind != "Pod" {
			continue
		}
		if rjob.Annotations[domain.NodeNameAnnotation] != nodeName {
			continue
		}
		nodePods = append(nodePods, rjob)
	}
	primary := nodePods[0]
	for _, p := range nodePods[1:] {
		pTime := p.CreationTimestamp
		cTime := primary.CreationTimestamp
		if pTime.Before(&cTime) {
			primary = p
			continue
		}
		if cTime.Equal(&pTime) && p.Name < primary.Name {
			primary = p
		}
	}
	matchedUIDs := make([]types.UID, 0, len(nodePods))
	for _, p := range nodePods {
		matchedUIDs = append(matchedUIDs, p.UID)
	}
	return domain.CorrelationResult{
		Matched:     true,
		GroupID:     domain.NewCorrelationGroupID(),
		PrimaryUID:  primary.UID,
		Reason:      "multi-pod-same-node",
		MatchedUIDs: matchedUIDs,
	}, nil
}
