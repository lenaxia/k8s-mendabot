package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/circuitbreaker"
	"github.com/lenaxia/k8s-mechanic/internal/config"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
	"github.com/lenaxia/k8s-mechanic/internal/readiness"
)

// SourceProviderReconciler is a controller-runtime Reconciler that wraps a SourceProvider.
// It handles fetch, skip-if-not-found, ExtractFinding, dedup-by-CRD, and
// RemediationJob creation. Source-specific logic is entirely in the SourceProvider.
type SourceProviderReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Log           *zap.Logger
	Cfg           config.Config
	Provider      domain.SourceProvider
	EventRecorder record.EventRecorder
	// ReadinessChecker gates RemediationJob creation. If non-nil, Check must
	// return nil before any RemediationJob is created. Use readiness.All to
	// combine multiple checkers (sink + LLM). A nil value disables the gate.
	ReadinessChecker readiness.Checker
	// CircuitBreaker gates self-remediation findings by enforcing a cooldown
	// between successive self-remediation attempts. Only consulted when
	// finding.ChainDepth > 0. A nil value disables the circuit breaker.
	CircuitBreaker circuitbreaker.Gater
	// SinkCloser closes open GitHub sinks when a finding resolves.
	// nil disables auto-close (equivalent to PR_AUTO_CLOSE=false).
	SinkCloser domain.SinkCloser
	firstSeen  *BoundedMap
	initOnce   sync.Once
}

// ReadinessCacheTTL is the recommended TTL for CachedChecker wrappers around
// readiness probes. The requeue interval on a failed gate is set to this same
// duration so that the cache is always expired before the next reconcile fires.
const ReadinessCacheTTL = 60 * time.Second

// initFirstSeen initializes the firstSeen map with thread-safe lazy initialization.
func (r *SourceProviderReconciler) initFirstSeen() {
	r.initOnce.Do(func() {
		// Default configuration: max 1000 entries, TTL = stabilization window * 2 (or 1 hour if window is 0)
		ttl := r.Cfg.StabilisationWindow * 2
		if ttl == 0 {
			ttl = time.Hour
		}
		r.firstSeen = NewBoundedMap(1000, ttl, 0)
	})
}

// Reconcile implements ctrl.Reconciler.
func (r *SourceProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.initFirstSeen()

	obj := r.Provider.ObjectType().DeepCopyObject().(client.Object)
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		r.firstSeen.Clear()

		var rjobList v1alpha1.RemediationJobList
		if listErr := r.List(ctx, &rjobList, client.InNamespace(r.Cfg.AgentNamespace)); listErr != nil {
			return ctrl.Result{}, listErr
		}
		var cancelErrs []error
		for i := range rjobList.Items {
			rjob := &rjobList.Items[i]
			if rjob.Spec.SourceResultRef.Name != req.Name ||
				rjob.Spec.SourceResultRef.Namespace != req.Namespace {
				continue
			}
			phase := rjob.Status.Phase
			// Cancel Pending, Dispatched, Running, Suppressed, and Phase=="" jobs.
			// Suppressed jobs must also be cancelled: when their source object is
			// deleted, the correlated investigation is permanently moot and the job
			// must not leak indefinitely (TTL only applies to Succeeded).
			// Also cancel Phase=="" (blank) because there is a real race window between
			// client.Create() and the RemediationJobReconciler's first reconcile that
			// transitions "" → Pending. A source deletion arriving in that window must
			// still cancel the job. Do NOT remove the phase != "" check even though the
			// controller now initialises phase immediately — the race window exists
			// and removing this will silently reintroduce the bug.
			if phase != v1alpha1.PhasePending && phase != v1alpha1.PhaseDispatched &&
				phase != v1alpha1.PhaseRunning && phase != v1alpha1.PhaseSuppressed && phase != "" {
				continue
			}
			// Path A: auto-close the sink before cancellation so we still have the
			// full rjob status available (SinkRef is cleared after deletion).
			// Failure is non-fatal — log and proceed with cancellation.
			if r.Cfg.PRAutoClose && r.SinkCloser != nil && rjob.Status.SinkRef.URL != "" {
				closeReason := fmt.Sprintf(
					"Closing automatically: the underlying issue (%s/%s %s) has resolved. No manual fix is required.",
					rjob.Spec.Finding.Kind, rjob.Spec.Finding.Name, rjob.Spec.Finding.Namespace)
				if closeErr := r.SinkCloser.Close(ctx, rjob, closeReason); closeErr != nil {
					if r.Log != nil {
						r.Log.Error("auto-close sink failed; continuing with cancellation",
							zap.Bool("audit", true),
							zap.String("event", "sink.close_failed"),
							zap.String("remediationJob", rjob.Name),
							zap.String("sinkURL", rjob.Status.SinkRef.URL),
							zap.Error(closeErr),
						)
					}
				}
			}
			// Patch phase to Cancelled before deleting so observers see the terminal state.
			rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
			rjob.Status.Phase = v1alpha1.PhaseCancelled
			if patchErr := r.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); patchErr != nil && !apierrors.IsNotFound(patchErr) {
				cancelErrs = append(cancelErrs, patchErr)
				continue
			}
			if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
				cancelErrs = append(cancelErrs, delErr)
			} else {
				if r.EventRecorder != nil {
					r.EventRecorder.Event(rjob, corev1.EventTypeNormal, "SourceDeleted", "Source object deleted; investigation cancelled")
				}
				if r.Log != nil {
					r.Log.Info("RemediationJob cancelled",
						zap.Bool("audit", true),
						zap.String("event", "remediationjob.cancelled"),
						zap.String("provider", r.Provider.ProviderName()),
						zap.String("remediationJob", rjob.Name),
						zap.String("reason", "source_deleted"),
						zap.String("sourceRef", req.Name),
					)
				}
			}
		}
		if len(cancelErrs) > 0 {
			return ctrl.Result{}, fmt.Errorf("cancelling RemediationJobs: %w", errors.Join(cancelErrs...))
		}
		// Path B: auto-close any PhaseSucceeded sinks for this source ref.
		// Reuses the rjobList already fetched above (full AgentNamespace scope).
		// Do NOT delete Succeeded rjobs — they are the dedup tombstone.
		if r.Cfg.PRAutoClose && r.SinkCloser != nil {
			r.autoCloseSucceededSinks(ctx, req.Name, req.Namespace, &rjobList)
		}
		return ctrl.Result{}, nil
	}

	finding, err := r.Provider.ExtractFinding(obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	if finding == nil {
		r.firstSeen.Clear()
		if r.EventRecorder != nil {
			r.EventRecorder.Event(obj, corev1.EventTypeNormal, "FindingCleared", "Finding cleared; no active finding on this object")
		}
		// Path B: auto-close Succeeded sinks for this source ref.
		// Only pay the List cost when auto-close is enabled.
		if r.Cfg.PRAutoClose && r.SinkCloser != nil {
			var rjobList v1alpha1.RemediationJobList
			if listErr := r.List(ctx, &rjobList, client.InNamespace(r.Cfg.AgentNamespace)); listErr == nil {
				r.autoCloseSucceededSinks(ctx, req.Name, req.Namespace, &rjobList)
			} else if r.Log != nil {
				r.Log.Error("auto-close: failed to list rjobs for finding-cleared path",
					zap.Error(listErr),
				)
			}
		}
		return ctrl.Result{}, nil
	}

	if domain.DetectInjection(finding.Errors) {
		if r.Log != nil {
			r.Log.Warn("potential prompt injection detected in finding errors",
				zap.Bool("audit", true),
				zap.String("event", "finding.injection_detected"),
				zap.String("provider", r.Provider.ProviderName()),
				zap.String("kind", finding.Kind),
				zap.String("namespace", finding.Namespace),
				zap.String("name", finding.Name),
			)
		}
		if r.Cfg.InjectionDetectionAction == "suppress" {
			return ctrl.Result{}, nil
		}
	}

	if domain.DetectInjection(finding.Details) {
		if r.Log != nil {
			r.Log.Warn("potential prompt injection detected in finding details",
				zap.Bool("audit", true),
				zap.String("event", "finding.injection_detected_in_details"),
				zap.String("provider", r.Provider.ProviderName()),
				zap.String("kind", finding.Kind),
				zap.String("namespace", finding.Namespace),
				zap.String("name", finding.Name),
			)
		}
		if r.Cfg.InjectionDetectionAction == "suppress" {
			return ctrl.Result{}, nil
		}
	}

	if finding.Namespace != "" {
		if len(r.Cfg.WatchNamespaces) > 0 {
			allowed := false
			for _, ns := range r.Cfg.WatchNamespaces {
				if ns == finding.Namespace {
					allowed = true
					break
				}
			}
			if !allowed {
				if r.Log != nil {
					r.Log.Debug("namespace filter: skipping finding (not in WatchNamespaces)",
						zap.String("provider", r.Provider.ProviderName()),
						zap.String("namespace", finding.Namespace),
						zap.String("kind", finding.Kind),
						zap.String("name", finding.Name),
					)
				}
				return ctrl.Result{}, nil
			}
		}
		for _, ns := range r.Cfg.ExcludeNamespaces {
			if ns == finding.Namespace {
				if r.Log != nil {
					r.Log.Debug("namespace filter: skipping finding (in ExcludeNamespaces)",
						zap.String("provider", r.Provider.ProviderName()),
						zap.String("namespace", finding.Namespace),
						zap.String("kind", finding.Kind),
						zap.String("name", finding.Name),
					)
				}
				return ctrl.Result{}, nil
			}
		}
		var ns corev1.Namespace
		if err := r.Get(ctx, client.ObjectKey{Name: finding.Namespace}, &ns); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("fetching namespace %s: %w", finding.Namespace, err)
			}
		} else if domain.ShouldSkip(ns.GetAnnotations(), time.Now()) {
			if r.Log != nil {
				r.Log.Debug("namespace annotation gate: skipping finding",
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("namespace", finding.Namespace),
					zap.String("kind", finding.Kind),
					zap.String("name", finding.Name),
				)
			}
			return ctrl.Result{}, nil
		}
	}

	minSeverity := r.Cfg.MinSeverity
	if minSeverity == "" {
		minSeverity = domain.SeverityLow
	}
	if !domain.MeetsSeverityThreshold(finding.Severity, minSeverity) {
		if r.Log != nil {
			r.Log.Info("finding suppressed",
				zap.Bool("audit", true),
				zap.String("event", "finding.suppressed.min_severity"),
				zap.String("provider", r.Provider.ProviderName()),
				zap.String("kind", finding.Kind),
				zap.String("namespace", finding.Namespace),
				zap.String("severity", string(finding.Severity)),
				zap.String("minSeverity", string(minSeverity)),
			)
		}
		return ctrl.Result{}, nil
	}

	// Self-remediation depth gate — only applies to ChainDepth > 0 findings.
	// Must come after namespace and severity filters and before fingerprinting.
	if finding.ChainDepth > 0 {
		maxDepth := r.Cfg.SelfRemediationMaxDepth
		if maxDepth == 0 || finding.ChainDepth > maxDepth {
			if r.Log != nil {
				r.Log.Warn("self-remediation suppressed",
					zap.Bool("audit", true),
					zap.String("event", "self_remediation.depth_exceeded"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("kind", finding.Kind),
					zap.String("namespace", finding.Namespace),
					zap.String("name", finding.Name),
					zap.Int("chainDepth", finding.ChainDepth),
					zap.Int("maxDepth", maxDepth),
				)
			}
			return ctrl.Result{}, nil
		}

		if r.CircuitBreaker != nil {
			allowed, remaining := r.CircuitBreaker.ShouldAllow()
			if !allowed {
				if r.Log != nil {
					r.Log.Info("self-remediation suppressed by circuit breaker",
						zap.Bool("audit", true),
						zap.String("event", "self_remediation.circuit_breaker"),
						zap.String("provider", r.Provider.ProviderName()),
						zap.String("kind", finding.Kind),
						zap.String("namespace", finding.Namespace),
						zap.String("name", finding.Name),
						zap.Int("chainDepth", finding.ChainDepth),
						zap.Duration("remaining", remaining),
					)
				}
				return ctrl.Result{RequeueAfter: remaining}, nil
			}
		}
	}

	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("computing fingerprint: %w", err)
	}
	if len(fp) < 12 {
		return ctrl.Result{}, fmt.Errorf("fingerprint too short: got %d chars, need at least 12", len(fp))
	}

	if r.Log != nil {
		r.Log.Info("finding detected",
			zap.Bool("audit", true),
			zap.String("event", "finding.detected"),
			zap.String("provider", r.Provider.ProviderName()),
			zap.String("kind", finding.Kind),
			zap.String("namespace", finding.Namespace),
			zap.String("name", finding.Name),
			zap.String("fingerprint", fp[:12]),
		)
	}

	priorityCritical := obj.GetAnnotations()[domain.AnnotationPriority] == "critical"
	if priorityCritical {
		if r.Log != nil {
			r.Log.Info("stabilisation window bypassed by priority annotation",
				zap.Bool("audit", true),
				zap.String("event", "finding.stabilisation_window_bypassed"),
				zap.String("provider", r.Provider.ProviderName()),
				zap.String("fingerprint", fp[:12]),
				zap.String("kind", finding.Kind),
				zap.String("namespace", finding.Namespace),
				zap.String("name", finding.Name),
			)
		}
	}
	if !priorityCritical && r.Cfg.StabilisationWindow != 0 {
		if first, seen := r.firstSeen.Get(fp); !seen {
			r.firstSeen.Set(fp)
			if r.Log != nil {
				r.Log.Info("finding suppressed",
					zap.Bool("audit", true),
					zap.String("event", "finding.suppressed.stabilisation_window"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("fingerprint", fp[:12]),
					zap.String("reason", "first_seen"),
					zap.Duration("window", r.Cfg.StabilisationWindow),
				)
			}
			return ctrl.Result{RequeueAfter: r.Cfg.StabilisationWindow}, nil
		} else {
			elapsed := time.Since(first)
			if elapsed < r.Cfg.StabilisationWindow {
				remaining := r.Cfg.StabilisationWindow - elapsed
				if r.Log != nil {
					r.Log.Info("finding suppressed",
						zap.Bool("audit", true),
						zap.String("event", "finding.suppressed.stabilisation_window"),
						zap.String("provider", r.Provider.ProviderName()),
						zap.String("fingerprint", fp[:12]),
						zap.String("reason", "window_open"),
						zap.Duration("remaining", remaining),
					)
				}
				return ctrl.Result{RequeueAfter: remaining}, nil
			}
		}
	}

	var rjobList v1alpha1.RemediationJobList
	if err := r.List(ctx, &rjobList,
		client.InNamespace(r.Cfg.AgentNamespace),
		client.MatchingLabels{"remediation.mechanic.io/fingerprint": fp[:12]},
	); err != nil {
		return ctrl.Result{}, err
	}
	for i := range rjobList.Items {
		rjob := &rjobList.Items[i]
		if rjob.Spec.Fingerprint != fp {
			continue
		}
		switch rjob.Status.Phase {
		case v1alpha1.PhasePermanentlyFailed:
			if r.Log != nil {
				r.Log.Info("RemediationJob permanently failed; suppressing re-dispatch",
					zap.Bool("audit", true),
					zap.String("event", "remediationjob.permanently_failed_suppressed"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("remediationJob", rjob.Name),
					zap.String("fingerprint", fp[:12]),
				)
			}
			if r.EventRecorder != nil {
				r.EventRecorder.Eventf(obj, corev1.EventTypeWarning, "RemediationJobPermanentlyFailed", "RemediationJob %s is permanently failed after %d retries", rjob.Name, rjob.Status.RetryCount)
			}
			return ctrl.Result{}, nil
		case v1alpha1.PhaseFailed:
			if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
				return ctrl.Result{}, delErr
			}
		default:
			if r.Log != nil {
				r.Log.Info("finding suppressed",
					zap.Bool("audit", true),
					zap.String("event", "finding.suppressed.duplicate"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("fingerprint", fp[:12]),
					zap.String("remediationJob", rjob.Name),
					zap.String("phase", string(rjob.Status.Phase)),
				)
			}
			if r.EventRecorder != nil {
				r.EventRecorder.Eventf(obj, corev1.EventTypeNormal, "DuplicateFingerprint", "Existing RemediationJob %s already covers this finding", rjob.Name)
			}
			return ctrl.Result{}, nil
		}
	}

	// Readiness gate: do not create RemediationJobs until the sink and LLM
	// dependencies are confirmed available. Log at error level and requeue so
	// the finding is re-evaluated once the dependency comes back up.
	if r.ReadinessChecker != nil {
		if err := r.ReadinessChecker.Check(ctx); err != nil {
			if r.Log != nil {
				r.Log.Error("readiness check failed, suppressing RemediationJob creation",
					zap.Bool("audit", true),
					zap.String("event", "readiness.check_failed"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.Error(err),
					zap.String("checker", r.ReadinessChecker.Name()),
					zap.String("fingerprint", fp[:12]),
					zap.String("kind", finding.Kind),
					zap.String("name", finding.Name),
					zap.String("namespace", finding.Namespace),
				)
			}
			return ctrl.Result{RequeueAfter: ReadinessCacheTTL}, nil
		}
	}

	agentSA := r.Cfg.AgentSA
	if r.Cfg.AgentRBACScope == "namespace" {
		agentSA = "mechanic-agent-ns"
	}

	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mechanic-" + fp[:12],
			Namespace: r.Cfg.AgentNamespace,
			Labels: map[string]string{
				"remediation.mechanic.io/fingerprint": fp[:12],
				"app.kubernetes.io/managed-by":        "mechanic-watcher",
			},
			Annotations: map[string]string{
				"remediation.mechanic.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType: r.Provider.ProviderName(),
			SinkType:   r.Cfg.SinkType,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
			Fingerprint: fp,
			Finding: v1alpha1.FindingSpec{
				Kind:         finding.Kind,
				Name:         finding.Name,
				Namespace:    finding.Namespace,
				ParentObject: finding.ParentObject,
				Errors:       finding.Errors,
				Details:      finding.Details,
				ChainDepth:   int32(finding.ChainDepth),
			},
			GitOpsRepo:         r.Cfg.GitOpsRepo,
			GitOpsManifestRoot: r.Cfg.GitOpsManifestRoot,
			AgentImage:         r.Cfg.AgentImage,
			AgentSA:            agentSA,
			MaxRetries:         r.Cfg.MaxInvestigationRetries,
			Severity:           string(finding.Severity),
		},
	}

	if finding.NodeName != "" {
		rjob.Annotations[domain.NodeNameAnnotation] = finding.NodeName
	}

	if err := r.Create(ctx, rjob); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if r.Log != nil {
				r.Log.Info("finding suppressed",
					zap.Bool("audit", true),
					zap.String("event", "finding.suppressed.duplicate"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("fingerprint", fp[:12]),
					zap.String("reason", "create_race"),
				)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating RemediationJob: %w", err)
	}

	if r.Log != nil {
		r.Log.Info("RemediationJob created",
			zap.Bool("audit", true),
			zap.String("event", "remediationjob.created"),
			zap.String("provider", r.Provider.ProviderName()),
			zap.String("fingerprint", fp[:12]),
			zap.String("kind", finding.Kind),
			zap.String("namespace", finding.Namespace),
			zap.String("parentObject", finding.ParentObject),
			zap.String("remediationJob", rjob.Name),
		)
	}
	if r.EventRecorder != nil {
		r.EventRecorder.Eventf(obj, corev1.EventTypeNormal, "FindingDetected", "Provider %s detected %s/%s in namespace %s", r.Provider.ProviderName(), finding.Kind, finding.Name, finding.Namespace)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *SourceProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.Provider.ObjectType()).
		Complete(r)
}

// autoCloseSucceededSinks iterates rjobList (which must cover the full
// AgentNamespace — do not scope it to a single source ref before passing it in,
// the helper filters internally) and calls SinkCloser.Close on every
// PhaseSucceeded job whose SourceResultRef matches sourceRefName/Namespace and
// whose SinkRef.URL is non-empty.
//
// The caller is responsible for guarding with PRAutoClose and SinkCloser nil
// checks before invoking this method.
//
// Succeeded rjobs are intentionally NOT deleted — they are the dedup tombstone.
// Removing them would allow re-dispatch before TTL expires.
//
// Successive calls for the same source ref are idempotent: GitHubSinkCloser
// treats GitHub's 422 (already-closed) as success, so no API spam occurs on
// repeated reconciles.
func (r *SourceProviderReconciler) autoCloseSucceededSinks(
	ctx context.Context,
	sourceRefName, sourceRefNamespace string,
	rjobList *v1alpha1.RemediationJobList,
) {
	for i := range rjobList.Items {
		rjob := &rjobList.Items[i]
		if rjob.Spec.SourceResultRef.Name != sourceRefName ||
			rjob.Spec.SourceResultRef.Namespace != sourceRefNamespace {
			continue
		}
		if rjob.Status.Phase != v1alpha1.PhaseSucceeded || rjob.Status.SinkRef.URL == "" {
			continue
		}
		reason := fmt.Sprintf(
			"Closing automatically: the underlying issue (%s/%s %s) has resolved. No manual fix is required.",
			rjob.Spec.Finding.Kind, rjob.Spec.Finding.Name, rjob.Spec.Finding.Namespace)
		if err := r.SinkCloser.Close(ctx, rjob, reason); err != nil {
			if r.Log != nil {
				r.Log.Error("auto-close succeeded sink failed",
					zap.Bool("audit", true),
					zap.String("event", "sink.close_succeeded_failed"),
					zap.String("remediationJob", rjob.Name),
					zap.String("sinkURL", rjob.Status.SinkRef.URL),
					zap.Error(err),
				)
			}
		}
	}
}
