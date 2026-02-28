package correlator_test

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/correlator"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

// fakeRule is a test double for domain.CorrelationRule.
type fakeRule struct {
	name   string
	result domain.CorrelationResult
	err    error
	called bool
}

func (r *fakeRule) Name() string { return r.name }

func (r *fakeRule) Evaluate(
	_ context.Context,
	_ *v1alpha1.RemediationJob,
	_ []*v1alpha1.RemediationJob,
	_ client.Client,
) (domain.CorrelationResult, error) {
	r.called = true
	return r.result, r.err
}

func newRJob(name string, uid types.UID) *v1alpha1.RemediationJob {
	return &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       uid,
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "abc123",
			Finding: v1alpha1.FindingSpec{
				Kind:      "Pod",
				Name:      name + "-pod",
				Namespace: "default",
			},
		},
	}
}

// TestCorrelator_NoRules_NoMatch verifies that a Correlator with no rules returns
// an empty CorrelationGroup, false, nil.
func TestCorrelator_NoRules_NoMatch(t *testing.T) {
	c := &correlator.Correlator{
		Rules: nil,
	}
	candidate := newRJob("candidate", "uid-candidate")
	group, found, err := c.Evaluate(context.Background(), candidate, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for empty rule set, got true")
	}
	if group.GroupID != "" {
		t.Errorf("expected empty GroupID, got %q", group.GroupID)
	}
}

// TestCorrelator_FirstRuleMatches verifies that when the first rule matches, the correlator
// returns the match immediately without calling further rules.
func TestCorrelator_FirstRuleMatches(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")
	peer := newRJob("peer", "uid-peer")

	matchResult := domain.CorrelationResult{
		Matched:    true,
		GroupID:    "group-abc",
		PrimaryUID: "uid-candidate",
		Reason:     "test-rule",
	}

	rule1 := &fakeRule{name: "rule1", result: matchResult}
	rule2 := &fakeRule{name: "rule2"}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule1, rule2},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected found=true, got false")
	}
	if group.GroupID == "" {
		t.Error("expected non-empty GroupID")
	}
	if rule2.called {
		t.Error("expected rule2 NOT to be called when rule1 already matched")
	}
	if !rule1.called {
		t.Error("expected rule1 to be called")
	}
}

// TestCorrelator_FirstRuleNoMatch_SecondRuleMatches verifies the correlator falls through
// to the second rule when the first does not match.
func TestCorrelator_FirstRuleNoMatch_SecondRuleMatches(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")
	peer := newRJob("peer", "uid-peer")

	noMatchResult := domain.CorrelationResult{Matched: false}
	matchResult := domain.CorrelationResult{
		Matched:    true,
		GroupID:    "group-xyz",
		PrimaryUID: "uid-candidate",
		Reason:     "rule2-match",
	}

	rule1 := &fakeRule{name: "rule1", result: noMatchResult}
	rule2 := &fakeRule{name: "rule2", result: matchResult}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule1, rule2},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected found=true after second rule matched, got false")
	}
	if group.GroupID == "" {
		t.Error("expected non-empty GroupID")
	}
	if !rule1.called {
		t.Error("expected rule1 to be called")
	}
	if !rule2.called {
		t.Error("expected rule2 to be called")
	}
}

// TestCorrelator_RuleError_PropagatesError verifies that a rule error is wrapped and
// returned immediately without calling further rules.
func TestCorrelator_RuleError_PropagatesError(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")

	ruleErr := errors.New("api unreachable")
	rule1 := &fakeRule{name: "rule1", err: ruleErr}
	rule2 := &fakeRule{name: "rule2"}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule1, rule2},
	}

	_, found, err := c.Evaluate(context.Background(), candidate, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ruleErr) {
		t.Errorf("error chain does not contain original error: %v", err)
	}
	if found {
		t.Error("expected found=false on error, got true")
	}
	if rule2.called {
		t.Error("expected rule2 NOT to be called after rule1 errored")
	}
}

// TestCorrelator_AllFindings_PopulatedOnMatch verifies that AllFindings in the returned
// CorrelationGroup contains the matched peer's finding but NOT the primary's own finding
// (which is the candidate in this test).
func TestCorrelator_AllFindings_PopulatedOnMatch(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")
	candidate.Spec.Finding.Name = "candidate-pod"

	peer := newRJob("peer", "uid-peer")
	peer.Spec.Finding.Name = "peer-pod"

	// candidate is the primary — its finding must NOT appear in AllFindings.
	matchResult := domain.CorrelationResult{
		Matched:    true,
		GroupID:    "group-123",
		PrimaryUID: "uid-candidate",
		Reason:     "test",
	}
	rule := &fakeRule{name: "rule", result: matchResult}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	// AllFindings must contain the peer's finding.
	peerFound := false
	for _, f := range group.AllFindings {
		if f.Name == "peer-pod" {
			peerFound = true
		}
	}
	if !peerFound {
		t.Error("expected peer's finding (peer-pod) in AllFindings")
	}
	// AllFindings must NOT contain the primary's (candidate's) own finding.
	for _, f := range group.AllFindings {
		if f.Name == "candidate-pod" {
			t.Errorf("primary's own finding (candidate-pod) must NOT be in AllFindings, got: %+v", group.AllFindings)
		}
	}
}

// TestCorrelator_PrimaryUID_FromRule verifies the PrimaryUID in CorrelationGroup
// reflects the PrimaryUID returned by the matching rule.
func TestCorrelator_PrimaryUID_FromRule(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")
	peer := newRJob("peer", "uid-peer")

	matchResult := domain.CorrelationResult{
		Matched:    true,
		GroupID:    "grp-1",
		PrimaryUID: "uid-peer",
		Reason:     "peer-is-primary",
	}
	rule := &fakeRule{name: "rule", result: matchResult}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if group.PrimaryUID != "uid-peer" {
		t.Errorf("PrimaryUID = %q, want %q", group.PrimaryUID, "uid-peer")
	}
}

// TestCorrelator_MultiPodSameNodeRule_AllFindingsOnlyMatchedNode verifies that AllFindings
// in the returned CorrelationGroup contains exactly the findings of pods on the matching
// node (those in MatchedUIDs), not peers from other nodes that were below threshold.
func TestCorrelator_MultiPodSameNodeRule_AllFindingsOnlyMatchedNode(t *testing.T) {
	// 3 pods on node-abc (meets threshold=3) + 2 pods on node-def (below threshold).
	// After correlation, AllFindings must contain exactly 3 findings (the node-abc pods).
	pod1 := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod1",
			Namespace:   "default",
			UID:         "uid-pod1",
			Annotations: map[string]string{domain.NodeNameAnnotation: "node-abc"},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-pod1",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod1", Namespace: "default"},
		},
	}
	pod2 := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod2",
			Namespace:   "default",
			UID:         "uid-pod2",
			Annotations: map[string]string{domain.NodeNameAnnotation: "node-abc"},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-pod2",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod2", Namespace: "default"},
		},
	}
	pod3 := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod3",
			Namespace:   "default",
			UID:         "uid-pod3",
			Annotations: map[string]string{domain.NodeNameAnnotation: "node-abc"},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-pod3",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod3", Namespace: "default"},
		},
	}
	pod4 := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod4",
			Namespace:   "default",
			UID:         "uid-pod4",
			Annotations: map[string]string{domain.NodeNameAnnotation: "node-def"},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-pod4",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod4", Namespace: "default"},
		},
	}
	pod5 := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod5",
			Namespace:   "default",
			UID:         "uid-pod5",
			Annotations: map[string]string{domain.NodeNameAnnotation: "node-def"},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-pod5",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod5", Namespace: "default"},
		},
	}

	rule := correlator.MultiPodSameNodeRule{Threshold: 3}
	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule},
	}

	group, found, err := c.Evaluate(
		context.Background(),
		pod1,
		[]*v1alpha1.RemediationJob{pod2, pod3, pod4, pod5},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true: node-abc has 3 pods at threshold=3")
	}
	if len(group.AllFindings) != 2 {
		t.Errorf("expected AllFindings to have exactly 2 entries (non-primary node-abc pods only), got %d: %+v",
			len(group.AllFindings), group.AllFindings)
	}
	for _, f := range group.AllFindings {
		if f.Name == "pod4" || f.Name == "pod5" {
			t.Errorf("AllFindings must not include node-def pods, but found %q", f.Name)
		}
		if f.Name == "pod1" {
			t.Errorf("AllFindings must not include the primary's own finding (pod1), but found %q", f.Name)
		}
	}
}

// TestCorrelator_CorrelatedUIDs_PopulatedOnMatch verifies that CorrelatedUIDs in the
// returned CorrelationGroup contains the UIDs of all non-primary jobs in the group.
func TestCorrelator_CorrelatedUIDs_PopulatedOnMatch(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")
	peer := newRJob("peer", "uid-peer")

	matchResult := domain.CorrelationResult{
		Matched:     true,
		GroupID:     "group-999",
		PrimaryUID:  "uid-candidate",
		Reason:      "test",
		MatchedUIDs: []types.UID{"uid-candidate", "uid-peer"},
	}
	rule := &fakeRule{name: "rule", result: matchResult}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	// CorrelatedUIDs must contain only the non-primary UIDs.
	if len(group.CorrelatedUIDs) != 1 {
		t.Fatalf("expected CorrelatedUIDs to have 1 entry (peer only), got %d: %v", len(group.CorrelatedUIDs), group.CorrelatedUIDs)
	}
	if group.CorrelatedUIDs[0] != "uid-peer" {
		t.Errorf("CorrelatedUIDs[0] = %q, want %q", group.CorrelatedUIDs[0], "uid-peer")
	}
}

// TestCorrelator_CorrelatedUIDs_PopulatedInFallbackPath verifies that CorrelatedUIDs
// is populated even when the rule returns no MatchedUIDs (fallback path), so that
// suppressCorrelatedPeers can suppress all non-primary peers regardless of rule type.
func TestCorrelator_CorrelatedUIDs_PopulatedInFallbackPath(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")
	peer := newRJob("peer", "uid-peer")

	// Rule returns no MatchedUIDs — backward-compat fallback path.
	matchResult := domain.CorrelationResult{
		Matched:    true,
		GroupID:    "group-abc",
		PrimaryUID: "uid-candidate",
		Reason:     "test",
		// MatchedUIDs is nil — fallback path
	}
	rule := &fakeRule{name: "rule", result: matchResult}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	// CorrelatedUIDs should be populated in the fallback path so suppressCorrelatedPeers works.
	if len(group.CorrelatedUIDs) == 0 {
		t.Error("expected CorrelatedUIDs to be non-empty in fallback path so peers can be suppressed")
	}
	// Peer's UID must be in CorrelatedUIDs; primary's UID must not be.
	foundPeer := false
	for _, uid := range group.CorrelatedUIDs {
		if uid == "uid-candidate" {
			t.Errorf("primary UID must not appear in CorrelatedUIDs, got %v", group.CorrelatedUIDs)
		}
		if uid == "uid-peer" {
			foundPeer = true
		}
	}
	if !foundPeer {
		t.Errorf("peer UID 'uid-peer' must appear in CorrelatedUIDs, got %v", group.CorrelatedUIDs)
	}
}

// TestCorrelator_AllFindings_ExcludesPrimaryFinding verifies that AllFindings in the
// returned CorrelationGroup contains ONLY non-primary findings. The primary's own
// finding is already available on rjob.Spec.Finding at dispatch time and must not
// be duplicated in AllFindings.
func TestCorrelator_AllFindings_ExcludesPrimaryFinding(t *testing.T) {
	candidate := newRJob("primary", "uid-primary")
	candidate.Spec.Finding.Name = "primary-pod"

	peer := newRJob("peer", "uid-peer")
	peer.Spec.Finding.Name = "peer-pod"

	// candidate is the primary.
	matchResult := domain.CorrelationResult{
		Matched:     true,
		GroupID:     "group-excl",
		PrimaryUID:  "uid-primary",
		Reason:      "test",
		MatchedUIDs: []types.UID{"uid-primary", "uid-peer"},
	}
	rule := &fakeRule{name: "rule", result: matchResult}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}

	// The peer's finding MUST be in AllFindings.
	peerFound := false
	for _, f := range group.AllFindings {
		if f.Name == "peer-pod" {
			peerFound = true
		}
	}
	if !peerFound {
		t.Error("expected peer's finding (peer-pod) in AllFindings, got none")
	}

	// The primary's own finding MUST NOT be in AllFindings.
	for _, f := range group.AllFindings {
		if f.Name == "primary-pod" {
			t.Errorf("primary's own finding (primary-pod) must NOT be in AllFindings, but was found: %+v", group.AllFindings)
		}
	}

	// Exactly 1 finding expected (only the peer).
	if len(group.AllFindings) != 1 {
		t.Errorf("expected exactly 1 finding in AllFindings (peer only), got %d: %+v", len(group.AllFindings), group.AllFindings)
	}
}

// TestCorrelator_AllFindings_ExcludesPrimaryFinding_FallbackPath verifies that the
// fallback path (no MatchedUIDs returned by rule) also excludes the primary's own
// finding. The fallback uses the candidate + all peers, so the candidate's finding
// must be omitted when candidate == primary.
func TestCorrelator_AllFindings_ExcludesPrimaryFinding_FallbackPath(t *testing.T) {
	candidate := newRJob("primary", "uid-primary")
	candidate.Spec.Finding.Name = "primary-pod"

	peer1 := newRJob("peer1", "uid-peer1")
	peer1.Spec.Finding.Name = "peer-pod-1"

	peer2 := newRJob("peer2", "uid-peer2")
	peer2.Spec.Finding.Name = "peer-pod-2"

	// No MatchedUIDs returned by rule — triggers fallback path.
	// candidate is the primary.
	matchResult := domain.CorrelationResult{
		Matched:    true,
		GroupID:    "group-fallback-excl",
		PrimaryUID: "uid-primary",
		Reason:     "test",
		// MatchedUIDs is nil — fallback path.
	}
	rule := &fakeRule{name: "rule", result: matchResult}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer1, peer2}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}

	// Both peer findings must be present.
	foundPeer1, foundPeer2 := false, false
	for _, f := range group.AllFindings {
		switch f.Name {
		case "peer-pod-1":
			foundPeer1 = true
		case "peer-pod-2":
			foundPeer2 = true
		case "primary-pod":
			t.Errorf("primary's own finding (primary-pod) must NOT be in AllFindings (fallback path), found in: %+v", group.AllFindings)
		}
	}
	if !foundPeer1 {
		t.Error("expected peer1's finding in AllFindings")
	}
	if !foundPeer2 {
		t.Error("expected peer2's finding in AllFindings")
	}
	// Exactly 2 findings (peer1 + peer2, NOT primary).
	if len(group.AllFindings) != 2 {
		t.Errorf("expected exactly 2 findings in AllFindings (peers only), got %d: %+v", len(group.AllFindings), group.AllFindings)
	}
}

// TestCorrelator_MultipleRulesNoneMatch verifies that when no rule matches, found=false
// and GroupID is empty.
func TestCorrelator_MultipleRulesNoneMatch(t *testing.T) {
	candidate := newRJob("candidate", "uid-candidate")

	rule1 := &fakeRule{name: "rule1", result: domain.CorrelationResult{Matched: false}}
	rule2 := &fakeRule{name: "rule2", result: domain.CorrelationResult{Matched: false}}
	rule3 := &fakeRule{name: "rule3", result: domain.CorrelationResult{Matched: false}}

	c := &correlator.Correlator{
		Rules: []domain.CorrelationRule{rule1, rule2, rule3},
	}

	group, found, err := c.Evaluate(context.Background(), candidate, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false when no rules match, got true")
	}
	if group.GroupID != "" {
		t.Errorf("expected empty GroupID, got %q", group.GroupID)
	}
	if !rule1.called || !rule2.called || !rule3.called {
		t.Error("expected all rules to be tried when none match")
	}
}

// TestMultiPodSameNodeRule_BelowThreshold_NoMatch verifies that fewer pods than
// the threshold on the same node does not produce a match.
func TestMultiPodSameNodeRule_BelowThreshold_NoMatch(t *testing.T) {
	// Threshold=3, only 2 pods on node-a (below threshold).
	candidate := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-candidate-d15a",
			Namespace: "default",
			UID:       "uid-candidate-d15a",
			Annotations: map[string]string{
				domain.NodeNameAnnotation: "node-a",
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-candidate-d15a",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod-candidate-d15a", Namespace: "default"},
		},
	}
	peer := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-peer-d15a",
			Namespace: "default",
			UID:       "uid-peer-d15a",
			Annotations: map[string]string{
				domain.NodeNameAnnotation: "node-a",
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-peer-d15a",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod-peer-d15a", Namespace: "default"},
		},
	}

	rule := correlator.MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), candidate, []*v1alpha1.RemediationJob{peer}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false: only 2 pods on node-a, threshold=3")
	}
}

// TestMultiPodSameNodeRule_ExactlyAtThreshold_Matches verifies that exactly
// threshold pods on the same node produces a match.
func TestMultiPodSameNodeRule_ExactlyAtThreshold_Matches(t *testing.T) {
	// Threshold=3, exactly 3 pods on node-a.
	makePodRJob := func(name, uid string) *v1alpha1.RemediationJob {
		return &v1alpha1.RemediationJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
				UID:       types.UID(uid),
				Annotations: map[string]string{
					domain.NodeNameAnnotation: "node-a",
				},
			},
			Spec: v1alpha1.RemediationJobSpec{
				Fingerprint: "fp-" + name,
				Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: name, Namespace: "default"},
			},
		}
	}

	pod1 := makePodRJob("pod-d15b-1", "uid-pod-d15b-1")
	pod2 := makePodRJob("pod-d15b-2", "uid-pod-d15b-2")
	pod3 := makePodRJob("pod-d15b-3", "uid-pod-d15b-3")

	rule := correlator.MultiPodSameNodeRule{Threshold: 3}
	result, err := rule.Evaluate(context.Background(), pod1, []*v1alpha1.RemediationJob{pod2, pod3}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected Matched=true: exactly 3 pods at threshold=3")
	}
	if len(result.MatchedUIDs) != 3 {
		t.Errorf("expected MatchedUIDs to have 3 entries, got %d: %v", len(result.MatchedUIDs), result.MatchedUIDs)
	}
}

// TestMultiPodSameNodeRule_LexicographicTiebreak_EarliestNameWins verifies that
// when all pods share the same CreationTimestamp, the lexicographically smallest
// name wins as primary.
func TestMultiPodSameNodeRule_LexicographicTiebreak_EarliestNameWins(t *testing.T) {
	// Threshold=2; all 3 pods have the SAME CreationTimestamp.
	// "pod-a" must win lexicographically.
	ts := metav1.NewTime(time.Now().Add(-1 * time.Hour))

	makePodWithTS := func(name, uid string) *v1alpha1.RemediationJob {
		return &v1alpha1.RemediationJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         "default",
				UID:               types.UID(uid),
				CreationTimestamp: ts,
				Annotations: map[string]string{
					domain.NodeNameAnnotation: "node-a",
				},
			},
			Spec: v1alpha1.RemediationJobSpec{
				Fingerprint: "fp-" + name,
				Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: name, Namespace: "default"},
			},
		}
	}

	podZ := makePodWithTS("pod-z", "uid-pod-z-d15c")
	podA := makePodWithTS("pod-a", "uid-pod-a-d15c")
	podM := makePodWithTS("pod-m", "uid-pod-m-d15c")

	rule := correlator.MultiPodSameNodeRule{Threshold: 2}
	// Evaluate with pod-z as candidate, pod-a and pod-m as peers.
	result, err := rule.Evaluate(context.Background(), podZ, []*v1alpha1.RemediationJob{podA, podM}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected Matched=true: 3 pods >= threshold=2")
	}
	// pod-a has the lexicographically smallest name — it must be primary.
	if result.PrimaryUID != podA.UID {
		t.Errorf("expected PrimaryUID=%s (pod-a, lex smallest), got %s", podA.UID, result.PrimaryUID)
	}
}

// TestPVCPodRule_VCCandidate_PodGone_NoMatch verifies that when the candidate is a
// PVC and the peer Pod no longer exists (client.Get returns NotFound), the rule
// returns Matched=false, nil — a non-fatal miss.
func TestPVCPodRule_VCCandidate_PodGone_NoMatch(t *testing.T) {
	// Build a real fake client with corev1 scheme but WITHOUT any Pod object.
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("clientgoscheme.AddToScheme: %v", err)
	}
	if err := v1alpha1.AddRemediationToScheme(s); err != nil {
		t.Fatalf("v1alpha1.AddToScheme: %v", err)
	}
	// Do NOT add any corev1.Pod to the store.
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	pvcRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-pvc-d16",
			Namespace: "default",
			UID:       "uid-pvc-d16",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-pvc-d16",
			Finding:     v1alpha1.FindingSpec{Kind: "PersistentVolumeClaim", Name: "my-pvc", Namespace: "default"},
		},
	}
	podRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-pod-d16",
			Namespace: "default",
			UID:       "uid-pod-d16",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-pod-d16",
			Finding:     v1alpha1.FindingSpec{Kind: "Pod", Name: "pod-gone", Namespace: "default"},
		},
	}

	rule := correlator.PVCPodRule{}
	result, err := rule.Evaluate(context.Background(), pvcRJob, []*v1alpha1.RemediationJob{podRJob}, fakeClient)
	if err != nil {
		t.Fatalf("unexpected error (pod gone must be non-fatal): %v", err)
	}
	if result.Matched {
		t.Error("expected Matched=false when pod is gone (NotFound)")
	}
}

// TestSameNamespaceParentRule_EmptyParentObject_NoMatch verifies that when either
// the candidate or the peer has an empty ParentObject, no match is produced.
func TestSameNamespaceParentRule_EmptyParentObject_NoMatch(t *testing.T) {
	// Sub-case 1: candidate has ParentObject="" but peer has ParentObject="my-app".
	candidateEmpty := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-cand-d17a",
			Namespace: "default",
			UID:       "uid-cand-d17a",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-cand-d17a",
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-d17a",
				Namespace:    "default",
				ParentObject: "", // empty
			},
		},
	}
	peerWithParent := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-peer-d17a",
			Namespace: "default",
			UID:       "uid-peer-d17a",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-peer-d17a",
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-peer-d17a",
				Namespace:    "default",
				ParentObject: "my-app",
			},
		},
	}

	rule := correlator.SameNamespaceParentRule{}
	result1, err := rule.Evaluate(context.Background(), candidateEmpty, []*v1alpha1.RemediationJob{peerWithParent}, nil)
	if err != nil {
		t.Fatalf("sub-case 1 unexpected error: %v", err)
	}
	if result1.Matched {
		t.Error("sub-case 1: expected Matched=false when candidate ParentObject is empty")
	}

	// Sub-case 2: candidate has ParentObject="my-app" but peer has ParentObject="".
	candidateWithParent := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-cand-d17b",
			Namespace: "default",
			UID:       "uid-cand-d17b",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-cand-d17b",
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-d17b",
				Namespace:    "default",
				ParentObject: "my-app",
			},
		},
	}
	peerEmpty := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rjob-peer-d17b",
			Namespace: "default",
			UID:       "uid-peer-d17b",
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: "fp-peer-d17b",
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-peer-d17b",
				Namespace:    "default",
				ParentObject: "", // empty
			},
		},
	}

	result2, err := rule.Evaluate(context.Background(), candidateWithParent, []*v1alpha1.RemediationJob{peerEmpty}, nil)
	if err != nil {
		t.Fatalf("sub-case 2 unexpected error: %v", err)
	}
	if result2.Matched {
		t.Error("sub-case 2: expected Matched=false when peer ParentObject is empty")
	}
}
