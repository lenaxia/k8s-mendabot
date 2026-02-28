package native

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

// Compile-time assertion: pvcProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*pvcProvider)(nil)

type pvcProvider struct {
	client client.Client
}

// NewPVCProvider constructs a pvcProvider. Panics if c is nil.
func NewPVCProvider(c client.Client) domain.SourceProvider {
	if c == nil {
		panic("NewPVCProvider: client must not be nil")
	}
	return &pvcProvider{client: c}
}

// ProviderName returns the stable identifier for this provider.
func (p *pvcProvider) ProviderName() string { return "native" }

// ObjectType returns the runtime.Object type this provider watches.
func (p *pvcProvider) ObjectType() client.Object { return &corev1.PersistentVolumeClaim{} }

// ExtractFinding converts a watched PersistentVolumeClaim into a Finding.
// Returns (nil, nil) if the PVC is not Pending, or if Pending but has no ProvisioningFailed event.
// Returns (nil, err) if obj is not a *corev1.PersistentVolumeClaim.
func (p *pvcProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil, fmt.Errorf("pvcProvider: expected *corev1.PersistentVolumeClaim, got %T", obj)
	}

	if pvc.Status.Phase != corev1.ClaimPending {
		return nil, nil
	}

	eventMsg, found, err := p.latestProvisioningFailedMessage(pvc)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	type errorEntry struct {
		Text string `json:"text"`
	}

	errText := fmt.Sprintf("pvc %s: ProvisioningFailed: %s", pvc.Name, truncate(domain.StripDelimiters(domain.RedactSecrets(eventMsg)), maxCSIEventMessage))
	errorsJSON, err := json.Marshal([]errorEntry{{Text: errText}})
	if err != nil {
		return nil, fmt.Errorf("pvcProvider: serialising errors: %w", err)
	}

	parent := getParent(context.Background(), p.client, pvc.ObjectMeta, "PersistentVolumeClaim")

	return &domain.Finding{
		Kind:         "PersistentVolumeClaim",
		Name:         pvc.Name,
		Namespace:    pvc.Namespace,
		ParentObject: parent,
		Errors:       string(errorsJSON),
		Severity:     domain.SeverityHigh,
	}, nil
}

// latestProvisioningFailedMessage lists all events in the PVC's namespace, filters for
// those that involve this specific PVC (by name and kind), then returns the message of
// the most recent ProvisioningFailed event. Returns ("", false, nil) if none found.
func (p *pvcProvider) latestProvisioningFailedMessage(pvc *corev1.PersistentVolumeClaim) (string, bool, error) {
	var eventList corev1.EventList
	if err := p.client.List(context.Background(), &eventList, client.InNamespace(pvc.Namespace)); err != nil {
		return "", false, fmt.Errorf("pvcProvider: listing events: %w", err)
	}

	var failures []corev1.Event
	for _, ev := range eventList.Items {
		if ev.InvolvedObject.Name != pvc.Name {
			continue
		}
		if ev.InvolvedObject.Kind != "PersistentVolumeClaim" {
			continue
		}
		if ev.Reason != "ProvisioningFailed" {
			continue
		}
		failures = append(failures, ev)
	}

	if len(failures) == 0 {
		return "", false, nil
	}

	sort.Slice(failures, func(i, j int) bool {
		ti := failures[i].LastTimestamp.Time
		tj := failures[j].LastTimestamp.Time
		return ti.After(tj)
	})

	return failures[0].Message, true, nil
}
