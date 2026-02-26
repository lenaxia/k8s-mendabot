package config_test

import (
	"os"
	"strings"
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

// TestFromEnv_TTLOverflow verifies that REMEDIATION_JOB_TTL_SECONDS > MaxInt32 returns an error.
func TestFromEnv_TTLOverflow(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("REMEDIATION_JOB_TTL_SECONDS", "2147483648")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for REMEDIATION_JOB_TTL_SECONDS overflow, got nil")
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
				"STABILISATION_WINDOW_SECONDS": "0",
			},
			validate: func(t *testing.T, cfg config.Config) {
				if cfg.StabilisationWindow != 0 {
					t.Errorf("StabilisationWindow: got %v, want 0", cfg.StabilisationWindow)
				}
			},
		},
		{
			name: "max values for all limits",
			envVars: map[string]string{
				"STABILISATION_WINDOW_SECONDS": "3600",
				"MAX_CONCURRENT_JOBS":          "100",
				"REMEDIATION_JOB_TTL_SECONDS":  "2592000",
			},
			validate: func(t *testing.T, cfg config.Config) {
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
			name: "invalid integer values",
			envVars: map[string]string{
				"MAX_CONCURRENT_JOBS":          "not-a-number",
				"REMEDIATION_JOB_TTL_SECONDS":  "also-not-a-number",
				"STABILISATION_WINDOW_SECONDS": "still-not-a-number",
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
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
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

// TestFromEnv_LLMProviderDefault tests that LLM_PROVIDER defaults to empty string (disabled).
func TestFromEnv_LLMProviderDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("LLM_PROVIDER")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMProvider != "" {
		t.Errorf("LLMProvider default: got %q, want %q", cfg.LLMProvider, "")
	}
}

// TestFromEnv_LLMProviderValidValues tests each accepted LLM_PROVIDER value.
func TestFromEnv_LLMProviderValidValues(t *testing.T) {
	for _, provider := range []string{"openai"} {
		t.Run(provider, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("LLM_PROVIDER", provider)

			cfg, err := config.FromEnv()
			if err != nil {
				t.Fatalf("unexpected error for LLM_PROVIDER=%q: %v", provider, err)
			}
			if cfg.LLMProvider != provider {
				t.Errorf("LLMProvider: got %q, want %q", cfg.LLMProvider, provider)
			}
		})
	}
}

// TestFromEnv_LLMProviderUnimplementedValues tests that reserved-but-unimplemented
// providers are rejected at startup with a clear error.
func TestFromEnv_LLMProviderUnimplementedValues(t *testing.T) {
	for _, provider := range []string{"bedrock", "vertex"} {
		t.Run(provider, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("LLM_PROVIDER", provider)

			_, err := config.FromEnv()
			if err == nil {
				t.Fatalf("expected error for unimplemented LLM_PROVIDER=%q, got nil", provider)
			}
			if !strings.Contains(err.Error(), "not yet implemented") {
				t.Errorf("error should say 'not yet implemented', got: %v", err)
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
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
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
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
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

// TestFromEnv_MaxInvestigationRetries covers all valid and invalid input cases.
func TestFromEnv_MaxInvestigationRetries(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantValue int32
		wantErr   bool
	}{
		{
			name:      "unset uses default 3",
			envValue:  "",
			wantValue: 3,
		},
		{
			name:      "explicit value 1",
			envValue:  "1",
			wantValue: 1,
		},
		{
			name:      "explicit value 5",
			envValue:  "5",
			wantValue: 5,
		},
		{
			name:      "explicit value 10",
			envValue:  "10",
			wantValue: 10,
		},
		{
			name:     "zero is invalid",
			envValue: "0",
			wantErr:  true,
		},
		{
			name:     "negative is invalid",
			envValue: "-1",
			wantErr:  true,
		},
		{
			name:     "non-integer is invalid",
			envValue: "three",
			wantErr:  true,
		},
		{
			name:     "float is invalid",
			envValue: "3.5",
			wantErr:  true,
		},
		{
			name:     "overflow above MaxInt32 is invalid",
			envValue: "2147483648",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("MAX_INVESTIGATION_RETRIES", tt.envValue)

			cfg, err := config.FromEnv()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for MAX_INVESTIGATION_RETRIES=%q, got nil", tt.envValue)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.MaxInvestigationRetries != tt.wantValue {
				t.Errorf("MaxInvestigationRetries = %d, want %d", cfg.MaxInvestigationRetries, tt.wantValue)
			}
		})
	}
}

// TestFromEnv_AgentTypeDefault tests that unset AGENT_TYPE defaults to "opencode".
func TestFromEnv_AgentTypeDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("AGENT_TYPE")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentType != config.AgentTypeOpenCode {
		t.Errorf("AgentType default: got %q, want %q", cfg.AgentType, config.AgentTypeOpenCode)
	}
}

// TestFromEnv_AgentTypeValidValues tests each accepted AGENT_TYPE value.
func TestFromEnv_AgentTypeValidValues(t *testing.T) {
	tests := []struct {
		input string
		want  config.AgentType
	}{
		{"opencode", config.AgentTypeOpenCode},
		{"claude", config.AgentTypeClaude},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("AGENT_TYPE", tt.input)

			cfg, err := config.FromEnv()
			if err != nil {
				t.Fatalf("unexpected error for AGENT_TYPE=%q: %v", tt.input, err)
			}
			if cfg.AgentType != tt.want {
				t.Errorf("AgentType: got %q, want %q", cfg.AgentType, tt.want)
			}
		})
	}
}

// TestFromEnv_AgentTypeInvalidValue tests that an unknown AGENT_TYPE is rejected.
func TestFromEnv_AgentTypeInvalidValue(t *testing.T) {
	for _, bad := range []string{"kiro", "copilot", "OPENCODE", ""} {
		name := bad
		if name == "" {
			name = "(forced-empty-via-setenv)"
		}
		t.Run(name, func(t *testing.T) {
			setRequiredEnv(t)
			if bad == "" {
				// force empty via explicit set to distinguish from unset
				t.Setenv("AGENT_TYPE", " ")
			} else {
				t.Setenv("AGENT_TYPE", bad)
			}
			_, err := config.FromEnv()
			if err == nil {
				t.Fatalf("expected error for AGENT_TYPE=%q, got nil", bad)
			}
			if !strings.Contains(err.Error(), "AGENT_TYPE") {
				t.Errorf("error should mention AGENT_TYPE, got: %v", err)
			}
		})
	}
}

// TestFromEnv_WatchNamespacesDefault tests that unset WATCH_NAMESPACES defaults to nil.
func TestFromEnv_WatchNamespacesDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("WATCH_NAMESPACES")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WatchNamespaces != nil {
		t.Errorf("WatchNamespaces default: got %v, want nil", cfg.WatchNamespaces)
	}
}

// TestFromEnv_WatchNamespacesBlank tests that WATCH_NAMESPACES="" results in nil.
func TestFromEnv_WatchNamespacesBlank(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WATCH_NAMESPACES", "")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WatchNamespaces != nil {
		t.Errorf("WatchNamespaces blank: got %v, want nil", cfg.WatchNamespaces)
	}
}

// TestFromEnv_WatchNamespacesSingle tests that a single namespace is parsed correctly.
func TestFromEnv_WatchNamespacesSingle(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WATCH_NAMESPACES", "production")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"production"}
	if len(cfg.WatchNamespaces) != len(want) {
		t.Fatalf("WatchNamespaces length: got %d, want %d", len(cfg.WatchNamespaces), len(want))
	}
	for i, ns := range want {
		if cfg.WatchNamespaces[i] != ns {
			t.Errorf("WatchNamespaces[%d]: got %q, want %q", i, cfg.WatchNamespaces[i], ns)
		}
	}
}

// TestFromEnv_WatchNamespacesMultiple tests that multiple comma-separated namespaces are parsed in order.
func TestFromEnv_WatchNamespacesMultiple(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WATCH_NAMESPACES", "default,production,staging")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"default", "production", "staging"}
	if len(cfg.WatchNamespaces) != len(want) {
		t.Fatalf("WatchNamespaces length: got %d, want %d", len(cfg.WatchNamespaces), len(want))
	}
	for i, ns := range want {
		if cfg.WatchNamespaces[i] != ns {
			t.Errorf("WatchNamespaces[%d]: got %q, want %q", i, cfg.WatchNamespaces[i], ns)
		}
	}
}

// TestFromEnv_WatchNamespacesWhitespace tests that whitespace around namespace names is trimmed.
func TestFromEnv_WatchNamespacesWhitespace(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WATCH_NAMESPACES", " default , staging ")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"default", "staging"}
	if len(cfg.WatchNamespaces) != len(want) {
		t.Fatalf("WatchNamespaces length: got %d, want %d", len(cfg.WatchNamespaces), len(want))
	}
	for i, ns := range want {
		if cfg.WatchNamespaces[i] != ns {
			t.Errorf("WatchNamespaces[%d]: got %q, want %q", i, cfg.WatchNamespaces[i], ns)
		}
	}
}

// TestFromEnv_WatchNamespacesWhitespaceOnly tests that a whitespace-only value results in nil.
func TestFromEnv_WatchNamespacesWhitespaceOnly(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WATCH_NAMESPACES", "  ,  ")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WatchNamespaces != nil {
		t.Errorf("WatchNamespaces whitespace-only: got %v, want nil", cfg.WatchNamespaces)
	}
}

// TestFromEnv_ExcludeNamespacesDefault tests that unset EXCLUDE_NAMESPACES defaults to nil.
func TestFromEnv_ExcludeNamespacesDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("EXCLUDE_NAMESPACES")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ExcludeNamespaces != nil {
		t.Errorf("ExcludeNamespaces default: got %v, want nil", cfg.ExcludeNamespaces)
	}
}

// TestFromEnv_ExcludeNamespacesBlank tests that EXCLUDE_NAMESPACES="" results in nil.
func TestFromEnv_ExcludeNamespacesBlank(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EXCLUDE_NAMESPACES", "")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ExcludeNamespaces != nil {
		t.Errorf("ExcludeNamespaces blank: got %v, want nil", cfg.ExcludeNamespaces)
	}
}

// TestFromEnv_ExcludeNamespacesSingle tests that a single namespace is parsed correctly.
func TestFromEnv_ExcludeNamespacesSingle(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EXCLUDE_NAMESPACES", "kube-system")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"kube-system"}
	if len(cfg.ExcludeNamespaces) != len(want) {
		t.Fatalf("ExcludeNamespaces length: got %d, want %d", len(cfg.ExcludeNamespaces), len(want))
	}
	for i, ns := range want {
		if cfg.ExcludeNamespaces[i] != ns {
			t.Errorf("ExcludeNamespaces[%d]: got %q, want %q", i, cfg.ExcludeNamespaces[i], ns)
		}
	}
}

// TestFromEnv_ExcludeNamespacesMultiple tests that multiple comma-separated namespaces are parsed in order.
func TestFromEnv_ExcludeNamespacesMultiple(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EXCLUDE_NAMESPACES", "kube-system,cert-manager,flux-system")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"kube-system", "cert-manager", "flux-system"}
	if len(cfg.ExcludeNamespaces) != len(want) {
		t.Fatalf("ExcludeNamespaces length: got %d, want %d", len(cfg.ExcludeNamespaces), len(want))
	}
	for i, ns := range want {
		if cfg.ExcludeNamespaces[i] != ns {
			t.Errorf("ExcludeNamespaces[%d]: got %q, want %q", i, cfg.ExcludeNamespaces[i], ns)
		}
	}
}

// TestFromEnv_ExcludeNamespacesWhitespaceOnly tests that a whitespace-only value results in nil.
func TestFromEnv_ExcludeNamespacesWhitespaceOnly(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EXCLUDE_NAMESPACES", "  ,  ")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ExcludeNamespaces != nil {
		t.Errorf("ExcludeNamespaces whitespace-only: got %v, want nil", cfg.ExcludeNamespaces)
	}
}

// TestFromEnv_MinSeverity covers all valid, default, and invalid MIN_SEVERITY cases.
func TestFromEnv_MinSeverity(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "unset defaults to low",
			envValue:  "",
			wantValue: "low",
		},
		{
			name:      "critical",
			envValue:  "critical",
			wantValue: "critical",
		},
		{
			name:      "high",
			envValue:  "high",
			wantValue: "high",
		},
		{
			name:      "medium",
			envValue:  "medium",
			wantValue: "medium",
		},
		{
			name:      "low",
			envValue:  "low",
			wantValue: "low",
		},
		{
			name:     "bogus value returns error",
			envValue: "bogus",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			if tt.envValue == "" {
				os.Unsetenv("MIN_SEVERITY")
			} else {
				t.Setenv("MIN_SEVERITY", tt.envValue)
			}

			cfg, err := config.FromEnv()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for MIN_SEVERITY=%q, got nil", tt.envValue)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(cfg.MinSeverity) != tt.wantValue {
				t.Errorf("MinSeverity = %q, want %q", cfg.MinSeverity, tt.wantValue)
			}
		})
	}
}

// TestFromEnv_BothFiltersCoexist tests that both WATCH_NAMESPACES and EXCLUDE_NAMESPACES
// can be set simultaneously without error.
func TestFromEnv_BothFiltersCoexist(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WATCH_NAMESPACES", "production")
	t.Setenv("EXCLUDE_NAMESPACES", "kube-system")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.WatchNamespaces) != 1 || cfg.WatchNamespaces[0] != "production" {
		t.Errorf("WatchNamespaces: got %v, want [production]", cfg.WatchNamespaces)
	}
	if len(cfg.ExcludeNamespaces) != 1 || cfg.ExcludeNamespaces[0] != "kube-system" {
		t.Errorf("ExcludeNamespaces: got %v, want [kube-system]", cfg.ExcludeNamespaces)
	}
}

func TestFromEnv_SelfRemediationMaxDepth_Default(t *testing.T) {
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

func TestFromEnv_SelfRemediationMaxDepth_Valid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "5")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SelfRemediationMaxDepth != 5 {
		t.Errorf("SelfRemediationMaxDepth: got %d, want 5", cfg.SelfRemediationMaxDepth)
	}
}

func TestFromEnv_SelfRemediationMaxDepth_Zero(t *testing.T) {
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

func TestFromEnv_SelfRemediationMaxDepth_Negative(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Error("expected error for negative SELF_REMEDIATION_MAX_DEPTH, got nil")
	}
}

func TestFromEnv_SelfRemediationMaxDepth_NonInteger(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "foo")

	_, err := config.FromEnv()
	if err == nil {
		t.Error("expected error for non-integer SELF_REMEDIATION_MAX_DEPTH, got nil")
	}
}

func TestFromEnv_SelfRemediationCooldown_Default(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("SELF_REMEDIATION_COOLDOWN_SECONDS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SelfRemediationCooldown != 300*time.Second {
		t.Errorf("SelfRemediationCooldown default: got %v, want 300s", cfg.SelfRemediationCooldown)
	}
}

func TestFromEnv_SelfRemediationCooldown_Valid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "60")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SelfRemediationCooldown != 60*time.Second {
		t.Errorf("SelfRemediationCooldown: got %v, want 60s", cfg.SelfRemediationCooldown)
	}
}

func TestFromEnv_SelfRemediationCooldown_Zero(t *testing.T) {
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

func TestFromEnv_SelfRemediationCooldown_Negative(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Error("expected error for negative SELF_REMEDIATION_COOLDOWN_SECONDS, got nil")
	}
}

func TestFromEnv_SelfRemediationCooldown_NonInteger(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_COOLDOWN_SECONDS", "foo")

	_, err := config.FromEnv()
	if err == nil {
		t.Error("expected error for non-integer SELF_REMEDIATION_COOLDOWN_SECONDS, got nil")
	}
}

func TestFromEnv_DryRunDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("DRY_RUN")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DryRun != false {
		t.Errorf("DryRun default: got %v, want false", cfg.DryRun)
	}
}

func TestFromEnv_DryRunFalse(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DRY_RUN", "false")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DryRun {
		t.Error("DryRun 'false': got true, want false")
	}
}

func TestFromEnv_DryRunZero(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DRY_RUN", "0")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DryRun {
		t.Error("DryRun '0': got true, want false")
	}
}

func TestFromEnv_DryRunTrue(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DRY_RUN", "true")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DryRun {
		t.Error("DryRun: got false, want true")
	}
}

func TestFromEnv_DryRunOne(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DRY_RUN", "1")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DryRun {
		t.Error("DryRun '1': got false, want true")
	}
}

func TestFromEnv_DryRunInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DRY_RUN", "yes")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for DRY_RUN=yes, got nil")
	}
	if !strings.Contains(err.Error(), "DRY_RUN") {
		t.Errorf("error should mention DRY_RUN, got: %v", err)
	}
}

// TestFromEnv_CorrelationWindowDefault verifies CORRELATION_WINDOW_SECONDS defaults to 30.
func TestFromEnv_CorrelationWindowDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("CORRELATION_WINDOW_SECONDS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CorrelationWindowSeconds != 30 {
		t.Errorf("CorrelationWindowSeconds default: got %d, want 30", cfg.CorrelationWindowSeconds)
	}
}

// TestFromEnv_CorrelationWindowCustom verifies CORRELATION_WINDOW_SECONDS parses correctly.
func TestFromEnv_CorrelationWindowCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_WINDOW_SECONDS", "60")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CorrelationWindowSeconds != 60 {
		t.Errorf("CorrelationWindowSeconds: got %d, want 60", cfg.CorrelationWindowSeconds)
	}
}

// TestFromEnv_CorrelationWindowZero verifies CORRELATION_WINDOW_SECONDS=0 is valid
// (disables the window hold).
func TestFromEnv_CorrelationWindowZero(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_WINDOW_SECONDS", "0")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CorrelationWindowSeconds != 0 {
		t.Errorf("CorrelationWindowSeconds zero: got %d, want 0", cfg.CorrelationWindowSeconds)
	}
}

// TestFromEnv_CorrelationWindowInvalid verifies an invalid value returns an error.
func TestFromEnv_CorrelationWindowInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_WINDOW_SECONDS", "not-a-number")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid CORRELATION_WINDOW_SECONDS, got nil")
	}
}

// TestFromEnv_DisableCorrelationTrue verifies DISABLE_CORRELATION=true sets the flag.
func TestFromEnv_DisableCorrelationTrue(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DISABLE_CORRELATION", "true")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DisableCorrelation {
		t.Error("expected DisableCorrelation=true, got false")
	}
}

// TestFromEnv_DisableCorrelationOne verifies DISABLE_CORRELATION=1 also sets the flag.
func TestFromEnv_DisableCorrelationOne(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DISABLE_CORRELATION", "1")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DisableCorrelation {
		t.Error("expected DisableCorrelation=true when DISABLE_CORRELATION=1, got false")
	}
}

// TestFromEnv_DisableCorrelationUnset verifies DisableCorrelation defaults to false.
func TestFromEnv_DisableCorrelationUnset(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("DISABLE_CORRELATION")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DisableCorrelation {
		t.Error("expected DisableCorrelation=false by default, got true")
	}
}

// TestFromEnv_DisableCorrelationFalseExplicit verifies DISABLE_CORRELATION=false is false.
func TestFromEnv_DisableCorrelationFalseExplicit(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DISABLE_CORRELATION", "false")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DisableCorrelation {
		t.Error("expected DisableCorrelation=false for explicit 'false', got true")
	}
}

// TestFromEnv_MultiPodThresholdDefault verifies CORRELATION_MULTI_POD_THRESHOLD defaults to 3.
func TestFromEnv_MultiPodThresholdDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("CORRELATION_MULTI_POD_THRESHOLD")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MultiPodThreshold != 3 {
		t.Errorf("MultiPodThreshold default: got %d, want 3", cfg.MultiPodThreshold)
	}
}

// TestFromEnv_MultiPodThresholdCustom verifies CORRELATION_MULTI_POD_THRESHOLD parses correctly.
func TestFromEnv_MultiPodThresholdCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_MULTI_POD_THRESHOLD", "5")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MultiPodThreshold != 5 {
		t.Errorf("MultiPodThreshold: got %d, want 5", cfg.MultiPodThreshold)
	}
}

// TestFromEnv_MultiPodThresholdInvalid verifies an invalid value returns an error.
func TestFromEnv_MultiPodThresholdInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_MULTI_POD_THRESHOLD", "not-a-number")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid CORRELATION_MULTI_POD_THRESHOLD, got nil")
	}
}

// TestFromEnv_MultiPodThresholdZeroInvalid verifies CORRELATION_MULTI_POD_THRESHOLD=0 is rejected
// (threshold must be >= 1).
func TestFromEnv_MultiPodThresholdZeroInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_MULTI_POD_THRESHOLD", "0")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for CORRELATION_MULTI_POD_THRESHOLD=0, got nil")
	}
}

// TestFromEnv_MultiPodThresholdOneValid verifies CORRELATION_MULTI_POD_THRESHOLD=1 is accepted
// (minimum valid value is 1).
func TestFromEnv_MultiPodThresholdOneValid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_MULTI_POD_THRESHOLD", "1")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error for CORRELATION_MULTI_POD_THRESHOLD=1: %v", err)
	}
	if cfg.MultiPodThreshold != 1 {
		t.Errorf("MultiPodThreshold: got %d, want 1", cfg.MultiPodThreshold)
	}
}

// TestFromEnv_MultiPodThresholdNegativeInvalid verifies negative values are rejected.
func TestFromEnv_MultiPodThresholdNegativeInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_MULTI_POD_THRESHOLD", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for CORRELATION_MULTI_POD_THRESHOLD=-1, got nil")
	}
}

// TestFromEnv_CorrelationWindowNegative verifies CORRELATION_WINDOW_SECONDS=-1 is rejected.
func TestFromEnv_CorrelationWindowNegative(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORRELATION_WINDOW_SECONDS", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for CORRELATION_WINDOW_SECONDS=-1, got nil")
	}
}
