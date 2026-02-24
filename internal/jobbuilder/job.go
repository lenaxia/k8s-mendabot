package jobbuilder

import (
	"encoding/json"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

var _ domain.JobBuilder = (*Builder)(nil)

type Config struct {
	AgentNamespace string
	AgentType      config.AgentType
	// TTLSeconds controls TTLSecondsAfterFinished on the batch/v1 Job.
	// Zero means use the default (86400 = 24h).
	TTLSeconds int32
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
	return &Builder{cfg: cfg}, nil
}

func ptr[T any](v T) *T { return &v }

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

	jobName := "mendabot-agent-" + rjob.Spec.Fingerprint[:12]
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
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}

	mainContainer := corev1.Container{
		Name:  "mendabot-agent",
		Image: rjob.Spec.AgentImage,
		Env: []corev1.EnvVar{
			{Name: "FINDING_KIND", Value: rjob.Spec.Finding.Kind},
			{Name: "FINDING_NAME", Value: rjob.Spec.Finding.Name},
			{Name: "FINDING_NAMESPACE", Value: rjob.Spec.Finding.Namespace},
			{Name: "FINDING_PARENT", Value: rjob.Spec.Finding.ParentObject},
			{Name: "FINDING_ERRORS", Value: rjob.Spec.Finding.Errors},
			{Name: "FINDING_DETAILS", Value: rjob.Spec.Finding.Details},
			{Name: "FINDING_FINGERPRINT", Value: rjob.Spec.Fingerprint},
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
				MountPath: "/var/run/secrets/mendabot/serviceaccount",
				ReadOnly:  true,
			},
		},
		SecurityContext: &corev1.SecurityContext{
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
					SecretName: "mendabot-agent-token",
				},
			},
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: b.cfg.AgentNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":            "mendabot-watcher",
				"remediation.mendabot.io/fingerprint":     rjob.Spec.Fingerprint[:12],
				"remediation.mendabot.io/remediation-job": rjob.Name,
				"remediation.mendabot.io/finding-kind":    rjob.Spec.Finding.Kind,
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": rjob.Spec.Fingerprint,
				"remediation.mendabot.io/finding-parent":   rjob.Spec.Finding.ParentObject,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
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
					InitContainers: []corev1.Container{initContainer},
					Containers:     []corev1.Container{mainContainer},
					Volumes:        volumes,
				},
			},
		},
	}

	return job, nil
}
