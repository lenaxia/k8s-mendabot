package controller

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// RemediationJobReconciler watches RemediationJob objects and drives the Job lifecycle.
// It is provider-agnostic — it acts on all RemediationJob objects regardless of source.
type RemediationJobReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Log        *zap.Logger
	JobBuilder domain.JobBuilder
	Cfg        config.Config
}

// Reconcile implements ctrl.Reconciler.
func (r *RemediationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	panic("not implemented")
}

// SetupWithManager registers the reconciler with the controller manager.
func (r *RemediationJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	panic("not implemented")
}
