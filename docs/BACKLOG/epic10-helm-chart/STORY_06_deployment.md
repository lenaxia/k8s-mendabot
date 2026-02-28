# Story 06: Watcher Deployment Template

**Epic:** [epic10-helm-chart](README.md)
**Priority:** High (core deliverable)
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **cluster operator**, I want the chart to deploy the watcher so the only things
I must configure are `gitops.repo`, `gitops.manifestRoot`, and the two Secret names.

---

## Acceptance Criteria

- [ ] `charts/mechanic/templates/deployment-watcher.yaml` renders a valid `apps/v1 Deployment`
- [ ] Deployment name: `{{ include "mechanic.fullname" . }}`
- [ ] `spec.selector.matchLabels` uses `include "mechanic.selectorLabels"`
- [ ] `spec.template.metadata.labels` uses `include "mechanic.selectorLabels"`
- [ ] `spec.template.spec.serviceAccountName`: `{{ include "mechanic.watcherSAName" . }}`
- [ ] `securityContext` on Pod: `runAsNonRoot: true`, `runAsUser: 1000`,
  `seccompProfile.type: RuntimeDefault` — identical to Kustomize source
- [ ] Container security context: `allowPrivilegeEscalation: false`,
  `readOnlyRootFilesystem: true`, `capabilities.drop: ["ALL"]`
- [ ] Container image: `{{ include "mechanic.watcherImage" . }}`
- [ ] `imagePullPolicy`: `{{ .Values.image.pullPolicy }}`
- [ ] All env vars wired from values:

  | Env var | Source |
  |---------|--------|
  | `GITOPS_REPO` | `{{ required "gitops.repo is required" .Values.gitops.repo }}` |
  | `GITOPS_MANIFEST_ROOT` | `{{ required "gitops.manifestRoot is required" .Values.gitops.manifestRoot }}` |
  | `AGENT_IMAGE` | `{{ include "mechanic.agentImage" . }}` |
  | `AGENT_NAMESPACE` | `{{ .Release.Namespace }}` |
  | `AGENT_SA` | `{{ include "mechanic.agentSAName" . }}` |
  | `SINK_TYPE` | `{{ .Values.watcher.sinkType }}` |
  | `LOG_LEVEL` | `{{ .Values.watcher.logLevel }}` |
  | `MAX_CONCURRENT_JOBS` | `{{ .Values.watcher.maxConcurrentJobs \| quote }}` |
  | `REMEDIATION_JOB_TTL_SECONDS` | `{{ .Values.watcher.remediationJobTTLSeconds \| quote }}` |
  | `STABILISATION_WINDOW_SECONDS` | `{{ .Values.watcher.stabilisationWindowSeconds \| quote }}` |
  | `SELF_REMEDIATION_MAX_DEPTH` | `{{ .Values.selfRemediation.maxDepth \| quote }}` |
  | `MECHANIC_UPSTREAM_REPO` | `{{ .Values.selfRemediation.upstreamRepo }}` |
  | `MECHANIC_DISABLE_UPSTREAM_CONTRIBUTIONS` | `{{ .Values.selfRemediation.disableUpstreamContributions \| quote }}` |

  All three self-remediation env vars are always emitted unconditionally.
  `config.go` reads them on startup with defaults regardless; there is no
  on/off toggle. Self-remediation is automatically triggered by `JobProvider`
  when it detects a failed Job with `app.kubernetes.io/managed-by: mechanic-watcher`.
  To disable it, operators set `selfRemediation.maxDepth: 0`.

- [ ] Ports: `metrics 8080`, `healthz 8081`
- [ ] Liveness probe: `httpGet /healthz :8081`, `initialDelaySeconds 15`, `periodSeconds 20`
- [ ] Readiness probe: `httpGet /readyz :8081`, `initialDelaySeconds 5`, `periodSeconds 10`
- [ ] Resources: requests `cpu: 50m / memory: 64Mi`; limits `cpu: 200m / memory: 128Mi`
- [ ] `helm template` with missing `gitops.repo` causes a render error (required check)

---

## Tasks

- [ ] Read `deploy/kustomize/deployment-watcher.yaml` for exact field values
- [ ] Write `templates/deployment-watcher.yaml`
- [ ] Verify all env vars are present in rendered output
- [ ] Verify `required` function fires when `gitops.repo` is empty
- [ ] Verify image tag fallback to `.Chart.AppVersion` when `image.tag` is empty

---

## Notes

- `AGENT_NAMESPACE` is always `{{ .Release.Namespace }}` — watcher and agent share
  a namespace. This is a deliberate simplification vs. the Kustomize version where
  it is a hardcoded `"default"`.
- Integer values must be quoted in env vars (Kubernetes requires string values).
  Use `| quote` on all numeric values.
- `selfRemediation.enabled` controls `SELF_REMEDIATION_ENABLED` env var presence.
  The other self-remediation env vars are always emitted — the Go config reads them
  regardless and ignores them when enabled=false.

---

## Dependencies

**Depends on:** STORY_02 (helpers + image helpers), STORY_04 (SA name helpers)
**Blocks:** nothing

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] All env vars present in `helm template` output
- [ ] Missing required values produce a clear error message
