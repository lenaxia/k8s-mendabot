package native

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: statefulSetProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*statefulSetProvider)(nil)

type statefulSetProvider struct {
	client client.Client
}

// NewStatefulSetProvider constructs a statefulSetProvider. Panics if c is nil.
func NewStatefulSetProvider(c client.Client) domain.SourceProvider {
	if c == nil {
		panic("NewStatefulSetProvider: client must not be nil")
	}
	return &statefulSetProvider{client: c}
}

// ProviderName returns the stable identifier for this provider.
func (p *statefulSetProvider) ProviderName() string { return "native" }

// ObjectType returns the runtime.Object type this provider watches.
func (p *statefulSetProvider) ObjectType() client.Object { return &appsv1.StatefulSet{} }

// ExtractFinding converts a watched StatefulSet into a Finding.
// Returns (nil, nil) if the statefulset is healthy.
// Returns (nil, err) if obj is not a *appsv1.StatefulSet.
func (p *statefulSetProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return nil, fmt.Errorf("statefulSetProvider: expected *appsv1.StatefulSet, got %T", obj)
	}

	type errorEntry struct {
		Text string `json:"text"`
	}

	var errors []errorEntry

	// Replica mismatch check.
	// Only report when generation == observedGeneration (not currently scaling).
	// When generation != observedGeneration, the controller has not yet converged
	// on the new spec — this is a transient, not a failure.
	if sts.Spec.Replicas != nil {
		specReplicas := *sts.Spec.Replicas
		notScaling := sts.Generation == sts.Status.ObservedGeneration
		if notScaling && sts.Status.ReadyReplicas < specReplicas {
			text := fmt.Sprintf("statefulset %s: %d/%d replicas ready",
				sts.Name, sts.Status.ReadyReplicas, specReplicas)
			errors = append(errors, errorEntry{Text: text})
		}
	}

	// Available=False condition — always reported regardless of generation/scaling state.
	// This condition type was added in Kubernetes 1.26; on older clusters it will not
	// be present and this loop will simply find nothing.
	for _, cond := range sts.Status.Conditions {
		if cond.Type == "Available" && cond.Status == corev1.ConditionFalse {
			text := fmt.Sprintf("statefulset %s: condition Available is False: %s: %s",
				sts.Name, cond.Reason, cond.Message)
			errors = append(errors, errorEntry{Text: text})
			break
		}
	}

	if len(errors) == 0 {
		return nil, nil
	}

	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return nil, fmt.Errorf("statefulSetProvider: serialising errors: %w", err)
	}

	parent := getParent(context.Background(), p.client, sts.ObjectMeta, "StatefulSet")

	return &domain.Finding{
		Kind:         "StatefulSet",
		Name:         sts.Name,
		Namespace:    sts.Namespace,
		ParentObject: parent,
		Errors:       string(errorsJSON),
		SourceRef: domain.SourceRef{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
			Name:       sts.Name,
			Namespace:  sts.Namespace,
		},
	}, nil
}
