package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/cascade"
	"github.com/lenaxia/k8s-mendabot/internal/circuitbreaker"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/metrics"
)

// SourceProviderReconciler is a controller-runtime Reconciler that wraps a SourceProvider.
// It handles fetch, skip-if-not-found, ExtractFinding, dedup-by-CRD, and
// RemediationJob creation. Source-specific logic is entirely in the SourceProvider.
type SourceProviderReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Log       *zap.Logger
	Cfg       config.Config
	Provider  domain.SourceProvider
	firstSeen *BoundedMap
	// circuitBreaker implements a persistent, thread-safe circuit breaker
	// to prevent rapid cascades of self-remediations.
	circuitBreaker *circuitbreaker.CircuitBreaker
	// cascadeChecker implements infrastructure cascade detection
	// to suppress findings caused by broader infrastructure issues.
	cascadeChecker cascade.Checker
	// initOnce ensures thread-safe lazy initialization of firstSeen map
	initOnce sync.Once
	// initCascadeOnce ensures thread-safe lazy initialization of cascadeChecker
	initCascadeOnce sync.Once
	// initCBOnce ensures thread-safe lazy initialization of circuitBreaker
	initCBOnce sync.Once
}

// FirstSeen returns the firstSeen map for inspection in tests.
// Do not call this from production code.
func (r *SourceProviderReconciler) FirstSeen() map[string]time.Time {
	r.initFirstSeen()
	return r.firstSeen.Copy()
}

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

// SetFirstSeenForTest sets a firstSeen entry for testing.
// This should only be used in tests.
func (r *SourceProviderReconciler) SetFirstSeenForTest(key string, timestamp time.Time) {
	// Ensure firstSeen is initialized (calls initOnce)
	r.initFirstSeen()
	r.firstSeen.SetForTest(key, timestamp)
}

// Reconcile implements ctrl.Reconciler.
func (r *SourceProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Thread-safe lazy initialization of firstSeen map
	r.initFirstSeen()

	obj := r.Provider.ObjectType().DeepCopyObject().(client.Object)
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		// Watched object was deleted — clear the firstSeen map entirely.
		r.firstSeen.Clear()

		// Cancel any Pending/Dispatched RemediationJobs.
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
			// Cancel Pending, Dispatched, Running, and unset-phase (freshly created) jobs.
			// Succeeded, Failed, and Cancelled jobs are left intact.
			if phase != v1alpha1.PhasePending && phase != v1alpha1.PhaseDispatched &&
				phase != v1alpha1.PhaseRunning && phase != "" {
				continue
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
			}
		}
		if len(cancelErrs) > 0 {
			return ctrl.Result{}, fmt.Errorf("cancelling RemediationJobs: %w", errors.Join(cancelErrs...))
		}
		return ctrl.Result{}, nil
	}

	finding, err := r.Provider.ExtractFinding(obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	if finding == nil {
		r.firstSeen.Clear()
		return ctrl.Result{}, nil
	}

	// Cascade checker for infrastructure failures
	r.initCascadeOnce.Do(func() {
		cascadeCfg := cascade.Config{
			Enabled:                 !r.Cfg.DisableCascadeCheck,
			NamespaceFailurePercent: r.Cfg.CascadeNamespaceThreshold,
			NodeCacheTTL:            r.Cfg.CascadeNodeCacheTTL,
		}
		checker, err := cascade.NewChecker(cascadeCfg)
		if err != nil {
			r.Log.Error("failed to create cascade checker", zap.Error(err))
			return
		}
		r.cascadeChecker = checker
	})
	if r.cascadeChecker != nil {
		suppress, reason, err := r.cascadeChecker.ShouldSuppress(ctx, finding, r.Client)
		if err != nil {
			r.Log.Error("cascade check error", zap.Error(err))
			// Continue with investigation rather than fail
		} else if suppress {
			r.Log.Info("suppressing finding due to cascade",
				zap.String("reason", reason),
				zap.String("kind", finding.Kind),
				zap.String("namespace", finding.Namespace),
			)
			// Record metrics for cascade suppression
			metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "infrastructure_cascade")
			metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "infrastructure_issue", reason)
			return ctrl.Result{}, nil
		}
	}

	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("computing fingerprint: %w", err)
	}
	if len(fp) < 12 {
		return ctrl.Result{}, fmt.Errorf("fingerprint too short: got %d chars, need at least 12", len(fp))
	}

	// Circuit breaker for self-remediations
	if finding.IsSelfRemediation {
		// Initialize circuit breaker if not already initialized
		r.initCBOnce.Do(func() {
			r.circuitBreaker = circuitbreaker.New(r.Client, r.Cfg.AgentNamespace, r.Cfg.SelfRemediationCooldown)
		})

		allowed, remaining, err := r.circuitBreaker.ShouldAllow(ctx)
		if err != nil {
			r.Log.Error("circuit breaker error", zap.Error(err))
			return ctrl.Result{}, fmt.Errorf("circuit breaker error: %w", err)
		}

		if !allowed {
			if r.Log != nil {
				r.Log.Info("circuit breaker: skipping self-remediation due to cooldown",
					zap.String("fingerprint", fp[:12]),
					zap.Duration("remaining", remaining),
					zap.Int("chainDepth", finding.ChainDepth),
				)
			}
			// Record metrics for circuit breaker activation
			metrics.RecordCircuitBreakerActivation(r.Provider.ProviderName(), finding.Namespace)
			metrics.SetCircuitBreakerCooldown(r.Provider.ProviderName(), finding.Namespace, remaining.Seconds())
			metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "circuit_breaker")
			metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "cooldown_active",
				fmt.Sprintf("Circuit breaker cooldown active: %v remaining", remaining))
			return ctrl.Result{RequeueAfter: remaining}, nil
		}

		// Log cascade warning for deep chains
		if finding.ChainDepth > 1 {
			if r.Log != nil {
				r.Log.Warn("deep cascade detected in self-remediation",
					zap.String("fingerprint", fp[:12]),
					zap.Int("chainDepth", finding.ChainDepth),
					zap.String("findingName", finding.Name),
				)
			}
			// Record chain depth metrics
			metrics.RecordChainDepth(r.Provider.ProviderName(), finding.Namespace, finding.ChainDepth)

			// Record max depth exceeded if chain is at or beyond configured max depth
			if finding.ChainDepth >= r.Cfg.SelfRemediationMaxDepth {
				metrics.RecordMaxDepthExceeded(r.Provider.ProviderName(), finding.Namespace, finding.ChainDepth)
				metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "max_depth")
				metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "chain_too_deep",
					fmt.Sprintf("Chain depth %d exceeds maximum recommended depth", finding.ChainDepth))
			}
		}
	}

	// Stabilisation window logic.
	if r.Cfg.StabilisationWindow == 0 {
		// fast path: no window, proceed directly to dedup + Job creation
	} else {
		// window logic: consult firstSeen map
		if first, seen := r.firstSeen.Get(fp); !seen {
			r.firstSeen.Set(fp)
			// Record stabilisation window start
			if finding.IsSelfRemediation {
				metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "stabilisation_window")
				metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "stabilisation_start",
					"Starting stabilisation window for self-remediation")
			}
			return ctrl.Result{RequeueAfter: r.Cfg.StabilisationWindow}, nil
		} else {
			elapsed := time.Since(first)
			if elapsed < r.Cfg.StabilisationWindow {
				remaining := r.Cfg.StabilisationWindow - elapsed
				// Record stabilisation window suppression
				if finding.IsSelfRemediation {
					metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "stabilisation_window")
					metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "window_active",
						fmt.Sprintf("Stabilisation window active: %v remaining", remaining))
				}
				return ctrl.Result{RequeueAfter: remaining}, nil
			}
			// Window has elapsed — fall through to dedup + Job creation.
			// Leave the firstSeen entry so repeated reconciles after Job creation
			// do not restart the window.
		}
	}

	var rjobList v1alpha1.RemediationJobList
	if err := r.List(ctx, &rjobList,
		client.InNamespace(r.Cfg.AgentNamespace),
		client.MatchingLabels{"remediation.mendabot.io/fingerprint": fp[:12]},
	); err != nil {
		return ctrl.Result{}, err
	}
	for i := range rjobList.Items {
		rjob := &rjobList.Items[i]
		if rjob.Spec.Fingerprint != fp {
			continue
		}
		if rjob.Status.Phase != v1alpha1.PhaseFailed {
			return ctrl.Result{}, nil
		}
		// Failed RemediationJob with the same fingerprint — delete it so a new
		// investigation can be dispatched.
		if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
			return ctrl.Result{}, delErr
		}
	}

	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: r.Cfg.AgentNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/fingerprint": fp[:12],
				"app.kubernetes.io/managed-by":        "mendabot-watcher",
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": fp,
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
			},
			GitOpsRepo:         r.Cfg.GitOpsRepo,
			GitOpsManifestRoot: r.Cfg.GitOpsManifestRoot,
			AgentImage:         r.Cfg.AgentImage,
			AgentSA:            r.Cfg.AgentSA,
			IsSelfRemediation:  finding.IsSelfRemediation,
			ChainDepth:         finding.ChainDepth,
		},
	}

	if err := r.Create(ctx, rjob); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating RemediationJob: %w", err)
	}

	if r.Log != nil {
		r.Log.Info("created RemediationJob",
			zap.String("fingerprint", fp[:12]),
			zap.String("kind", finding.Kind),
			zap.String("parentObject", finding.ParentObject),
			zap.String("remediationJob", rjob.Name),
		)
	}

	// Record metrics for remediation attempt
	if finding.IsSelfRemediation {
		// Clear circuit breaker cooldown since we allowed this remediation.
		// The actual attempt outcome (success/failure) is recorded in the
		// RemediationJob controller when the job completes.
		metrics.ClearCircuitBreakerCooldown(r.Provider.ProviderName(), finding.Namespace)
	} else {
		// For regular remediations, record chain depth if applicable
		if finding.ChainDepth > 0 {
			metrics.RecordChainDepth(r.Provider.ProviderName(), finding.Namespace, finding.ChainDepth)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *SourceProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.Provider.ObjectType()).
		Complete(r)
}
