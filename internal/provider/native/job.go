package native

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: jobProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*jobProvider)(nil)

type jobProvider struct {
	client client.Client
	cfg    config.Config
}

// NewJobProvider constructs a jobProvider. Panics if c is nil.
func NewJobProvider(c client.Client, cfg config.Config) domain.SourceProvider {
	if c == nil {
		panic("NewJobProvider: client must not be nil")
	}
	return &jobProvider{client: c, cfg: cfg}
}

// ProviderName returns the stable identifier for this provider.
func (p *jobProvider) ProviderName() string { return "native" }

// ObjectType returns the runtime.Object type this provider watches.
func (p *jobProvider) ObjectType() client.Object { return &batchv1.Job{} }

// ExtractFinding converts a watched Job into a Finding.
// Returns (nil, nil) if the job is healthy, still running, succeeded, suspended,
// owned by a CronJob, or exceeds self-remediation depth limit.
// Returns (nil, err) if obj is not a *batchv1.Job.
func (p *jobProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("jobProvider: expected *batchv1.Job, got %T", obj)
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

	// Check if this is a mendabot job (self-remediation detection)
	isMendabotJob := false
	chainDepth := 0
	if job.Labels != nil && job.Labels["app.kubernetes.io/managed-by"] == "mendabot-watcher" {
		isMendabotJob = true

		// Try to get chain depth from owner RemediationJob first (atomic source)
		parentDepth, err := p.getChainDepthFromOwner(context.Background(), job)
		if err != nil {
			// Fall back to annotation for backward compatibility
			chainDepth = p.getChainDepthFromAnnotation(job)
		} else {
			chainDepth = parentDepth
		}

		// Increment chain depth for self-remediation
		chainDepth++

		// Check if we've exceeded max depth
		if chainDepth > p.cfg.SelfRemediationMaxDepth {
			return nil, nil
		}
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
				condText := fmt.Sprintf("job %s: %s: %s", job.Name, cond.Reason, domain.RedactSecrets(cond.Message))
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
		SourceRef: domain.SourceRef{
			APIVersion: "batch/v1",
			Kind:       "Job",
			Name:       job.Name,
			Namespace:  job.Namespace,
		},
		IsSelfRemediation: isMendabotJob,
		ChainDepth:        chainDepth,
	}

	// For self-remediations, add context about mendabot failure
	if isMendabotJob {
		finding.Details = fmt.Sprintf("Mendabot agent job failed (chain depth: %d). This may indicate a bug in mendabot itself or a transient issue.", chainDepth)
	}

	return finding, nil
}

// getChainDepthFromOwner reads the chain depth from the owner RemediationJob.
// This provides atomic chain depth tracking since RemediationJob updates are
// controlled by the controller with Patch operations.
func (p *jobProvider) getChainDepthFromOwner(ctx context.Context, job *batchv1.Job) (int, error) {
	// Find RemediationJob owner
	var ownerRef *metav1.OwnerReference
	for _, ref := range job.OwnerReferences {
		if ref.APIVersion == "remediation.mendabot.io/v1alpha1" && ref.Kind == "RemediationJob" {
			ownerRef = &ref
			break
		}
	}

	if ownerRef == nil {
		return 0, fmt.Errorf("no RemediationJob owner found")
	}

	// Read the RemediationJob
	rjob := &v1alpha1.RemediationJob{}
	key := client.ObjectKey{Name: ownerRef.Name, Namespace: job.Namespace}
	if err := p.client.Get(ctx, key, rjob); err != nil {
		return 0, fmt.Errorf("reading owner RemediationJob %s: %w", ownerRef.Name, err)
	}

	return rjob.Spec.ChainDepth, nil
}

// getChainDepthFromAnnotation reads chain depth from Job annotations (legacy).
// Used for backward compatibility when owner RemediationJob cannot be read.
func (p *jobProvider) getChainDepthFromAnnotation(job *batchv1.Job) int {
	if job.Annotations != nil {
		if depthStr, ok := job.Annotations["remediation.mendabot.io/chain-depth"]; ok {
			if depth, err := strconv.Atoi(depthStr); err == nil {
				return depth
			}
		}
	}
	return 0
}
