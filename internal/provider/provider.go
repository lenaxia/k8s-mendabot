package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// SourceProviderReconciler is a controller-runtime Reconciler that wraps a SourceProvider.
// It handles fetch, skip-if-not-found, ExtractFinding, dedup-by-CRD, and
// RemediationJob creation. Source-specific logic is entirely in the SourceProvider.
//
// firstSeen is not mutex-protected: controller-runtime guarantees a single
// worker goroutine per controller (MaxConcurrentReconciles defaults to 1).
// If MaxConcurrentReconciles is ever set > 1 for this reconciler, replace
// this map with a sync.Map or add a sync.Mutex.
type SourceProviderReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Log       *zap.Logger
	Cfg       config.Config
	Provider  domain.SourceProvider
	firstSeen map[string]time.Time
}

// FirstSeen returns the firstSeen map for inspection in tests.
// Do not call this from production code.
func (r *SourceProviderReconciler) FirstSeen() map[string]time.Time {
	if r.firstSeen == nil {
		r.firstSeen = make(map[string]time.Time)
	}
	return r.firstSeen
}

// Reconcile implements ctrl.Reconciler.
func (r *SourceProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.firstSeen == nil {
		r.firstSeen = make(map[string]time.Time)
	}

	obj := r.Provider.ObjectType().DeepCopyObject().(client.Object)
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		// Watched object was deleted — clear the firstSeen map entirely.
		r.firstSeen = make(map[string]time.Time)

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
		r.firstSeen = make(map[string]time.Time)
		return ctrl.Result{}, nil
	}

	fp, err := domain.FindingFingerprint(finding)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("computing fingerprint: %w", err)
	}
	if len(fp) < 12 {
		return ctrl.Result{}, fmt.Errorf("fingerprint too short: got %d chars, need at least 12", len(fp))
	}

	// Stabilisation window logic.
	if r.Cfg.StabilisationWindow == 0 {
		// fast path: no window, proceed directly to dedup + Job creation
	} else {
		// window logic: consult firstSeen map
		if first, seen := r.firstSeen[fp]; !seen {
			r.firstSeen[fp] = time.Now()
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

	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *SourceProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.Provider.ObjectType()).
		Complete(r)
}
