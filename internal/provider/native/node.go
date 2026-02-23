package native

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: nodeProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*nodeProvider)(nil)

// ignoredNodeConditions lists condition types that are never failure indicators,
// even when their Status is ConditionTrue.
var ignoredNodeConditions = map[corev1.NodeConditionType]struct{}{
	"EtcdIsVoter": {},
}

type nodeProvider struct {
	client client.Client
}

// NewNodeProvider constructs a nodeProvider. Panics if c is nil.
func NewNodeProvider(c client.Client) domain.SourceProvider {
	if c == nil {
		panic("NewNodeProvider: client must not be nil")
	}
	return &nodeProvider{client: c}
}

// ProviderName returns the stable identifier for this provider.
func (n *nodeProvider) ProviderName() string { return "native" }

// ObjectType returns the runtime.Object type this provider watches.
func (n *nodeProvider) ObjectType() client.Object { return &corev1.Node{} }

// ExtractFinding converts a watched Node into a Finding.
// Returns (nil, nil) if the node is healthy (no failure conditions detected).
// Returns (nil, err) if obj is not a *corev1.Node.
//
// Failure conditions checked:
//   - NodeReady == False or Unknown
//   - NodeMemoryPressure == True
//   - NodeDiskPressure == True
//   - NodePIDPressure == True
//   - NodeNetworkUnavailable == True
//
// EtcdIsVoter (k3s) is explicitly ignored even when True.
func (n *nodeProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil, fmt.Errorf("nodeProvider: expected *corev1.Node, got %T", obj)
	}

	type errorEntry struct {
		Text string `json:"text"`
	}

	var errors []errorEntry

	for _, cond := range node.Status.Conditions {
		if _, ignored := ignoredNodeConditions[cond.Type]; ignored {
			continue
		}

		switch cond.Type {
		case corev1.NodeReady:
			if cond.Status == corev1.ConditionFalse || cond.Status == corev1.ConditionUnknown {
				errors = append(errors, errorEntry{Text: buildNodeConditionText(node.Name, cond)})
			}
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
			if cond.Status == corev1.ConditionTrue {
				errors = append(errors, errorEntry{Text: buildNodeConditionText(node.Name, cond)})
			}
		default:
			if cond.Status == corev1.ConditionTrue {
				if _, ignored := ignoredNodeConditions[cond.Type]; !ignored {
					errors = append(errors, errorEntry{Text: buildNodeConditionText(node.Name, cond)})
				}
			}
		}
	}

	if len(errors) == 0 {
		return nil, nil
	}

	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return nil, fmt.Errorf("nodeProvider: serialising errors: %w", err)
	}

	parent := getParent(context.Background(), n.client, node.ObjectMeta, "Node")

	return &domain.Finding{
		Kind:         "Node",
		Name:         node.Name,
		Namespace:    "",
		ParentObject: parent,
		Errors:       string(errorsJSON),
		SourceRef: domain.SourceRef{
			APIVersion: "v1",
			Kind:       "Node",
			Name:       node.Name,
			Namespace:  "",
		},
	}, nil
}

// buildNodeConditionText constructs the error message for a failing node condition.
// Format: "node <name> has condition <Type> (<Reason>): <Message>"
func buildNodeConditionText(nodeName string, cond corev1.NodeCondition) string {
	return fmt.Sprintf("node %s has condition %s (%s): %s",
		nodeName, cond.Type, cond.Reason, domain.RedactSecrets(cond.Message))
}
