package provider

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// SourceProviderReconciler is a controller-runtime Reconciler that wraps a SourceProvider.
// It handles fetch, skip-if-not-found, ExtractFinding, Fingerprint, dedup-by-CRD, and
// RemediationJob creation. Source-specific logic is entirely in the SourceProvider.
type SourceProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      *zap.Logger
	Cfg      config.Config
	Provider domain.SourceProvider
}

// Reconcile implements ctrl.Reconciler.
func (r *SourceProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	panic("not implemented")
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *SourceProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	panic("not implemented")
}
