# Epic 10: Helm Chart

## Purpose

Package mechanic as a Helm chart so external users can deploy it with a single
`helm install` command, without managing raw Kustomize manifests. The chart is
fully custom templates (no library dependencies) and covers every resource
currently in `deploy/kustomize/` plus a CRD upgrade hook and optional metrics.

## Status: In Progress (all stories implemented; awaiting PR merge)

## Dependencies

- epic00 — Foundation complete
- epic01 — Controller complete
- epic02 — Job Builder complete
- epic04 — Deploy manifests complete (source of truth for RBAC shape)
- epic05 — Prompt complete (source of truth for prompt content)

## Blocks

- Nothing; this is an independent packaging epic

---

## Problem Statement

The only deployment mechanism today is `deploy/kustomize/`, which requires:
- Manual `kubectl apply -k` with no parameterisation
- Editing raw YAML files to change image tags, namespaces, or configuration
- No upgrade path for CRDs (Kustomize has no hook concept)
- No way for external operators to install mechanic without forking the repo

A Helm chart solves all of these. It also unlocks:
- Helm repository hosting via GitHub Pages + Chart Releaser Action
- `helm upgrade --reuse-values` for rolling updates
- Schema-validated values via `values.schema.json` (future)
- External operator ecosystem familiarity

---

## Architecture

### Design principles

- **Fully custom templates**: no library dependencies. The chart is straightforward
  (one Deployment, two ServiceAccounts, RBAC, one ConfigMap, one CRD). A library
  dependency would add more friction than it removes.
- **Secrets never created by the chart**: GitHub App private key and LLM API key
  must never appear in Helm release history. The chart references pre-existing
  Secrets by name.
- **CRD in `crds/` + pre-upgrade hook**: Helm installs CRDs from `crds/` on fresh
  install but deliberately skips them on `helm upgrade`. A `pre-upgrade` hook Job
  that runs `kubectl apply` compensates for this gap.
- **Prompt content in `files/prompts/`**: sourced via `.Files.Get` so operators can
  supply a full `prompt.override` without forking the chart.
- **Image tags lockstepped to `Chart.appVersion`**: watcher and agent images are
  always built and released together; diverging versions cause undefined behaviour.

### Values schema (top-level keys)

```
image                              watcher image (repository, tag, pullPolicy)
agent.image                        agent image (repository, tag)
gitops.repo                        required; GITOPS_REPO
gitops.manifestRoot                required; GITOPS_MANIFEST_ROOT
watcher.stabilisationWindowSeconds default 120
watcher.maxConcurrentJobs          default 3
watcher.remediationJobTTLSeconds   default 604800 (7 days)
watcher.sinkType                   default "github"
watcher.logLevel                   default "info"
selfRemediation.maxDepth           SELF_REMEDIATION_MAX_DEPTH; default 2; set 0 to disable
selfRemediation.upstreamRepo       MECHANIC_UPSTREAM_REPO; default "lenaxia/k8s-mechanic"
selfRemediation.disableUpstreamContributions  MECHANIC_DISABLE_UPSTREAM_CONTRIBUTIONS; default false
secrets.githubApp.name             existing Secret name; chart never creates it; default "github-app"
secrets.llm.name                   existing Secret name; chart never creates it; default "llm-credentials"
prompt.name                        selects files/prompts/<name>.txt; default "default"
prompt.override                    full content override; takes precedence over name
rbac.create                        gate for all RBAC creation; default true
createNamespace                    create Release.Namespace; default false
metrics.enabled                    expose metrics Service on port 8080; default false
metrics.serviceMonitor.enabled     create ServiceMonitor CRD; default false
metrics.serviceMonitor.interval    default "30s"
metrics.serviceMonitor.scrapeTimeout  default "10s"
metrics.serviceMonitor.labels      additional labels for ServiceMonitor; default {}
```

### Secret structure

The two pre-existing Secrets must have these exact key names (hardcoded in
`internal/jobbuilder/job.go` — changing them requires a code change):

| Secret (`secrets.githubApp.name`, default: `github-app`) | Key | Used by |
|---|---|---|
| GitHub App ID | `app-id` | init container (`GITHUB_APP_ID`) |
| GitHub App installation ID | `installation-id` | init container (`GITHUB_APP_INSTALLATION_ID`) |
| GitHub App private key (PEM) | `private-key` | init container (`GITHUB_APP_PRIVATE_KEY`) |

| Secret (`secrets.llm.name`, default: `llm-credentials`) | Key | Used by |
|---|---|---|
| LLM API key | `api-key` | main container (`OPENAI_API_KEY`) |
| LLM base URL | `base-url` | main container (`OPENAI_BASE_URL`) |
| LLM model name | `model` | main container (`OPENAI_MODEL`) |
| Kubernetes API server URL | `kube-api-server` | main container (`KUBE_API_SERVER`) |

### CRD upgrade hook

The hook consists of three resources plus a Job, all with
`helm.sh/hook: pre-upgrade,pre-install` annotation:
- `ServiceAccount`: `mechanic-crd-upgrader`
- `ClusterRole`: permission to `get`, `create`, `update`, `patch`
  `customresourcedefinitions`
- `ClusterRoleBinding`
- `Job`: runs `kubectl apply -f /crds/remediationjob.yaml` using a ConfigMap
  volume that holds the CRD YAML; image: `bitnami/kubectl:latest`

All four carry `helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded`
so they are cleaned up after a successful upgrade.

### File layout

```
charts/mechanic/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── _helpers.tpl
│   ├── namespace.yaml
│   ├── serviceaccount-watcher.yaml
│   ├── serviceaccount-agent.yaml
│   ├── clusterrole-watcher.yaml
│   ├── clusterrole-agent.yaml
│   ├── clusterrolebinding-watcher.yaml
│   ├── clusterrolebinding-agent.yaml
│   ├── role-watcher.yaml
│   ├── rolebinding-watcher.yaml
│   ├── role-agent.yaml
│   ├── rolebinding-agent.yaml
│   ├── deployment-watcher.yaml
│   ├── configmap-prompt.yaml
│   ├── serviceaccount-crd-hook.yaml   (hook)
│   ├── clusterrole-crd-hook.yaml      (hook)
│   ├── clusterrolebinding-crd-hook.yaml  (hook)
│   ├── job-crd-upgrade.yaml           (hook)
│   ├── service-metrics.yaml
│   ├── servicemonitor.yaml
│   └── NOTES.txt
├── crds/
│   └── remediationjob.yaml
└── files/
    └── prompts/
        └── default.txt
```

---

## Success Criteria

- [ ] `helm lint charts/mechanic/` passes with no errors
- [ ] `helm template charts/mechanic/ --set gitops.repo=org/repo --set gitops.manifestRoot=kubernetes | kubectl apply --dry-run=client -f -` succeeds
- [ ] `helm install` on a fresh cluster creates all expected resources including CRD
- [ ] `mechanic-agent-token` Secret of type `kubernetes.io/service-account-token`
  is created and auto-populated by the token controller (agent Jobs can authenticate)
- [ ] `helm upgrade` applies any CRD schema changes via the pre-upgrade hook
- [ ] All values documented and defaulted in `values.yaml`
- [ ] RBAC exactly mirrors the existing Kustomize manifests
- [ ] Prompt ConfigMap renders from `files/prompts/default.txt` by default; `prompt.override` takes precedence
- [ ] Secrets are referenced by name only; chart never creates Secret content
- [ ] Metrics Service and ServiceMonitor only rendered when enabled
- [ ] `createNamespace: true` creates the namespace; `false` omits it
- [ ] GitHub Actions workflow runs `helm lint` on every PR that touches `charts/`
- [ ] README.md Quick Start section updated with `helm repo add` + `helm install` commands

---

## Stories

| Story | File | Status |
|-------|------|--------|
| Chart scaffold and Chart.yaml | [STORY_01_chart_scaffold.md](STORY_01_chart_scaffold.md) | Complete |
| Values schema and _helpers.tpl | [STORY_02_values_schema.md](STORY_02_values_schema.md) | Complete |
| Namespace template | [STORY_03_namespace.md](STORY_03_namespace.md) | Complete |
| ServiceAccount templates | [STORY_04_serviceaccounts.md](STORY_04_serviceaccounts.md) | Complete |
| RBAC templates | [STORY_05_rbac.md](STORY_05_rbac.md) | Complete |
| Watcher Deployment template | [STORY_06_deployment.md](STORY_06_deployment.md) | Complete |
| Prompt ConfigMap and files | [STORY_07_prompt_configmap.md](STORY_07_prompt_configmap.md) | Complete |
| CRD install and upgrade hook | [STORY_08_crd_install_upgrade.md](STORY_08_crd_install_upgrade.md) | Complete |
| Metrics Service and ServiceMonitor | [STORY_09_metrics.md](STORY_09_metrics.md) | Complete |
| NOTES.txt and Secret guidance | [STORY_10_notes_secrets.md](STORY_10_notes_secrets.md) | Complete |
| CI: helm lint workflow | [STORY_11_ci_chart_test.md](STORY_11_ci_chart_test.md) | Complete |
| README: Helm install instructions | [STORY_12_readme_helm.md](STORY_12_readme_helm.md) | Complete |
| Agent token Secret | [STORY_13_agent_token_secret.md](STORY_13_agent_token_secret.md) | Complete |

---

## Implementation Order

```
STORY_01 (Chart.yaml scaffold)
    └── STORY_02 (values.yaml + _helpers.tpl)
            ├── STORY_03 (namespace)
            ├── STORY_04 (ServiceAccounts)
            │       └── STORY_13 (agent-token Secret)  ← depends on SA name helper
            ├── STORY_05 (RBAC)
            ├── STORY_06 (Deployment)  ← depends on STORY_04
            ├── STORY_07 (prompt ConfigMap)
            ├── STORY_08 (CRD + upgrade hook)
            ├── STORY_09 (metrics)
            └── STORY_10 (NOTES.txt)
                    └── STORY_11 (CI workflow)
                            └── STORY_12 (README)
```

---

## Definition of Done

- [ ] All stories complete
- [ ] `helm lint` passes with no errors or warnings
- [ ] Dry-run apply succeeds against a real cluster API (`--dry-run=server` preferred)
- [ ] CI workflow added and passing
- [ ] README updated with Helm instructions
- [ ] Worklog written
