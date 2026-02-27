// Package charts contains regression tests for the Helm chart templates.
// These tests parse the raw YAML files directly — no Helm binary or cluster is needed.
// They exist to catch RBAC gaps that escape code review and unit testing because
// the agent kubeconfig is only exercised in a live cluster.
//
// Background: v0.3.25 shipped with the agent ClusterRole missing "patch" on
// remediationjobs/status. The agent's `kubectl patch --subresource=status` call
// was silently rejected with 403, leaving status.sinkRef empty and preventing
// Epic 26 auto-close from ever firing. This file ensures that regression cannot
// recur undetected.
package charts

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

// policyRule mirrors the subset of an RBAC rule we need to inspect.
type policyRule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

// rbacManifest is a minimal struct for parsing ClusterRole / Role YAML.
type rbacManifest struct {
	Kind  string       `yaml:"kind"`
	Rules []policyRule `yaml:"rules"`
}

// chartsDir returns the absolute path to charts/mendabot/templates relative to
// this test file, regardless of where `go test` is invoked from.
func chartsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file = .../internal/charts/rbac_test.go
	// repo root = three levels up
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "charts", "mendabot", "templates")
}

// helmLineRE matches lines that are pure Helm template directives, e.g.:
//
//	{{- if .Values.rbac.create }}
//	{{- end }}
//	{{- include "foo.labels" . | nindent 4 }}
var helmLineRE = regexp.MustCompile(`(?m)^\s*\{\{[^}]*\}\}\s*$`)

// helmInlineRE matches Helm expressions embedded within a YAML value, e.g.:
//
//	name: {{ include "mendabot.fullname" . }}-agent
//	namespace: {{ .Release.Namespace }}
var helmInlineRE = regexp.MustCompile(`\{\{[^}]*\}\}`)

// loadRBAC parses a single RBAC YAML file, stripping Helm template directives
// so that the standard YAML parser can handle it.
//
// Two passes are made:
//  1. Lines that consist entirely of a Helm expression are removed.
//  2. Remaining inline Helm expressions (e.g. in metadata.name) are replaced
//     with the placeholder string "HELM_VALUE" so the YAML stays structurally
//     valid.
func loadRBAC(t *testing.T, path string) rbacManifest {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	// Pass 1: remove pure-Helm lines.
	cleaned := helmLineRE.ReplaceAll(raw, nil)
	// Pass 2: replace inline Helm expressions with a placeholder.
	cleaned = helmInlineRE.ReplaceAll(cleaned, []byte("HELM_VALUE"))

	var m rbacManifest
	if err := yaml.Unmarshal(cleaned, &m); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return m
}

// hasRule returns true if the manifest contains a rule that grants all of
// wantVerbs on wantResource in wantGroup.
func hasRule(m rbacManifest, wantGroup, wantResource string, wantVerbs ...string) bool {
	for _, rule := range m.Rules {
		if !containsStr(rule.APIGroups, wantGroup) {
			continue
		}
		if !containsStr(rule.Resources, wantResource) {
			continue
		}
		allFound := true
		for _, v := range wantVerbs {
			if !containsStr(rule.Verbs, v) {
				allFound = false
				break
			}
		}
		if allFound {
			return true
		}
	}
	return false
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestAgentClusterRole_CanPatchRemediationJobStatus ensures the ClusterRole
// (used when agentRBACScope=cluster, the default) grants "patch" on the
// remediationjobs/status subresource.
//
// Regression: v0.3.25 shipped without this permission, causing the agent's
// `kubectl patch --subresource=status` to be rejected with 403. The sinkRef
// field was never written, so Epic 26 auto-close never triggered.
func TestAgentClusterRole_CanPatchRemediationJobStatus(t *testing.T) {
	path := filepath.Join(chartsDir(t), "clusterrole-agent.yaml")
	m := loadRBAC(t, path)

	const (
		group    = "remediation.mendabot.io"
		resource = "remediationjobs/status"
	)

	if !hasRule(m, group, resource, "patch") {
		t.Errorf(
			"%s: ClusterRole is missing \"patch\" on %s/%s\n"+
				"The agent writes status.sinkRef via kubectl patch --subresource=status.\n"+
				"Without this permission the patch is rejected with 403 and sinkRef is\n"+
				"never populated, so Epic 26 auto-close never fires.",
			path, group, resource,
		)
	}
}

// TestAgentClusterRole_CanGetRemediationJobStatus ensures the ClusterRole
// also retains "get" on remediationjobs/status (needed for kubectl get -o json
// inside agent scripts).
func TestAgentClusterRole_CanGetRemediationJobStatus(t *testing.T) {
	path := filepath.Join(chartsDir(t), "clusterrole-agent.yaml")
	m := loadRBAC(t, path)

	if !hasRule(m, "remediation.mendabot.io", "remediationjobs/status", "get") {
		t.Errorf("%s: ClusterRole is missing \"get\" on remediationjobs/status", path)
	}
}

// TestAgentClusterRole_CanListWatchRemediationJobs ensures the ClusterRole
// retains list/watch on the main remediationjobs resource (used by the agent
// to look up its own rjob at startup).
func TestAgentClusterRole_CanListWatchRemediationJobs(t *testing.T) {
	path := filepath.Join(chartsDir(t), "clusterrole-agent.yaml")
	m := loadRBAC(t, path)

	for _, verb := range []string{"get", "list", "watch"} {
		if !hasRule(m, "remediation.mendabot.io", "remediationjobs", verb) {
			t.Errorf("%s: ClusterRole is missing %q on remediationjobs", path, verb)
		}
	}
}

// TestAgentNamespaceRole_CanPatchRemediationJobStatus ensures the namespace-scoped
// Role (used when agentRBACScope=namespace) also grants "patch" on
// remediationjobs/status.
func TestAgentNamespaceRole_CanPatchRemediationJobStatus(t *testing.T) {
	path := filepath.Join(chartsDir(t), "role-agent.yaml")
	m := loadRBAC(t, path)

	if !hasRule(m, "remediation.mendabot.io", "remediationjobs/status", "patch") {
		t.Errorf(
			"%s: namespace-scoped Role is missing \"patch\" on remediationjobs/status",
			path,
		)
	}
}

// TestAgentNamespaceRole_CanGetRemediationJobStatus ensures the namespace-scoped
// Role retains "get" on remediationjobs/status.
func TestAgentNamespaceRole_CanGetRemediationJobStatus(t *testing.T) {
	path := filepath.Join(chartsDir(t), "role-agent.yaml")
	m := loadRBAC(t, path)

	if !hasRule(m, "remediation.mendabot.io", "remediationjobs/status", "get") {
		t.Errorf("%s: namespace-scoped Role is missing \"get\" on remediationjobs/status", path)
	}
}

// TestAgentClusterRole_NoDuplicateStatusVerbs is a hygiene check: verifies
// there is exactly one rule covering remediationjobs/status so that future
// authors don't accidentally create a rule that looks correct but doesn't
// actually grant patch (e.g. a separate rule with only get/list).
func TestAgentClusterRole_NoDuplicateStatusRules(t *testing.T) {
	path := filepath.Join(chartsDir(t), "clusterrole-agent.yaml")
	m := loadRBAC(t, path)

	var count int
	for _, rule := range m.Rules {
		if containsStr(rule.APIGroups, "remediation.mendabot.io") &&
			containsStr(rule.Resources, "remediationjobs/status") {
			count++
		}
	}
	if count != 1 {
		t.Errorf(
			"%s: expected exactly 1 rule covering remediationjobs/status, found %d\n"+
				"Multiple rules for the same resource can cause confusion about effective permissions.",
			path, count,
		)
	}
}
