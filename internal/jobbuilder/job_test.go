package jobbuilder

import (
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

var testRJob = &v1alpha1.RemediationJob{
	ObjectMeta: metav1.ObjectMeta{
		Name: "mendabot-abc123def456",
		UID:  types.UID("test-uid-1234"),
	},
	Spec: v1alpha1.RemediationJobSpec{
		AgentImage:         "ghcr.io/lenaxia/mendabot-agent:latest",
		AgentSA:            "mendabot-agent",
		GitOpsRepo:         "lenaxia/talos-ops-prod",
		GitOpsManifestRoot: "kubernetes/",
		SinkType:           "github",
		Fingerprint:        "abcdef012345abcdef012345abcdef012345abcdef012345abcdef012345abcd",
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
	b, err := New(Config{AgentNamespace: "mendabot"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job, err := b.Build(testRJob)
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
	b, err := New(Config{AgentNamespace: "mendabot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil Builder")
	}
}

var _ func(*v1alpha1.RemediationJob) (*batchv1.Job, error) = (*Builder)(nil).Build

func TestBuild_JobName(t *testing.T) {
	job := buildJob(t)
	want := "mendabot-agent-abcdef012345"
	if job.Name != want {
		t.Errorf("job name = %q, want %q", job.Name, want)
	}
}

func TestBuild_JobNameDeterministic(t *testing.T) {
	b, err := New(Config{AgentNamespace: "mendabot"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job1, err := b.Build(testRJob)
	if err != nil {
		t.Fatalf("Build first: %v", err)
	}
	job2, err := b.Build(testRJob)
	if err != nil {
		t.Fatalf("Build second: %v", err)
	}
	if job1.Name != job2.Name {
		t.Errorf("non-deterministic: got %q then %q", job1.Name, job2.Name)
	}
}

func TestBuild_Namespace(t *testing.T) {
	job := buildJob(t)
	if job.Namespace != "mendabot" {
		t.Errorf("namespace = %q, want %q", job.Namespace, "mendabot")
	}
}

func TestBuild_ServiceAccount(t *testing.T) {
	job := buildJob(t)
	got := job.Spec.Template.Spec.ServiceAccountName
	if got != "mendabot-agent" {
		t.Errorf("ServiceAccountName = %q, want %q", got, "mendabot-agent")
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
		"GITOPS_REPO",
		"GITOPS_MANIFEST_ROOT",
		"SINK_TYPE",
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
		"OPENAI_MODEL",
		"KUBE_API_SERVER",
	}
	for _, name := range required {
		if _, ok := getEnv(main, name); !ok {
			t.Errorf("env var %q missing from main container", name)
		}
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
		if c.Name == "mendabot-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("main container named \"mendabot-agent\" not found")
	}
}

func TestBuild_MainContainer_NoCommandOverride(t *testing.T) {
	job := buildJob(t)
	var main corev1.Container
	for _, c := range job.Spec.Template.Spec.Containers {
		if c.Name == "mendabot-agent" {
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
		{"OPENAI_API_KEY", "llm-credentials", "api-key"},
		{"OPENAI_BASE_URL", "llm-credentials", "base-url"},
		{"OPENAI_MODEL", "llm-credentials", "model"},
		{"KUBE_API_SERVER", "llm-credentials", "kube-api-server"},
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

	podVolumeTests := []string{"shared-workspace", "prompt-configmap", "github-app-secret"}
	for _, name := range podVolumeTests {
		if _, ok := findVolume(podSpec, name); !ok {
			t.Errorf("pod volume %q not found", name)
		}
	}

	var main corev1.Container
	for _, c := range podSpec.Containers {
		if c.Name == "mendabot-agent" {
			main = c
			break
		}
	}
	for _, name := range []string{"shared-workspace", "prompt-configmap"} {
		if _, ok := findVolumeMount(main, name); !ok {
			t.Errorf("main container volume mount %q not found", name)
		}
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
	for _, name := range []string{"shared-workspace", "github-app-secret"} {
		if _, ok := findVolumeMount(initContainer, name); !ok {
			t.Errorf("init container volume mount %q not found", name)
		}
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
		{"app.kubernetes.io/managed-by", "mendabot-watcher"},
		{"remediation.mendabot.io/fingerprint", "abcdef012345"},
		{"remediation.mendabot.io/remediation-job", "mendabot-abc123def456"},
		{"remediation.mendabot.io/finding-kind", "Deployment"},
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
		{"remediation.mendabot.io/fingerprint-full", "abcdef012345abcdef012345abcdef012345abcdef012345abcdef012345abcd"},
		{"remediation.mendabot.io/finding-parent", "my-app"},
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
	if ref.APIVersion != "remediation.mendabot.io/v1alpha1" {
		t.Errorf("APIVersion = %q, want %q", ref.APIVersion, "remediation.mendabot.io/v1alpha1")
	}
	if ref.Kind != "RemediationJob" {
		t.Errorf("Kind = %q, want %q", ref.Kind, "RemediationJob")
	}
	if ref.Name != "mendabot-abc123def456" {
		t.Errorf("Name = %q, want %q", ref.Name, "mendabot-abc123def456")
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
	b, err := New(Config{AgentNamespace: "mendabot"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rjob := *testRJob
	rjob.Spec.Finding.Errors = ""
	job, err := b.Build(&rjob)
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
	b, err := New(Config{AgentNamespace: "mendabot"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	longDetails := strings.Repeat("x", 100_000)
	rjob := *testRJob
	rjob.Spec.Finding.Details = longDetails
	job, err := b.Build(&rjob)
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
	b, _ := New(Config{AgentNamespace: "mendabot"})
	_, err := b.Build(nil)
	if err == nil {
		t.Fatal("expected error for nil rjob, got nil")
	}
}

func TestBuild_EmptyFingerprint(t *testing.T) {
	b, _ := New(Config{AgentNamespace: "mendabot"})
	_, err := b.Build(&v1alpha1.RemediationJob{})
	if err == nil {
		t.Fatal("expected error for empty fingerprint, got nil")
	}
}

func TestBuild_ShortFingerprint(t *testing.T) {
	b, _ := New(Config{AgentNamespace: "mendabot"})
	_, err := b.Build(&v1alpha1.RemediationJob{
		Spec: v1alpha1.RemediationJobSpec{Fingerprint: "abc"},
	})
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
