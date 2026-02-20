# Domain: Job Builder — Low-Level Design

**Version:** 2.0
**Date:** 2026-02-20
**Status:** Implementation Ready
**HLD Reference:** [Sections 4.3, 5, 12](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

The job builder constructs a fully-specified `batch/v1 Job` object from a `Result` CRD
and a precomputed fingerprint. It is the single source of truth for what the agent Job
looks like — the controller calls it and creates whatever it returns.

### 1.2 Responsibilities

- Produce a deterministic Job name from the fingerprint
- Inject all finding context as environment variables
- Mount the prompt ConfigMap and the GitHub App Secret
- Configure the init container (git clone) and main container (opencode)
- Set all Job lifecycle settings (deadlines, TTL, restart policy, backoff)

### 1.3 Design Principles

- **Pure function** — `Build()` has no side effects; it only constructs a struct
- **Deterministic naming** — same fingerprint always produces the same Job name; this is
  what makes `AlreadyExists` detection in the controller safe
- **No defaults at call sites** — all required fields must be provided via `Config`; fail
  at startup if anything is missing, not at Job creation time

---

## 2. Package Structure

```
internal/
└── jobbuilder/
    ├── job.go        # Builder struct and Build() method
    └── job_test.go
```

---

## 3. Builder Config

```go
type Config struct {
    GitOpsRepo          string // e.g. "lenaxia/talos-ops-prod"
    GitOpsManifestRoot  string // e.g. "kubernetes" — path within the cloned repo
    AgentImage          string // e.g. "ghcr.io/lenaxia/mendabot-agent:latest"
    AgentNamespace      string // namespace where Jobs are created — must equal watcher namespace
    AgentSA             string // ServiceAccount for the agent Job
}

type Builder struct {
    cfg Config
}

func New(cfg Config) *Builder
```

---

## 4. Build() Method

```go
func (b *Builder) Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error)
```

The input is now a `*RemediationJob`. The fingerprint and all finding context are read
from `rjob.Spec` — the builder no longer accepts a separate fingerprint argument.

### Job name

```go
jobName := "mendabot-agent-" + rjob.Spec.Fingerprint[:12]
```

12 characters provides sufficient collision resistance for a single cluster workload
(2^48 space). The full fingerprint is available as an environment variable inside the Job.

### Init container spec

```
name:    "git-token-clone"
image:   b.cfg.AgentImage   (same image as main container — debian-slim, has bash/openssl/curl/jq/git)
command: ["/bin/bash", "-c"]
args:    ["<inline shell script — see §5>"]
env:
  - GITHUB_APP_ID              (from Secret github-app, key: app-id)
  - GITHUB_APP_INSTALLATION_ID (from Secret github-app, key: installation-id)
  - GITHUB_APP_PRIVATE_KEY     (from Secret github-app, key: private-key)
  - GITOPS_REPO                (from Config)
volumeMounts:
  - name: shared-workspace, mountPath: /workspace
  - name: github-app-secret,  mountPath: /secrets/github-app, readOnly: true
```

### Main container spec

```
name:    "mendabot-agent"
image:   b.cfg.AgentImage
command: ["/usr/local/bin/agent-entrypoint.sh"]
env:
  - FINDING_KIND          = rjob.Spec.Finding.Kind
  - FINDING_NAME          = rjob.Spec.Finding.Name
  - FINDING_NAMESPACE     = rjob.Spec.Finding.Namespace
  - FINDING_PARENT        = rjob.Spec.Finding.ParentObject
  - FINDING_ERRORS        = rjob.Spec.Finding.Errors  (already redacted JSON)
  - FINDING_DETAILS       = rjob.Spec.Finding.Details
  - FINDING_FINGERPRINT   = rjob.Spec.Fingerprint (full 64-char hex)
  - GITOPS_REPO           = rjob.Spec.GitOpsRepo
  - GITOPS_MANIFEST_ROOT  = rjob.Spec.GitOpsManifestRoot
  - OPENAI_API_KEY             (from Secret llm-credentials, key: api-key)
  - OPENAI_BASE_URL            (from Secret llm-credentials, key: base-url, optional)
  - OPENAI_MODEL               (from Secret llm-credentials, key: model, optional)
volumeMounts:
  - name: shared-workspace, mountPath: /workspace
  - name: prompt-configmap,  mountPath: /prompt, readOnly: true
  - name: github-app-secret, mountPath: /secrets/github-app, readOnly: true
```

**Secret key mapping:** The Secret keys (`api-key`, `base-url`, `model`) differ from the
environment variable names. The `secretKeyRef.key` in the Job spec must reference the
Secret's key names exactly — not the env var name.

**GitHub App credentials in main container:** `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`,
and `GITHUB_APP_PRIVATE_KEY` are intentionally NOT injected into the main container. The
main container only needs the installation token, which is read from `/workspace/github-token`
(written by the init container). Exposing the long-lived private key to the LLM agent is a
security risk — a compromised or manipulated agent could use it to mint arbitrary tokens.

### Job spec

```go
batchv1.Job{
    ObjectMeta: metav1.ObjectMeta{
        Name:      jobName,
        Namespace: b.cfg.AgentNamespace,
        Labels: map[string]string{
            "app.kubernetes.io/managed-by":            "mendabot-watcher",
            "remediation.mendabot.io/fingerprint":       rjob.Spec.Fingerprint[:12],
            "remediation.mendabot.io/remediation-job":   rjob.Name,
            "remediation.mendabot.io/finding-kind":      rjob.Spec.Finding.Kind,
        },
        Annotations: map[string]string{
            "remediation.mendabot.io/fingerprint-full":  rjob.Spec.Fingerprint,
            "remediation.mendabot.io/finding-parent":    rjob.Spec.Finding.ParentObject,
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
        TTLSecondsAfterFinished: ptr(int32(86400)),
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                ServiceAccountName: b.cfg.AgentSA,
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
```

### Volumes

```
- name: shared-workspace
  emptyDir: {}

- name: prompt-configmap
  configMap:
    name: opencode-prompt

- name: github-app-secret
  secret:
    secretName: github-app
```

**Container-level securityContext** (applied to both init and main containers):

```go
SecurityContext: &corev1.SecurityContext{
    AllowPrivilegeEscalation: ptr(false),
    Capabilities: &corev1.Capabilities{
        Drop: []corev1.Capability{"ALL"},
    },
}
```

Note: `ReadOnlyRootFilesystem` is intentionally NOT set on the main container — the
entrypoint writes to `/tmp/rendered-prompt.txt`. If root filesystem read-only is required,
add an `emptyDir` volume mounted at `/tmp`.

---

## 5. Init Container Shell Script

The init container uses the same `AgentImage` (debian-slim). It runs an inline bash script
that:

1. Reads GitHub App credentials from environment variables (injected from the mounted Secret)
2. Calls `get-github-app-token.sh` (baked into the agent image) to obtain an installation token
3. Writes the token to `/workspace/github-token` for the main container to consume
4. Clones the GitOps repo into `/workspace/repo`

```bash
#!/bin/bash
set -euo pipefail

TOKEN=$(get-github-app-token.sh)
printf '%s' "$TOKEN" > /workspace/github-token

git clone "https://x-access-token:${TOKEN}@github.com/${GITOPS_REPO}.git" /workspace/repo
```

**Why the same image for init and main containers:**
- The agent image (debian-slim) has `bash`, `openssl`, `curl`, `jq`, and `git` — all tools
  needed by both the token exchange script and the git clone
- Using the same image avoids maintaining a separate init image, removes the alpine constraint,
  and ensures the `get-github-app-token.sh` script is always available
- The script is inline (injected as `args` to the container command) — no ConfigMap needed

**Token path:** `/workspace/github-token` — written by init container, read by main
container. Both containers mount `shared-workspace` at `/workspace`.

---

## 6. FINDING_ERRORS — Already Redacted

In the CRD-based design, `FINDING_ERRORS` is pre-computed during `RemediationJob`
creation (in the `ResultReconciler`) and stored in `rjob.Spec.Finding.Errors` as a
redacted JSON string. The job builder reads this field directly — it does not perform
Sensitive field redaction. Redaction is the ResultReconciler's responsibility.

This simplifies the builder and makes the stored `RemediationJob` spec itself auditable:
what the agent receives is exactly what is stored in the CRD.

---

## 7. Testing Strategy

All tests remain pure unit tests — no cluster, no envtest. The `Build()` method is a pure
function that takes a `*RemediationJob` and returns a `*batchv1.Job`.

| Test | Description |
|---|---|
| `TestBuild_JobName` | Name is `mendabot-agent-<first-12-of-fp>` |
| `TestBuild_JobNameDeterministic` | Same input twice → same Job name |
| `TestBuild_Namespace` | Job is in configured namespace |
| `TestBuild_ServiceAccount` | Job uses configured ServiceAccount |
| `TestBuild_EnvVars_AllPresent` | All FINDING_*, GITOPS_REPO, GITOPS_MANIFEST_ROOT env vars present |
| `TestBuild_EnvVars_FindingNameNoNamespacePrefix` | FINDING_NAME is plain name |
| `TestBuild_EnvVars_FindingNamespace` | FINDING_NAMESPACE equals rjob.Spec.Finding.Namespace |
| `TestBuild_EnvVars_ErrorsJSON` | FINDING_ERRORS equals rjob.Spec.Finding.Errors verbatim |
| `TestBuild_InitContainer_Present` | Init container named "git-token-clone" exists |
| `TestBuild_InitContainer_UsesAgentImage` | Init container uses same image as main container |
| `TestBuild_MainContainer_Present` | Main container named "mendabot-agent" exists |
| `TestBuild_MainContainer_Command` | Main container command is ["/usr/local/bin/agent-entrypoint.sh"] |
| `TestBuild_SecretKeyRefs` | secretKeyRef keys match Secret key names |
| `TestBuild_Volumes_AllPresent` | shared-workspace, prompt-configmap, github-app-secret |
| `TestBuild_JobSettings` | BackoffLimit=1, ActiveDeadlineSeconds=900, TTL=86400 |
| `TestBuild_RestartPolicy` | RestartPolicy is Never |
| `TestBuild_Labels` | managed-by, fingerprint, remediation-job labels present |
| `TestBuild_OwnerReference` | OwnerReference points at RemediationJob with correct UID |
| `TestBuild_EmptyErrors` | Empty Errors string → FINDING_ERRORS is empty string |
| `TestBuild_LongDetails` | Very long Details string does not truncate or error |
