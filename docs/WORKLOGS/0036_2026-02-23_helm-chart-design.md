# Worklog: Helm Chart Design — bjw-s Common Library, Values Schema, Self-Remediation

**Date:** 2026-02-23
**Session:** README updates, Helm chart architecture planning
**Status:** Complete

---

## Objective

Update the public README to reflect the post-epic09 native provider architecture, and
design the Helm chart that will allow external users to deploy mechanic — including all
configuration knobs, the bjw-s common library integration strategy, CRD upgrade
automation, prompt file management, and self-remediation capabilities.

---

## Work Completed

### 1. README.md overhaul

Replaced the sparse post-epic09 README with a comprehensive user-facing document:

- Architecture diagram (Mermaid `flowchart TD`, straight edges via `curve: linear`)
- Detected failure conditions table (all six providers)
- What the agent does — investigation steps and three-outcome model
- `RemediationJob` CRD section with `kubectl get rjob` example output
- `RemediationJob` lifecycle state diagram (Mermaid `stateDiagram-v2`)
- Agent image tools table with versions and purposes
- GitHub App permissions and Secret key structure
- Configuration split into Required / Optional / Secrets sections
- Fixed Mermaid `\n` → `<br/>` for correct line break rendering
- Replaced nested subgraphs (label-inside-box confusion) with single-level subgraph

Commits: `6a5eb1b`, `af8f845`, `8e3e28c`, `7da8842`

### 2. Helm chart architecture design

Decided to create a Helm chart at `charts/mechanic/` using the bjw-s common library
(`common` v4.6.2) as a dependency.

**Integration strategy:**

| Concern | Approach |
|---|---|
| Watcher Deployment | `controllers.watcher` via bjw-s common |
| ServiceAccounts | `serviceAccount.watcher` + `serviceAccount.agent` via bjw-s common |
| RBAC | Custom templates gated by `rbac.create` |
| Prompt ConfigMap | Custom template — reads from `files/prompts/<name>.txt` via `.Files.Get` |
| Metrics Service | `service.metrics` via bjw-s common |
| ServiceMonitor | `serviceMonitor.metrics` via bjw-s common |
| CRD (install) | `crds/remediationjob.yaml` — Helm native install-time directory |
| CRD (upgrade) | `pre-upgrade` hook Job — `kubectl apply` on CRD YAML |

**bjw-s naming convention confirmed:** identifier suffix is only appended when multiple
resources of the same kind are enabled, or `global.alwaysAppendIdentifierToResourceName`
is true. With two ServiceAccounts (`watcher`, `agent`), both will be named
`<fullname>-watcher` and `<fullname>-agent`. AGENT_SA env var is set to
`{{ include "bjw-s.common.lib.chart.names.fullname" . }}-agent`.

### 3. Complete values schema design

**Top-level custom keys:**

```
image                          — watcher image (repository, tag, pullPolicy)
agent.image                    — agent image (lockstepped to Chart.appVersion)
gitops.repo                    — required; GITOPS_REPO
gitops.manifestRoot            — required; GITOPS_MANIFEST_ROOT
watcher.stabilisationWindowSeconds
watcher.maxConcurrentJobs
watcher.remediationJobTTLSeconds
watcher.sinkType
watcher.logLevel
selfRemediation.enabled
selfRemediation.maxDepth
selfRemediation.upstreamRepo
secrets.githubApp.name         — existing Secret name; chart does not create
secrets.llm.name               — existing Secret name; chart does not create
prompt.name                    — selects built-in prompt file (default: "default")
prompt.override                — full content override; takes precedence over name
rbac.create                    — gate for all RBAC creation
metrics.enabled
metrics.serviceMonitor.enabled / interval / scrapeTimeout / labels
```

**Key decisions locked in this session:**

| Decision | Rationale |
|---|---|
| Watcher + agent image tags lockstepped to `Chart.appVersion` | Avoids version skew between the Job creator and the Job runner; they are always built and released together |
| `crds/` + pre-upgrade hook | Helm `crds/` handles fresh install; hook handles subsequent upgrades (Helm deliberately does not auto-upgrade CRDs from `crds/`) |
| Agent namespace = `Release.Namespace` | Simplifies RBAC; watcher and agent Jobs share a namespace; no cross-namespace complexity |
| Secrets referenced by name, not created by chart | Private keys and API keys must never appear in Helm release history or `values.yaml` |
| Prompt in `files/prompts/` | Enables future multi-prompt support (different prompts per scenario) and user override via `prompt.name` without forking the chart |

### 4. Self-remediation capability design

Mechanic can analyze its own component failures up to a configurable depth, and open
upstream PRs to the mechanic repository when it identifies a mechanic bug vs. a user
config issue.

**New env vars injected into watcher:**

| Env var | Values key | Default |
|---|---|---|
| `SELF_REMEDIATION_ENABLED` | `selfRemediation.enabled` | `false` |
| `SELF_REMEDIATION_MAX_DEPTH` | `selfRemediation.maxDepth` | `1` |
| `SELF_REMEDIATION_UPSTREAM_REPO` | `selfRemediation.upstreamRepo` | `lenaxia/k8s-mechanic` |

**Depth semantics:** depth 1 means mechanic can investigate and fix mechanic, but any
agent Job spawned for a mechanic failure will not itself spawn further Jobs for
mechanic failures. This prevents infinite investigation loops.

**Two-target model for agent:** when handling a mechanic failure, the agent determines
whether the root cause is a user configuration error (→ PR on user's GitOps repo) or a
mechanic bug (→ PR on `SELF_REMEDIATION_UPSTREAM_REPO`). This requires the agent prompt
to be aware of the distinction; a dedicated `self-remediation` prompt variant is the
right long-term approach.

---

## Key Decisions

- **`charts/mechanic/` not `deploy/helm/`**: charts live in `charts/` for Helm repository
  packaging conventions (Chart Releaser Action expects this layout).
- **bjw-s common v4.6.2**: latest stable; requires Kubernetes ≥ 1.28.0 which aligns with
  mechanic's own requirement.
- **Custom RBAC templates over bjw-s `rbac`**: the dual-SA setup (watcher ClusterRole +
  namespaced Role; agent ClusterRole + namespaced Role) is complex enough that raw YAML
  templates are clearer than the bjw-s RBAC DSL.
- **`prompt.override` as escape hatch**: users who need a custom prompt don't need to fork
  the chart; they can supply the full content via values. The `files/prompts/` mechanism
  is for chart-managed named variants.

---

## Blockers

None. Design is complete. Implementation deferred to next session.

---

## Next Steps

- Create `charts/mechanic/` directory structure
- Write `Chart.yaml`, `values.yaml`, `_helpers.tpl`
- Copy CRD to `charts/mechanic/crds/`
- Move prompt to `charts/mechanic/files/prompts/default.txt`
- Write all templates (Deployment via bjw-s, custom RBAC, prompt ConfigMap, CRD hook)
- Run `helm dep update` and `helm lint`
- Update README with Helm install instructions

---

## Files Modified

- `README.md` — full overhaul; architecture + lifecycle diagrams; config reference;
  agent tool table; GitHub App setup; Mermaid fixes
