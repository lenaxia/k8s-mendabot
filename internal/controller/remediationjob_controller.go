package controller

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

//+kubebuilder:rbac:groups=remediation.mendabot.io,resources=remediationjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=remediation.mendabot.io,resources=remediationjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=remediation.mendabot.io,resources=remediationjobs/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// RemediationJobReconciler watches RemediationJob objects and drives the Job lifecycle.
// It is provider-agnostic — it acts on all RemediationJob objects regardless of source.
type RemediationJobReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Log        *zap.Logger
	JobBuilder domain.JobBuilder
	// Cfg holds operator-wide configuration. MaxConcurrentJobs == 0 means unlimited.
	Cfg config.Config
}

// Reconcile implements ctrl.Reconciler.
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var rjob v1alpha1.RemediationJob
	if err := r.Get(ctx, req.NamespacedName, &rjob); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialise phase: a freshly-created object arrives with Phase=="". Transition
	// to Pending immediately so the phase is never blank in kubectl output, and so
	// the rest of the reconcile logic can rely on only named phase constants.
	if rjob.Status.Phase == "" {
		rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
		rjob.Status.Phase = v1alpha1.PhasePending
		if err := r.Status().Patch(ctx, &rjob, client.MergeFrom(rjobCopy)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	ttl := time.Duration(r.Cfg.RemediationJobTTLSeconds) * time.Second

	switch rjob.Status.Phase {
	case v1alpha1.PhaseSucceeded:
		if rjob.Status.CompletedAt != nil {
			deadline := rjob.Status.CompletedAt.Add(ttl)
			if time.Now().Before(deadline) {
				return ctrl.Result{RequeueAfter: time.Until(deadline)}, nil
			}
			if err := r.Delete(ctx, &rjob); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			if r.Log != nil {
				r.Log.Info("RemediationJob deleted by TTL",
					zap.Bool("audit", true),
					zap.String("event", "remediationjob.deleted_ttl"),
					zap.String("remediationJob", rjob.Name),
					zap.String("namespace", rjob.Namespace),
					zap.String("prRef", rjob.Status.PRRef),
				)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil

	case v1alpha1.PhaseFailed:
		return ctrl.Result{}, nil

	case v1alpha1.PhaseCancelled:
		return ctrl.Result{}, nil
	}

	// List owned Jobs by label.
	var ownedJobs batchv1.JobList
	if err := r.List(ctx, &ownedJobs,
		client.InNamespace(r.Cfg.AgentNamespace),
		client.MatchingLabels{"remediation.mendabot.io/remediation-job": rjob.Name},
	); err != nil {
		return ctrl.Result{}, err
	}
	if len(ownedJobs.Items) > 0 {
		job := &ownedJobs.Items[0]
		newPhase := syncPhaseFromJob(job)
		rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
		rjob.Status.Phase = newPhase
		if newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed {
			if rjob.Status.CompletedAt == nil {
				now := metav1.Now()
				rjob.Status.CompletedAt = &now
			}
			condType := v1alpha1.ConditionJobComplete
			if newPhase == v1alpha1.PhaseFailed {
				condType = v1alpha1.ConditionJobFailed
			}
			apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
				Type:               condType,
				Status:             metav1.ConditionTrue,
				Reason:             string(newPhase),
				LastTransitionTime: metav1.Now(),
			})
		}
		if rjob.Status.JobRef == "" {
			rjob.Status.JobRef = job.Name
		}
		if err := r.Status().Patch(ctx, &rjob, client.MergeFrom(rjobCopy)); err != nil {
			return ctrl.Result{}, err
		}
		if r.Log != nil && (newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed) {
			event := "job.succeeded"
			if newPhase == v1alpha1.PhaseFailed {
				event = "job.failed"
			}
			r.Log.Info("agent job terminal",
				zap.Bool("audit", true),
				zap.String("event", event),
				zap.String("remediationJob", rjob.Name),
				zap.String("job", job.Name),
				zap.String("namespace", rjob.Namespace),
				zap.String("prRef", rjob.Status.PRRef),
			)
		}
		return ctrl.Result{}, nil
	}

	// Check MAX_CONCURRENT_JOBS.
	var allJobs batchv1.JobList
	if err := r.List(ctx, &allJobs,
		client.InNamespace(r.Cfg.AgentNamespace),
		client.MatchingLabels{"app.kubernetes.io/managed-by": "mendabot-watcher"},
	); err != nil {
		return ctrl.Result{}, err
	}
	activeCount := 0
	for i := range allJobs.Items {
		j := &allJobs.Items[i]
		if j.Status.Active > 0 || (j.Status.Succeeded == 0 && j.Status.CompletionTime == nil) {
			activeCount++
		}
	}
	if r.Cfg.MaxConcurrentJobs > 0 && activeCount >= r.Cfg.MaxConcurrentJobs {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, r.dispatch(ctx, &rjob)
}

// syncPhaseFromJob maps the current state of a batch/v1 Job to a RemediationJobPhase.
func syncPhaseFromJob(job *batchv1.Job) v1alpha1.RemediationJobPhase {
	if job.Status.Succeeded > 0 {
		return v1alpha1.PhaseSucceeded
	}
	var backoffLimit int32 = 6
	if job.Spec.BackoffLimit != nil {
		backoffLimit = *job.Spec.BackoffLimit
	}
	if job.Status.Failed >= backoffLimit+1 {
		return v1alpha1.PhaseFailed
	}
	if job.Status.Active > 0 {
		return v1alpha1.PhaseRunning
	}
	return v1alpha1.PhaseDispatched
}

// dispatch builds and creates the batch/v1 Job, then patches the RemediationJob
// status to Dispatched.
func (r *RemediationJobReconciler) dispatch(
	ctx context.Context,
	rjob *v1alpha1.RemediationJob,
) error {
	job, err := r.JobBuilder.Build(rjob, nil)
	if err != nil {
		return fmt.Errorf("building Job: %w", err)
	}

	if err := r.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			var existing batchv1.Job
			if getErr := r.Get(ctx, client.ObjectKeyFromObject(job), &existing); getErr != nil {
				return getErr
			}
			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjob.Status.Phase = syncPhaseFromJob(&existing)
			if rjob.Status.JobRef == "" {
				rjob.Status.JobRef = existing.Name
			}
			if patchErr := r.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); patchErr != nil {
				return patchErr
			}
			return nil
		}
		return fmt.Errorf("creating Job: %w", err)
	}

	rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	now := metav1.Now()
	rjob.Status.Phase = v1alpha1.PhaseDispatched
	rjob.Status.JobRef = job.Name
	rjob.Status.DispatchedAt = &now
	apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionJobDispatched,
		Status:             metav1.ConditionTrue,
		Reason:             "JobCreated",
		LastTransitionTime: now,
	})
	if err := r.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
		return err
	}

	if r.Log != nil {
		r.Log.Info("dispatched agent job",
			zap.String("remediationJob", rjob.Name),
			zap.String("job", job.Name),
			zap.String("namespace", job.Namespace),
		)
	}
	return nil
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *RemediationJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.RemediationJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
