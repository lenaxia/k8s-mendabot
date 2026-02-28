# Worklog: Fix finding 2026-02-23-005 — ConfigMap write verbs in ClusterRole

**Date:** 2026-02-23
**Session:** Security remediation — remove ConfigMap write verbs from ClusterRole; add them to namespace-scoped Role
**Status:** Complete

---

## Objective

Fix finding 2026-02-23-005: the watcher ClusterRole granted `create/update/patch` on ConfigMaps
cluster-wide, violating least-privilege. The watcher only needs to write ConfigMaps in its own
namespace (`mechanic`). ConfigMap reads must remain cluster-wide so the watcher can watch prompt
ConfigMap changes across namespaces.

---

## Work Completed

### 1. `deploy/kustomize/clusterrole-watcher.yaml`

Split the single first rule that covered `pods, pvcs, nodes, namespaces, events, configmaps` with
`get/list/watch/create/update/patch` into two rules:

- `pods, persistentvolumeclaims, nodes, namespaces, events` — `get/list/watch/create/update/patch` (unchanged)
- `configmaps` — `get/list/watch` only (write verbs removed)

All other rules (apps, batch, remediation.mechanic.io) are unchanged.

### 2. `deploy/kustomize/role-watcher.yaml`

Added a new rule for namespace-scoped ConfigMap writes:

```yaml
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
```

---

## Key Decisions

- `create/update/patch` removed from ClusterRole configmaps rule — watcher never needs to write
  ConfigMaps outside its own namespace.
- Write verbs placed in the namespace-scoped Role, not a second ClusterRole, so they are
  constrained to the `mechanic` namespace at the RoleBinding level.
- `delete` not added to the Role configmaps rule — the watcher has no use case for deleting
  ConfigMaps.

---

## Blockers

None.

---

## Tests Run

- `kubectl apply -k deploy/kustomize/ --dry-run=client` — cluster unreachable (192.168.3.30:6443
  timeout); fallback executed.
- `go build ./...` — exit 0, all Go packages compile cleanly.
- `grep -n configmaps deploy/kustomize/clusterrole-watcher.yaml` — line 10 shows only
  `resources: ["configmaps"]` with verbs `get/list/watch`.
- `grep -n configmaps deploy/kustomize/role-watcher.yaml` — line 14 shows `resources: ["configmaps"]`
  with verbs `get/list/watch/create/update/patch`.
- `git diff` confirms only the two intended changes; no other rules modified.

---

## Next Steps

Continue with remaining epic 12 security findings on the `feature/epic12-security-remediation`
branch.

---

## Files Modified

- `deploy/kustomize/clusterrole-watcher.yaml`
- `deploy/kustomize/role-watcher.yaml`
- `docs/WORKLOGS/0053_2026-02-23_finding-005-configmap-clusterrole.md` (this file)
- `docs/WORKLOGS/README.md` (index updated)
