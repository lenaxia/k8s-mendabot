package main

import (
	"log"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"go.uber.org/zap"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/controller"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/jobbuilder"
	"github.com/lenaxia/k8s-mendabot/internal/logging"
	"github.com/lenaxia/k8s-mendabot/internal/provider"
	k8sgpt "github.com/lenaxia/k8s-mendabot/internal/provider/k8sgpt"
)

// Version is embedded at build time via ldflags:
//
//	-X main.Version=sha-<commit>
//
// It defaults to "dev" for local builds.
var Version = "dev"

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		log.Fatalf("logger init failed: %v", err)
	}
	defer logger.Sync() //nolint:errcheck

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		logger.Fatal("failed to add client-go scheme", zap.Error(err))
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		logger.Fatal("failed to add batchv1 scheme", zap.Error(err))
	}
	if err := v1alpha1.AddResultToScheme(scheme); err != nil {
		logger.Fatal("failed to add v1alpha1 result scheme", zap.Error(err))
	}
	if err := v1alpha1.AddRemediationToScheme(scheme); err != nil {
		logger.Fatal("failed to add v1alpha1 remediation scheme", zap.Error(err))
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		LeaderElection:         false,
		Metrics:                metricsserver.Options{BindAddress: ":8080"},
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		log.Fatalf("unable to start manager: %v", err)
	}

	jb, err := jobbuilder.New(jobbuilder.Config{
		AgentNamespace: cfg.AgentNamespace,
	})
	if err != nil {
		logger.Fatal("jobbuilder init failed", zap.Error(err))
	}

	if err := (&controller.RemediationJobReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Log:        logger,
		JobBuilder: jb,
		Cfg:        cfg,
	}).SetupWithManager(mgr); err != nil {
		logger.Fatal("RemediationJobReconciler setup failed", zap.Error(err))
	}

	enabledProviders := []domain.SourceProvider{
		&k8sgpt.K8sGPTProvider{},
	}
	for _, p := range enabledProviders {
		if err := (&provider.SourceProviderReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Log:      logger,
			Cfg:      cfg,
			Provider: p,
		}).SetupWithManager(mgr); err != nil {
			logger.Fatal("provider setup failed", zap.Error(err))
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatalf("unable to set up health check: %v", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatalf("unable to set up ready check: %v", err)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatalf("problem running manager: %v", err)
	}
}
