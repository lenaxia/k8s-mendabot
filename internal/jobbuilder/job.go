package jobbuilder

import (
	"encoding/json"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/config"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

var _ domain.JobBuilder = (*Builder)(nil)

type Config struct {
	AgentNamespace string
	AgentType      config.AgentType
	// TTLSeconds controls TTLSecondsAfterFinished on the batch/v1 Job.
	// Zero means use the default (86400 = 24h).
	TTLSeconds          int32
	DryRun              bool
	HardenAgentKubectl  bool
	ExtraRedactPatterns []string
	// Resource limits applied to all three Job containers.
	// Empty strings fall back to the package defaults (100m/128Mi/500m/512Mi).
	CPURequest string
	MemRequest string
	CPULimit   string
	MemLimit   string
}

const defaultTTLSeconds int32 = 86400

type Builder struct {
	cfg Config
}

func New(cfg Config) (*Builder, error) {
	if cfg.AgentNamespace == "" {
		return nil, fmt.Errorf("jobbuilder: AgentNamespace must not be empty")
	}
	if cfg.AgentType == "" {
		cfg.AgentType = config.AgentTypeOpenCode
	}
	if cfg.TTLSeconds == 0 {
		cfg.TTLSeconds = defaultTTLSeconds
	}
	// Apply resource limit defaults.
	if cfg.CPURequest == "" {
		cfg.CPURequest = "100m"
	}
	if cfg.MemRequest == "" {
		cfg.MemRequest = "128Mi"
	}
	if cfg.CPULimit == "" {
		cfg.CPULimit = "500m"
	}
	if cfg.MemLimit == "" {
		cfg.MemLimit = "512Mi"
	}
	return &Builder{cfg: cfg}, nil
}

// containerResources returns a ResourceRequirements using the configured limits.
// A panic here means the calling code supplied an invalid quantity string; this
// is a programmer error that should be caught in tests before production.
func (b *Builder) containerResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(b.cfg.CPURequest),
			corev1.ResourceMemory: resource.MustParse(b.cfg.MemRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(b.cfg.CPULimit),
			corev1.ResourceMemory: resource.MustParse(b.cfg.MemLimit),
		},
	}
}

func ptr[T any](v T) *T { return &v }

// buildGateCommand returns the sh command for the dry-run-gate init container.
// Only writes the sentinels that are actually needed.
func buildGateCommand(dryRun, hardenKubectl bool) string {
	var cmds []string
	if dryRun {
		cmds = append(cmds, "echo -n 'true' > /mechanic-cfg/dry-run && chmod 444 /mechanic-cfg/dry-run")
	}
	if hardenKubectl {
		cmds = append(cmds, "echo -n 'true' > /mechanic-cfg/harden-kubectl && chmod 444 /mechanic-cfg/harden-kubectl")
	}
	return strings.Join(cmds, " && ")
}

const initScript = `#!/bin/bash
set -euo pipefail

TOKEN=$(get-github-app-token.sh)
printf '%s' "$TOKEN" > /workspace/github-token

echo "Cloning repository: ${GITOPS_REPO}"
if ! git clone "https://x-access-token:${TOKEN}@github.com/${GITOPS_REPO}.git" /workspace/repo; then
  echo "ERROR: Failed to clone ${GITOPS_REPO}"
  echo "The GitHub App token does not have access to this repository"
  exit 1
fi`

func (b *Builder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error) {
	if rjob == nil {
		return nil, fmt.Errorf("jobbuilder: RemediationJob must not be nil")
	}
	if len(rjob.Spec.Fingerprint) < 12 {
		return nil, fmt.Errorf("jobbuilder: Fingerprint must be at least 12 characters, got %d", len(rjob.Spec.Fingerprint))
	}

	jobName := "mechanic-agent-" + rjob.Spec.Fingerprint[:12]
	secretName := "llm-credentials-" + string(b.cfg.AgentType)
	coreCMName := "agent-prompt-core"
	agentCMName := "agent-prompt-" + string(b.cfg.AgentType)

	initContainer := corev1.Container{
		Name:    "git-token-clone",
		Image:   rjob.Spec.AgentImage,
		Command: []string{"/bin/bash", "-c"},
		Args:    []string{initScript},
		Env: []corev1.EnvVar{
			{
				Name: "GITHUB_APP_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "github-app"},
						Key:                  "app-id",
					},
				},
			},
			{
				Name: "GITHUB_APP_INSTALLATION_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "github-app"},
						Key:                  "installation-id",
					},
				},
			},
			{
				Name: "GITHUB_APP_PRIVATE_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "github-app"},
						Key:                  "private-key",
					},
				},
			},
			{
				Name:  "GITOPS_REPO",
				Value: rjob.Spec.GitOpsRepo,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "shared-workspace",
				MountPath: "/workspace",
			},
		},
		Resources: b.containerResources(),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}

	mainContainer := corev1.Container{
		Name:  "mechanic-agent",
		Image: rjob.Spec.AgentImage,
		Env: []corev1.EnvVar{
			{Name: "FINDING_KIND", Value: rjob.Spec.Finding.Kind},
			{Name: "FINDING_NAME", Value: rjob.Spec.Finding.Name},
			{Name: "FINDING_NAMESPACE", Value: rjob.Spec.Finding.Namespace},
			{Name: "FINDING_PARENT", Value: rjob.Spec.Finding.ParentObject},
			{Name: "FINDING_ERRORS", Value: rjob.Spec.Finding.Errors},
			{Name: "FINDING_DETAILS", Value: rjob.Spec.Finding.Details},
			{Name: "FINDING_FINGERPRINT", Value: rjob.Spec.Fingerprint[:12]},
			{Name: "FINDING_SEVERITY", Value: rjob.Spec.Severity},
			{Name: "GITOPS_REPO", Value: rjob.Spec.GitOpsRepo},
			{Name: "GITOPS_MANIFEST_ROOT", Value: rjob.Spec.GitOpsManifestRoot},
			{Name: "SINK_TYPE", Value: rjob.Spec.SinkType},
			{
				Name: "AGENT_PROVIDER_CONFIG",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
						Key:                  "provider-config",
					},
				},
			},
			{Name: "AGENT_TYPE", Value: string(b.cfg.AgentType)},
			// AGENT_NAMESPACE tells emit_dry_run_report() which namespace to write
			// the dry-run ConfigMap into. It equals the Job/Pod namespace.
			{Name: "AGENT_NAMESPACE", Value: b.cfg.AgentNamespace},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "shared-workspace",
				MountPath: "/workspace",
			},
			{
				Name:      "prompt-configmap",
				MountPath: "/prompt",
				ReadOnly:  true,
			},
			{
				// Legacy SA token (no audience claim) — accepted by all API servers
				// including Talos where projected tokens fail audience validation.
				// Mounted at a non-standard path so entrypoint-common.sh can detect
				// its presence and prefer it over the auto-mounted projected token.
				Name:      "agent-token",
				MountPath: "/var/run/secrets/mechanic/serviceaccount",
				ReadOnly:  true,
			},
			{
				Name:      "tmp",
				MountPath: "/tmp",
			},
			{
				// /home/agent must be writable so that entrypoint-common.sh can run
				// "mkdir -p /home/agent/.kube" when ReadOnlyRootFilesystem=true.
				Name:      "agent-home",
				MountPath: "/home/agent",
			},
		},
		Resources: b.containerResources(),
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem:   ptr(true),
			RunAsNonRoot:             ptr(true),
			AllowPrivilegeEscalation: ptr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}

	if len(correlatedFindings) > 0 {
		raw, err := json.Marshal(correlatedFindings)
		if err != nil {
			return nil, fmt.Errorf("jobbuilder: marshal correlated findings: %w", err)
		}
		mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{Name: "FINDING_CORRELATED_FINDINGS", Value: string(raw)})
	}
	if b.cfg.DryRun {
		mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{
			Name:  "DRY_RUN",
			Value: "true",
		})
	}
	if b.cfg.DryRun || b.cfg.HardenAgentKubectl {
		// Mount the mechanic-cfg sentinel volume read-only so the wrappers can detect
		// mode via a tamper-proof file rather than an env var that a child shell could unset.
		mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, corev1.VolumeMount{
			Name:      "mechanic-cfg",
			MountPath: "/mechanic-cfg",
			ReadOnly:  true,
		})
	}
	if b.cfg.HardenAgentKubectl {
		mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{Name: "HARDEN_KUBECTL", Value: "true"})
	}
	if len(b.cfg.ExtraRedactPatterns) > 0 {
		mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{
			Name:  "EXTRA_REDACT_PATTERNS",
			Value: strings.Join(b.cfg.ExtraRedactPatterns, ","),
		})
	}
	if groupID, ok := rjob.Labels[domain.CorrelationGroupIDLabel]; ok && groupID != "" {
		mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{Name: "FINDING_CORRELATION_GROUP_ID", Value: groupID})
	}

	volumes := []corev1.Volume{
		{
			Name: "shared-workspace",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			// Projected volume merges core prompt (core.txt) and agent-specific prompt
			// (agent.txt) into a single /prompt directory. entrypoint-common.sh reads
			// both files and concatenates them at runtime.
			Name: "prompt-configmap",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ConfigMap: &corev1.ConfigMapProjection{
								LocalObjectReference: corev1.LocalObjectReference{Name: coreCMName},
								Items: []corev1.KeyToPath{
									{Key: "core.txt", Path: "core.txt"},
								},
							},
						},
						{
							ConfigMap: &corev1.ConfigMapProjection{
								LocalObjectReference: corev1.LocalObjectReference{Name: agentCMName},
								Items: []corev1.KeyToPath{
									{Key: "agent.txt", Path: "agent.txt"},
								},
							},
						},
					},
				},
			},
		},
		{
			// Legacy SA token secret — no audience claim, works on all distributions.
			// See entrypoint-common.sh for how this is used.
			Name: "agent-token",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "mechanic-agent-token",
				},
			},
		},
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			// Writable home directory for the agent user. Required because the main
			// container runs with ReadOnlyRootFilesystem=true and entrypoint-common.sh
			// creates /home/agent/.kube to build the in-cluster kubeconfig.
			Name: "agent-home",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	initContainers := []corev1.Container{initContainer}

	if b.cfg.DryRun || b.cfg.HardenAgentKubectl {
		// mechanic-cfg: writes sentinel files before the main container starts.
		// The main container mounts the same emptyDir read-only, so sentinel files
		// cannot be deleted or modified by any child process inside the agent.
		volumes = append(volumes, corev1.Volume{
			Name: "mechanic-cfg",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		gateContainer := corev1.Container{
			Name:    "dry-run-gate",
			Image:   rjob.Spec.AgentImage,
			Command: []string{"/bin/sh", "-c"},
			Args:    []string{buildGateCommand(b.cfg.DryRun, b.cfg.HardenAgentKubectl)},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "mechanic-cfg", MountPath: "/mechanic-cfg"},
			},
			Resources: b.containerResources(),
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
			},
		}
		initContainers = append(initContainers, gateContainer)
	}

	annotations := map[string]string{
		"remediation.mechanic.io/fingerprint-full": rjob.Spec.Fingerprint,
		"remediation.mechanic.io/finding-parent":   rjob.Spec.Finding.ParentObject,
	}
	if b.cfg.DryRun {
		annotations["mechanic.io/dry-run"] = "true"
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: b.cfg.AgentNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":            "mechanic-watcher",
				"remediation.mechanic.io/fingerprint":     rjob.Spec.Fingerprint[:12],
				"remediation.mechanic.io/remediation-job": rjob.Name,
				"remediation.mechanic.io/finding-kind":    rjob.Spec.Finding.Kind,
			},
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mechanic.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               rjob.Name,
					UID:                rjob.UID,
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr(int32(1)),
			ActiveDeadlineSeconds:   ptr(int64(900)),
			TTLSecondsAfterFinished: ptr(b.cfg.TTLSeconds),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: rjob.Spec.AgentSA,
					RestartPolicy:      corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr(true),
						RunAsUser:    ptr(int64(1000)),
					},
					InitContainers: initContainers,
					Containers:     []corev1.Container{mainContainer},
					Volumes:        volumes,
				},
			},
		},
	}

	return job, nil
}
