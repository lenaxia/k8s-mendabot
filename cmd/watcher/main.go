package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
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

	gozapr "github.com/go-logr/zapr"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/circuitbreaker"
	"github.com/lenaxia/k8s-mechanic/internal/config"
	"github.com/lenaxia/k8s-mechanic/internal/controller"
	"github.com/lenaxia/k8s-mechanic/internal/correlator"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
	igithub "github.com/lenaxia/k8s-mechanic/internal/github"
	"github.com/lenaxia/k8s-mechanic/internal/jobbuilder"
	"github.com/lenaxia/k8s-mechanic/internal/logging"
	"github.com/lenaxia/k8s-mechanic/internal/provider"
	"github.com/lenaxia/k8s-mechanic/internal/provider/native"
	"github.com/lenaxia/k8s-mechanic/internal/readiness"
	"github.com/lenaxia/k8s-mechanic/internal/readiness/llm"
	"github.com/lenaxia/k8s-mechanic/internal/readiness/sink"
	sinkhub "github.com/lenaxia/k8s-mechanic/internal/sink/github"
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

	ctrl.SetLogger(gozapr.NewLogger(logger))

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
	restCfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(restCfg, opts)
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

	if cfg.DryRun {
		logger.Info("dry-run mode enabled — agent Jobs will not create PRs; investigation reports stored in status.message",
			zap.Bool("dryRun", true),
		)
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

	corr, err := buildCorrelator(cfg)
	if err != nil {
		logger.Fatal("buildCorrelator failed", zap.Error(err))
	}
	if corr == nil {
		logger.Info("multi-signal correlation disabled (DISABLE_CORRELATION=true)")
	}

	if err := (&controller.RemediationJobReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Log:        logger,
		JobBuilder: jb,
		Cfg:        cfg,
		Recorder:   mgr.GetEventRecorderFor("mechanic-watcher"),
		APIReader:  mgr.GetAPIReader(),
		Correlator: corr,
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

	// Wire GitHub App token provider and sink closer.
	// secretKeyRef entries on the Deployment (conditional on prAutoClose) guarantee these vars are present when the pod starts.
	// If PRAutoClose is false, use NoopSinkCloser — no credentials needed.
	var sinkCloser domain.SinkCloser
	if cfg.PRAutoClose {
		appID, err := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
		if err != nil || appID <= 0 {
			logger.Fatal("GITHUB_APP_ID is missing or invalid; cannot start with PR_AUTO_CLOSE=true",
				zap.Error(err))
		}
		installID, err := strconv.ParseInt(os.Getenv("GITHUB_APP_INSTALLATION_ID"), 10, 64)
		if err != nil || installID <= 0 {
			logger.Fatal("GITHUB_APP_INSTALLATION_ID is missing or invalid; cannot start with PR_AUTO_CLOSE=true",
				zap.Error(err))
		}
		privKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(os.Getenv("GITHUB_APP_PRIVATE_KEY")))
		if err != nil {
			logger.Fatal("GITHUB_APP_PRIVATE_KEY is missing or invalid; cannot start with PR_AUTO_CLOSE=true",
				zap.Error(err))
		}
		sinkCloser = &sinkhub.GitHubSinkCloser{
			TokenProvider: &igithub.GitHubAppTokenProvider{
				AppID:          appID,
				InstallationID: installID,
				PrivateKey:     privKey,
			},
		}
		logger.Info("auto-close sink enabled", zap.Bool("prAutoClose", true))
	} else {
		sinkCloser = domain.NoopSinkCloser{}
		logger.Info("auto-close sink disabled (PR_AUTO_CLOSE=false)")
	}

	for _, p := range enabledProviders {
		if err := (&provider.SourceProviderReconciler{
			Client:           mgr.GetClient(),
			Scheme:           mgr.GetScheme(),
			Log:              logger,
			Cfg:              cfg,
			Provider:         p,
			EventRecorder:    mgr.GetEventRecorderFor("mechanic-watcher"),
			ReadinessChecker: combinedChecker,
			CircuitBreaker:   cb,
			SinkCloser:       sinkCloser,
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

// buildCorrelator constructs a Correlator with all built-in rules, or returns nil
// when DisableCorrelation is set. A nil Correlator causes the reconciler to skip
// the window hold and dispatch immediately.
// Returns an error if cfg.MultiPodThreshold is <= 0.
//
// Rule priority (first match wins):
//  1. PVCPodRule — a PVC failure and the pod that depends on it share a root cause;
//     the PVC is always primary. Checked before SameNamespaceParentRule because a PVC
//     finding and its dependent pod often also share a ParentObject prefix, and the
//     PVC-specific grouping is more precise than the generic parent-prefix match.
//  2. SameNamespaceParentRule — cross-provider findings for the same application
//     (same namespace, parent-name prefix relationship).
//  3. MultiPodSameNodeRule — multiple pods failing on the same node (node is root cause).
func buildCorrelator(cfg config.Config) (*correlator.Correlator, error) {
	if cfg.DisableCorrelation {
		// Emit nothing here — caller logs using the configured logger.
		return nil, nil
	}
	if cfg.MultiPodThreshold <= 0 {
		return nil, fmt.Errorf("buildCorrelator: MultiPodThreshold must be > 0, got %d (check CORRELATION_MULTI_POD_THRESHOLD)", cfg.MultiPodThreshold)
	}
	return &correlator.Correlator{
		Rules: []domain.CorrelationRule{
			correlator.PVCPodRule{},
			correlator.SameNamespaceParentRule{},
			correlator.MultiPodSameNodeRule{Threshold: cfg.MultiPodThreshold},
		},
	}, nil
}
