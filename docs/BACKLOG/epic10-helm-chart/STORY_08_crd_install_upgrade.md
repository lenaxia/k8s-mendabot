# Story 08: CRD Install and Pre-Upgrade Hook

**Epic:** [epic10-helm-chart](README.md)
**Priority:** High (without this, helm upgrade breaks CRD schema changes)
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **cluster operator**, I want CRDs installed automatically on `helm install` and
upgraded automatically on `helm upgrade`, without needing to run `kubectl apply`
manually between chart version bumps.

---

## Background

Helm has a deliberate design limitation: it installs CRDs from the `crds/` directory
on `helm install` but **never modifies them on `helm upgrade`**, even if the CRD YAML
has changed. This is to prevent accidental schema changes to production data.

For mechanic this is a real problem: as `RemediationJob` evolves (new fields, new
status conditions), operators on older versions would miss schema updates silently.

The solution is a `pre-upgrade` hook Job that runs `kubectl apply` against the CRD
YAML. Because the hook runs before the main chart resources are upgraded, the CRD
schema is always current before the new watcher version starts reconciling.

---

## Acceptance Criteria

**Fresh install path (`helm install`):**
- [ ] `charts/mechanic/crds/remediationjob.yaml` installs the CRD automatically
  via Helm's native `crds/` mechanism (no template needed for this)

**Upgrade path (`helm upgrade`):**
- [ ] `templates/configmap-crd-hook.yaml` — ConfigMap containing the CRD YAML,
  annotated as a pre-upgrade hook
- [ ] `templates/serviceaccount-crd-hook.yaml` — ServiceAccount for the hook Job
- [ ] `templates/clusterrole-crd-hook.yaml` — ClusterRole with rules:
  `apiGroups: [apiextensions.k8s.io]`, `resources: [customresourcedefinitions]`,
  `verbs: [get, create, update, patch]`
- [ ] `templates/clusterrolebinding-crd-hook.yaml` — binds the role to the hook SA
- [ ] `templates/job-crd-upgrade.yaml` — Job that runs `kubectl apply -f /crds/remediationjob.yaml`

All five hook resources must carry:
```yaml
annotations:
  "helm.sh/hook": pre-upgrade,pre-install
  "helm.sh/hook-weight": "-5"
  "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
```

The Job spec:
- Image: `bitnami/kubectl:latest`
- Command: `["kubectl", "apply", "-f", "/crds/remediationjob.yaml"]`
- Volume: `configMap` mounting the hook ConfigMap at `/crds/`
- `restartPolicy: Never`
- `backoffLimit: 3`
- SA: the hook ServiceAccount
- Carries standard labels

---

## Tasks

- [ ] Verify `charts/mechanic/crds/remediationjob.yaml` exists (from STORY_01)
- [ ] Write `templates/configmap-crd-hook.yaml` — uses `.Files.Get "crds/remediationjob.yaml"`
  to embed the CRD in the ConfigMap data
- [ ] Write `templates/serviceaccount-crd-hook.yaml`
- [ ] Write `templates/clusterrole-crd-hook.yaml`
- [ ] Write `templates/clusterrolebinding-crd-hook.yaml`
- [ ] Write `templates/job-crd-upgrade.yaml`
- [ ] Verify all 5 resources have correct hook annotations
- [ ] Verify `helm template` renders the hook Job correctly

---

## Notes

- Hook resources are cleaned up on success (`hook-delete-policy: hook-succeeded`).
  On failure they remain for debugging. `before-hook-creation` ensures a clean slate
  on repeated upgrades.
- The `pre-install` annotation on the hook means it also runs during fresh install,
  but since `crds/` already handles that case idempotently, a second `kubectl apply`
  is harmless.
- Hook weight `-5` runs these resources before any other pre-upgrade hooks.
- The hook SA/ClusterRole names should be `{{ include "mechanic.fullname" . }}-crd-upgrader`
  to avoid collision with the main watcher/agent RBAC.
- **Image**: use `registry.k8s.io/kubectl:v1.28.16` — this is the official upstream
  kubectl image from the Kubernetes project registry, not a third-party vendor image.
  Pin to a specific patch version matching the chart's `kubeVersion: >=1.28.0-0`.
  Bump the tag when the chart minimum Kubernetes version is raised.

---

## Dependencies

**Depends on:** STORY_01 (CRD file must exist in `crds/`), STORY_02 (helpers)
**Blocks:** nothing

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] `helm template ... | grep "helm.sh/hook"` shows all 5 hook resources
- [ ] Hook Job spec includes the ConfigMap volume at `/crds/`
- [ ] `hook-delete-policy` is correct on all 5 resources
