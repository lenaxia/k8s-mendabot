package main

import (
	"testing"

	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/correlator"
)

// TestBuildCorrelator_DisableCorrelation verifies that DISABLE_CORRELATION=true
// causes buildCorrelator to return (nil, nil), leaving the reconciler with no
// Correlator and therefore no window hold.
func TestBuildCorrelator_DisableCorrelation(t *testing.T) {
	cfg := config.Config{
		DisableCorrelation: true,
		MultiPodThreshold:  3,
	}
	corr, err := buildCorrelator(cfg)
	if err != nil {
		t.Fatalf("expected nil error with DisableCorrelation=true, got %v", err)
	}
	if corr != nil {
		t.Errorf("expected nil Correlator with DisableCorrelation=true, got %+v", corr)
	}
}

// TestBuildCorrelator_ZeroThreshold verifies that MultiPodThreshold<=0 returns an error.
func TestBuildCorrelator_ZeroThreshold(t *testing.T) {
	cfg := config.Config{
		DisableCorrelation: false,
		MultiPodThreshold:  0,
	}
	_, err := buildCorrelator(cfg)
	if err == nil {
		t.Fatal("expected error when MultiPodThreshold=0, got nil")
	}
}

// TestBuildCorrelator_NegativeThreshold verifies that MultiPodThreshold<0 returns an error.
func TestBuildCorrelator_NegativeThreshold(t *testing.T) {
	cfg := config.Config{
		DisableCorrelation: false,
		MultiPodThreshold:  -1,
	}
	_, err := buildCorrelator(cfg)
	if err == nil {
		t.Fatal("expected error when MultiPodThreshold=-1, got nil")
	}
}

// TestBuildCorrelator_ValidConfig verifies that a valid config returns a non-nil
// Correlator with exactly three rules in the documented priority order:
// PVCPodRule, SameNamespaceParentRule, MultiPodSameNodeRule.
func TestBuildCorrelator_ValidConfig(t *testing.T) {
	cfg := config.Config{
		DisableCorrelation: false,
		MultiPodThreshold:  3,
	}
	corr, err := buildCorrelator(cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if corr == nil {
		t.Fatal("expected non-nil Correlator with valid config, got nil")
	}
	if len(corr.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(corr.Rules))
	}
	// Verify rule order by name.
	wantOrder := []string{"PVCPod", "SameNamespaceParent", "MultiPodSameNode"}
	for i, rule := range corr.Rules {
		if rule.Name() != wantOrder[i] {
			t.Errorf("rule[%d]: got %q, want %q", i, rule.Name(), wantOrder[i])
		}
	}
}

// TestBuildCorrelator_ThresholdPropagated verifies that the configured threshold is
// propagated into the MultiPodSameNodeRule.
func TestBuildCorrelator_ThresholdPropagated(t *testing.T) {
	const threshold = 5
	cfg := config.Config{
		DisableCorrelation: false,
		MultiPodThreshold:  threshold,
	}
	corr, err := buildCorrelator(cfg)
	if err != nil {
		t.Fatalf("buildCorrelator: %v", err)
	}
	// The last rule must be a MultiPodSameNodeRule with Threshold==5.
	last := corr.Rules[len(corr.Rules)-1]
	mpsnr, ok := last.(correlator.MultiPodSameNodeRule)
	if !ok {
		t.Fatalf("last rule is %T, want correlator.MultiPodSameNodeRule", last)
	}
	if mpsnr.Threshold != threshold {
		t.Errorf("MultiPodSameNodeRule.Threshold = %d, want %d", mpsnr.Threshold, threshold)
	}
}
