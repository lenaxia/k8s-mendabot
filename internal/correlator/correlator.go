package correlator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// CorrelationGroup is returned by Correlator.Evaluate when a rule matches.
// It identifies the group, the primary job, and the combined findings slice
// passed to JobBuilder.Build.
type CorrelationGroup struct {
	GroupID    string
	PrimaryUID types.UID
	// CorrelatedUIDs contains the UIDs of all non-primary jobs in the group.
	// Populated only when the matching rule returns MatchedUIDs; nil otherwise.
	CorrelatedUIDs []types.UID
	Rule           string
	AllFindings    []v1alpha1.FindingSpec
}

// Correlator applies an ordered list of CorrelationRules to a candidate
// RemediationJob and a set of peers. The first matching rule determines the
// correlation group; subsequent rules are not evaluated.
type Correlator struct {
	Rules []domain.CorrelationRule
}

// Evaluate applies rules in order, returning (group, true, nil) on the first match.
// Returns (CorrelationGroup{}, false, nil) when no rule matches.
func (c *Correlator) Evaluate(
	ctx context.Context,
	candidate *v1alpha1.RemediationJob,
	peers []*v1alpha1.RemediationJob,
	cl client.Client,
) (CorrelationGroup, bool, error) {
	for _, rule := range c.Rules {
		result, err := rule.Evaluate(ctx, candidate, peers, cl)
		if err != nil {
			return CorrelationGroup{}, false, fmt.Errorf("correlator: rule %s: %w", rule.Name(), err)
		}
		if !result.Matched {
			continue
		}

		group := CorrelationGroup{
			GroupID:    result.GroupID,
			PrimaryUID: result.PrimaryUID,
			Rule:       rule.Name(),
		}

		// Populate AllFindings. When the rule populated MatchedUIDs, restrict to only
		// those findings — this prevents peers from other correlation groups that happen
		// to be in the peer list from leaking into this group's investigation context.
		// When MatchedUIDs is empty (e.g. stub rules in tests), fall back to including
		// all peers so the correlator remains backward-compatible.
		// In both paths, the primary's own finding is excluded: it is already available
		// on rjob.Spec.Finding at dispatch time and including it here would duplicate it.
		if len(result.MatchedUIDs) > 0 {
			matchedSet := make(map[types.UID]bool, len(result.MatchedUIDs))
			for _, uid := range result.MatchedUIDs {
				matchedSet[uid] = true
			}
			group.AllFindings = make([]v1alpha1.FindingSpec, 0, len(result.MatchedUIDs))
			// Include candidate only if it is in the matched set AND is not the primary.
			if matchedSet[candidate.UID] && candidate.UID != result.PrimaryUID {
				group.AllFindings = append(group.AllFindings, candidate.Spec.Finding)
			}
			for _, p := range peers {
				if matchedSet[p.UID] && p.UID != result.PrimaryUID {
					group.AllFindings = append(group.AllFindings, p.Spec.Finding)
				}
			}
			// Populate CorrelatedUIDs with all non-primary matched UIDs.
			for _, uid := range result.MatchedUIDs {
				if uid != result.PrimaryUID {
					group.CorrelatedUIDs = append(group.CorrelatedUIDs, uid)
				}
			}
		} else {
			// Fallback path (no MatchedUIDs): include candidate and all peers,
			// excluding the primary's own finding. Also populate CorrelatedUIDs
			// from all peers so suppressCorrelatedPeers can suppress them.
			group.AllFindings = make([]v1alpha1.FindingSpec, 0, len(peers)+1)
			if candidate.UID != result.PrimaryUID {
				group.AllFindings = append(group.AllFindings, candidate.Spec.Finding)
				group.CorrelatedUIDs = append(group.CorrelatedUIDs, candidate.UID)
			}
			for _, p := range peers {
				if p.UID != result.PrimaryUID {
					group.AllFindings = append(group.AllFindings, p.Spec.Finding)
					group.CorrelatedUIDs = append(group.CorrelatedUIDs, p.UID)
				}
			}
		}

		return group, true, nil
	}
	return CorrelationGroup{}, false, nil
}
