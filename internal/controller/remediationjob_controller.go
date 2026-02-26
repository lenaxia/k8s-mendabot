package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/correlator"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

//+kubebuilder:rbac:groups=remediation.mendabot.io,resources=remediationjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=remediation.mendabot.io,resources=remediationjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;delete,namespace=agent

// RemediationJobReconciler watches RemediationJob objects and drives the Job lifecycle.
// It is provider-agnostic — it acts on all RemediationJob objects regardless of source.
type RemediationJobReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Log        *zap.Logger
	JobBuilder domain.JobBuilder
	// Cfg holds operator-wide configuration. MaxConcurrentJobs == 0 means unlimited.
	Cfg      config.Config
	Recorder record.EventRecorder
	// APIReader bypasses the controller-runtime informer cache for direct API server reads.
	// Used by fetchDryRunReport to avoid a race where the ConfigMap written by the agent
	// has not yet propagated to the local cache by the time the reconciler runs.
	// If nil, falls back to r.Client (cache-backed) — acceptable in test environments.
	APIReader client.Reader
	// Correlator holds jobs for the correlation window before dispatching.
	// nil disables correlation entirely (escape hatch).
	Correlator *correlator.Correlator
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
		if rjob.Status.CompletedAt == nil {
			// Safety net: CompletedAt was never set (e.g. status patch was lost on a
			// prior reconcile, or the object was externally mutated). Set it now so the
			// TTL path can run correctly on the next reconcile.  Without this guard the
			// object would live forever in etcd and the dedup fingerprint would be
			// permanently suppressed.
			now := metav1.Now()
			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjobCopy.Status.CompletedAt = &now
			if err := r.Status().Patch(ctx, rjobCopy, client.MergeFrom(&rjob)); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
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

	case v1alpha1.PhaseFailed:
		return ctrl.Result{}, nil

	case v1alpha1.PhasePermanentlyFailed:
		return ctrl.Result{}, nil

	case v1alpha1.PhaseCancelled:
		return ctrl.Result{}, nil

	case v1alpha1.PhaseSuppressed:
		// Suppressed jobs are terminal; apply the same TTL deletion as PhaseSucceeded
		// so they don't accumulate indefinitely in etcd.
		if rjob.Status.CompletedAt == nil {
			// Safety net: CompletedAt was not set during suppression (e.g. patch lost).
			now := metav1.Now()
			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjobCopy.Status.CompletedAt = &now
			if err := r.Status().Patch(ctx, rjobCopy, client.MergeFrom(&rjob)); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
		deadline := rjob.Status.CompletedAt.Add(ttl)
		if time.Now().Before(deadline) {
			return ctrl.Result{RequeueAfter: time.Until(deadline)}, nil
		}
		if err := r.Delete(ctx, &rjob); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case v1alpha1.PhaseDispatched, v1alpha1.PhaseRunning:
		// Fall through to Step 3 (owned-jobs sync). If the owned batch/v1 Job still
		// exists, Step 3 updates the phase from it and returns. If the owned job has
		// been GC'd, Step 3 finds no jobs and returns; the explicit guard below Step 3
		// then returns ctrl.Result{} to prevent double-dispatch.
	}

	// Step 3: list owned Jobs by label. This must run before the correlation block so
	// that jobs that already have an owned batch/v1 Job (e.g. dispatched, running, or
	// completed) are synced from that job's status immediately, without waiting for the
	// correlation window to elapse.
	var ownedJobs batchv1.JobList
	if err := r.List(ctx, &ownedJobs,
		client.InNamespace(r.Cfg.AgentNamespace),
		client.MatchingLabels{"remediation.mendabot.io/remediation-job": rjob.Name},
	); err != nil {
		return ctrl.Result{}, err
	}
	if len(ownedJobs.Items) > 0 {
		job := &ownedJobs.Items[0]
		// Guard against stale Jobs from a previous RemediationJob that had the
		// same name (e.g. after a deployment rollout that preserved fingerprints).
		// The batch/v1 Job carries an OwnerReference with the exact rjob UID that
		// created it. If the UID doesn't match the current rjob we must not copy
		// its terminal phase — doing so would skip the Pending → correlation window
		// → Dispatched flow entirely.  Delete the stale Job so the name is free for
		// the new dispatch, then fall through to the correlation block.
		if !isOwnedBy(job, rjob.UID) {
			if delErr := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); delErr != nil && !apierrors.IsNotFound(delErr) {
				return ctrl.Result{}, fmt.Errorf("deleting stale agent Job %s: %w", job.Name, delErr)
			}
			if r.Log != nil {
				r.Log.Info("deleted stale agent Job owned by previous RemediationJob",
					zap.String("job", job.Name),
					zap.String("remediationJob", rjob.Name),
					zap.String("namespace", job.Namespace),
				)
			}
		} else {
			newPhase := syncPhaseFromJob(job)
			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjob.Status.Phase = newPhase
			var effectiveMaxRetries int32
			if newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed {
				if newPhase == v1alpha1.PhaseFailed {
					// Only increment RetryCount when transitioning *into* Failed for the
					// first time (not on subsequent reconciles of an already-Failed rjob).
					// rjobCopy holds the pre-mutation phase; rjob.Status.Phase has already
					// been set to newPhase above, so compare against the copy.
					if rjobCopy.Status.Phase != v1alpha1.PhaseFailed {
						rjob.Status.RetryCount++
					}
					maxRetries := rjob.Spec.MaxRetries
					if maxRetries <= 0 {
						maxRetries = 3
					}
					effectiveMaxRetries = maxRetries
					if rjob.Status.RetryCount >= maxRetries {
						rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed
						apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
							Type:               v1alpha1.ConditionPermanentlyFailed,
							Status:             metav1.ConditionTrue,
							Reason:             "RetryCapReached",
							Message:            fmt.Sprintf("RetryCount %d reached MaxRetries %d", rjob.Status.RetryCount, maxRetries),
							LastTransitionTime: metav1.Now(),
						})
					} else {
						apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
							Type:               v1alpha1.ConditionJobFailed,
							Status:             metav1.ConditionTrue,
							Reason:             string(newPhase),
							LastTransitionTime: metav1.Now(),
						})
					}
				} else {
					apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
						Type:               v1alpha1.ConditionJobComplete,
						Status:             metav1.ConditionTrue,
						Reason:             string(newPhase),
						LastTransitionTime: metav1.Now(),
					})
				}

				if rjob.Status.CompletedAt == nil {
					now := metav1.Now()
					rjob.Status.CompletedAt = &now
				}
			}
			if rjob.Status.JobRef == "" {
				rjob.Status.JobRef = job.Name
			}
			if newPhase == v1alpha1.PhaseSucceeded &&
				job.Annotations["mendabot.io/dry-run"] == "true" &&
				rjob.Status.Message == "" {
				rjob.Status.Message = r.fetchDryRunReport(ctx, &rjob)
			}
			if err := r.Status().Patch(ctx, &rjob, client.MergeFrom(rjobCopy)); err != nil {
				return ctrl.Result{}, err
			}
			if r.Log != nil && (newPhase == v1alpha1.PhaseSucceeded || newPhase == v1alpha1.PhaseFailed) {
				switch {
				case newPhase == v1alpha1.PhaseSucceeded:
					r.Log.Info("agent job terminal",
						zap.Bool("audit", true),
						zap.String("event", "job.succeeded"),
						zap.String("remediationJob", rjob.Name),
						zap.String("job", job.Name),
						zap.String("namespace", rjob.Namespace),
						zap.String("prRef", rjob.Status.PRRef),
					)
				case rjob.Status.Phase == v1alpha1.PhasePermanentlyFailed:
					r.Log.Info("agent job permanently failed",
						zap.Bool("audit", true),
						zap.String("event", "job.permanently_failed"),
						zap.String("remediationJob", rjob.Name),
						zap.String("job", job.Name),
						zap.String("namespace", rjob.Namespace),
						zap.Int32("retryCount", rjob.Status.RetryCount),
						zap.Int32("maxRetries", effectiveMaxRetries),
					)
				case newPhase == v1alpha1.PhaseFailed:
					r.Log.Info("agent job terminal",
						zap.Bool("audit", true),
						zap.String("event", "job.failed"),
						zap.String("remediationJob", rjob.Name),
						zap.String("job", job.Name),
						zap.String("namespace", rjob.Namespace),
						zap.String("prRef", rjob.Status.PRRef),
					)
				}
			}
			if r.Recorder != nil {
				switch newPhase {
				case v1alpha1.PhaseSucceeded:
					prRef := rjob.Status.PRRef
					if prRef != "" {
						r.Recorder.Eventf(&rjob, corev1.EventTypeNormal, "JobSucceeded",
							"Agent Job completed; PR: %s", prRef)
					} else {
						r.Recorder.Event(&rjob, corev1.EventTypeNormal, "JobSucceeded",
							"Agent Job completed")
					}
				case v1alpha1.PhaseFailed:
					if rjob.Status.Phase == v1alpha1.PhasePermanentlyFailed {
						r.Recorder.Eventf(&rjob, corev1.EventTypeWarning, "JobPermanentlyFailed",
							"Agent Job permanently failed after %d attempt(s); no further retries", rjob.Status.RetryCount)
					} else {
						r.Recorder.Eventf(&rjob, corev1.EventTypeWarning, "JobFailed",
							"Agent Job failed after %d attempt(s)", job.Status.Failed)
					}
				}
			}
			return ctrl.Result{}, nil
		} // end else isOwnedBy
	}

	// Guard: PhaseDispatched / PhaseRunning jobs whose owned batch/v1 Job has been
	// GC'd must NOT dispatch a second Job. There is nothing left to do until a new
	// finding triggers a fresh Pending → Dispatched cycle.
	if rjob.Status.Phase == v1alpha1.PhaseDispatched || rjob.Status.Phase == v1alpha1.PhaseRunning {
		return ctrl.Result{}, nil
	}

	// Correlation window hold: if correlation is enabled, optionally hold jobs for
	// CorrelationWindowSeconds to allow peer findings to appear, then run
	// the correlator before dispatching. When window==0 the hold is skipped but
	// the correlator still runs. This block runs AFTER the owned-jobs sync above
	// so that jobs with an existing batch/v1 Job are not blocked by the window hold.
	if r.Correlator != nil {
		window := time.Duration(r.Cfg.CorrelationWindowSeconds) * time.Second
		if window > 0 {
			age := time.Since(rjob.CreationTimestamp.Time)
			if age < window {
				return ctrl.Result{RequeueAfter: window - age}, nil
			}
		}
		peers, peersErr := r.pendingPeers(ctx, &rjob)
		if peersErr != nil {
			return ctrl.Result{}, fmt.Errorf("listing pending peers: %w", peersErr)
		}
		group, found, err := r.Correlator.Evaluate(ctx, &rjob, peers, r.Client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("correlator evaluate: %w", err)
		}
		if found {
			if group.PrimaryUID != rjob.UID {
				// Check whether the designated primary still exists. We need only the
				// existence flag — we do not use the primary's content, so avoid
				// storing it in a variable that would mislead future readers.
				primaryGone := true
				var allInNS v1alpha1.RemediationJobList
				if listErr := r.List(ctx, &allInNS,
					client.InNamespace(r.Cfg.AgentNamespace),
					client.MatchingLabels{"app.kubernetes.io/managed-by": "mendabot-watcher"},
				); listErr != nil {
					return ctrl.Result{}, fmt.Errorf("listing all jobs to check primary liveness: %w", listErr)
				} else {
					for i := range allInNS.Items {
						if allInNS.Items[i].UID == group.PrimaryUID {
							primaryGone = false
							break
						}
					}
				}
				// Use a minimum grace period of 10 s so that window==0 configurations
				// still give the primary a chance to reconcile before the non-primary
				// falls back to solo dispatch.
				const minGracePeriod = 10 * time.Second
				gracePeriod := 3 * window
				if gracePeriod < minGracePeriod {
					gracePeriod = minGracePeriod
				}
				waitedLongEnough := time.Since(rjob.CreationTimestamp.Time) > gracePeriod+window
				if primaryGone && waitedLongEnough {
					if r.Log != nil {
						r.Log.Warn("correlation: primary disappeared after grace period; falling back to solo dispatch",
							zap.String("remediationJob", rjob.Name),
							zap.String("expectedPrimaryUID", string(group.PrimaryUID)),
						)
					}
					// Explicit solo dispatch — do not fall through to the bottom of the
					// function, which would also solo-dispatch but without this log context.
					if limited, res, err := r.concurrencyGate(ctx); err != nil {
						return ctrl.Result{}, err
					} else if limited {
						return res, nil
					}
					return ctrl.Result{}, r.dispatch(ctx, &rjob, nil)
				} else {
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
				}
			}

			if group.PrimaryUID == rjob.UID {
				if existing := rjob.Status.CorrelationGroupID; existing != "" {
					group.GroupID = existing
				} else if existing := rjob.Labels[domain.CorrelationGroupIDLabel]; existing != "" {
					group.GroupID = existing
				}
				if err := r.suppressCorrelatedPeers(ctx, peers, group); err != nil {
					return ctrl.Result{}, err
				}
				rjobStatusCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
				rjob.Status.CorrelationGroupID = group.GroupID
				if err := r.Status().Patch(ctx, &rjob, client.MergeFrom(rjobStatusCopy)); err != nil {
					return ctrl.Result{}, err
				}
				rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
				if rjob.Labels == nil {
					rjob.Labels = make(map[string]string)
				}
				rjob.Labels[domain.CorrelationGroupIDLabel] = group.GroupID
				rjob.Labels[domain.CorrelationGroupRoleLabel] = domain.CorrelationRolePrimary
				if err := r.Patch(ctx, &rjob, client.MergeFrom(rjobCopy)); err != nil {
					return ctrl.Result{}, err
				}
				if r.Log != nil {
					r.Log.Info("dispatching correlated primary",
						zap.String("remediationJob", rjob.Name),
						zap.String("rule", group.Rule),
						zap.String("groupID", group.GroupID),
						zap.Int("correlatedPeers", len(group.CorrelatedUIDs)),
					)
				}
				return ctrl.Result{}, r.dispatch(ctx, &rjob, group.AllFindings)
			}
		}

		if rjob.Status.CorrelationGroupID != "" && rjob.Status.Phase == v1alpha1.PhasePending {
			var allPeers v1alpha1.RemediationJobList
			if listErr := r.List(ctx, &allPeers,
				client.InNamespace(r.Cfg.AgentNamespace),
				client.MatchingLabels{"app.kubernetes.io/managed-by": "mendabot-watcher"},
			); listErr != nil {
				return ctrl.Result{}, fmt.Errorf("listing peers for correlation recovery: %w", listErr)
			}
			var recoveredFindings []v1alpha1.FindingSpec
			for i := range allPeers.Items {
				p := &allPeers.Items[i]
				if p.UID == rjob.UID {
					continue
				}
				if p.Status.CorrelationGroupID == rjob.Status.CorrelationGroupID &&
					p.Status.Phase == v1alpha1.PhaseSuppressed {
					recoveredFindings = append(recoveredFindings, p.Spec.Finding)
				}
			}
			if len(recoveredFindings) == 0 && r.Log != nil {
				r.Log.Warn("correlation recovery: no suppressed peers found for group; dispatching primary as solo",
					zap.String("remediationJob", rjob.Name),
					zap.String("groupID", rjob.Status.CorrelationGroupID),
				)
			}
			if rjob.Labels[domain.CorrelationGroupIDLabel] == "" {
				rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
				if rjob.Labels == nil {
					rjob.Labels = make(map[string]string)
				}
				rjob.Labels[domain.CorrelationGroupIDLabel] = rjob.Status.CorrelationGroupID
				rjob.Labels[domain.CorrelationGroupRoleLabel] = domain.CorrelationRolePrimary
				if err := r.Patch(ctx, &rjob, client.MergeFrom(rjobCopy)); err != nil {
					return ctrl.Result{}, err
				}
			}
			if limited, result, err := r.concurrencyGate(ctx); err != nil {
				return ctrl.Result{}, err
			} else if limited {
				return result, nil
			}
			return ctrl.Result{}, r.dispatch(ctx, &rjob, recoveredFindings)
		}
	}

	// Step 4: check MAX_CONCURRENT_JOBS.
	if limited, result, err := r.concurrencyGate(ctx); err != nil {
		return ctrl.Result{}, err
	} else if limited {
		return result, nil
	}

	// Step 5+6+7: build, create, and dispatch the Job with no correlated findings.
	return ctrl.Result{}, r.dispatch(ctx, &rjob, nil)
}

// DryRunCMName returns the name of the ConfigMap written by the agent at the
// end of a dry-run job. The name mirrors the Job name (mendabot-agent-<fp12>)
// so the controller can derive it directly from the RJob fingerprint.
// Exported so tests can derive the expected name without duplicating the logic.
func DryRunCMName(fingerprint string) string {
	if len(fingerprint) > 12 {
		fingerprint = fingerprint[:12]
	}
	return "mendabot-dryrun-" + fingerprint
}

// fetchDryRunReport reads the dry-run report ConfigMap written by the agent,
// assembles the report+patch content, then deletes the ConfigMap.
// The ConfigMap name is derived from rjob.Spec.Fingerprint.
//
// The Get uses APIReader (direct API server read) to bypass the controller-runtime
// informer cache. The agent writes the ConfigMap just before the pod exits; the
// reconciler fires almost immediately on Job status change. If we used the cached
// client, there is a window where the informer has not yet synced the new CM and
// returns NotFound. Combined with the idempotency guard on rjob.Status.Message,
// that would permanently lose the report for that run.
func (r *RemediationJobReconciler) fetchDryRunReport(ctx context.Context, rjob *v1alpha1.RemediationJob) string {
	cmName := DryRunCMName(rjob.Spec.Fingerprint)

	// Prefer the direct API reader; fall back to the cached client only in
	// environments (e.g. unit tests) where APIReader is not wired up.
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}

	var cm corev1.ConfigMap
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: r.Cfg.AgentNamespace,
		Name:      cmName,
	}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("dry-run report unavailable: ConfigMap %s/%s not found (agent may have crashed before writing it)", r.Cfg.AgentNamespace, cmName)
		}
		return fmt.Sprintf("dry-run report unavailable: get ConfigMap: %v", err)
	}

	report := cm.Data["report"]
	patch := cm.Data["patch"]

	var sb strings.Builder
	sb.WriteString(report)
	if patch != "" {
		sb.WriteString("\n\n=== PROPOSED PATCH ===\n")
		sb.WriteString(patch)
		sb.WriteString("\n=== END PATCH ===")
	}

	// Best-effort delete — if this fails the CM is orphaned but harmless;
	// the idempotency guard on rjob.Status.Message prevents re-reading it.
	_ = r.Delete(ctx, &cm)

	return strings.TrimSpace(sb.String())
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

// pendingPeers lists all Pending RemediationJob objects in AgentNamespace,
// excluding the candidate itself. Returns an error if the API server is unavailable
// so the caller can requeue rather than silently treating the cluster as having zero peers.
// The managed-by label selector restricts the list to mendabot-owned objects, preventing
// O(N) full-namespace scans from amplifying into O(N²) API-server load on every reconcile.
//
// Contract: this function only returns jobs that carry app.kubernetes.io/managed-by=mendabot-watcher.
// RemediationJobs created without this label (e.g. manually) are invisible to correlation.
// SourceProviderReconciler always sets this label at creation time — see provider.go.
func (r *RemediationJobReconciler) pendingPeers(ctx context.Context, candidate *v1alpha1.RemediationJob) ([]*v1alpha1.RemediationJob, error) {
	var list v1alpha1.RemediationJobList
	if err := r.List(ctx, &list,
		client.InNamespace(r.Cfg.AgentNamespace),
		client.MatchingLabels{"app.kubernetes.io/managed-by": "mendabot-watcher"},
	); err != nil {
		return nil, err
	}
	peers := make([]*v1alpha1.RemediationJob, 0, len(list.Items))
	for i := range list.Items {
		p := &list.Items[i]
		if p.UID == candidate.UID {
			continue
		}
		if p.Status.Phase != v1alpha1.PhasePending {
			continue
		}
		peers = append(peers, p)
	}
	return peers, nil
}

// suppressCorrelatedPeers calls transitionSuppressed on every peer whose UID
// appears in group.CorrelatedUIDs. It is called by the primary job's reconcile
// to suppress all non-primary members of the group before the primary dispatches,
// ensuring they cannot dispatch independently.
func (r *RemediationJobReconciler) suppressCorrelatedPeers(
	ctx context.Context,
	peers []*v1alpha1.RemediationJob,
	group correlator.CorrelationGroup,
) error {
	correlated := make(map[types.UID]struct{}, len(group.CorrelatedUIDs))
	for _, uid := range group.CorrelatedUIDs {
		correlated[uid] = struct{}{}
	}
	for _, peer := range peers {
		if _, ok := correlated[peer.UID]; !ok {
			continue
		}
		if err := r.transitionSuppressed(ctx, peer, group.GroupID, group.PrimaryUID); err != nil {
			return err
		}
	}
	return nil
}

// dispatch builds and creates the batch/v1 Job, then patches the RemediationJob
// status to Dispatched. correlatedFindings is non-nil when this is a correlated
// primary job; nil means single-finding dispatch.
func (r *RemediationJobReconciler) dispatch(
	ctx context.Context,
	rjob *v1alpha1.RemediationJob,
	correlatedFindings []v1alpha1.FindingSpec,
) error {
	if domain.DetectInjection(rjob.Spec.Finding.Errors) || domain.DetectInjection(rjob.Spec.Finding.Details) {
		if r.Log != nil {
			r.Log.Warn("injection detected in RemediationJob spec — suppressing dispatch",
				zap.Bool("audit", true),
				zap.String("event", "finding.injection_detected"),
				zap.String("source", "controller"),
				zap.String("remediationJob", rjob.Name),
				zap.String("namespace", rjob.Namespace),
			)
		}
		if r.Cfg.InjectionDetectionAction == "suppress" {
			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjob.Status.Phase = v1alpha1.PhasePermanentlyFailed
			apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionPermanentlyFailed,
				Status:             metav1.ConditionTrue,
				Reason:             "InjectionDetected",
				Message:            "injection pattern detected in finding; dispatch suppressed",
				LastTransitionTime: metav1.Now(),
			})
			return r.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy))
		}
	}

	job, err := r.JobBuilder.Build(rjob, correlatedFindings)
	if err != nil {
		return fmt.Errorf("building Job: %w", err)
	}

	if err := r.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			var existing batchv1.Job
			if getErr := r.Get(ctx, client.ObjectKeyFromObject(job), &existing); getErr != nil {
				return getErr
			}
			// If the existing Job is not owned by this rjob (stale from a prior rjob
			// with the same name), delete it and requeue so the next reconcile creates
			// a fresh one. This is a safety net; the primary guard is in Step 3.
			if !isOwnedBy(&existing, rjob.UID) {
				if delErr := r.Delete(ctx, &existing, client.PropagationPolicy(metav1.DeletePropagationBackground)); delErr != nil && !apierrors.IsNotFound(delErr) {
					return fmt.Errorf("deleting stale agent Job %s in dispatch: %w", existing.Name, delErr)
				}
				return fmt.Errorf("stale agent Job %s deleted; requeueing for fresh dispatch", existing.Name)
			}
			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjob.Status.Phase = syncPhaseFromJob(&existing)
			if rjob.Status.JobRef == "" {
				rjob.Status.JobRef = existing.Name
			}
			if patchErr := r.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); patchErr != nil {
				return patchErr
			}
			if r.Recorder != nil {
				r.Recorder.Eventf(rjob, corev1.EventTypeNormal, "JobDispatched",
					"Created agent Job %s", job.Name)
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
			zap.Bool("audit", true),
			zap.String("event", "job.dispatched"),
			zap.String("remediationJob", rjob.Name),
			zap.String("job", job.Name),
			zap.String("namespace", job.Namespace),
		)
	}
	if r.Recorder != nil {
		r.Recorder.Eventf(rjob, corev1.EventTypeNormal, "JobDispatched",
			"Created agent Job %s", job.Name)
	}
	return nil
}

// transitionSuppressed patches the RemediationJob to PhaseSuppressed, sets
// CorrelationGroupID, appends the ConditionCorrelationSuppressed condition,
// and labels the object with its correlation group role.
// It is idempotent: if the job is already Suppressed it returns nil without
// re-patching (avoids spurious LastTransitionTime churn on re-reconcile).
// It also guards against stale-read races: if the job transitioned out of Pending
// between pendingPeers() and this call (e.g. it was dispatched by another goroutine),
// it is skipped — only Pending jobs should be suppressed.
func (r *RemediationJobReconciler) transitionSuppressed(
	ctx context.Context,
	rjob *v1alpha1.RemediationJob,
	groupID string,
	primaryUID types.UID,
) error {
	// Idempotency guard: skip if already suppressed — avoids spurious condition churn.
	if rjob.Status.Phase == v1alpha1.PhaseSuppressed {
		return nil
	}
	// Phase guard: only suppress Pending jobs. If the job transitioned out of Pending
	// between pendingPeers() listing it and this suppression call (stale-read race),
	// do not touch it — patching a non-Pending job would corrupt its terminal state.
	if rjob.Status.Phase != v1alpha1.PhasePending {
		return nil
	}
	rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	rjob.Status.Phase = v1alpha1.PhaseSuppressed
	rjob.Status.CorrelationGroupID = groupID
	now := metav1.Now()
	rjob.Status.CompletedAt = &now
	apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionCorrelationSuppressed,
		Status:             metav1.ConditionTrue,
		Reason:             "CorrelatedGroupFound",
		Message:            fmt.Sprintf("suppressed: primary job UID %s handles investigation", string(primaryUID)),
		LastTransitionTime: metav1.Now(),
	})
	if err := r.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
		return err
	}

	rjobCopy2 := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	if rjob.Labels == nil {
		rjob.Labels = make(map[string]string)
	}
	rjob.Labels[domain.CorrelationGroupIDLabel] = groupID
	rjob.Labels[domain.CorrelationGroupRoleLabel] = domain.CorrelationRoleCorrelated
	return r.Patch(ctx, rjob, client.MergeFrom(rjobCopy2))
}

// concurrencyGate checks whether the current number of active batch/v1 Jobs
// has reached MaxConcurrentJobs. When MaxConcurrentJobs==0 the gate is always open.
// Returns (true, result, nil) when limited; (false, {}, nil) when the gate is open;
// (false, {}, err) on a list error.
func (r *RemediationJobReconciler) concurrencyGate(ctx context.Context) (bool, ctrl.Result, error) {
	if r.Cfg.MaxConcurrentJobs == 0 {
		return false, ctrl.Result{}, nil
	}
	var jobs batchv1.JobList
	if err := r.List(ctx, &jobs,
		client.InNamespace(r.Cfg.AgentNamespace),
		client.MatchingLabels{"app.kubernetes.io/managed-by": "mendabot-watcher"},
	); err != nil {
		return false, ctrl.Result{}, err
	}
	activeCount := 0
	for i := range jobs.Items {
		j := &jobs.Items[i]
		if j.Status.Active > 0 || (j.Status.Succeeded == 0 && j.Status.CompletionTime == nil) {
			activeCount++
		}
	}
	if activeCount >= r.Cfg.MaxConcurrentJobs {
		return true, ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	return false, ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *RemediationJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.RemediationJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// isOwnedBy reports whether job has a controller OwnerReference pointing to uid.
// This is used to distinguish a legitimately owned batch/v1 Job from a stale Job
// left over from a previous RemediationJob that happened to have the same name
// (same fingerprint) but a different UID (e.g. from a prior deployment rollout).
func isOwnedBy(job *batchv1.Job, uid types.UID) bool {
	for _, ref := range job.OwnerReferences {
		if ref.Controller != nil && *ref.Controller && ref.UID == uid {
			return true
		}
	}
	return false
}
