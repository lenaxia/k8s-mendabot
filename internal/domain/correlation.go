package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

const (
	CorrelationGroupIDLabel = "mechanic.io/correlation-group-id"
	// CorrelationGroupRoleLabel is the metadata label recording a job's role within a
	// correlation group ("primary" or "correlated").
	// Design note: STORY_00 originally specified this constant as "CorrelationRoleLabel";
	// the implementation chose the more descriptive name "CorrelationGroupRoleLabel" to
	// clarify that it belongs to the correlation group subsystem. All code uses the
	// implementation name consistently — no runtime impact from the naming divergence.
	CorrelationGroupRoleLabel = "mechanic.io/correlation-role"
	CorrelationRolePrimary    = "primary"
	CorrelationRoleCorrelated = "correlated"
	// NodeNameAnnotation is set on a RemediationJob by the provider to record the
	// Kubernetes node name of the failing pod. MultiPodSameNodeRule reads this annotation
	// to group pods that fail on the same node.
	NodeNameAnnotation = "mechanic.io/node-name"
)

// CorrelationResult is returned by a CorrelationRule evaluation.
type CorrelationResult struct {
	Matched    bool
	GroupID    string
	PrimaryUID types.UID
	Reason     string
	// MatchedUIDs contains the UIDs of ALL jobs that are part of this correlation group.
	// Rules must populate this field with every job that belongs to the match — including
	// the candidate when it is part of the group. The correlator uses this to restrict
	// AllFindings to only the matched subset, preventing unrelated peers from other
	// correlation groups from leaking into a group's investigation context.
	MatchedUIDs []types.UID
}

// CorrelationRule evaluates whether candidate and one or more peers should be
// grouped into a single investigation.
type CorrelationRule interface {
	// Name returns a stable identifier for the rule (used in log lines).
	Name() string
	// Evaluate returns a CorrelationResult. If Matched is false, the rule did
	// not find a correlation; the correlator tries the next rule.
	Evaluate(ctx context.Context, candidate *v1alpha1.RemediationJob, peers []*v1alpha1.RemediationJob, c client.Client) (CorrelationResult, error)
}

// NewCorrelationGroupID returns a 12-character lowercase hex string suitable
// for use as a correlation group identifier.
func NewCorrelationGroupID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic("correlation: failed to read random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}
