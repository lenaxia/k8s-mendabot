package provider

import (
	"testing"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

func zeroRequest() ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{}}
}

// TestSourceProviderReconciler_FieldsCompile verifies that SourceProviderReconciler
// can be constructed with its required fields, which ensures the struct definition
// matches the expected shape.
func TestSourceProviderReconciler_FieldsCompile(t *testing.T) {
	r := &SourceProviderReconciler{
		Scheme:   &runtime.Scheme{},
		Log:      zap.NewNop(),
		Cfg:      config.Config{},
		Provider: nil, // domain.SourceProvider — nil is valid for compile check
	}
	if r == nil {
		t.Fatal("expected non-nil SourceProviderReconciler")
	}
	if r.Scheme == nil {
		t.Fatal("expected non-nil Scheme")
	}
}

// TestSourceProviderReconciler_ReconcilePanics verifies that Reconcile panics
// with "not implemented" as required by the skeleton contract.
func TestSourceProviderReconciler_ReconcilePanics(t *testing.T) {
	r := &SourceProviderReconciler{}
	defer func() {
		if rec := recover(); rec == nil {
			t.Error("expected panic, got none")
		}
	}()
	_, _ = r.Reconcile(nil, zeroRequest())
}

// TestSourceProviderReconciler_SetupWithManagerPanics verifies that SetupWithManager
// panics with "not implemented" as required by the skeleton contract.
func TestSourceProviderReconciler_SetupWithManagerPanics(t *testing.T) {
	r := &SourceProviderReconciler{}
	defer func() {
		if rec := recover(); rec == nil {
			t.Error("expected panic, got none")
		}
	}()
	_ = r.SetupWithManager(nil)
}

// Compile-time assertion: SourceProviderReconciler holds a domain.SourceProvider field.
var _ domain.SourceProvider = (SourceProviderReconciler{}).Provider
