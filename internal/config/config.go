package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the mendabot-watcher controller.
// All fields are populated from environment variables at startup via FromEnv.
type Config struct {
	GitOpsRepo                string        // GITOPS_REPO — required
	GitOpsManifestRoot        string        // GITOPS_MANIFEST_ROOT — required
	AgentImage                string        // AGENT_IMAGE — required
	AgentNamespace            string        // AGENT_NAMESPACE — required; must equal watcher namespace
	AgentSA                   string        // AGENT_SA — required
	SinkType                  string        // SINK_TYPE — default "github"
	LogLevel                  string        // LOG_LEVEL — default "info"
	MaxConcurrentJobs         int           // MAX_CONCURRENT_JOBS — default 3
	RemediationJobTTLSeconds  int           // REMEDIATION_JOB_TTL_SECONDS — default 604800 (7 days)
	StabilisationWindow       time.Duration // STABILISATION_WINDOW_SECONDS — default 120s; 0 disables
	SelfRemediationMaxDepth   int           // SELF_REMEDIATION_MAX_DEPTH — default 2
	SelfRemediationCooldown   time.Duration // SELF_REMEDIATION_COOLDOWN_SECONDS — default 300s (5 minutes)
	DisableCascadeCheck       bool          // DISABLE_CASCADE_CHECK — default false
	CascadeNamespaceThreshold int           // CASCADE_NAMESPACE_THRESHOLD — default 50
	CascadeNodeCacheTTL       time.Duration // CASCADE_NODE_CACHE_TTL_SECONDS — default 30s
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

	// Self-remediation configuration
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
		// Validate reasonable upper bound for cascade prevention
		if n > 10 {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_MAX_DEPTH=%d exceeds maximum reasonable value of 10 for cascade prevention", n)
		}
		cfg.SelfRemediationMaxDepth = n
	}

	// Self-remediation cooldown configuration
	cooldownStr := os.Getenv("SELF_REMEDIATION_COOLDOWN_SECONDS")
	if cooldownStr == "" {
		cfg.SelfRemediationCooldown = 300 * time.Second // 5 minutes
	} else {
		n, err := strconv.Atoi(cooldownStr)
		if err != nil {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS must be >= 0, got %d", n)
		}
		// Validate reasonable upper bound for cooldown (1 hour)
		if n > 3600 {
			return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS=%d exceeds maximum reasonable value of 3600 (1 hour)", n)
		}
		cfg.SelfRemediationCooldown = time.Duration(n) * time.Second
	}

	// Cascade checker configuration
	disableCascadeStr := os.Getenv("DISABLE_CASCADE_CHECK")
	if disableCascadeStr == "true" || disableCascadeStr == "1" {
		cfg.DisableCascadeCheck = true
	} else {
		cfg.DisableCascadeCheck = false
	}

	namespaceThresholdStr := os.Getenv("CASCADE_NAMESPACE_THRESHOLD")
	if namespaceThresholdStr == "" {
		cfg.CascadeNamespaceThreshold = 50
	} else {
		n, err := strconv.Atoi(namespaceThresholdStr)
		if err != nil {
			return Config{}, fmt.Errorf("CASCADE_NAMESPACE_THRESHOLD must be an integer: %w", err)
		}
		if n < 0 || n > 100 {
			return Config{}, fmt.Errorf("CASCADE_NAMESPACE_THRESHOLD must be between 0 and 100, got %d", n)
		}
		cfg.CascadeNamespaceThreshold = n
	}

	nodeCacheTTLStr := os.Getenv("CASCADE_NODE_CACHE_TTL_SECONDS")
	if nodeCacheTTLStr == "" {
		cfg.CascadeNodeCacheTTL = 30 * time.Second
	} else {
		n, err := strconv.Atoi(nodeCacheTTLStr)
		if err != nil {
			return Config{}, fmt.Errorf("CASCADE_NODE_CACHE_TTL_SECONDS must be an integer: %w", err)
		}
		if n < 0 {
			return Config{}, fmt.Errorf("CASCADE_NODE_CACHE_TTL_SECONDS must be >= 0, got %d", n)
		}
		cfg.CascadeNodeCacheTTL = time.Duration(n) * time.Second
	}

	return cfg, nil
}
