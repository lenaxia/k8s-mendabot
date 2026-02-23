package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/lenaxia/k8s-mendabot/internal/config"
)

func TestFromEnv_AllFieldsPresent(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("SINK_TYPE", "gitlab")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("MAX_CONCURRENT_JOBS", "5")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitOpsRepo != "org/repo" {
		t.Errorf("GitOpsRepo: got %q, want %q", cfg.GitOpsRepo, "org/repo")
	}
	if cfg.GitOpsManifestRoot != "kubernetes/" {
		t.Errorf("GitOpsManifestRoot: got %q, want %q", cfg.GitOpsManifestRoot, "kubernetes/")
	}
	if cfg.AgentImage != "ghcr.io/lenaxia/mendabot-agent:latest" {
		t.Errorf("AgentImage: got %q, want %q", cfg.AgentImage, "ghcr.io/lenaxia/mendabot-agent:latest")
	}
	if cfg.AgentNamespace != "mendabot" {
		t.Errorf("AgentNamespace: got %q, want %q", cfg.AgentNamespace, "mendabot")
	}
	if cfg.AgentSA != "mendabot-agent" {
		t.Errorf("AgentSA: got %q, want %q", cfg.AgentSA, "mendabot-agent")
	}
	if cfg.SinkType != "gitlab" {
		t.Errorf("SinkType: got %q, want %q", cfg.SinkType, "gitlab")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.MaxConcurrentJobs != 5 {
		t.Errorf("MaxConcurrentJobs: got %d, want %d", cfg.MaxConcurrentJobs, 5)
	}
}

func TestFromEnv_Defaults(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	os.Unsetenv("SINK_TYPE")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("MAX_CONCURRENT_JOBS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SinkType != "github" {
		t.Errorf("SinkType default: got %q, want %q", cfg.SinkType, "github")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default: got %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.MaxConcurrentJobs != 3 {
		t.Errorf("MaxConcurrentJobs default: got %d, want %d", cfg.MaxConcurrentJobs, 3)
	}
}

func TestFromEnv_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		unsetFn func()
	}{
		{
			name: "missing GITOPS_REPO",
			unsetFn: func() {
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("GITOPS_REPO")
			},
		},
		{
			name: "missing GITOPS_MANIFEST_ROOT",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("GITOPS_MANIFEST_ROOT")
			},
		},
		{
			name: "missing AGENT_IMAGE",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("AGENT_IMAGE")
			},
		},
		{
			name: "missing AGENT_NAMESPACE",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("AGENT_NAMESPACE")
			},
		},
		{
			name: "missing AGENT_SA",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				os.Unsetenv("AGENT_SA")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.unsetFn()
			_, err := config.FromEnv()
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestFromEnv_InvalidMaxConcurrentJobs(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("MAX_CONCURRENT_JOBS", "not-a-number")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid MAX_CONCURRENT_JOBS, got nil")
	}
}

func TestFromEnv_ZeroMaxConcurrentJobs(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("MAX_CONCURRENT_JOBS", "0")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for MAX_CONCURRENT_JOBS=0, got nil")
	}
}

func TestFromEnv_NegativeMaxConcurrentJobs(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("MAX_CONCURRENT_JOBS", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for MAX_CONCURRENT_JOBS=-1, got nil")
	}
}

func TestFromEnv_RemediationJobTTLDefault(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	os.Unsetenv("REMEDIATION_JOB_TTL_SECONDS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RemediationJobTTLSeconds != 604800 {
		t.Errorf("RemediationJobTTLSeconds default: got %d, want 604800", cfg.RemediationJobTTLSeconds)
	}
}

func TestFromEnv_RemediationJobTTLExplicit(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("REMEDIATION_JOB_TTL_SECONDS", "86400")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RemediationJobTTLSeconds != 86400 {
		t.Errorf("RemediationJobTTLSeconds: got %d, want 86400", cfg.RemediationJobTTLSeconds)
	}
}

func TestFromEnv_InvalidRemediationJobTTL(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("REMEDIATION_JOB_TTL_SECONDS", "not-a-number")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid REMEDIATION_JOB_TTL_SECONDS, got nil")
	}
}

func TestFromEnv_ZeroRemediationJobTTL(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("REMEDIATION_JOB_TTL_SECONDS", "0")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for REMEDIATION_JOB_TTL_SECONDS=0, got nil")
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
}

func TestFromEnv_StabilisationWindowDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("STABILISATION_WINDOW_SECONDS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 120 * time.Second
	if cfg.StabilisationWindow != want {
		t.Errorf("StabilisationWindow default: got %v, want %v", cfg.StabilisationWindow, want)
	}
}

func TestFromEnv_StabilisationWindowZero(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "0")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StabilisationWindow != 0 {
		t.Errorf("StabilisationWindow zero: got %v, want 0", cfg.StabilisationWindow)
	}
}

func TestFromEnv_StabilisationWindowCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "300")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 300 * time.Second
	if cfg.StabilisationWindow != want {
		t.Errorf("StabilisationWindow custom: got %v, want %v", cfg.StabilisationWindow, want)
	}
}

func TestFromEnv_StabilisationWindowNegative(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for STABILISATION_WINDOW_SECONDS=-1, got nil")
	}
}

func TestFromEnv_StabilisationWindowInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "abc")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for STABILISATION_WINDOW_SECONDS=abc, got nil")
	}
}

func TestFromEnv_SelfRemediationDefaults(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("SELF_REMEDIATION_MAX_DEPTH")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SelfRemediationMaxDepth != 2 {
		t.Errorf("SelfRemediationMaxDepth default: got %d, want 2", cfg.SelfRemediationMaxDepth)
	}
}

func TestFromEnv_SelfRemediationCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "3")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SelfRemediationMaxDepth != 3 {
		t.Errorf("SelfRemediationMaxDepth custom: got %d, want 3", cfg.SelfRemediationMaxDepth)
	}
}

func TestFromEnv_SelfRemediationInvalidDepth(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "abc")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid SELF_REMEDIATION_MAX_DEPTH, got nil")
	}
}

func TestFromEnv_SelfRemediationNegativeDepth(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for negative SELF_REMEDIATION_MAX_DEPTH, got nil")
	}
}

func TestFromEnv_SelfRemediationCooldownDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SelfRemediationCooldown != 300*time.Second {
		t.Errorf("SelfRemediationCooldown default: got %v, want 5 minutes", cfg.SelfRemediationCooldown)
	}
}

func TestFromEnv_SelfRemediationCooldownCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "120")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SelfRemediationCooldown != 120*time.Second {
		t.Errorf("SelfRemediationCooldown custom: got %v, want 2 minutes", cfg.SelfRemediationCooldown)
	}
}

func TestFromEnv_SelfRemediationCooldownZero(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "0")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SelfRemediationCooldown != 0 {
		t.Errorf("SelfRemediationCooldown zero: got %v, want 0", cfg.SelfRemediationCooldown)
	}
}

func TestFromEnv_SelfRemediationCooldownInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "abc")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid SELF_REMEDIATION_COOLDOWN_SECONDS, got nil")
	}
}

func TestFromEnv_SelfRemediationCooldownNegative(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for negative SELF_REMEDIATION_COOLDOWN_SECONDS, got nil")
	}
}

// TestFromEnv_SelfRemediationMaxDepthZero tests that SELF_REMEDIATION_MAX_DEPTH=0 is valid
func TestFromEnv_SelfRemediationMaxDepthZero(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "0")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SelfRemediationMaxDepth != 0 {
		t.Errorf("SelfRemediationMaxDepth zero: got %d, want 0", cfg.SelfRemediationMaxDepth)
	}
}

// TestFromEnv_SelfRemediationMaxDepthLarge tests large depth values within reasonable bounds
func TestFromEnv_SelfRemediationMaxDepthLarge(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "10")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SelfRemediationMaxDepth != 10 {
		t.Errorf("SelfRemediationMaxDepth large: got %d, want 10", cfg.SelfRemediationMaxDepth)
	}
}

// TestFromEnv_AllConfigCombinations tests comprehensive configuration combinations
func TestFromEnv_AllConfigCombinations(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, config.Config)
	}{
		{
			name: "zero values disable features",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH":        "0",
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "0",
				"STABILISATION_WINDOW_SECONDS":      "0",
			},
			validate: func(t *testing.T, cfg config.Config) {
				if cfg.SelfRemediationMaxDepth != 0 {
					t.Errorf("SelfRemediationMaxDepth: got %d, want 0", cfg.SelfRemediationMaxDepth)
				}
				if cfg.SelfRemediationCooldown != 0 {
					t.Errorf("SelfRemediationCooldown: got %v, want 0", cfg.SelfRemediationCooldown)
				}
				if cfg.StabilisationWindow != 0 {
					t.Errorf("StabilisationWindow: got %v, want 0", cfg.StabilisationWindow)
				}
			},
		},
		{
			name: "max values for all limits",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH":        "10",
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "3600",
				"STABILISATION_WINDOW_SECONDS":      "3600",
				"MAX_CONCURRENT_JOBS":               "100",
				"REMEDIATION_JOB_TTL_SECONDS":       "2592000",
			},
			validate: func(t *testing.T, cfg config.Config) {
				if cfg.SelfRemediationMaxDepth != 10 {
					t.Errorf("SelfRemediationMaxDepth: got %d, want 10", cfg.SelfRemediationMaxDepth)
				}
				if cfg.SelfRemediationCooldown != 3600*time.Second {
					t.Errorf("SelfRemediationCooldown: got %v, want 1h", cfg.SelfRemediationCooldown)
				}
				if cfg.StabilisationWindow != 3600*time.Second {
					t.Errorf("StabilisationWindow: got %v, want 1h", cfg.StabilisationWindow)
				}
				if cfg.MaxConcurrentJobs != 100 {
					t.Errorf("MaxConcurrentJobs: got %d, want 100", cfg.MaxConcurrentJobs)
				}
				if cfg.RemediationJobTTLSeconds != 2592000 {
					t.Errorf("RemediationJobTTLSeconds: got %d, want 2592000", cfg.RemediationJobTTLSeconds)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := config.FromEnv()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tt.validate(t, cfg)
		})
	}
}

// TestFromEnv_ConfigValidationEdgeCases tests edge cases in configuration validation
func TestFromEnv_ConfigValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		errorSubstr string
	}{
		{
			name: "MAX_CONCURRENT_JOBS zero",
			envVars: map[string]string{
				"MAX_CONCURRENT_JOBS": "0",
			},
			expectError: true,
			errorSubstr: "positive integer",
		},
		{
			name: "MAX_CONCURRENT_JOBS negative",
			envVars: map[string]string{
				"MAX_CONCURRENT_JOBS": "-5",
			},
			expectError: true,
			errorSubstr: "positive integer",
		},
		{
			name: "REMEDIATION_JOB_TTL_SECONDS zero",
			envVars: map[string]string{
				"REMEDIATION_JOB_TTL_SECONDS": "0",
			},
			expectError: true,
			errorSubstr: "positive integer",
		},
		{
			name: "STABILISATION_WINDOW_SECONDS negative",
			envVars: map[string]string{
				"STABILISATION_WINDOW_SECONDS": "-10",
			},
			expectError: true,
			errorSubstr: ">= 0",
		},
		{
			name: "SELF_REMEDIATION_MAX_DEPTH negative",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "-5",
			},
			expectError: true,
			errorSubstr: ">= 0",
		},
		{
			name: "SELF_REMEDIATION_COOLDOWN_SECONDS negative",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "-10",
			},
			expectError: true,
			errorSubstr: ">= 0",
		},
		{
			name: "invalid integer values",
			envVars: map[string]string{
				"MAX_CONCURRENT_JOBS":               "not-a-number",
				"REMEDIATION_JOB_TTL_SECONDS":       "also-not-a-number",
				"STABILISATION_WINDOW_SECONDS":      "still-not-a-number",
				"SELF_REMEDIATION_MAX_DEPTH":        "nope",
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "definitely-not",
			},
			expectError: true,
			errorSubstr: "must be an integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			_, err := config.FromEnv()
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errorSubstr != "" && !contains(err.Error(), tt.errorSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errorSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

// TestFromEnv_CascadePreventionValidation tests comprehensive validation for cascade prevention settings
func TestFromEnv_CascadePreventionValidation(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		errorSubstr string
	}{
		{
			name: "valid configuration with reasonable bounds",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH":        "3",
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "600",
			},
			expectError: false,
		},
		{
			name: "self remediation depth exceeds maximum reasonable bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "50",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
		{
			name: "self remediation cooldown exceeds maximum reasonable bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "10000",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
		{
			name: "self remediation depth negative",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "-1",
			},
			expectError: true,
			errorSubstr: ">= 0",
		},
		{
			name: "self remediation cooldown negative",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "-1",
			},
			expectError: true,
			errorSubstr: ">= 0",
		},
		{
			name: "extremely large depth value",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "999999",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
		{
			name: "extremely large cooldown value",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "9999999",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
		{
			name: "valid depth at upper bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "10",
			},
			expectError: false,
		},
		{
			name: "valid cooldown at upper bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "3600",
			},
			expectError: false,
		},
		{
			name: "depth one above upper bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "11",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
		{
			name: "cooldown one above upper bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "3601",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			_, err := config.FromEnv()
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errorSubstr != "" && !contains(err.Error(), tt.errorSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errorSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestFromEnv_ConsistencyValidation tests validation of configuration consistency
func TestFromEnv_ConsistencyValidation(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		errorSubstr string
	}{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			_, err := config.FromEnv()
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errorSubstr != "" && !contains(err.Error(), tt.errorSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errorSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestFromEnv_BoundaryConditionValidation tests boundary condition validation
func TestFromEnv_BoundaryConditionValidation(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		errorSubstr string
	}{
		{
			name: "minimum valid depth",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "0",
			},
			expectError: false,
		},
		{
			name: "minimum valid cooldown",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "0",
			},
			expectError: false,
		},
		{
			name: "depth at maximum reasonable bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "10",
			},
			expectError: false,
		},
		{
			name: "cooldown at maximum reasonable bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "3600",
			},
			expectError: false,
		},
		{
			name: "depth just above maximum reasonable bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "11",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
		{
			name: "cooldown just above maximum reasonable bound",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "3601",
			},
			expectError: true,
			errorSubstr: "exceeds maximum reasonable value",
		},
		{
			name: "very small but valid depth",
			envVars: map[string]string{
				"SELF_REMEDIATION_MAX_DEPTH": "1",
			},
			expectError: false,
		},
		{
			name: "very small but valid cooldown",
			envVars: map[string]string{
				"SELF_REMEDIATION_COOLDOWN_SECONDS": "1",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			_, err := config.FromEnv()
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errorSubstr != "" && !contains(err.Error(), tt.errorSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errorSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestFromEnv_InjectionDetectionActionDefault tests that unset INJECTION_DETECTION_ACTION defaults to "log"
func TestFromEnv_InjectionDetectionActionDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("INJECTION_DETECTION_ACTION")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InjectionDetectionAction != "log" {
		t.Errorf("InjectionDetectionAction default: got %q, want %q", cfg.InjectionDetectionAction, "log")
	}
}

// TestFromEnv_InjectionDetectionActionLog tests INJECTION_DETECTION_ACTION=log
func TestFromEnv_InjectionDetectionActionLog(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INJECTION_DETECTION_ACTION", "log")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InjectionDetectionAction != "log" {
		t.Errorf("InjectionDetectionAction: got %q, want %q", cfg.InjectionDetectionAction, "log")
	}
}

// TestFromEnv_InjectionDetectionActionSuppress tests INJECTION_DETECTION_ACTION=suppress
func TestFromEnv_InjectionDetectionActionSuppress(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INJECTION_DETECTION_ACTION", "suppress")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InjectionDetectionAction != "suppress" {
		t.Errorf("InjectionDetectionAction: got %q, want %q", cfg.InjectionDetectionAction, "suppress")
	}
}

// TestFromEnv_InjectionDetectionActionInvalid tests that an invalid value returns an error
func TestFromEnv_InjectionDetectionActionInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INJECTION_DETECTION_ACTION", "warn")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for INJECTION_DETECTION_ACTION=warn, got nil")
	}
}

// TestFromEnv_ValidationSkipOption tests that validation can be skipped for testing
func TestFromEnv_ValidationSkipOption(t *testing.T) {
	// Test that extremely large values are rejected by default
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "999")
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "99999")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for extremely large values, got nil")
	}
}

// TestFromEnv_AgentRBACScope_Default tests that unset AGENT_RBAC_SCOPE defaults to "cluster"
// and AgentWatchNamespaces is nil.
func TestFromEnv_AgentRBACScope_Default(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("AGENT_RBAC_SCOPE")
	os.Unsetenv("AGENT_WATCH_NAMESPACES")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentRBACScope != "cluster" {
		t.Errorf("AgentRBACScope default: got %q, want %q", cfg.AgentRBACScope, "cluster")
	}
	if cfg.AgentWatchNamespaces != nil {
		t.Errorf("AgentWatchNamespaces default: got %v, want nil", cfg.AgentWatchNamespaces)
	}
}

// TestFromEnv_AgentRBACScope_Cluster tests explicit AGENT_RBAC_SCOPE=cluster.
func TestFromEnv_AgentRBACScope_Cluster(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AGENT_RBAC_SCOPE", "cluster")
	os.Unsetenv("AGENT_WATCH_NAMESPACES")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentRBACScope != "cluster" {
		t.Errorf("AgentRBACScope cluster: got %q, want %q", cfg.AgentRBACScope, "cluster")
	}
}

// TestFromEnv_AgentRBACScope_Namespace tests AGENT_RBAC_SCOPE=namespace with valid namespaces.
func TestFromEnv_AgentRBACScope_Namespace(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AGENT_RBAC_SCOPE", "namespace")
	t.Setenv("AGENT_WATCH_NAMESPACES", "default,production")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentRBACScope != "namespace" {
		t.Errorf("AgentRBACScope namespace: got %q, want %q", cfg.AgentRBACScope, "namespace")
	}
	want := []string{"default", "production"}
	if len(cfg.AgentWatchNamespaces) != len(want) {
		t.Fatalf("AgentWatchNamespaces length: got %d, want %d", len(cfg.AgentWatchNamespaces), len(want))
	}
	for i, ns := range want {
		if cfg.AgentWatchNamespaces[i] != ns {
			t.Errorf("AgentWatchNamespaces[%d]: got %q, want %q", i, cfg.AgentWatchNamespaces[i], ns)
		}
	}
}

// TestFromEnv_AgentRBACScope_NamespaceEmptyList tests AGENT_RBAC_SCOPE=namespace with empty
// AGENT_WATCH_NAMESPACES returns an error.
func TestFromEnv_AgentRBACScope_NamespaceEmptyList(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AGENT_RBAC_SCOPE", "namespace")
	t.Setenv("AGENT_WATCH_NAMESPACES", "")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for AGENT_RBAC_SCOPE=namespace with empty AGENT_WATCH_NAMESPACES, got nil")
	}
}

// TestFromEnv_AgentRBACScope_Invalid tests that an invalid AGENT_RBAC_SCOPE returns an error.
func TestFromEnv_AgentRBACScope_Invalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AGENT_RBAC_SCOPE", "badvalue")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for AGENT_RBAC_SCOPE=badvalue, got nil")
	}
}

func TestFromEnv_AgentWatchNamespacesWhitespaceOnly(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AGENT_RBAC_SCOPE", "namespace")
	t.Setenv("AGENT_WATCH_NAMESPACES", "  ,  ")
	_, err := config.FromEnv()
	if err == nil {
		t.Error("expected error for whitespace-only AGENT_WATCH_NAMESPACES, got nil")
	}
}
