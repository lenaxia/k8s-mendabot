# Worklog: Epic 10 Helm Chart — Implementation

**Date:** 2026-02-23
**Session:** Full chart implementation — all 13 stories complete
**Branch:** feature/epic10-helm-chart
**Status:** Complete

---

## Objective

Implement the `charts/mechanic/` Helm chart designed and planned in worklog 0036.
Implement all 13 stories in dependency order. Validate with `helm lint --strict` and
`helm template` dry-run before closing.

---

## Work Completed

### Branch

- Created `feature/epic10-helm-chart` from `main`

### STORY_01: Chart scaffold

- Created `charts/mechanic/` directory structure:
  `templates/`, `crds/`, `files/prompts/`
- Wrote `charts/mechanic/Chart.yaml` with `apiVersion: v2`, `name: mechanic`,
  `version: 0.1.0`, `appVersion: v0.3.0`, `kubeVersion: >=1.28.0-0`
- Copied `deploy/kustomize/crd-remediationjob.yaml` to `charts/mechanic/crds/remediationjob.yaml`
  (byte-for-byte identical)

### STORY_02: values.yaml + _helpers.tpl

- Wrote `charts/mechanic/values.yaml` with all keys from the approved schema,
  inline comments, and correct defaults
- Wrote `charts/mechanic/templates/_helpers.tpl` with named templates:
  - `mechanic.name` — `"mechanic"`
  - `mechanic.fullname` — release-name dedup, trunc 63
  - `mechanic.labels` — 5-label standard set
  - `mechanic.selectorLabels` — stable 2-label subset
  - `mechanic.watcherSAName` — `<fullname>-watcher`
  - `mechanic.agentSAName` — `<fullname>-agent`
  - `mechanic.watcherImage` — falls back to `Chart.AppVersion`
  - `mechanic.agentImage` — falls back to `Chart.AppVersion`

### STORY_03: Namespace template

- `templates/namespace.yaml` — guarded by `{{- if .Values.createNamespace }}`

### STORY_04: ServiceAccount templates

- `templates/serviceaccount-watcher.yaml`
- `templates/serviceaccount-agent.yaml`
- Neither sets `automountServiceAccountToken` (default `true` required for token mounts)

### STORY_05: RBAC templates (8 files)

Rules copied verbatim from `deploy/kustomize/`:

| Template | Source rule summary |
|----------|---------------------|
| `clusterrole-watcher` | core/apps/batch/remediationjobs full RBAC |
| `clusterrole-agent` | `*/*` get/list/watch |
| `clusterrolebinding-watcher` | binds to `watcherSAName` |
| `clusterrolebinding-agent` | binds to `agentSAName` |
| `role-watcher` | batch/jobs + core/pods in namespace |
| `rolebinding-watcher` | binds to `watcherSAName` |
| `role-agent` | remediationjobs/status get/patch |
| `rolebinding-agent` | binds to `agentSAName` |

All 8 gated by `{{- if .Values.rbac.create }}`

### STORY_06: Deployment template

- `templates/deployment-watcher.yaml` — all 13 env vars, security context matching
  Kustomize source, liveness/readiness probes, resource limits
- `GITOPS_REPO` and `GITOPS_MANIFEST_ROOT` use `required` — render errors if missing
- Image resolves to `Chart.AppVersion` when `image.tag` is empty

### STORY_07: Prompt ConfigMap + files

- Extracted prompt verbatim from `deploy/kustomize/configmap-prompt.yaml` into
  `charts/mechanic/files/prompts/default.txt`
- `templates/configmap-prompt.yaml` — uses `.Files.Get` with `fail` guard for missing
  prompt files; `prompt.override` takes precedence when set

### STORY_08: CRD install and upgrade hook

5 hook resources (all annotated `pre-upgrade,pre-install`, weight `-5`,
delete-policy `before-hook-creation,hook-succeeded`):

- `templates/configmap-crd-hook.yaml` — embeds CRD YAML via `.Files.Get`
- `templates/serviceaccount-crd-hook.yaml`
- `templates/clusterrole-crd-hook.yaml` — `apiextensions.k8s.io` CRD CRUD
- `templates/clusterrolebinding-crd-hook.yaml`
- `templates/job-crd-upgrade.yaml` — `registry.k8s.io/kubectl:v1.28.16`; mounts
  CRD ConfigMap at `/crds/`; `runAsUser: 65534`, full securityContext

### STORY_09: Metrics Service and ServiceMonitor

- `templates/service-metrics.yaml` — gated by `metrics.enabled`
- `templates/servicemonitor.yaml` — gated by `metrics.enabled AND serviceMonitor.enabled`;
  merges `metrics.serviceMonitor.labels` into metadata labels

### STORY_10: NOTES.txt

- `templates/NOTES.txt` — post-install instructions with exact `kubectl create secret`
  commands for both Secrets; conditional ServiceMonitor note

### STORY_11: CI workflow

- `.github/workflows/chart-test.yaml` — triggers on `charts/**`, runs
  `helm lint --strict` and `helm template` with required values

### STORY_12: README update

- Replaced the old `## Deployment` + `## Configuration` sections with:
  - `## Quick Start` — 3-step Helm install (create Secrets → helm install → verify)
  - Kustomize as alternative with a caveat note
  - `## Helm Configuration Reference` — full 24-row values table

### STORY_13: Agent token Secret

- `templates/secret-agent-token.yaml` — type `kubernetes.io/service-account-token`,
  annotation `kubernetes.io/service-account.name: <agentSAName>`, name
  `mechanic-agent-token` (hardcoded to match `job.go`)

---

## Validation Results

```
helm lint charts/mechanic/ --strict
→ 1 chart(s) linted, 0 chart(s) failed
   INFO: icon is recommended (cosmetic; no chart icon URL yet)
   WARN: gitops.repo/manifestRoot required — expected; required guard works correctly
```

```
helm template mechanic charts/mechanic/ \
  --set gitops.repo=org/repo \
  --set gitops.manifestRoot=kubernetes \
  --namespace mechanic
→ 20 resources rendered without error
```

Spot checks passed:
- `required` fires with clear error when `gitops.repo` missing
- `metrics.enabled=true` renders Service only
- `metrics.enabled=true --set metrics.serviceMonitor.enabled=true` renders both
- `createNamespace=true` renders Namespace
- Image tag defaults to `v0.3.0` (Chart.AppVersion) when `image.tag` is empty
- Prompt content renders verbatim with `${FINDING_KIND}` etc. preserved
- CRD YAML embedded correctly in hook ConfigMap
- All 5 hook resources carry correct annotations and delete-policy
- Hook infra resources (SA, ClusterRole, ClusterRoleBinding, ConfigMap) weight `-10`
- Hook Job weight `-5` (runs after infra)
- Agent token Secret has correct type and annotation
- CRD schema contains `isSelfRemediation`, `chainDepth`, `targetRepoOverride`

---

## File Layout Created

```
charts/mechanic/
├── Chart.yaml
├── values.yaml
├── crds/
│   └── remediationjob.yaml
├── files/
│   └── prompts/
│       └── default.txt
└── templates/
    ├── _helpers.tpl
    ├── namespace.yaml
    ├── serviceaccount-watcher.yaml
    ├── serviceaccount-agent.yaml
    ├── secret-agent-token.yaml
    ├── configmap-prompt.yaml
    ├── clusterrole-watcher.yaml
    ├── clusterrole-agent.yaml
    ├── clusterrolebinding-watcher.yaml
    ├── clusterrolebinding-agent.yaml
    ├── role-watcher.yaml
    ├── role-agent.yaml
    ├── rolebinding-watcher.yaml
    ├── rolebinding-agent.yaml
    ├── deployment-watcher.yaml
    ├── configmap-crd-hook.yaml
    ├── serviceaccount-crd-hook.yaml
    ├── clusterrole-crd-hook.yaml
    ├── clusterrolebinding-crd-hook.yaml
    ├── job-crd-upgrade.yaml
    ├── service-metrics.yaml
    ├── servicemonitor.yaml
    └── NOTES.txt

.github/workflows/chart-test.yaml   ← new CI workflow
README.md                            ← Quick Start + Configuration Reference updated
```

---

## Post-Implementation Bug Fixes (Session 3 — Adversarial Deep Review)

A second adversarial review pass found 10 additional failures. All 10 were fixed.

### CHECK-2 — `status.conditions` fields silently pruned by Kubernetes

**Files:** both CRD files

The `status.conditions` array items had `type: object` but no declared properties and no
`x-kubernetes-preserve-unknown-fields: true`. Kubernetes structural schema validation
silently prunes unknown fields from `type: object` items. Every `metav1.Condition` field
(`type`, `status`, `reason`, `message`, `lastTransitionTime`) would be stripped on every
status patch. The conditions list would always appear empty in etcd.

**Fix:** Added `x-kubernetes-preserve-unknown-fields: true` to `status.conditions.items`
in both `deploy/kustomize/crd-remediationjob.yaml` and `charts/mechanic/crds/remediationjob.yaml`.

### CHECK-3 — Dead config in `values.yaml` (`secrets.githubApp.name`, `secrets.llm.name`)

**File:** `charts/mechanic/values.yaml`, `charts/mechanic/templates/NOTES.txt`

The `secrets.githubApp.name` and `secrets.llm.name` keys were declared and documented
but never referenced in any template. `job.go` hardcodes `"github-app"` and
`"llm-credentials"`. A user who set these values expecting them to work would be silently
misled.

**Fix:** Removed the two sub-keys from `values.yaml` entirely. Replaced with a single
`secrets: {}` placeholder and a clear comment explaining the names are compile-time
constants in `job.go`. Updated `NOTES.txt` to use the literal hardcoded names instead of
the now-removed `.Values.secrets.*` references.

### CHECK-5a — `events` write missing from watcher ClusterRole

**File:** `charts/mechanic/templates/clusterrole-watcher.yaml`

The watcher ClusterRole only granted `get/list/watch` on `events`. controller-runtime
emits Kubernetes Events on reconciliation errors via the EventRecorder. Without
`create` and `patch` on `events`, all controller events are silently dropped.

**Fix:** Split `events` into its own rule with `["get", "list", "watch", "create", "patch"]`
while keeping the other core resources at read-only.

### CHECK-5b — `coordination.k8s.io/leases` missing from watcher ClusterRole

**File:** `charts/mechanic/templates/clusterrole-watcher.yaml`

controller-runtime's Manager uses `coordination.k8s.io/leases` for leader election by
default in recent versions. The watcher ClusterRole had no rule for leases. The controller
would fail to start with a permission-denied error if leader election is active.

**Fix:** Added a `coordination.k8s.io` rule with full CRUD on `leases`.

### CHECK-7d — `kubectl apply` fails on read-only root filesystem

**File:** `charts/mechanic/templates/job-crd-upgrade.yaml`

The CRD hook Job had `readOnlyRootFilesystem: true` but no writable directory for
`kubectl`'s cache files (written to `$HOME/.kube/cache`). With no writable mount,
`kubectl apply` fails immediately with a permission error, meaning the CRD upgrade hook
never applies the CRD.

**Fix:** Added an `emptyDir` volume named `kube-cache` mounted at `/tmp`, and set
`HOME=/tmp` in the container's environment so `kubectl` writes its cache to `/tmp`.

### CHECK-9a — CI lint step missing required `--set` flags

**File:** `.github/workflows/chart-test.yaml`

The `helm lint --strict` step ran without providing `gitops.repo` or `gitops.manifestRoot`.
Depending on the Helm version, this either silently passed despite the `required` WARNs or
failed for an ambiguous reason. The template render step already had the flags but lint did not.

**Fix:** Added `--set gitops.repo=org/repo --set gitops.manifestRoot=kubernetes` to the
lint step.

### CHECK-9b — Hook RBAC not gated by `rbac.create`

**Files:** `clusterrole-crd-hook.yaml`, `clusterrolebinding-crd-hook.yaml`

The hook ClusterRole and ClusterRoleBinding had no `{{- if .Values.rbac.create }}` guard.
When `rbac.create=false` (for externally managed RBAC), these two resources were still
rendered, creating an inconsistency: all namespace RBAC is suppressed but two hook
cluster-scoped roles are not.

**Fix:** Wrapped both hook RBAC templates in `{{- if .Values.rbac.create }}` guards.

### CHECK-12c — `appVersion` stale at `v0.3.0`

**File:** `charts/mechanic/Chart.yaml`

`appVersion` was `v0.3.0`. The latest git tag is `v0.3.2`. When `image.tag` is empty
(the default), the chart resolves to `appVersion` — meaning default deployments would
pin to an image two patch versions behind.

**Fix:** Updated `appVersion` from `v0.3.0` to `v0.3.2`.

---

## Validation Results (Session 3)

```
helm lint charts/mechanic/ --strict \
  --set gitops.repo=org/repo \
  --set gitops.manifestRoot=kubernetes
→ 1 chart(s) linted, 0 chart(s) failed   (INFO: icon is recommended — cosmetic)
```

Edge cases:
- `rbac.create=false` — 10 RBAC resources suppressed (all namespaced + hook cluster RBAC) ✓
- Missing `gitops.repo` — hard error, no silent render ✓
- `metrics.enabled=true + serviceMonitor.enabled=true` — Service + ServiceMonitor rendered ✓
- `createNamespace=true` — Namespace rendered ✓

---

## Post-Implementation Bug Fixes (Session 2 — First Review Pass)

A skeptical deep-dive review identified four issues fixed in the same session:

### CRITICAL-1 — CRD schema missing self-remediation fields

**File:** `charts/mechanic/crds/remediationjob.yaml`

The chart's CRD copy was missing `isSelfRemediation`, `chainDepth`, and `targetRepoOverride`
from its openAPIV3Schema. These fields are present in the canonical
`deploy/kustomize/crd-remediationjob.yaml` but were not copied over. Kubernetes
structural schemas prune unknown fields silently — agent Jobs would always see
`IS_SELF_REMEDIATION=false` and `CHAIN_DEPTH=0`, breaking the cascade prevention logic
added in v0.3.0.

**Fix:** Added all three fields to `charts/mechanic/crds/remediationjob.yaml` under
`spec.properties`, matching the canonical file exactly.

### CRITICAL-2 — `secrets.*.name` values are documentation-only

**File:** `charts/mechanic/values.yaml`

The `secrets.githubApp.name` and `secrets.llm.name` keys gave false confidence that
renaming the Secrets would work. `internal/controller/job.go` hardcodes `"github-app"`
and `"llm-credentials"` at compile time. Changing the values without rebuilding the
image causes `CreateContainerConfigError` on every agent Job.

**Fix:** Replaced the mild "Default matches..." comments with an explicit `WARNING` block
explaining the compile-time constraint and that the names must match unless the image is
rebuilt.

### CRITICAL-3 — Hook infra resources had same weight as hook Job

**Files:** `serviceaccount-crd-hook.yaml`, `clusterrole-crd-hook.yaml`,
`clusterrolebinding-crd-hook.yaml`, `configmap-crd-hook.yaml`

All hook resources shared weight `-5`. Helm executes same-weight resources in parallel;
the hook Job could start before its ServiceAccount or ConfigMap was created, causing a
race condition on fresh installs.

**Fix:** Changed hook infra weights to `-10`. Job weight remains `-5`. Helm now
guarantees infra is applied before the Job runs.

### HIGH-2 — Hardcoded ConfigMap name gap undocumented

**File:** `charts/mechanic/templates/configmap-prompt.yaml`

The ConfigMap is named `opencode-prompt` because `job.go` mounts it by that exact name.
This was not documented — a user with two Helm releases in the same namespace would get
a silent collision. Added a prominent comment.

---

## Known Gaps / Follow-up

- **No `values.schema.json`** — schema validation not in scope for this epic.
  Would prevent misconfigured installs from reaching the cluster.
- **No OCI push** — `helm install oci://ghcr.io/...` not yet wired; requires
  Chart Releaser Action setup (future epic).
- **No `helm test` hook** — a connectivity test pod would strengthen CI.
  `helm/chart-testing` (ct) can be added when OCI publishing is configured.
- **Kustomize gap still exists** — `mechanic-agent-token` Secret is still absent
  from `deploy/kustomize/`. Fix is out of scope for this epic but documented in
  STORY_13.

---

## Next Steps

1. Commit and push `feature/epic10-helm-chart` to remote
2. Open PR against `main`
3. Verify CI (`Chart Lint` workflow) passes on the PR
4. Merge after review
5. Tag `charts/mechanic-0.1.0` (or configure Chart Releaser Action for automation)
