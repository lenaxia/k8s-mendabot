# Worklog: Dry-Run ConfigMap Channel — Hardening Audit

**Date:** 2026-02-26
**Session:** Post-merge audit and hardening of the ConfigMap-based dry-run report channel
**Branch:** `fix/dryrun-configmap-gaps` → merged to `main` as PR #13
**Tag:** `v0.3.20`
**Status:** Complete

---

## Objective

Audit and harden the dry-run report ConfigMap channel introduced in the previous
session (epic20, worklog-0089). A skeptical second-pass audit was requested before
merging. Two sessions were required; this worklog covers the second session.

---

## Context

The previous session replaced log-tail scraping with a Kubernetes ConfigMap channel:
the agent writes `mendabot-dryrun-<fp12>` at the end of a dry-run job; the controller
reads it via `APIReader` (bypassing the informer cache) and stores the content in
`rjob.status.message`. Several issues were fixed in the first audit pass. A second
audit was requested and its findings were partially captured before the session ended.

---

## Audit Findings (Second Pass)

### FINDING-1 — Entrypoints use Layer 3 only for fork/exec decision (Medium)

**Location:** `docker/scripts/entrypoint-opencode.sh:25`,
`docker/scripts/entrypoint-claude.sh:23`

**Problem:** Both entrypoints used `[ "${DRY_RUN:-false}" = "true" ]` (env var,
Layer 3) to decide whether to `fork` (call `emit_dry_run_report` after agent exits)
or `exec` (replace the shell). If a subprocess ran `unset DRY_RUN` before the agent
returned to the entrypoint, the entrypoint would take the `exec` branch, replace the
shell process, and `emit_dry_run_report` would never execute — the report would be
silently and permanently lost. The `gh` and `git` wrappers correctly check all three
layers; the entrypoints were inconsistent.

**Fix:** Both entrypoints now use the same three-layer check as the wrappers:
1. `/mendabot-cfg/dry-run` sentinel file (tamper-proof read-only emptyDir mount)
2. `/proc/1/environ` (PID-1 env, immutable inside the container)
3. `$DRY_RUN` env var (fallback / local testing)

Any layer returning `true` triggers the fork path and `emit_dry_run_report`.

### FINDING-2 — `clusterrole-agent.yaml` grants cluster-wide read on `configmaps` and `secrets` (Medium)

**Location:** `charts/mendabot/templates/clusterrole-agent.yaml`

**Problem:** The agent SA had a ClusterRole (cluster-wide via ClusterRoleBinding)
granting `get/list/watch` on `configmaps` and `secrets`. The dry-run report channel
requires the agent to `create` a ConfigMap in its own namespace — this is correctly
handled by the namespaced `role-agent.yaml`. The cluster-wide read of ConfigMaps and
Secrets is unnecessary and violates the principle of least privilege. In particular,
`secrets: get/list/watch` cluster-wide means the agent can enumerate credentials,
TLS certificates, and other sensitive secrets in any namespace.

**Fix:** Removed `configmaps` and `secrets` from the core resources in
`clusterrole-agent.yaml`. Workloads in watched namespaces that need secret reads for
diagnosis are covered by the per-namespace `role-agent-ns.yaml` (which grants
`get/list/watch` on all resources in explicitly-configured namespaces).

---

## Files Changed

| File | Change |
|------|--------|
| `docker/scripts/entrypoint-opencode.sh` | Three-layer `_entrypoint_dry_run` check replaces single `$DRY_RUN` check |
| `docker/scripts/entrypoint-claude.sh` | Same three-layer check applied to the stub entrypoint |
| `charts/mendabot/templates/clusterrole-agent.yaml` | Removed `configmaps` and `secrets` from cluster-wide read rule |

---

## First-Pass Audit Fixes (carried over from previous session, included in this PR)

These fixes were committed in the same branch but documented here for completeness:

| Issue | Fix |
|-------|-----|
| CRIT-1: cache race — `r.Client` returns NotFound before informer syncs new CM | Added `APIReader client.Reader` field; `fetchDryRunReport` uses direct API server read |
| CRIT-2: `pods/log` not removed from all RBAC files | Removed from `role-watcher.yaml` and `clusterrole-agent.yaml` |
| CRIT-3: `configmaps:get,delete` added to ClusterRole (cluster-wide) | Removed from ClusterRole; added new namespaced `role-watcher-dryrun.yaml` scoped to `Release.Namespace` |
| GAP-1: `AGENT_NAMESPACE` not asserted in jobbuilder tests | Added value assertion to `TestBuild_EnvVars_AllPresent` |
| GAP-2: Dryrun reconciler test passing trivially for wrong reason | Replaced with `TestReconcile_DryRunSucceeded_WrongNamespaceCMNotFound` |
| GAP-3: `dryRunCMName` duplicated between production and test | Exported as `DryRunCMName`; added boundary tests |

---

## Test Results

```
go build ./... && go test -timeout 30s -race ./...
ok  github.com/lenaxia/k8s-mendabot/api/v1alpha1
ok  github.com/lenaxia/k8s-mendabot/cmd/redact
ok  github.com/lenaxia/k8s-mendabot/cmd/watcher
ok  github.com/lenaxia/k8s-mendabot/internal/circuitbreaker
ok  github.com/lenaxia/k8s-mendabot/internal/config
ok  github.com/lenaxia/k8s-mendabot/internal/controller
ok  github.com/lenaxia/k8s-mendabot/internal/domain
ok  github.com/lenaxia/k8s-mendabot/internal/jobbuilder
ok  github.com/lenaxia/k8s-mendabot/internal/logging
ok  github.com/lenaxia/k8s-mendabot/internal/provider
ok  github.com/lenaxia/k8s-mendabot/internal/provider/native
ok  github.com/lenaxia/k8s-mendabot/internal/readiness
ok  github.com/lenaxia/k8s-mendabot/internal/readiness/llm
ok  github.com/lenaxia/k8s-mendabot/internal/readiness/sink
```

---

## Deployment Notes

- Tag `v0.3.19` already existed (pointed to the original `feat(dryrun)` commit).
  This release is tagged `v0.3.20`.
- CI will build and push `ghcr.io/lenaxia/mendabot-agent:v0.3.20` on tag push.
- After CI completes:
  ```
  helm upgrade mendabot charts/mendabot --reuse-values
  ```
- Validate: after a dry-run job completes, check:
  ```
  kubectl get remediationjob <name> -o jsonpath='{.status.message}'
  ```
  Expected: investigation report + `=== PROPOSED PATCH ===` section.
