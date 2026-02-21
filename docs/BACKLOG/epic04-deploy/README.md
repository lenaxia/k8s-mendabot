# Epic: Deploy

## Purpose

Write every Kubernetes manifest needed to deploy the watcher and agent into a cluster.
All manifests live in `deploy/kustomize/` and are designed to be applied via
`kubectl apply -k` or a Flux `Kustomization`.

## Status: Complete

## Dependencies

- Controller epic complete (watcher container spec is finalised)
- Job Builder epic complete (agent Job spec is finalised)
- Agent Image epic complete (image reference is known)

## Blocks

- Prompt epic (prompt ConfigMap is part of this epic)

## Success Criteria

- [ ] `kubectl apply -k deploy/kustomize/ --dry-run=client` succeeds against a real cluster
- [ ] Watcher Deployment starts and reaches Ready
- [ ] Watcher ServiceAccount has the correct ClusterRole and Role bindings
- [ ] Agent ServiceAccount has the correct ClusterRole binding (read-only)
- [ ] Secret placeholder files contain no real values
- [ ] Prompt ConfigMap is present and mounted correctly
- [ ] Flux `Kustomization` example in DEPLOY_LLD.md works against talos-ops-prod

## Stories

| Story | File | Status |
|-------|------|--------|
| Namespace and ServiceAccounts | [STORY_01_namespace_sa.md](STORY_01_namespace_sa.md) | Complete |
| Watcher ClusterRole and bindings | [STORY_02_watcher_rbac.md](STORY_02_watcher_rbac.md) | Complete |
| Agent ClusterRole and bindings | [STORY_03_agent_rbac.md](STORY_03_agent_rbac.md) | Complete |
| Watcher Role and RoleBinding (own namespace) | [STORY_04_watcher_role.md](STORY_04_watcher_role.md) | Complete |
| Secret placeholder files | [STORY_05_secrets.md](STORY_05_secrets.md) | Complete |
| Watcher Deployment manifest | [STORY_06_deployment.md](STORY_06_deployment.md) | Complete |
| kustomization.yaml | [STORY_07_kustomization.md](STORY_07_kustomization.md) | Complete |
| Flux integration example | [STORY_08_flux.md](STORY_08_flux.md) | Complete |

## Technical Overview

The complete manifest set and RBAC design are specified in
[`docs/DESIGN/lld/DEPLOY_LLD.md`](../../DESIGN/lld/DEPLOY_LLD.md). Implement exactly
what is specified there — do not invent new resources or permissions without updating
the LLD first.

Security constraints:
- Watcher runs as non-root with `readOnlyRootFilesystem: true`
- Agent ClusterRole has no mutating verbs
- Secret manifests must never contain real values in git

## Definition of Done

- [ ] `--dry-run=client` passes against a live cluster
- [ ] All RBAC verified with `kubectl auth can-i` checks
- [ ] Secret placeholders clearly marked `REPLACE_ME`
- [ ] Kustomize renders without errors
