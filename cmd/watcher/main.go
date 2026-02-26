package main

import (
	"fmt"
	"log"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"go.uber.org/zap"
	zapr "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/circuitbreaker"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/controller"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/jobbuilder"
	"github.com/lenaxia/k8s-mendabot/internal/logging"
	"github.com/lenaxia/k8s-mendabot/internal/provider"
	"github.com/lenaxia/k8s-mendabot/internal/provider/native"
	"github.com/lenaxia/k8s-mendabot/internal/readiness"
	"github.com/lenaxia/k8s-mendabot/internal/readiness/llm"
	"github.com/lenaxia/k8s-mendabot/internal/readiness/sink"
)

// Version is embedded at build time via ldflags:
//
//	-X main.Version=sha-<commit>
//
// It defaults to "dev" for local builds.
var Version = "dev"

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Println(Version)
			os.Exit(0)
		}
	}

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		log.Fatalf("logger init failed: %v", err)
	}
	defer logger.Sync() //nolint:errcheck

	ctrl.SetLogger(zapr.New())

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		logger.Fatal("failed to add client-go scheme", zap.Error(err))
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		logger.Fatal("failed to add batchv1 scheme", zap.Error(err))
	}
	if err := v1alpha1.AddRemediationToScheme(scheme); err != nil {
		logger.Fatal("failed to add v1alpha1 remediation scheme", zap.Error(err))
	}

	opts := ctrl.Options{
		Scheme:                  scheme,
		LeaderElection:          false,
		Metrics:                 metricsserver.Options{BindAddress: ":8080"},
		HealthProbeBindAddress:  ":8081",
		GracefulShutdownTimeout: func() *time.Duration { d := 30 * time.Second; return &d }(),
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Namespaces: map[string]cache.Config{
						cfg.AgentNamespace: {},
					},
				},
			},
		},
	}
	if len(cfg.AgentWatchNamespaces) > 0 {
		defaultNS := make(map[string]cache.Config)
		for _, ns := range cfg.AgentWatchNamespaces {
			defaultNS[ns] = cache.Config{}
		}
		opts.Cache.DefaultNamespaces = defaultNS
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts)
	if err != nil {
		log.Fatalf("unable to start manager: %v", err)
	}

	jb, err := jobbuilder.New(jobbuilder.Config{
		AgentNamespace: cfg.AgentNamespace,
		AgentType:      cfg.AgentType,
		TTLSeconds:     int32(cfg.RemediationJobTTLSeconds),
		DryRun:         cfg.DryRun,
	})
	if err != nil {
		logger.Fatal("jobbuilder init failed", zap.Error(err))
	}

	// Build the readiness checker that gates RemediationJob creation.
	// The sink checker is selected by SINK_TYPE; unset/unknown = NopChecker.
	// The LLM checker is selected by LLM_PROVIDER; unset = NopChecker (disabled).
	// provider.ReadinessCacheTTL is used for both the cache TTL and the requeue
	// interval on failure, ensuring the cache is always expired before retry.

	var sinkChecker readiness.Checker
	switch cfg.SinkType {
	case "github":
		sinkChecker = readiness.NewCachedChecker(
			sink.NewGitHubAppChecker(mgr.GetClient(), cfg.AgentNamespace),
			provider.ReadinessCacheTTL,
		)
	default:
		sinkChecker = readiness.NewNopChecker("sink")
		logger.Info("no readiness checker for sink type; sink check disabled",
			zap.String("sinkType", cfg.SinkType))
	}

	var llmChecker readiness.Checker
	switch cfg.LLMProvider {
	case "openai":
		llmChecker = readiness.NewCachedChecker(
			llm.NewOpenAIChecker(mgr.GetClient(), cfg.AgentNamespace, string(cfg.AgentType)),
			provider.ReadinessCacheTTL,
		)
	default:
		llmChecker = readiness.NewNopChecker("llm")
		logger.Info("LLM_PROVIDER not set; LLM readiness check disabled")
	}

	combinedChecker := readiness.All(sinkChecker, llmChecker)

	if err := (&controller.RemediationJobReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Log:        logger,
		JobBuilder: jb,
		Cfg:        cfg,
		Recorder:   mgr.GetEventRecorderFor("mendabot-watcher"),
	}).SetupWithManager(mgr); err != nil {
		logger.Fatal("RemediationJobReconciler setup failed", zap.Error(err))
	}

	nativeClient := mgr.GetClient()

	// Construct circuit breaker once; shared across all providers.
	// nil when cooldown is 0 (disabled).
	var cb circuitbreaker.Gater
	if cfg.SelfRemediationCooldown > 0 {
		cb = circuitbreaker.New(cfg.SelfRemediationCooldown)
	}

	enabledProviders := []domain.SourceProvider{
		native.NewPodProvider(nativeClient),
		native.NewDeploymentProvider(nativeClient),
		native.NewPVCProvider(nativeClient),
		native.NewNodeProvider(nativeClient),
		native.NewStatefulSetProvider(nativeClient),
		native.NewJobProvider(nativeClient),
	}
	for _, p := range enabledProviders {
		if err := (&provider.SourceProviderReconciler{
			Client:           mgr.GetClient(),
			Scheme:           mgr.GetScheme(),
			Log:              logger,
			Cfg:              cfg,
			Provider:         p,
			EventRecorder:    mgr.GetEventRecorderFor("mendabot-watcher"),
			ReadinessChecker: combinedChecker,
			CircuitBreaker:   cb,
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
