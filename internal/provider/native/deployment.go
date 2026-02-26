package native

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: deploymentProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*deploymentProvider)(nil)

type deploymentProvider struct {
	client client.Client
}

// NewDeploymentProvider constructs a deploymentProvider. Panics if c is nil.
func NewDeploymentProvider(c client.Client) domain.SourceProvider {
	if c == nil {
		panic("NewDeploymentProvider: client must not be nil")
	}
	return &deploymentProvider{client: c}
}

// ProviderName returns the stable identifier for this provider.
func (p *deploymentProvider) ProviderName() string { return "native" }

// ObjectType returns the runtime.Object type this provider watches.
func (p *deploymentProvider) ObjectType() client.Object { return &appsv1.Deployment{} }

// ExtractFinding converts a watched Deployment into a Finding.
// Returns (nil, nil) if the deployment is healthy.
// Returns (nil, err) if obj is not a *appsv1.Deployment.
func (p *deploymentProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	deploy, ok := obj.(*appsv1.Deployment)
	if !ok {
		return nil, fmt.Errorf("deploymentProvider: expected *appsv1.Deployment, got %T", obj)
	}

	type errorEntry struct {
		Text string `json:"text"`
	}

	var errors []errorEntry
	replicaFired := false

	// Replica mismatch check.
	// Skip the check when status.replicas > spec.replicas — this is a scaling-down
	// transient where old pods are still terminating. It is not a failure.
	if deploy.Spec.Replicas != nil {
		specReplicas := *deploy.Spec.Replicas
		if deploy.Status.Replicas <= specReplicas && deploy.Status.ReadyReplicas < specReplicas {
			text := fmt.Sprintf("deployment %s: %d/%d replicas ready",
				deploy.Name, deploy.Status.ReadyReplicas, specReplicas)
			errors = append(errors, errorEntry{Text: text})
			replicaFired = true
		}
	}

	// Available=False condition — always reported regardless of replica counts.
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionFalse {
			text := fmt.Sprintf("deployment %s: Available=False reason=%s message=%s",
				deploy.Name, cond.Reason, truncate(domain.StripDelimiters(domain.RedactSecrets(cond.Message)), maxConditionMessage))
			errors = append(errors, errorEntry{Text: text})
			break
		}
	}

	if len(errors) == 0 {
		return nil, nil
	}

	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return nil, fmt.Errorf("deploymentProvider: serialising errors: %w", err)
	}

	parent := getParent(context.Background(), p.client, deploy.ObjectMeta, "Deployment")

	var specReplicas int32
	if deploy.Spec.Replicas != nil {
		specReplicas = *deploy.Spec.Replicas
	}

	return &domain.Finding{
		Kind:         "Deployment",
		Name:         deploy.Name,
		Namespace:    deploy.Namespace,
		ParentObject: parent,
		Errors:       string(errorsJSON),
		Severity:     computeDeploymentSeverity(deploy.Status.ReadyReplicas, specReplicas, replicaFired),
	}, nil
}

// computeDeploymentSeverity returns the highest severity for the given deployment state.
// replicaFired indicates the replica mismatch check fired.
func computeDeploymentSeverity(readyReplicas, specReplicas int32, replicaFired bool) domain.Severity {
	sev := domain.SeverityMedium

	if replicaFired && specReplicas > 0 {
		if readyReplicas == 0 {
			return domain.SeverityCritical
		}
		if readyReplicas < specReplicas/2 {
			sev = domain.SeverityHigh
		}
	}

	return sev
}
