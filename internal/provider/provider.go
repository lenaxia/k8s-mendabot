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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/readiness"
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
	firstSeen        *BoundedMap
	initOnce         sync.Once
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
			// Cancel Pending, Dispatched, and Running jobs. Also cancel Phase==""
			// (blank) because there is a real race window between client.Create()
			// and the RemediationJobReconciler's first reconcile that transitions
			// "" → Pending. A source deletion arriving in that window must still
			// cancel the job. Do NOT remove the phase != "" check even though the
			// controller now initialises phase immediately — the race window exists
			// and removing this will silently reintroduce the bug.
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

	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("computing fingerprint: %w", err)
	}
	if len(fp) < 12 {
		return ctrl.Result{}, fmt.Errorf("fingerprint too short: got %d chars, need at least 12", len(fp))
	}

	if r.Cfg.StabilisationWindow != 0 {
		if first, seen := r.firstSeen.Get(fp); !seen {
			r.firstSeen.Set(fp)
			return ctrl.Result{RequeueAfter: r.Cfg.StabilisationWindow}, nil
		} else {
			elapsed := time.Since(first)
			if elapsed < r.Cfg.StabilisationWindow {
				remaining := r.Cfg.StabilisationWindow - elapsed
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

	// Readiness gate: do not create RemediationJobs until the sink and LLM
	// dependencies are confirmed available. Log at error level and requeue so
	// the finding is re-evaluated once the dependency comes back up.
	if r.ReadinessChecker != nil {
		if err := r.ReadinessChecker.Check(ctx); err != nil {
			if r.Log != nil {
				r.Log.Error("readiness check failed, suppressing RemediationJob creation",
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
		)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *SourceProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.Provider.ObjectType()).
		Complete(r)
}
