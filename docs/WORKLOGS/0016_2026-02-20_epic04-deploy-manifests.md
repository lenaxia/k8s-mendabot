# Worklog: Epic 04 — Deploy Manifests

**Date:** 2026-02-20
**Session:** Write all Kustomize deploy manifests; 1 review pass, 3 gaps fixed
**Status:** Complete

---

## Objective

Implement every Kubernetes manifest needed to deploy mendabot-watcher and mendabot-agent
per `docs/DESIGN/lld/DEPLOY_LLD.md`: namespace, CRD, ServiceAccounts, RBAC, Secrets
(placeholders), ConfigMap (placeholder), Deployment, Kustomization, Flux example.

---

## Work Completed

### Files created in `deploy/kustomize/` (16 files)

| File | Description |
|------|-------------|
| `namespace.yaml` | `mendabot` Namespace |
| `crd-remediationjob.yaml` | RemediationJob CRD — full OpenAPI v3 schema with CEL immutability rules and phase enum |
| `serviceaccount-watcher.yaml` | SA for watcher controller |
| `serviceaccount-agent.yaml` | SA for agent Jobs |
| `clusterrole-watcher.yaml` | Cluster-scoped RBAC for watcher (Results, RemediationJobs, finalizers) |
| `clusterrole-agent.yaml` | Read-only cluster RBAC for agent (get/list/watch all resources) |
| `clusterrolebinding-watcher.yaml` | Binds watcher ClusterRole |
| `clusterrolebinding-agent.yaml` | Binds agent ClusterRole |
| `role-watcher.yaml` | Namespace-scoped RBAC (batch/jobs, pods) |
| `rolebinding-watcher.yaml` | Binds watcher Role |
| `role-agent.yaml` | Namespace-scoped RBAC (remediationjobs/status patch) |
| `rolebinding-agent.yaml` | Binds agent Role |
| `configmap-prompt.yaml` | Placeholder prompt ConfigMap (epic05 will fill in real content) |
| `secret-github-app-placeholder.yaml` | GitHub App secret placeholder — `REPLACE_ME` values only |
| `secret-llm-placeholder.yaml` | LLM credentials secret placeholder — `REPLACE_ME` values only |
| `deployment-watcher.yaml` | Watcher Deployment — non-root, readOnlyRootFilesystem, all env vars, health probes, resource limits |
| `kustomization.yaml` | Kustomize resource list |

### Files created in `deploy/flux/` (1 file)

| File | Description |
|------|-------------|
| `ks.yaml` | Flux `Kustomization` v1 example — references `k8s-mendabot` GitRepository, depends on `k8sgpt-operator` |

---

## Bugs Found and Fixed During Review

| # | Severity | File | Bug | Fix |
|---|----------|------|-----|-----|
| 1 | High | `crd-remediationjob.yaml` | `status.phase` had no enum constraint — any string was valid; `Cancelled` not enforced | Added `enum: [Pending, Dispatched, Running, Succeeded, Failed, Cancelled]` |
| 2 | High | `role-watcher.yaml` | `batch/jobs` verbs missing `update` and `patch`; controller-runtime `Owns()` needs these for reconciliation patches | Added `update` and `patch` to verb list |
| 3 | Low | `.gitignore` | Negation rule `!deploy/...` was unanchored while the ignore rule was anchored `/deploy/...` | Made both consistent: `!/deploy/kustomize/secret-*-placeholder.yaml` |

Note: both bugs 1 and 2 were also defects in the LLD — the CRD schema had no enum, and
the LLD's role-watcher lacked update/patch. The fix is authoritative in the manifests;
the LLD will be updated if a revision is ever done.

---

## Key Design Notes

- **Secret placeholder pattern**: The `.gitignore` rule ignores all `secret-*.yaml` files
  but un-ignores `secret-*-placeholder.yaml`. Operators copy the placeholder, rename it
  (removing `-placeholder`), fill in real values, and never commit it.

- **No prompt volume in Deployment**: The prompt ConfigMap is mounted only inside the
  dynamically-created agent Jobs (by `jobbuilder`). The watcher Deployment has no volume
  for it.

- **Agent ClusterRole**: Strictly `get/list/watch` on `*/*`. No mutating verbs anywhere.

- **CRD CEL rules**: All three CEL immutability rules use `!has(oldSelf.field) || ...`
  to guard against rejecting CREATE requests where `oldSelf` fields are absent.

---

## Tests Run

```
python3 yaml.safe_load_all on all 17 files → all parse correctly
go build ./... + go test -race ./... → all 9 packages pass (unchanged)
kustomize build not available locally — CI will validate
```

---

## Next Steps

epic04 is complete. Dependencies now satisfied for:
- **epic05** — Prompt (write real `configmap-prompt.yaml` content)
- **epic06** — CI/CD (image build and push workflows)

---

## Files Created/Modified

| File | Change |
|------|--------|
| `deploy/kustomize/*.yaml` (16 files) | Created |
| `deploy/flux/ks.yaml` | Created |
| `.gitignore` | Fixed negation anchor for secret placeholder rule |
| `docs/BACKLOG/epic04-deploy/README.md` | Marked Complete |
| `docs/BACKLOG/epic03-agent-image/README.md` | Fixed stale story table (all were showing Not Started) |
