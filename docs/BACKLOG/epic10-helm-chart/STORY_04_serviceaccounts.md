# Story 04: ServiceAccount Templates

**Epic:** [epic10-helm-chart](README.md)
**Priority:** High (Deployment template depends on SA names)
**Status:** Not Started
**Estimated Effort:** 15 minutes

---

## User Story

As a **cluster operator**, I want the chart to create dedicated ServiceAccounts for
the watcher and agent so the least-privilege RBAC bindings have named subjects.

---

## Acceptance Criteria

- [ ] `charts/mechanic/templates/serviceaccount-watcher.yaml` creates a ServiceAccount
  named `{{ include "mechanic.watcherSAName" . }}` in `{{ .Release.Namespace }}`
- [ ] `charts/mechanic/templates/serviceaccount-agent.yaml` creates a ServiceAccount
  named `{{ include "mechanic.agentSAName" . }}` in `{{ .Release.Namespace }}`
- [ ] Both ServiceAccounts carry standard chart labels via `include "mechanic.labels"`
- [ ] Neither ServiceAccount sets `automountServiceAccountToken` — the default (`true`)
  is required because the agent entrypoint reads `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`
  as a fallback when the `mechanic-agent-token` Secret is absent. The `agent-token`
  volume from `mechanic-agent-token` Secret (see STORY_13) provides the primary token;
  the SA-automounted token provides the CA cert path in all cases.
- [ ] Rendered SA names match what the Deployment template and RBAC bindings reference

---

## Tasks

- [ ] Write `templates/serviceaccount-watcher.yaml`
- [ ] Write `templates/serviceaccount-agent.yaml`
- [ ] Verify rendered names match `mechanic.watcherSAName` and `mechanic.agentSAName`
  helper output for a sample release name

---

## Notes

- The watcher SA name is injected into the Deployment via
  `spec.template.spec.serviceAccountName`.
- The agent SA name is injected into the watcher Deployment as the `AGENT_SA` env var,
  which the JobBuilder uses when constructing the agent Job spec.
- Both SAs are always created — there is no values gate. If operators want to bring
  their own SAs they should set `rbac.create: false` and manage everything externally.

---

## Dependencies

**Depends on:** STORY_02 (_helpers.tpl with `mechanic.watcherSAName` / `mechanic.agentSAName`)
**Blocks:** STORY_05 (RBAC binding subjects), STORY_06 (Deployment SA reference)

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] `helm template ... | grep ServiceAccount` shows both SA resources with correct names
