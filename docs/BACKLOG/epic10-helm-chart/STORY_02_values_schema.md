# Story 02: Values Schema and _helpers.tpl

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Critical (referenced by all template stories)
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **chart user**, I want a fully documented `values.yaml` with sensible defaults so
I can understand every configuration option without reading Go source code.

---

## Acceptance Criteria

- [ ] `charts/mechanic/values.yaml` contains all keys listed in the values schema
  below, with inline comments explaining each field
- [ ] `charts/mechanic/templates/_helpers.tpl` defines:
  - `mechanic.fullname` — `Release.Name` if it already contains `"mechanic"`,
    otherwise `Release.Name-mechanic`; truncated to 63 characters
  - `mechanic.name` — chart name (`mechanic`)
  - `mechanic.labels` — standard labels: `helm.sh/chart`, `app.kubernetes.io/name`,
    `app.kubernetes.io/instance`, `app.kubernetes.io/version`,
    `app.kubernetes.io/managed-by`
  - `mechanic.selectorLabels` — `app.kubernetes.io/name` + `app.kubernetes.io/instance`
  - `mechanic.watcherImage` — resolves watcher image tag: `image.tag` if set, else
    `.Chart.AppVersion`; produces `image.repository:tag`
  - `mechanic.agentImage` — same for `agent.image`
  - `mechanic.watcherSAName` — `mechanic.fullname`-watcher
  - `mechanic.agentSAName` — `mechanic.fullname`-agent
- [ ] `helm lint charts/mechanic/` passes after this story

---

## Values Schema

```yaml
# mechanic Helm chart values
# All fields are optional except gitops.repo and gitops.manifestRoot.

# Watcher image configuration
image:
  repository: ghcr.io/lenaxia/mechanic-watcher
  # tag defaults to Chart.appVersion when empty
  tag: ""
  pullPolicy: IfNotPresent

# Agent image configuration
# Tag defaults to Chart.appVersion when empty.
# Watcher and agent images are always released together — do not pin them to
# different versions.
agent:
  image:
    repository: ghcr.io/lenaxia/mechanic-agent
    tag: ""

# GitOps repository configuration (both fields are required)
gitops:
  # GitHub repository in org/repo format (e.g. "myorg/my-gitops-repo")
  repo: ""
  # Path within the repository containing Kubernetes manifests
  manifestRoot: ""

# Watcher operational tuning
watcher:
  # Seconds to wait after first detecting a failure before creating a RemediationJob.
  # Set to 0 to disable the window and create jobs immediately.
  stabilisationWindowSeconds: 120
  # Maximum number of agent Jobs running concurrently.
  maxConcurrentJobs: 3
  # Time-to-live for completed RemediationJob objects (seconds). Default: 7 days.
  remediationJobTTLSeconds: 604800
  # Sink type for PR creation. Currently only "github" is supported.
  sinkType: github
  # Zap log level: debug, info, warn, error
  logLevel: info

# Self-remediation configuration.
# These values set Go config fields that control self-remediation behaviour.
# All three env vars have defaults in config.go and are always emitted —
# there is no on/off switch. To disable self-remediation set maxDepth: 0.
# Note: self-remediation is triggered automatically by the JobProvider when
# it detects a failed batch/v1 Job labelled app.kubernetes.io/managed-by=mechanic-watcher.
selfRemediation:
  # Maximum self-remediation chain depth. Set to 0 to disable entirely.
  maxDepth: 2
  # Upstream repository where mechanic bug-fix PRs are opened.
  upstreamRepo: lenaxia/k8s-mechanic
  # Set to true to prevent any PR being opened against the upstream mechanic
  # repo (PRs go to gitops.repo only).
  disableUpstreamContributions: false

# References to pre-existing Secrets. The chart never creates Secret content.
# Create these Secrets before running helm install.
secrets:
  githubApp:
    # Name of the Secret containing GitHub App credentials.
    # Required keys: app-id, installation-id, private-key
    # Default matches the name hardcoded in deploy/kustomize/secret-github-app-placeholder.yaml
    # and referenced by job.go init container.
    name: github-app
  llm:
    # Name of the Secret containing LLM API credentials.
    # Required keys: api-key, base-url, model, kube-api-server
    # Default matches the name hardcoded in deploy/kustomize/secret-llm-placeholder.yaml
    # and referenced by job.go main container.
    name: llm-credentials

# Prompt configuration
prompt:
  # Selects a built-in prompt file from charts/mechanic/files/prompts/<name>.txt
  # Available: "default"
  name: default
  # Full prompt content override. When non-empty, takes precedence over prompt.name.
  override: ""

# RBAC resource creation. Set to false if you manage RBAC externally.
rbac:
  create: true

# Create the Release.Namespace if it does not exist.
# Default false — most operators pre-create namespaces.
createNamespace: false

# Prometheus metrics configuration
metrics:
  # Expose a metrics Service on port 8080. Required for ServiceMonitor.
  enabled: false
  serviceMonitor:
    # Create a ServiceMonitor CR for Prometheus Operator.
    # Requires metrics.enabled: true and Prometheus Operator installed.
    enabled: false
    interval: 30s
    scrapeTimeout: 10s
    # Additional labels to add to the ServiceMonitor (e.g. for Prometheus selector)
    labels: {}
```

---

## Tasks

- [ ] Write `values.yaml` with all fields and inline comments
- [ ] Write `templates/_helpers.tpl` with all named templates
- [ ] Verify `{{ include "mechanic.watcherImage" . }}` and `agentImage` produce
  correct `repository:tag` strings for both set and unset tag scenarios
- [ ] Run `helm lint charts/mechanic/` after adding stub templates

---

## Notes

- `mechanic.fullname` truncation is important: Kubernetes resource names are capped
  at 63 characters. Use `trunc 63 | trimSuffix "-"` as in the standard chart template.
- The `mechanic.watcherImage` helper must not emit `repository:` (empty tag) when
  `image.tag` is empty — it must fall back to `.Chart.AppVersion`.

---

## Dependencies

**Depends on:** STORY_01
**Blocks:** STORY_03, STORY_04, STORY_05, STORY_06, STORY_07, STORY_08, STORY_09, STORY_10

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] `helm template charts/mechanic/ --set gitops.repo=org/repo --set gitops.manifestRoot=k8s`
  renders without error
