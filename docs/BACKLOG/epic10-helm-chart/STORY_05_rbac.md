# Story 05: RBAC Templates

**Epic:** [epic10-helm-chart](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **cluster operator**, I want the chart to create all necessary RBAC resources
so mechanic can watch cluster resources and create Jobs, without needing to apply
separate RBAC manifests.

---

## Acceptance Criteria

All eight templates exist and are gated by `{{- if .Values.rbac.create }}`:

- [ ] `templates/clusterrole-watcher.yaml` — mirrors `deploy/kustomize/clusterrole-watcher.yaml`
- [ ] `templates/clusterrolebinding-watcher.yaml` — subject is `mechanic.watcherSAName`
- [ ] `templates/clusterrole-agent.yaml` — mirrors `deploy/kustomize/clusterrole-agent.yaml`
- [ ] `templates/clusterrolebinding-agent.yaml` — subject is `mechanic.agentSAName`
- [ ] `templates/role-watcher.yaml` — mirrors `deploy/kustomize/role-watcher.yaml`
  (namespace-scoped; namespace is `{{ .Release.Namespace }}`)
- [ ] `templates/rolebinding-watcher.yaml` — subject is `mechanic.watcherSAName`
- [ ] `templates/role-agent.yaml` — mirrors `deploy/kustomize/role-agent.yaml`
- [ ] `templates/rolebinding-agent.yaml` — subject is `mechanic.agentSAName`

All resources carry standard labels via `include "mechanic.labels"`.
ClusterRole and ClusterRoleBinding names are `{{ include "mechanic.fullname" . }}-watcher`
and `{{ include "mechanic.fullname" . }}-agent` to avoid name collision across installs.

---

## Tasks

- [ ] Read current Kustomize RBAC files to capture exact rules
- [ ] Write all 8 RBAC templates
- [ ] Verify rule sets are identical to Kustomize originals
- [ ] Verify `rbac.create: false` causes all 8 templates to render nothing
- [ ] Verify ClusterRole names include the release fullname (no two installs collide)

---

## RBAC source of truth

The rules must exactly mirror `deploy/kustomize/`:

| Template | Source file |
|----------|-------------|
| clusterrole-watcher | `clusterrole-watcher.yaml` |
| clusterrole-agent | `clusterrole-agent.yaml` |
| role-watcher | `role-watcher.yaml` |
| role-agent | `role-agent.yaml` |

Read those four files before writing the templates. Do not assume the rules — copy
them exactly.

---

## Notes

- ClusterRoleBindings reference `namespace: {{ .Release.Namespace }}` in the subject
  block even though ClusterRoleBindings are cluster-scoped. This is correct Kubernetes
  behaviour: the subject ServiceAccount lives in a namespace.
- RoleBindings are namespace-scoped and reference `{{ .Release.Namespace }}`.
- Binding names should match role names (e.g. `{{ include "mechanic.fullname" . }}-watcher`)
  for consistency.

---

## Dependencies

**Depends on:** STORY_02 (helpers), STORY_04 (SA names used in subjects)
**Blocks:** nothing (independent from Deployment template)

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] `helm template ... | grep -A5 "kind: ClusterRole"` shows correct rule sets
- [ ] `rbac.create: false` produces zero RBAC resources in rendered output
