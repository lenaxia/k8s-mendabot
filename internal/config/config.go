package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the mendabot-watcher controller.
// All fields are populated from environment variables at startup via FromEnv.
type Config struct {
	GitOpsRepo               string        // GITOPS_REPO — required
	GitOpsManifestRoot       string        // GITOPS_MANIFEST_ROOT — required
	AgentImage               string        // AGENT_IMAGE — required
	AgentNamespace           string        // AGENT_NAMESPACE — required; must equal watcher namespace
	AgentSA                  string        // AGENT_SA — required
	SinkType                 string        // SINK_TYPE — default "github"
	LogLevel                 string        // LOG_LEVEL — default "info"
	MaxConcurrentJobs        int           // MAX_CONCURRENT_JOBS — default 3
	RemediationJobTTLSeconds int           // REMEDIATION_JOB_TTL_SECONDS — default 604800 (7 days)
	StabilisationWindow      time.Duration // STABILISATION_WINDOW_SECONDS — default 120s; 0 disables
	// LLMProvider selects the LLM readiness checker used to gate RemediationJob
	// creation. Accepted values: "openai". Empty (default) disables the check.
	LLMProvider              string   // LLM_PROVIDER — default "" (disabled)
	InjectionDetectionAction string   // INJECTION_DETECTION_ACTION — "log" (default) or "suppress"
	AgentRBACScope           string   // AGENT_RBAC_SCOPE — "cluster" (default) or "namespace"
	AgentWatchNamespaces     []string // AGENT_WATCH_NAMESPACES — required when scope is "namespace"
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

	return cfg, nil
}
