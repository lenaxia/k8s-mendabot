# Worklog: Remove CRD Hook Job

**Date:** 2026-02-23
**Session:** Root cause analysis of CRD install failure led to removing the hook job entirely in favour of Helm's native crds/ directory
**Status:** Complete

---

## Objective

Remove the pre-install/pre-upgrade hook Job (and all supporting resources) that applied the `RemediationJob` CRD via `kubectl`, replacing it with Helm's built-in `crds/` directory mechanism which was already present and unused.

---

## Work Completed

### 1. Root cause analysis

- Investigated a `readOnlyRootFilesystem: true` failure on the hook Job: kubectl cannot function with a read-only root filesystem even when `/tmp` is mounted as an `emptyDir`, because it attempts writes outside `/tmp`.
- Confirmed the chart already has `charts/mechanic/crds/remediationjob.yaml`, which Helm applies automatically before any templates on both install and upgrade — rendering the hook Job entirely redundant.

### 2. Removed hook Job scaffold

Deleted all five templates that existed solely to support the hook:

- `charts/mechanic/templates/job-crd-upgrade.yaml`
- `charts/mechanic/templates/configmap-crd-hook.yaml`
- `charts/mechanic/templates/serviceaccount-crd-hook.yaml`
- `charts/mechanic/templates/clusterrole-crd-hook.yaml`
- `charts/mechanic/templates/clusterrolebinding-crd-hook.yaml`

### 3. Cleaned up values.yaml

Removed the `crdUpgrader.image` stanza from `values.yaml` — no longer referenced by any template.

---

## Key Decisions

- **Use `crds/` directory, not a hook Job.** Helm's native CRD handling applies CRDs before templates on every install and upgrade. No pod, no RBAC, no timing race, no filesystem permission issues. This is the correct pattern used by cert-manager, prometheus-operator, and other mature charts.
- **Do not delete `crds/remediationjob.yaml`.** Helm intentionally does not remove CRDs on `helm uninstall` — this is the correct behaviour. Deleting a CRD would destroy all `RemediationJob` instances in the cluster.
- **Do not add a `--skip-crds` escape hatch.** The added complexity is not warranted; operators who want to manage CRDs externally can do so before running `helm install`.

---

## Blockers

None.

---

## Tests Run

No Go tests affected (chart-only change). `helm template` can be used to verify no crd-upgrader resources are rendered:

```
helm template mechanic charts/mechanic --set gitops.repo=test/repo --set gitops.manifestRoot=/
```

---

## Files Modified

- `charts/mechanic/templates/job-crd-upgrade.yaml` — deleted
- `charts/mechanic/templates/configmap-crd-hook.yaml` — deleted
- `charts/mechanic/templates/serviceaccount-crd-hook.yaml` — deleted
- `charts/mechanic/templates/clusterrole-crd-hook.yaml` — deleted
- `charts/mechanic/templates/clusterrolebinding-crd-hook.yaml` — deleted
- `charts/mechanic/values.yaml` — removed `crdUpgrader` stanza
