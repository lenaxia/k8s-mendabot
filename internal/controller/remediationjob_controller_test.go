package controller

import (
	"testing"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// TestRemediationJobReconciler_FieldsCompile verifies that RemediationJobReconciler
// can be constructed with its required fields.
func TestRemediationJobReconciler_FieldsCompile(t *testing.T) {
	r := &RemediationJobReconciler{
		Scheme:     &runtime.Scheme{},
		Log:        zap.NewNop(),
		JobBuilder: nil, // domain.JobBuilder — nil is valid for compile check
		Cfg:        config.Config{},
	}
	if r == nil {
		t.Fatal("expected non-nil RemediationJobReconciler")
	}
	if r.Scheme == nil {
		t.Fatal("expected non-nil Scheme")
	}
}

// TestRemediationJobReconciler_ReconcilePanics verifies that Reconcile panics.
func TestRemediationJobReconciler_ReconcilePanics(t *testing.T) {
	r := &RemediationJobReconciler{}
	defer func() {
		if rec := recover(); rec == nil {
			t.Error("expected panic, got none")
		}
	}()
	req := ctrl.Request{NamespacedName: types.NamespacedName{}}
	_, _ = r.Reconcile(nil, req)
}

// TestRemediationJobReconciler_SetupWithManagerPanics verifies that SetupWithManager panics.
func TestRemediationJobReconciler_SetupWithManagerPanics(t *testing.T) {
	r := &RemediationJobReconciler{}
	defer func() {
		if rec := recover(); rec == nil {
			t.Error("expected panic, got none")
		}
	}()
	_ = r.SetupWithManager(nil)
}

// Compile-time assertion: RemediationJobReconciler holds a domain.JobBuilder field.
var _ domain.JobBuilder = (RemediationJobReconciler{}).JobBuilder
