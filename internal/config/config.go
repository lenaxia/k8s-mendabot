package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// AgentType identifies which agent runner binary the watcher dispatches.
type AgentType string

const (
	AgentTypeOpenCode AgentType = "opencode"
	AgentTypeClaude   AgentType = "claude"
)

// Config holds all runtime configuration for the mendabot-watcher controller.
// All fields are populated from environment variables at startup via FromEnv.
type Config struct {
	GitOpsRepo                string        // GITOPS_REPO — required
	GitOpsManifestRoot        string        // GITOPS_MANIFEST_ROOT — required
	AgentImage                string        // AGENT_IMAGE — required
	AgentNamespace            string        // AGENT_NAMESPACE — required; must equal watcher namespace
	AgentSA                   string        // AGENT_SA — required
	AgentType                 AgentType     // AGENT_TYPE — default "opencode"
	SinkType                  string        // SINK_TYPE — default "github"
	LogLevel                  string        // LOG_LEVEL — default "info"
	MaxConcurrentJobs         int           // MAX_CONCURRENT_JOBS — default 3
	RemediationJobTTLSeconds  int           // REMEDIATION_JOB_TTL_SECONDS — default 604800 (7 days)
	StabilisationWindow       time.Duration // STABILISATION_WINDOW_SECONDS — default 120s; 0 disables
	DisableCascadeCheck       bool          // DISABLE_CASCADE_CHECK — default false
	CascadeNamespaceThreshold int           // CASCADE_NAMESPACE_THRESHOLD — default 50
	CascadeNodeCacheTTL       time.Duration // CASCADE_NODE_CACHE_TTL_SECONDS — default 30s
	// LLMProvider selects the LLM readiness checker used to gate RemediationJob
	// creation. Accepted values: "openai". Empty (default) disables the check.
	LLMProvider              string   // LLM_PROVIDER — default "" (disabled)
	InjectionDetectionAction string   // INJECTION_DETECTION_ACTION — "log" (default) or "suppress"
	AgentRBACScope           string   // AGENT_RBAC_SCOPE — "cluster" (default) or "namespace"
	AgentWatchNamespaces     []string // AGENT_WATCH_NAMESPACES — required when scope is "namespace"
	WatchNamespaces          []string // WATCH_NAMESPACES — default nil (allow all)
	ExcludeNamespaces        []string // EXCLUDE_NAMESPACES — default nil (deny none)
	// MaxInvestigationRetries is the maximum number of times a RemediationJob's
	// owned batch/v1 Job may fail before the RemediationJob is permanently
	// tombstoned. Populated from MAX_INVESTIGATION_RETRIES env var; default 3.
	MaxInvestigationRetries int32 // MAX_INVESTIGATION_RETRIES — default 3
	// MinSeverity is the minimum severity level for which a RemediationJob is created.
	// Findings below this threshold are silently dropped.
	// Default: domain.SeverityLow (all findings pass).
	MinSeverity domain.Severity // MIN_SEVERITY — default "low"

	// SelfRemediationMaxDepth is the maximum allowed self-remediation chain depth.
	// A Finding with ChainDepth > SelfRemediationMaxDepth is suppressed.
	// 0 disables self-remediation entirely. Default: 2.
	SelfRemediationMaxDepth int // SELF_REMEDIATION_MAX_DEPTH — default 2; 0 = disabled

	// SelfRemediationCooldown is the minimum time between allowed self-remediations.
	// 0 disables the circuit breaker. Default: 300s.
	SelfRemediationCooldown time.Duration // SELF_REMEDIATION_COOLDOWN_SECONDS — default 300s; 0 = disabled

	// DRY_RUN — default false; set "true" or "1" to enable dry-run mode
	DryRun bool

	// CorrelationWindowSeconds is how long (in seconds) a Pending RemediationJob
	// is held before the correlator evaluates it. Default 30; 0 disables the hold.
	CorrelationWindowSeconds int // CORRELATION_WINDOW_SECONDS — default 30
	// DisableCorrelation skips the correlation window and correlator entirely,
	// restoring immediate-dispatch behaviour.
	DisableCorrelation bool // DISABLE_CORRELATION — default false
	// MultiPodThreshold is the minimum number of pod findings on the same node
	// required for MultiPodSameNodeRule to fire.
	MultiPodThreshold int // CORRELATION_MULTI_POD_THRESHOLD — default 3

	// PRAutoClose controls whether open sinks are auto-closed when a finding resolves.
	// Default: true. Set PR_AUTO_CLOSE=false to disable.
	PRAutoClose bool // PR_AUTO_CLOSE — default true
}

// FromEnv reads configuration from environment variables and returns a Config.
// Missing required variables or invalid values cause a descriptive error.
func FromEnv() (Config, error) {
	cfg := Config{}

	required := []struct {
		name string
		dest *string
	}{
		{"GITOPS_REPO", &cfg.GitOpsRepo},
		{"GITOPS_MANIFEST_ROOT", &cfg.GitOpsManifestRoot},
		{"AGENT_IMAGE", &cfg.AgentImage},
		{"AGENT_NAMESPACE", &cfg.AgentNamespace},
		{"AGENT_SA", &cfg.AgentSA},
	}

	for _, r := range required {
		val := os.Getenv(r.name)
		if val == "" {
			return Config{}, fmt.Errorf("required environment variable %s is not set", r.name)
		}
		*r.dest = val
	}

	agentTypeStr := os.Getenv("AGENT_TYPE")
	if agentTypeStr == "" {
		agentTypeStr = string(AgentTypeOpenCode)
	}
	switch AgentType(agentTypeStr) {
	case AgentTypeOpenCode, AgentTypeClaude:
		cfg.AgentType = AgentType(agentTypeStr)
	default:
		return Config{}, fmt.Errorf("AGENT_TYPE %q is not supported; accepted values: opencode, claude", agentTypeStr)
	}

	cfg.LogLevel = os.Getenv("LOG_LEVEL")
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	cfg.SinkType = os.Getenv("SINK_TYPE")
	if cfg.SinkType == "" {
		cfg.SinkType = "github"
	}

	maxJobsStr := os.Getenv("MAX_CONCURRENT_JOBS")
	if maxJobsStr == "" {
		cfg.MaxConcurrentJobs = 3
	} else {
		n, err := strconv.Atoi(maxJobsStr)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_CONCURRENT_JOBS must be an integer: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("MAX_CONCURRENT_JOBS must be a positive integer, got %d", n)
		}
		cfg.MaxConcurrentJobs = n
	}

	ttlStr := os.Getenv("REMEDIATION_JOB_TTL_SECONDS")
	if ttlStr == "" {
		cfg.RemediationJobTTLSeconds = 604800
	} else {
		n, err := strconv.Atoi(ttlStr)
		if err != nil {
			return Config{}, fmt.Errorf("REMEDIATION_JOB_TTL_SECONDS must be an integer: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("REMEDIATION_JOB_TTL_SECONDS must be a positive integer, got %d", n)
		}
		if n > math.MaxInt32 {
			return Config{}, fmt.Errorf("REMEDIATION_JOB_TTL_SECONDS must be at most %d, got %d", math.MaxInt32, n)
		}
		cfg.RemediationJobTTLSeconds = n
	}

	windowStr := os.Getenv("STABILISATION_WINDOW_SECONDS")
	if windowStr == "" {
		cfg.StabilisationWindow = 120 * time.Second
	} else {
		n, err := strconv.Atoi(windowStr)
		if err != nil {
			return Config{}, fmt.Errorf("STABILISATION_WINDOW_SECONDS must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("STABILISATION_WINDOW_SECONDS must be >= 0, got %d", n)
		}
		cfg.StabilisationWindow = time.Duration(n) * time.Second
	}

	// LLM provider selection — empty string disables the LLM readiness check.
	// bedrock and vertex are reserved for future implementation; configuring them
	// is a startup error rather than a silent runtime block.
	cfg.LLMProvider = os.Getenv("LLM_PROVIDER")
	switch cfg.LLMProvider {
	case "", "openai":
		// valid
	case "bedrock", "vertex":
		return Config{}, fmt.Errorf(
			"LLM_PROVIDER=%q is not yet implemented; set LLM_PROVIDER=openai or leave it unset to disable the LLM readiness check",
			cfg.LLMProvider,
		)
	default:
		return Config{}, fmt.Errorf(
			"LLM_PROVIDER %q is not supported; accepted values: openai (or unset to disable)",
			cfg.LLMProvider,
		)
	}

	action := os.Getenv("INJECTION_DETECTION_ACTION")
	if action == "" {
		action = "log"
	}
	if action != "log" && action != "suppress" {
		return Config{}, fmt.Errorf("INJECTION_DETECTION_ACTION must be 'log' or 'suppress', got %q", action)
	}
	cfg.InjectionDetectionAction = action

	scope := os.Getenv("AGENT_RBAC_SCOPE")
	if scope == "" {
		scope = "cluster"
	}
	if scope != "cluster" && scope != "namespace" {
		return Config{}, fmt.Errorf("AGENT_RBAC_SCOPE must be 'cluster' or 'namespace', got %q", scope)
	}
	cfg.AgentRBACScope = scope

	if scope == "namespace" {
		nsStr := os.Getenv("AGENT_WATCH_NAMESPACES")
		if nsStr == "" {
			return Config{}, fmt.Errorf("AGENT_WATCH_NAMESPACES is required when AGENT_RBAC_SCOPE=namespace")
		}
		for _, ns := range strings.Split(nsStr, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				cfg.AgentWatchNamespaces = append(cfg.AgentWatchNamespaces, ns)
			}
		}
		if len(cfg.AgentWatchNamespaces) == 0 {
			return Config{}, fmt.Errorf("AGENT_WATCH_NAMESPACES is empty after parsing")
		}
	}

	if nsStr := os.Getenv("WATCH_NAMESPACES"); nsStr != "" {
		for _, ns := range strings.Split(nsStr, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				cfg.WatchNamespaces = append(cfg.WatchNamespaces, ns)
			}
		}
	}

	if nsStr := os.Getenv("EXCLUDE_NAMESPACES"); nsStr != "" {
		for _, ns := range strings.Split(nsStr, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				cfg.ExcludeNamespaces = append(cfg.ExcludeNamespaces, ns)
			}
		}
	}

	retriesStr := os.Getenv("MAX_INVESTIGATION_RETRIES")
	if retriesStr == "" {
		cfg.MaxInvestigationRetries = 3
	} else {
		n, err := strconv.Atoi(retriesStr)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_INVESTIGATION_RETRIES must be an integer: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("MAX_INVESTIGATION_RETRIES must be a positive integer, got %d", n)
		}
		if n > math.MaxInt32 {
			return Config{}, fmt.Errorf("MAX_INVESTIGATION_RETRIES must be at most %d, got %d", math.MaxInt32, n)
		}
		cfg.MaxInvestigationRetries = int32(n)
	}

	rawMinSeverity := os.Getenv("MIN_SEVERITY")
	// An explicitly empty value is treated as absent — defaults to SeverityLow.
	if rawMinSeverity != "" {
		if sv, ok := domain.ParseSeverity(rawMinSeverity); ok {
			cfg.MinSeverity = sv
		} else {
			return Config{}, fmt.Errorf("invalid MIN_SEVERITY value %q: must be one of critical, high, medium, low", rawMinSeverity)
		}
	} else {
		cfg.MinSeverity = domain.SeverityLow
	}

	depthStr := os.Getenv("SELF_REMEDIATION_MAX_DEPTH")
	if depthStr == "" {
		cfg.SelfRemediationMaxDepth = 2
	} else {
		n, err := strconv.Atoi(depthStr)
		if err != nil {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_MAX_DEPTH must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_MAX_DEPTH must be >= 0, got %d", n)
		}
		if n > 10 {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_MAX_DEPTH=%d exceeds maximum reasonable value of 10; use a lower value to prevent infinite remediation chains", n)
		}
		cfg.SelfRemediationMaxDepth = n
	}

	cooldownStr := os.Getenv("SELF_REMEDIATION_COOLDOWN_SECONDS")
	if cooldownStr == "" {
		cfg.SelfRemediationCooldown = 300 * time.Second
	} else {
		n, err := strconv.Atoi(cooldownStr)
		if err != nil {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS must be >= 0, got %d", n)
		}
		if n > 3600 {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS=%d exceeds maximum reasonable value of 3600 (1 hour); use a lower value", n)
		}
		cfg.SelfRemediationCooldown = time.Duration(n) * time.Second
	}

	dryRunStr := os.Getenv("DRY_RUN")
	switch dryRunStr {
	case "", "false", "0":
		cfg.DryRun = false
	case "true", "1":
		cfg.DryRun = true
	default:
		return Config{}, fmt.Errorf("DRY_RUN must be 'true', 'false', '1', or '0', got %q", dryRunStr)
	}

	// Correlation window — how long to hold Pending jobs before dispatching.
	// Must be at least as long as stabilization window to allow related findings to be grouped.
	// Design invariant: CORRELATION_WINDOW >= STABILISATION_WINDOW
	corrWindowStr := os.Getenv("CORRELATION_WINDOW_SECONDS")
	if corrWindowStr == "" {
		cfg.CorrelationWindowSeconds = 30
	} else {
		n, err := strconv.Atoi(corrWindowStr)
		if err != nil {
			return Config{}, fmt.Errorf("CORRELATION_WINDOW_SECONDS must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("CORRELATION_WINDOW_SECONDS must be >= 0, got %d", n)
		}
		if n > 3600 {
			return Config{}, fmt.Errorf("CORRELATION_WINDOW_SECONDS=%d exceeds maximum reasonable value of 3600 (1 hour); set DISABLE_CORRELATION=true to skip correlation entirely", n)
		}
		cfg.CorrelationWindowSeconds = n
	}

	// Disable correlation entirely — restores immediate-dispatch behaviour.
	disableCorrStr := os.Getenv("DISABLE_CORRELATION")
	cfg.DisableCorrelation = disableCorrStr == "true" || disableCorrStr == "1"

	// MultiPodSameNodeRule threshold.
	multiPodStr := os.Getenv("CORRELATION_MULTI_POD_THRESHOLD")
	if multiPodStr == "" {
		cfg.MultiPodThreshold = 3
	} else {
		n, err := strconv.Atoi(multiPodStr)
		if err != nil {
			return Config{}, fmt.Errorf("CORRELATION_MULTI_POD_THRESHOLD must be an integer: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("CORRELATION_MULTI_POD_THRESHOLD must be a positive integer, got %d", n)
		}
		cfg.MultiPodThreshold = n
	}

	// PR_AUTO_CLOSE — default true; set "false" or "0" to disable sink auto-close.
	prAutoCloseStr := os.Getenv("PR_AUTO_CLOSE")
	switch prAutoCloseStr {
	case "", "true", "1":
		cfg.PRAutoClose = true
	case "false", "0":
		cfg.PRAutoClose = false
	default:
		return Config{}, fmt.Errorf("PR_AUTO_CLOSE must be 'true', 'false', '1', or '0', got %q", prAutoCloseStr)
	}

	return cfg, nil
}
