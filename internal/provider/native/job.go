package native

import (
	"context"
	"encoding/json"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: jobProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*jobProvider)(nil)

type jobProvider struct {
	client client.Client
}

// NewJobProvider constructs a jobProvider. Panics if c is nil.
func NewJobProvider(c client.Client) domain.SourceProvider {
	if c == nil {
		panic("NewJobProvider: client must not be nil")
	}
	return &jobProvider{client: c}
}

// ProviderName returns the stable identifier for this provider.
func (p *jobProvider) ProviderName() string { return "native" }

// ObjectType returns the runtime.Object type this provider watches.
func (p *jobProvider) ObjectType() client.Object { return &batchv1.Job{} }

// ExtractFinding converts a watched Job into a Finding.
// Returns (nil, nil) if the job is healthy, still running, succeeded, suspended,
// or owned by a CronJob.
// Returns (nil, err) if obj is not a *batchv1.Job.
func (p *jobProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("jobProvider: expected *batchv1.Job, got %T", obj)
	}

	// Self-exclusion: skip jobs created by mendabot-watcher (agent jobs).
	// Without this guard a failed agent job would trigger a new RemediationJob,
	// causing a self-triggering cascade loop.
	if job.Labels["app.kubernetes.io/managed-by"] == "mendabot-watcher" {
		return nil, nil
	}

	// CronJob exclusion — checked before any failure detection.
	// Jobs owned by a CronJob are transient by design; remediation should target
	// the CronJob, not the individual Job instance.
	for _, ref := range job.OwnerReferences {
		if ref.Kind == "CronJob" {
			return nil, nil
		}
	}

	// Suspended jobs are deliberate pauses, not failures.
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobSuspended && cond.Status == corev1.ConditionTrue {
			return nil, nil
		}
	}

	// Failure detection: all three conditions must be true simultaneously.
	// failed > 0: at least one pod attempt has failed.
	// active == 0: the Job is no longer running (has given up retrying).
	// completionTime == nil: the Job did not succeed.
	if job.Status.Failed == 0 || job.Status.Active != 0 || job.Status.CompletionTime != nil {
		return nil, nil
	}

	type errorEntry struct {
		Text string `json:"text"`
	}

	var errors []errorEntry

	// Primary error entry: failure summary with attempt count.
	baseText := fmt.Sprintf("job %s: failed (%d attempts, 0 active)", job.Name, job.Status.Failed)
	errors = append(errors, errorEntry{Text: baseText})

	// Append reason and message from the Failed condition when present.
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			if cond.Reason != "" || cond.Message != "" {
				condText := fmt.Sprintf("job %s: %s: %s", job.Name, cond.Reason, domain.RedactSecrets(truncate(cond.Message, 500)))
				errors = append(errors, errorEntry{Text: condText})
			}
			break
		}
	}

	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return nil, fmt.Errorf("jobProvider: serialising errors: %w", err)
	}

	parent := getParent(context.Background(), p.client, job.ObjectMeta, "Job")

	finding := &domain.Finding{
		Kind:         "Job",
		Name:         job.Name,
		Namespace:    job.Namespace,
		ParentObject: parent,
		Errors:       string(errorsJSON),
	}

	return finding, nil
}
