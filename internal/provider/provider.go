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
	Scheme          *runtime.Scheme
	Log             *zap.Logger
	Cfg             config.Config
	Provider        domain.SourceProvider
	EventRecorder   record.EventRecorder
	firstSeen       *BoundedMap
	circuitBreaker  *circuitbreaker.CircuitBreaker
	cascadeChecker  cascade.Checker
	initOnce        sync.Once
	initCascadeOnce sync.Once
	initCBOnce      sync.Once
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
			} else if r.Log != nil {
				r.Log.Info("RemediationJob cancelled",
					zap.Bool("audit", true),
					zap.String("event", "remediationjob.cancelled"),
					zap.String("remediationJob", rjob.Name),
					zap.String("reason", "source_deleted"),
					zap.String("sourceRef", req.Name),
				)
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

	r.initCascadeOnce.Do(func() {
		cascadeCfg := cascade.Config{
			Enabled:                 !r.Cfg.DisableCascadeCheck,
			NamespaceFailurePercent: r.Cfg.CascadeNamespaceThreshold,
			NodeCacheTTL:            r.Cfg.CascadeNodeCacheTTL,
		}
		checker, err := cascade.NewChecker(cascadeCfg)
		if err != nil {
			if r.Log != nil {
				r.Log.Error("failed to create cascade checker", zap.Error(err))
			}
			return
		}
		r.cascadeChecker = checker
	})
	if r.cascadeChecker != nil {
		suppress, reason, err := r.cascadeChecker.ShouldSuppress(ctx, finding, r.Client)
		if err != nil {
			if r.Log != nil {
				r.Log.Error("cascade check error", zap.Error(err))
			}
		} else if suppress {
			if r.Log != nil {
				r.Log.Info("finding suppressed",
					zap.Bool("audit", true),
					zap.String("event", "finding.suppressed.cascade"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("kind", finding.Kind),
					zap.String("namespace", finding.Namespace),
					zap.String("reason", reason),
				)
			}
			metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "infrastructure_cascade")
			metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "infrastructure_issue", reason)
			if r.EventRecorder != nil {
				r.EventRecorder.Event(obj, corev1.EventTypeWarning, "InfrastructureCascadeSuppressed",
					fmt.Sprintf("finding suppressed: %s (kind: %s, namespace: %s)", reason, finding.Kind, finding.Namespace))
			}
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

	if finding.IsSelfRemediation {
		r.initCBOnce.Do(func() {
			r.circuitBreaker = circuitbreaker.New(r.Client, r.Cfg.AgentNamespace, r.Cfg.SelfRemediationCooldown)
		})

		allowed, remaining, err := r.circuitBreaker.ShouldAllow(ctx)
		if err != nil {
			if r.Log != nil {
				r.Log.Error("circuit breaker error", zap.Error(err))
			}
			return ctrl.Result{}, fmt.Errorf("circuit breaker error: %w", err)
		}

		if !allowed {
			if r.Log != nil {
				r.Log.Info("finding suppressed",
					zap.Bool("audit", true),
					zap.String("event", "finding.suppressed.circuit_breaker"),
					zap.String("provider", r.Provider.ProviderName()),
					zap.String("namespace", finding.Namespace),
					zap.Duration("cooldownRemaining", remaining),
					zap.Int("chainDepth", finding.ChainDepth),
				)
			}
			metrics.RecordCircuitBreakerActivation(r.Provider.ProviderName(), finding.Namespace)
			metrics.SetCircuitBreakerCooldown(r.Provider.ProviderName(), finding.Namespace, remaining.Seconds())
			metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "circuit_breaker")
			metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "cooldown_active",
				fmt.Sprintf("Circuit breaker cooldown active: %v remaining", remaining))
			if r.EventRecorder != nil {
				r.EventRecorder.Event(obj, corev1.EventTypeWarning, "CircuitBreakerOpened",
					fmt.Sprintf("self-remediation suppressed: circuit breaker active, %v remaining (fingerprint: %s)", remaining, fp[:12]))
			}
			return ctrl.Result{RequeueAfter: remaining}, nil
		}

		if finding.ChainDepth > 1 {
			if r.Log != nil {
				r.Log.Warn("deep cascade detected in self-remediation",
					zap.String("fingerprint", fp[:12]),
					zap.Int("chainDepth", finding.ChainDepth),
					zap.String("findingName", finding.Name),
				)
			}
			metrics.RecordChainDepth(r.Provider.ProviderName(), finding.Namespace, finding.ChainDepth)
			if r.EventRecorder != nil {
				r.EventRecorder.Event(obj, corev1.EventTypeWarning, "DeepCascadeDetected",
					fmt.Sprintf("deep cascade at chain depth %d (fingerprint: %s)", finding.ChainDepth, fp[:12]))
			}

			if finding.ChainDepth >= r.Cfg.SelfRemediationMaxDepth {
				metrics.RecordMaxDepthExceeded(r.Provider.ProviderName(), finding.Namespace, finding.ChainDepth)
				metrics.RecordCascadeSuppression(r.Provider.ProviderName(), finding.Namespace, "max_depth")
				metrics.RecordCascadeSuppressionReason(r.Provider.ProviderName(), finding.Namespace, "chain_too_deep",
					fmt.Sprintf("Chain depth %d exceeds maximum recommended depth", finding.ChainDepth))
				if r.Log != nil {
					r.Log.Info("finding suppressed",
						zap.Bool("audit", true),
						zap.String("event", "finding.suppressed.max_depth"),
						zap.String("provider", r.Provider.ProviderName()),
						zap.String("namespace", finding.Namespace),
						zap.Int("chainDepth", finding.ChainDepth),
						zap.Int("maxDepth", r.Cfg.SelfRemediationMaxDepth),
					)
				}
				return ctrl.Result{}, nil
			}
		}
	}

	if r.Cfg.StabilisationWindow == 0 {
	} else {
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

	agentSA := r.Cfg.AgentSA
	if r.Cfg.AgentRBACScope == "namespace" {
		agentSA = "mendabot-agent-ns"
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
			AgentSA:            agentSA,
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
		r.Log.Info("RemediationJob created",
			zap.Bool("audit", true),
			zap.String("event", "remediationjob.created"),
			zap.String("provider", r.Provider.ProviderName()),
			zap.String("fingerprint", fp[:12]),
			zap.String("kind", finding.Kind),
			zap.String("namespace", finding.Namespace),
			zap.String("parentObject", finding.ParentObject),
			zap.String("remediationJob", rjob.Name),
			zap.Bool("isSelfRemediation", finding.IsSelfRemediation),
		)
	}

	if finding.IsSelfRemediation {
		metrics.ClearCircuitBreakerCooldown(r.Provider.ProviderName(), finding.Namespace)
	} else {
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
