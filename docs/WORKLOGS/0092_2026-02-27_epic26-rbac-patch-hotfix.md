# Worklog 0092 — Epic 26 RBAC Hotfix: Agent Missing `patch` on remediationjobs/status

**Date:** 2026-02-27
**Branch:** `main` (direct hotfix — single-file change, low risk)
**Version:** v0.3.25 → v0.3.26

## Summary

Epic 26 (auto-close resolved findings) shipped in v0.3.25 but never fired in
production. Root cause: the agent ClusterRole was missing `patch` on the
`remediationjobs/status` subresource. The agent's STEP 9 (`kubectl patch
--subresource=status`) was rejected with 403 on every rjob. Since
`status.sinkRef.URL` was always empty, `autoCloseSucceededSinks` skipped every
rjob silently, and no PR was ever auto-closed.

## Root Cause

Worklog 0091, STORY_05 stated:

> *"Agent RBAC already had `remediationjobs/status` with `patch` verb
> (verified in `charts/mechanic/templates/role-agent.yaml`)"*

This was incorrect in two ways:

1. **Wrong file checked.** `role-agent.yaml` is the namespace-scoped Role used
   only when `agentRBACScope=namespace`. The default scope is `cluster`, which
   uses `clusterrole-agent.yaml`. That file was **not** checked.

2. **ClusterRole rule was read-only.** `clusterrole-agent.yaml` had a single
   rule covering both `remediationjobs` and `remediationjobs/status` with only
   `["get", "list", "watch"]`. The `patch` verb was absent.

The `|| echo "WARNING: failed to patch sinkRef"` guard in the agent prompt
swallowed the 403 without surfacing it in the rjob status or watcher logs, so
no alert fired.

## Fix

**File:** `charts/mechanic/templates/clusterrole-agent.yaml`

Split the single combined rule into two separate rules:

```yaml
# Before (broken):
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs", "remediationjobs/status"]
  verbs: ["get", "list", "watch"]

# After (fixed):
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch"]
```

`role-agent.yaml` (namespace scope) was already correct — it already had
`["get", "patch"]` on `remediationjobs/status`.

## Regression Test

Added `internal/charts/rbac_test.go` — a pure-Go test package that parses the
Helm template YAML files directly (no Helm binary, no cluster needed). It:

- Strips Helm template directives (`{{- if ... }}`, `{{- end }}`) and inline
  expressions (`{{ include "..." }}`) with two regex passes before parsing
- Asserts the ClusterRole grants `patch` and `get` on `remediationjobs/status`
- Asserts the ClusterRole grants `get`, `list`, `watch` on `remediationjobs`
- Asserts the namespace-scoped Role grants `patch` and `get` on `remediationjobs/status`
- Asserts exactly one rule covers `remediationjobs/status` (hygiene check)

All 6 tests pass. This test now runs as part of `go test ./...` in CI.

## Lessons Learned

1. **Always check both RBAC files.** When `agentRBACScope` can be `cluster`
   (default) or `namespace`, any RBAC change must be applied to both
   `clusterrole-agent.yaml` **and** `role-agent.yaml`.

2. **Verify with `kubectl auth can-i`.** Before marking an RBAC story done,
   run `kubectl auth can-i patch remediationjobs/status --as=system:serviceaccount:mechanic:mechanic-agent`
   in a real cluster. Static file review is not sufficient.

3. **Swallowed errors hide RBAC failures.** The `|| echo "WARNING:"` pattern
   in shell scripts inside AI-generated prompts is convenient for resilience but
   must be paired with a way to surface the failure (e.g. annotate the rjob,
   log a structured event). A 403 that surfaces only as a shell echo is
   effectively invisible to the operator.

4. **Test Helm RBAC templates directly.** Unit tests only verify behaviour when
   the data is already populated correctly. A chart-level test that parses YAML
   would have caught this before the release.

## Commits

- `c026cb9` fix(rbac): grant agent patch on remediationjobs/status in ClusterRole
- `92ceaeb` test(charts): add RBAC regression tests for agent ClusterRole/Role
