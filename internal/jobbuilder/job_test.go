package jobbuilder

import (
	"encoding/json"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/config"
)

var testRJob = &v1alpha1.RemediationJob{
	ObjectMeta: metav1.ObjectMeta{
		Name: "mechanic-abc123def456",
		UID:  types.UID("test-uid-1234"),
	},
	Spec: v1alpha1.RemediationJobSpec{
		AgentImage:         "ghcr.io/lenaxia/mechanic-agent:latest",
		AgentSA:            "mechanic-agent",
		GitOpsRepo:         "lenaxia/talos-ops-prod",
		GitOpsManifestRoot: "kubernetes/",
		SinkType:           "github",
		Fingerprint:        "abcdef012345abcdef012345abcdef012345abcdef012345abcdef012345abcd",
		Severity:           "high",
		Finding: v1alpha1.FindingSpec{
			Kind:         "Deployment",
			Name:         "my-app",
			Namespace:    "production",
			ParentObject: "my-app",
			Errors:       `[{"text":"ImagePullBackOff"}]`,
			Details:      "some details",
		},
	},
}

func buildJob(t *testing.T) *batchv1.Job {
	t.Helper()
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return job
}

func getEnv(container corev1.Container, name string) (string, bool) {
	for _, e := range container.Env {
		if e.Name == name {
			if e.ValueFrom != nil {
				return "", true
			}
			return e.Value, true
		}
	}
	return "", false
}

func findVolumeMount(container corev1.Container, name string) (corev1.VolumeMount, bool) {
	for _, vm := range container.VolumeMounts {
		if vm.Name == name {
			return vm, true
		}
	}
	return corev1.VolumeMount{}, false
}

func findVolume(spec corev1.PodSpec, name string) (corev1.Volume, bool) {
	for _, v := range spec.Volumes {
		if v.Name == name {
			return v, true
		}
	}
	return corev1.Volume{}, false
}

func findSecretKeyRef(container corev1.Container, name string) *corev1.SecretKeySelector {
	for _, e := range container.Env {
		if e.Name == name && e.ValueFrom != nil {
			return e.ValueFrom.SecretKeyRef
		}
	}
	return nil
}

func TestNew_EmptyAgentNamespace_ReturnsError(t *testing.T) {
	_, err := New(Config{AgentNamespace: ""})
	if err == nil {
		t.Fatal("expected error when AgentNamespace is empty, got nil")
	}
}

func TestNew_ValidConfig_Succeeds(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil Builder")
	}
}

var _ func(*v1alpha1.RemediationJob, []v1alpha1.FindingSpec) (*batchv1.Job, error) = (*Builder)(nil).Build

func TestBuild_JobName(t *testing.T) {
	job := buildJob(t)
	want := "mechanic-agent-abcdef012345"
	if job.Name != want {
		t.Errorf("job name = %q, want %q", job.Name, want)
	}
}

func TestBuild_JobNameDeterministic(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job1, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build first: %v", err)
	}
	job2, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build second: %v", err)
	}
	if job1.Name != job2.Name {
		t.Errorf("non-deterministic: got %q then %q", job1.Name, job2.Name)
	}
}

func TestBuild_Namespace(t *testing.T) {
	job := buildJob(t)
	if job.Namespace != "mechanic" {
		t.Errorf("namespace = %q, want %q", job.Namespace, "mechanic")
	}
}

func TestBuild_ServiceAccount(t *testing.T) {
	job := buildJob(t)
	got := job.Spec.Template.Spec.ServiceAccountName
	if got != "mechanic-agent" {
		t.Errorf("ServiceAccountName = %q, want %q", got, "mechanic-agent")
	}
}

func TestBuild_EnvVars_AllPresent(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]

	required := []string{
		"FINDING_KIND",
		"FINDING_NAME",
		"FINDING_NAMESPACE",
		"FINDING_PARENT",
		"FINDING_ERRORS",
		"FINDING_DETAILS",
		"FINDING_FINGERPRINT",
		"FINDING_SEVERITY",
		"GITOPS_REPO",
		"GITOPS_MANIFEST_ROOT",
		"SINK_TYPE",
		"AGENT_PROVIDER_CONFIG",
		"AGENT_TYPE",
		"AGENT_NAMESPACE",
	}
	for _, name := range required {
		if _, ok := getEnv(main, name); !ok {
			t.Errorf("env var %q missing from main container", name)
		}
	}
	if val, ok := getEnv(main, "FINDING_SEVERITY"); !ok || val != "high" {
		t.Errorf("FINDING_SEVERITY = %q (ok=%v), want %q", val, ok, "high")
	}
	// FINDING_FINGERPRINT must be the 12-char short form so it stays below the
	// 40-char base64 redaction threshold and is not redacted in gh output.
	if val, ok := getEnv(main, "FINDING_FINGERPRINT"); !ok || val != "abcdef012345" {
		t.Errorf("FINDING_FINGERPRINT = %q (ok=%v), want %q", val, ok, "abcdef012345")
	}
	// AGENT_NAMESPACE must equal the builder's AgentNamespace so emit_dry_run_report()
	// writes the ConfigMap to the same namespace the controller reads from.
	if val, ok := getEnv(main, "AGENT_NAMESPACE"); !ok || val != "mechanic" {
		t.Errorf("AGENT_NAMESPACE = %q (ok=%v), want %q", val, ok, "mechanic")
	}
}

func TestBuild_EnvVars_FindingNameNoNamespacePrefix(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_NAME")
	if !ok {
		t.Fatal("FINDING_NAME not found")
	}
	if strings.Contains(val, "/") {
		t.Errorf("FINDING_NAME should be plain name, got %q", val)
	}
	if val != "my-app" {
		t.Errorf("FINDING_NAME = %q, want %q", val, "my-app")
	}
}

func TestBuild_EnvVars_FindingNamespace(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_NAMESPACE")
	if !ok {
		t.Fatal("FINDING_NAMESPACE not found")
	}
	if val != "production" {
		t.Errorf("FINDING_NAMESPACE = %q, want %q", val, "production")
	}
}

func TestBuild_EnvVars_ErrorsJSON(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_ERRORS")
	if !ok {
		t.Fatal("FINDING_ERRORS not found")
	}
	want := `[{"text":"ImagePullBackOff"}]`
	if val != want {
		t.Errorf("FINDING_ERRORS = %q, want %q", val, want)
	}
}

func TestBuild_EnvVars_SinkType(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "SINK_TYPE")
	if !ok {
		t.Fatal("SINK_TYPE not found")
	}
	if val != "github" {
		t.Errorf("SINK_TYPE = %q, want %q", val, "github")
	}
}

func TestBuild_InitContainer_Present(t *testing.T) {
	job := buildJob(t)
	inits := job.Spec.Template.Spec.InitContainers
	if len(inits) == 0 {
		t.Fatal("no init containers found")
	}
	found := false
	for _, c := range inits {
		if c.Name == "git-token-clone" {
			found = true
			break
		}
	}
	if !found {
		t.Error("init container named \"git-token-clone\" not found")
	}
}

func TestBuild_InitContainer_UsesAgentImage(t *testing.T) {
	job := buildJob(t)
	var initContainer corev1.Container
	for _, c := range job.Spec.Template.Spec.InitContainers {
		if c.Name == "git-token-clone" {
			initContainer = c
			break
		}
	}
	main := job.Spec.Template.Spec.Containers[0]
	if initContainer.Image != main.Image {
		t.Errorf("init container image = %q, main container image = %q; want same", initContainer.Image, main.Image)
	}
	if initContainer.Image != testRJob.Spec.AgentImage {
		t.Errorf("init container image = %q, want %q", initContainer.Image, testRJob.Spec.AgentImage)
	}
}

func TestBuild_MainContainer_Present(t *testing.T) {
	job := buildJob(t)
	containers := job.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		t.Fatal("no containers found")
	}
	found := false
	for _, c := range containers {
		if c.Name == "mechanic-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("main container named \"mechanic-agent\" not found")
	}
}

func TestBuild_MainContainer_NoCommandOverride(t *testing.T) {
	job := buildJob(t)
	var main corev1.Container
	for _, c := range job.Spec.Template.Spec.Containers {
		if c.Name == "mechanic-agent" {
			main = c
			break
		}
	}
	if len(main.Command) != 0 {
		t.Errorf("main container Command should be empty (entrypoint in image), got %v", main.Command)
	}
}

func TestBuild_SecretKeyRefs(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]

	tests := []struct {
		envName    string
		secretName string
		key        string
	}{
		{"AGENT_PROVIDER_CONFIG", "llm-credentials-opencode", "provider-config"},
	}

	for _, tt := range tests {
		t.Run(tt.envName, func(t *testing.T) {
			ref := findSecretKeyRef(main, tt.envName)
			if ref == nil {
				t.Fatalf("env var %q has no secretKeyRef", tt.envName)
			}
			if ref.Name != tt.secretName {
				t.Errorf("secretKeyRef.Name = %q, want %q", ref.Name, tt.secretName)
			}
			if ref.Key != tt.key {
				t.Errorf("secretKeyRef.Key = %q, want %q", ref.Key, tt.key)
			}
		})
	}

	var initContainer corev1.Container
	for _, c := range job.Spec.Template.Spec.InitContainers {
		if c.Name == "git-token-clone" {
			initContainer = c
			break
		}
	}
	initSecretTests := []struct {
		envName    string
		secretName string
		key        string
	}{
		{"GITHUB_APP_ID", "github-app", "app-id"},
		{"GITHUB_APP_INSTALLATION_ID", "github-app", "installation-id"},
		{"GITHUB_APP_PRIVATE_KEY", "github-app", "private-key"},
	}
	for _, tt := range initSecretTests {
		t.Run("init_"+tt.envName, func(t *testing.T) {
			ref := findSecretKeyRef(initContainer, tt.envName)
			if ref == nil {
				t.Fatalf("init env var %q has no secretKeyRef", tt.envName)
			}
			if ref.Name != tt.secretName {
				t.Errorf("secretKeyRef.Name = %q, want %q", ref.Name, tt.secretName)
			}
			if ref.Key != tt.key {
				t.Errorf("secretKeyRef.Key = %q, want %q", ref.Key, tt.key)
			}
		})
	}
}

func TestBuild_Volumes_AllPresent(t *testing.T) {
	job := buildJob(t)
	podSpec := job.Spec.Template.Spec

	podVolumeTests := []string{"shared-workspace", "prompt-configmap", "agent-token"}
	for _, name := range podVolumeTests {
		if _, ok := findVolume(podSpec, name); !ok {
			t.Errorf("pod volume %q not found", name)
		}
	}

	var main corev1.Container
	for _, c := range podSpec.Containers {
		if c.Name == "mechanic-agent" {
			main = c
			break
		}
	}
	for _, name := range []string{"shared-workspace", "prompt-configmap", "agent-token"} {
		if _, ok := findVolumeMount(main, name); !ok {
			t.Errorf("main container volume mount %q not found", name)
		}
	}
	if _, ok := findVolume(podSpec, "github-app-secret"); ok {
		t.Error("github-app-secret volume must NOT exist in pod spec — credentials are env-var injected, not file-mounted")
	}
	if _, ok := findVolumeMount(main, "github-app-secret"); ok {
		t.Error("main container must NOT mount github-app-secret (security)")
	}

	var initContainer corev1.Container
	for _, c := range podSpec.InitContainers {
		if c.Name == "git-token-clone" {
			initContainer = c
			break
		}
	}
	for _, name := range []string{"shared-workspace"} {
		if _, ok := findVolumeMount(initContainer, name); !ok {
			t.Errorf("init container volume mount %q not found", name)
		}
	}
	// github-app-secret volume must NOT be mounted in init container —
	// credentials are read via env vars (GITHUB_APP_ID etc.), not files.
	if _, ok := findVolumeMount(initContainer, "github-app-secret"); ok {
		t.Error("init container must NOT mount github-app-secret — credentials are env-var injected")
	}
}

func TestBuild_JobSettings(t *testing.T) {
	job := buildJob(t)
	spec := job.Spec

	if spec.BackoffLimit == nil || *spec.BackoffLimit != 1 {
		t.Errorf("BackoffLimit = %v, want 1", spec.BackoffLimit)
	}
	if spec.ActiveDeadlineSeconds == nil || *spec.ActiveDeadlineSeconds != 900 {
		t.Errorf("ActiveDeadlineSeconds = %v, want 900", spec.ActiveDeadlineSeconds)
	}
	if spec.TTLSecondsAfterFinished == nil || *spec.TTLSecondsAfterFinished != 86400 {
		t.Errorf("TTLSecondsAfterFinished = %v, want 86400", spec.TTLSecondsAfterFinished)
	}
}

func TestBuild_RestartPolicy(t *testing.T) {
	job := buildJob(t)
	got := job.Spec.Template.Spec.RestartPolicy
	if got != corev1.RestartPolicyNever {
		t.Errorf("RestartPolicy = %q, want %q", got, corev1.RestartPolicyNever)
	}
}

func TestBuild_Labels(t *testing.T) {
	job := buildJob(t)
	labels := job.Labels

	tests := []struct {
		key  string
		want string
	}{
		{"app.kubernetes.io/managed-by", "mechanic-watcher"},
		{"remediation.mechanic.io/fingerprint", "abcdef012345"},
		{"remediation.mechanic.io/remediation-job", "mechanic-abc123def456"},
		{"remediation.mechanic.io/finding-kind", "Deployment"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got, ok := labels[tt.key]; !ok {
				t.Errorf("label %q missing", tt.key)
			} else if got != tt.want {
				t.Errorf("label %q = %q, want %q", tt.key, got, tt.want)
			}
		})
	}

	annotations := job.Annotations
	annotationTests := []struct {
		key  string
		want string
	}{
		{"remediation.mechanic.io/fingerprint-full", "abcdef012345abcdef012345abcdef012345abcdef012345abcdef012345abcd"},
		{"remediation.mechanic.io/finding-parent", "my-app"},
	}
	for _, tt := range annotationTests {
		t.Run("annotation_"+tt.key, func(t *testing.T) {
			if got, ok := annotations[tt.key]; !ok {
				t.Errorf("annotation %q missing", tt.key)
			} else if got != tt.want {
				t.Errorf("annotation %q = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestBuild_OwnerReference(t *testing.T) {
	job := buildJob(t)
	refs := job.OwnerReferences
	if len(refs) != 1 {
		t.Fatalf("expected 1 OwnerReference, got %d", len(refs))
	}
	ref := refs[0]
	if ref.APIVersion != "remediation.mechanic.io/v1alpha1" {
		t.Errorf("APIVersion = %q, want %q", ref.APIVersion, "remediation.mechanic.io/v1alpha1")
	}
	if ref.Kind != "RemediationJob" {
		t.Errorf("Kind = %q, want %q", ref.Kind, "RemediationJob")
	}
	if ref.Name != "mechanic-abc123def456" {
		t.Errorf("Name = %q, want %q", ref.Name, "mechanic-abc123def456")
	}
	if ref.UID != "test-uid-1234" {
		t.Errorf("UID = %q, want %q", ref.UID, "test-uid-1234")
	}
	if ref.Controller == nil || !*ref.Controller {
		t.Error("Controller should be ptr(true)")
	}
	if ref.BlockOwnerDeletion == nil || !*ref.BlockOwnerDeletion {
		t.Error("BlockOwnerDeletion should be ptr(true)")
	}
}

func TestBuild_EmptyErrors(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rjob := *testRJob
	rjob.Spec.Finding.Errors = ""
	job, err := b.Build(&rjob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_ERRORS")
	if !ok {
		t.Fatal("FINDING_ERRORS not found")
	}
	if val != "" {
		t.Errorf("FINDING_ERRORS = %q, want empty string", val)
	}
}

func TestBuild_LongDetails(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	longDetails := strings.Repeat("x", 100_000)
	rjob := *testRJob
	rjob.Spec.Finding.Details = longDetails
	job, err := b.Build(&rjob, nil)
	if err != nil {
		t.Fatalf("Build with long details: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_DETAILS")
	if !ok {
		t.Fatal("FINDING_DETAILS not found")
	}
	if val != longDetails {
		t.Errorf("FINDING_DETAILS truncated: got len=%d, want len=%d", len(val), len(longDetails))
	}
}

func TestBuild_NilRJob(t *testing.T) {
	b, _ := New(Config{AgentNamespace: "mechanic"})
	_, err := b.Build(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil rjob, got nil")
	}
}

func TestBuild_EmptyFingerprint(t *testing.T) {
	b, _ := New(Config{AgentNamespace: "mechanic"})
	_, err := b.Build(&v1alpha1.RemediationJob{}, nil)
	if err == nil {
		t.Fatal("expected error for empty fingerprint, got nil")
	}
}

func TestBuild_ShortFingerprint(t *testing.T) {
	b, _ := New(Config{AgentNamespace: "mechanic"})
	_, err := b.Build(&v1alpha1.RemediationJob{
		Spec: v1alpha1.RemediationJobSpec{Fingerprint: "abc"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for short fingerprint, got nil")
	}
}

func TestBuild_SecurityContexts(t *testing.T) {
	job := buildJob(t)
	for _, c := range append(job.Spec.Template.Spec.InitContainers, job.Spec.Template.Spec.Containers...) {
		if c.SecurityContext == nil {
			t.Fatalf("container %q: SecurityContext is nil", c.Name)
		}
		if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
			t.Errorf("container %q: expected AllowPrivilegeEscalation=false", c.Name)
		}
		if c.SecurityContext.Capabilities == nil {
			t.Fatalf("container %q: Capabilities is nil", c.Name)
		}
		found := false
		for _, cap := range c.SecurityContext.Capabilities.Drop {
			if cap == "ALL" {
				found = true
			}
		}
		if !found {
			t.Errorf("container %q: expected Capabilities.Drop to contain ALL", c.Name)
		}
	}
	main := job.Spec.Template.Spec.Containers[0]
	if main.SecurityContext.ReadOnlyRootFilesystem != nil {
		t.Error("main container: ReadOnlyRootFilesystem must not be set")
	}
}

func TestBuild_InitScript_UsesGitHubAppToken(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var initContainer corev1.Container
	for _, c := range job.Spec.Template.Spec.InitContainers {
		if c.Name == "git-token-clone" {
			initContainer = c
			break
		}
	}

	script := initContainer.Args[0]
	if !contains(script, "get-github-app-token.sh") {
		t.Error("init script should call get-github-app-token.sh")
	}
	if !contains(script, "x-access-token:${TOKEN}") {
		t.Error("init script should use GitHub App token for authentication")
	}
}

func TestBuild_InitScript_HasErrorHandling(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var initContainer corev1.Container
	for _, c := range job.Spec.Template.Spec.InitContainers {
		if c.Name == "git-token-clone" {
			initContainer = c
			break
		}
	}

	script := initContainer.Args[0]
	if !contains(script, "ERROR: Failed to clone") {
		t.Error("init script should have error handling for clone failures")
	}
	if !contains(script, "The GitHub App token does not have access to this repository") {
		t.Error("init script should explain authentication failures")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestBuild_PodSecurityContext(t *testing.T) {
	job := buildJob(t)
	sc := job.Spec.Template.Spec.SecurityContext
	if sc == nil {
		t.Fatal("PodSecurityContext is nil")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("expected RunAsNonRoot=true")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser != 1000 {
		t.Errorf("expected RunAsUser=1000, got %v", sc.RunAsUser)
	}
}

func TestBuild_SingleFinding_NoCorrelatedEnvVar(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	if _, ok := getEnv(main, "FINDING_CORRELATED_FINDINGS"); ok {
		t.Error("FINDING_CORRELATED_FINDINGS must not be set for single-finding dispatch (nil)")
	}
	if _, ok := getEnv(main, "FINDING_CORRELATION_GROUP_ID"); ok {
		t.Error("FINDING_CORRELATION_GROUP_ID must not be set when rjob has no correlation label")
	}
}

func TestBuild_SingleElementSlice_SetsCorrelatedEnvVar(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	oneFindings := []v1alpha1.FindingSpec{
		{Kind: "Deployment", Name: "app-a", Namespace: "prod"},
	}
	job, err := b.Build(testRJob, oneFindings)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	if _, ok := getEnv(main, "FINDING_CORRELATED_FINDINGS"); !ok {
		t.Error("FINDING_CORRELATED_FINDINGS must be set when len(correlatedFindings) == 1")
	}
}

func TestBuild_TwoCorrelatedFindings_EnvVarSet(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	findings := []v1alpha1.FindingSpec{
		{Kind: "Deployment", Name: "app-a", Namespace: "prod", ParentObject: "app-a", Errors: `[{"text":"OOMKilled"}]`, Details: "detail-a"},
		{Kind: "Pod", Name: "pod-b", Namespace: "staging", ParentObject: "app-b", Errors: `[{"text":"CrashLoopBackOff"}]`, Details: "detail-b"},
	}
	job, err := b.Build(testRJob, findings)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_CORRELATED_FINDINGS")
	if !ok {
		t.Fatal("FINDING_CORRELATED_FINDINGS must be set for multi-finding dispatch")
	}
	if val == "" {
		t.Fatal("FINDING_CORRELATED_FINDINGS must not be empty")
	}

	var decoded []v1alpha1.FindingSpec
	if err := json.Unmarshal([]byte(val), &decoded); err != nil {
		t.Fatalf("FINDING_CORRELATED_FINDINGS is not valid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded %d findings, want 2", len(decoded))
	}
	if decoded[0].Kind != "Deployment" || decoded[0].Name != "app-a" {
		t.Errorf("decoded[0] = %+v, want Kind=Deployment Name=app-a", decoded[0])
	}
	if decoded[1].Kind != "Pod" || decoded[1].Name != "pod-b" {
		t.Errorf("decoded[1] = %+v, want Kind=Pod Name=pod-b", decoded[1])
	}
}

func TestBuild_CorrelatedFindings_AllFieldsEncoded(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	findings := []v1alpha1.FindingSpec{
		{Kind: "StatefulSet", Name: "db", Namespace: "database", ParentObject: "db-cluster", Errors: `[{"text":"PVCBound"}]`, Details: "storage issue"},
		{Kind: "Deployment", Name: "api", Namespace: "backend", ParentObject: "api-deploy", Errors: `[{"text":"ImageError"}]`, Details: "image pull"},
	}
	job, err := b.Build(testRJob, findings)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	val, _ := getEnv(main, "FINDING_CORRELATED_FINDINGS")

	var decoded []v1alpha1.FindingSpec
	if err := json.Unmarshal([]byte(val), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	tests := []struct {
		idx         int
		wantKind    string
		wantName    string
		wantNS      string
		wantParent  string
		wantErrors  string
		wantDetails string
	}{
		{0, "StatefulSet", "db", "database", "db-cluster", `[{"text":"PVCBound"}]`, "storage issue"},
		{1, "Deployment", "api", "backend", "api-deploy", `[{"text":"ImageError"}]`, "image pull"},
	}
	for _, tt := range tests {
		f := decoded[tt.idx]
		if f.Kind != tt.wantKind {
			t.Errorf("[%d] Kind = %q, want %q", tt.idx, f.Kind, tt.wantKind)
		}
		if f.Name != tt.wantName {
			t.Errorf("[%d] Name = %q, want %q", tt.idx, f.Name, tt.wantName)
		}
		if f.Namespace != tt.wantNS {
			t.Errorf("[%d] Namespace = %q, want %q", tt.idx, f.Namespace, tt.wantNS)
		}
		if f.ParentObject != tt.wantParent {
			t.Errorf("[%d] ParentObject = %q, want %q", tt.idx, f.ParentObject, tt.wantParent)
		}
		if f.Errors != tt.wantErrors {
			t.Errorf("[%d] Errors = %q, want %q", tt.idx, f.Errors, tt.wantErrors)
		}
		if f.Details != tt.wantDetails {
			t.Errorf("[%d] Details = %q, want %q", tt.idx, f.Details, tt.wantDetails)
		}
	}
}

func TestBuild_SecretName_ByAgentType(t *testing.T) {
	tests := []struct {
		agentType       config.AgentType
		wantSecretName  string
		wantAgentCMName string
	}{
		{config.AgentTypeOpenCode, "llm-credentials-opencode", "agent-prompt-opencode"},
		{config.AgentTypeClaude, "llm-credentials-claude", "agent-prompt-claude"},
	}
	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			b, err := New(Config{AgentNamespace: "mechanic", AgentType: tt.agentType})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			job, err := b.Build(testRJob, nil)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			main := job.Spec.Template.Spec.Containers[0]

			ref := findSecretKeyRef(main, "AGENT_PROVIDER_CONFIG")
			if ref == nil {
				t.Fatal("AGENT_PROVIDER_CONFIG has no secretKeyRef")
			}
			if ref.Name != tt.wantSecretName {
				t.Errorf("secret name = %q, want %q", ref.Name, tt.wantSecretName)
			}

			podSpec := job.Spec.Template.Spec
			vol, ok := findVolume(podSpec, "prompt-configmap")
			if !ok {
				t.Fatal("prompt-configmap volume not found")
			}
			if vol.Projected == nil {
				t.Fatal("prompt-configmap volume has no Projected source")
			}
			// Expect exactly two sources: core CM and agent CM.
			if len(vol.Projected.Sources) != 2 {
				t.Fatalf("projected sources len = %d, want 2", len(vol.Projected.Sources))
			}
			coreSrc := vol.Projected.Sources[0].ConfigMap
			agentSrc := vol.Projected.Sources[1].ConfigMap
			if coreSrc == nil {
				t.Fatal("projected source[0] has no ConfigMap")
			}
			if agentSrc == nil {
				t.Fatal("projected source[1] has no ConfigMap")
			}
			if coreSrc.Name != "agent-prompt-core" {
				t.Errorf("core CM name = %q, want %q", coreSrc.Name, "agent-prompt-core")
			}
			if agentSrc.Name != tt.wantAgentCMName {
				t.Errorf("agent CM name = %q, want %q", agentSrc.Name, tt.wantAgentCMName)
			}
			// Verify KeyToPath bindings — entrypoint-common.sh reads exactly
			// /prompt/core.txt and /prompt/agent.txt.
			if len(coreSrc.Items) != 1 || coreSrc.Items[0].Key != "core.txt" || coreSrc.Items[0].Path != "core.txt" {
				t.Errorf("core CM KeyToPath = %v, want [{core.txt core.txt}]", coreSrc.Items)
			}
			if len(agentSrc.Items) != 1 || agentSrc.Items[0].Key != "agent.txt" || agentSrc.Items[0].Path != "agent.txt" {
				t.Errorf("agent CM KeyToPath = %v, want [{agent.txt agent.txt}]", agentSrc.Items)
			}
			// AGENT_TYPE must be injected so the dispatcher entrypoint routes
			// to the correct agent binary for this agent type.
			agentTypeVal, ok := getEnv(main, "AGENT_TYPE")
			if !ok {
				t.Fatal("AGENT_TYPE env var missing from main container")
			}
			if agentTypeVal != string(tt.agentType) {
				t.Errorf("AGENT_TYPE = %q, want %q", agentTypeVal, string(tt.agentType))
			}
		})
	}
}

// --- BUG-1: TTL must come from Config, not be hardcoded ---

// TestBuild_TTL_DefaultIs86400 verifies that when no TTL is specified the Job
// still gets a sensible default (86400s = 24h).
func TestBuild_TTL_DefaultIs86400(t *testing.T) {
	job := buildJob(t)
	if job.Spec.TTLSecondsAfterFinished == nil {
		t.Fatal("TTLSecondsAfterFinished is nil")
	}
	if *job.Spec.TTLSecondsAfterFinished != 86400 {
		t.Errorf("TTLSecondsAfterFinished = %d, want 86400", *job.Spec.TTLSecondsAfterFinished)
	}
}

// TestBuild_TTL_HonorsConfig verifies that a non-default TTL set in Config is
// reflected in the Job spec. This test FAILS until Config gets a TTLSeconds
// field and job.go uses it.
func TestBuild_TTL_HonorsConfig(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic", TTLSeconds: 3600})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if job.Spec.TTLSecondsAfterFinished == nil {
		t.Fatal("TTLSecondsAfterFinished is nil")
	}
	if *job.Spec.TTLSecondsAfterFinished != 3600 {
		t.Errorf("TTLSecondsAfterFinished = %d, want 3600 (from Config.TTLSeconds)",
			*job.Spec.TTLSecondsAfterFinished)
	}
}

// --- BUG-2: Init container must NOT mount github-app-secret volume ---
// The init script reads GitHub App credentials via env vars only.
// The volume mount at /secrets/github-app is unused dead weight.

func TestBuild_InitContainer_DoesNotMountGithubAppSecret(t *testing.T) {
	job := buildJob(t)
	var init corev1.Container
	for _, c := range job.Spec.Template.Spec.InitContainers {
		if c.Name == "git-token-clone" {
			init = c
			break
		}
	}
	for _, vm := range init.VolumeMounts {
		if vm.Name == "github-app-secret" {
			t.Errorf("init container must NOT mount github-app-secret — credentials are read via env vars, not files")
		}
	}
}

// --- COMPLEXITY-1: AGENT_MODEL must NOT be injected into the Job container ---
// The model is embedded inside AGENT_PROVIDER_CONFIG (opaque blob).
// Injecting AGENT_MODEL as a separate env var is dead weight that misleads
// operators into thinking it drives model selection.

func TestBuild_AGENT_MODEL_NotInjected(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	for _, e := range main.Env {
		if e.Name == "AGENT_MODEL" {
			t.Error("AGENT_MODEL must not be injected into the Job container — model selection is driven by AGENT_PROVIDER_CONFIG blob")
		}
	}
}

func TestBuild_DefaultAgentType_IsOpenCode(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	ref := findSecretKeyRef(main, "AGENT_PROVIDER_CONFIG")
	if ref == nil {
		t.Fatal("AGENT_PROVIDER_CONFIG has no secretKeyRef")
	}
	if ref.Name != "llm-credentials-opencode" {
		t.Errorf("default secret name = %q, want %q", ref.Name, "llm-credentials-opencode")
	}
	// AGENT_TYPE must be injected into the Job container so the dispatcher
	// entrypoint script routes to the correct agent binary.
	agentTypeVal, ok := getEnv(main, "AGENT_TYPE")
	if !ok {
		t.Fatal("AGENT_TYPE env var missing from main container")
	}
	if agentTypeVal != "opencode" {
		t.Errorf("AGENT_TYPE = %q, want %q", agentTypeVal, "opencode")
	}
}

func TestBuild_FindingSeverity_ValueInjected(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rjob := *testRJob
	rjob.Spec.Severity = "critical"
	job, err := b.Build(&rjob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_SEVERITY")
	if !ok {
		t.Fatal("FINDING_SEVERITY not found in main container env")
	}
	if val != "critical" {
		t.Errorf("FINDING_SEVERITY = %q, want %q", val, "critical")
	}
}

func TestBuild_FindingSeverity_EmptyStringLegacy(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mechanic"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rjob := *testRJob
	rjob.Spec.Severity = ""
	job, err := b.Build(&rjob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "FINDING_SEVERITY")
	if !ok {
		t.Fatal("FINDING_SEVERITY must be present even when Severity is empty (legacy object)")
	}
	if val != "" {
		t.Errorf("FINDING_SEVERITY = %q, want empty string for legacy object", val)
	}
}

func buildDryRunJob(t *testing.T) *batchv1.Job {
	t.Helper()
	b, err := New(Config{AgentNamespace: "mechanic", DryRun: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return job
}

func TestBuild_DryRun_AnnotationPresent(t *testing.T) {
	job := buildDryRunJob(t)
	if got, ok := job.Annotations["mechanic.io/dry-run"]; !ok {
		t.Error("annotation mechanic.io/dry-run missing")
	} else if got != "true" {
		t.Errorf("annotation mechanic.io/dry-run = %q, want %q", got, "true")
	}
}

func TestBuild_DryRun_EnvVarPresent(t *testing.T) {
	job := buildDryRunJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	val, ok := getEnv(main, "DRY_RUN")
	if !ok {
		t.Fatal("DRY_RUN env var missing from main container")
	}
	if val != "true" {
		t.Errorf("DRY_RUN = %q, want %q", val, "true")
	}
}

func TestBuild_NoDryRun_AnnotationAbsent(t *testing.T) {
	job := buildJob(t)
	if _, ok := job.Annotations["mechanic.io/dry-run"]; ok {
		t.Error("annotation mechanic.io/dry-run must not be present when DryRun=false")
	}
}

func TestBuild_NoDryRun_EnvVarAbsent(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	if _, ok := getEnv(main, "DRY_RUN"); ok {
		t.Error("DRY_RUN env var must not be present when DryRun=false")
	}
}

func TestBuild_DryRun_InitContainerNoEnvVar(t *testing.T) {
	job := buildDryRunJob(t)
	var init corev1.Container
	for _, c := range job.Spec.Template.Spec.InitContainers {
		if c.Name == "git-token-clone" {
			init = c
			break
		}
	}
	if _, ok := getEnv(init, "DRY_RUN"); ok {
		t.Error("DRY_RUN must not be injected into the git-token-clone init container")
	}
}

func TestBuild_DryRun_GateInitContainerPresent(t *testing.T) {
	job := buildDryRunJob(t)
	var gate *corev1.Container
	for i := range job.Spec.Template.Spec.InitContainers {
		if job.Spec.Template.Spec.InitContainers[i].Name == "dry-run-gate" {
			gate = &job.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if gate == nil {
		t.Fatal("dry-run-gate init container missing when DryRun=true")
	}
	// Must write the sentinel file
	if len(gate.Args) == 0 || !strings.Contains(gate.Args[0], "/mechanic-cfg/dry-run") {
		t.Errorf("dry-run-gate args do not reference /mechanic-cfg/dry-run: %v", gate.Args)
	}
}

func TestBuild_NoDryRun_GateInitContainerAbsent(t *testing.T) {
	job := buildJob(t)
	for _, c := range job.Spec.Template.Spec.InitContainers {
		if c.Name == "dry-run-gate" {
			t.Error("dry-run-gate init container must not be present when DryRun=false")
		}
	}
}

func TestBuild_DryRun_MechanicCfgVolumePresent(t *testing.T) {
	job := buildDryRunJob(t)
	var found bool
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "mechanic-cfg" {
			found = true
			if v.EmptyDir == nil {
				t.Error("mechanic-cfg volume must be an emptyDir")
			}
			break
		}
	}
	if !found {
		t.Error("mechanic-cfg volume missing when DryRun=true")
	}
}

func TestBuild_NoDryRun_MechanicCfgVolumeAbsent(t *testing.T) {
	job := buildJob(t)
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "mechanic-cfg" {
			t.Error("mechanic-cfg volume must not be present when DryRun=false")
		}
	}
}

func TestBuild_DryRun_MainContainerMountReadOnly(t *testing.T) {
	job := buildDryRunJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	var mount *corev1.VolumeMount
	for i := range main.VolumeMounts {
		if main.VolumeMounts[i].Name == "mechanic-cfg" {
			mount = &main.VolumeMounts[i]
			break
		}
	}
	if mount == nil {
		t.Fatal("mechanic-cfg volume mount missing from main container when DryRun=true")
	}
	if !mount.ReadOnly {
		t.Error("mechanic-cfg volume mount must be ReadOnly=true in main container")
	}
	if mount.MountPath != "/mechanic-cfg" {
		t.Errorf("mechanic-cfg MountPath = %q, want /mechanic-cfg", mount.MountPath)
	}
}

func TestBuild_NoDryRun_MainContainerMountAbsent(t *testing.T) {
	job := buildJob(t)
	main := job.Spec.Template.Spec.Containers[0]
	for _, m := range main.VolumeMounts {
		if m.Name == "mechanic-cfg" {
			t.Error("mechanic-cfg volume mount must not be present when DryRun=false")
		}
	}
}
