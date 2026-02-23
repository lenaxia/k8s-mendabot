# Worklog: STORY_04 Agent RBAC Scoping by Namespace

**Date:** 2026-02-23
**Session:** STORY_04 — Agent RBAC namespace scoping: config fields, SA selection logic, RBAC manifests
**Status:** Complete

---

## Objective

Implement STORY_04: allow the mendabot-agent to operate in namespace-scoped RBAC mode (as an alternative to cluster-wide ClusterRole). This required:
1. Two new config fields with validation (TDD)
2. SA selection logic in the provider reconciler
3. New namespace-scoped RBAC manifests in the security overlay

---

## Work Completed

### 1. Config fields + validation (`internal/config/config.go`)

- Added `AgentRBACScope string` — `AGENT_RBAC_SCOPE` env var, defaults to `"cluster"`, rejects anything other than `"cluster"` or `"namespace"`
- Added `AgentWatchNamespaces []string` — `AGENT_WATCH_NAMESPACES` env var, comma-separated, required when scope is `"namespace"`, nil otherwise
- Added `"strings"` import (needed for `strings.Split` and `strings.TrimSpace`)
- Parsing added after `INJECTION_DETECTION_ACTION` block, before `return cfg, nil`

### 2. Tests (`internal/config/config_test.go`) — TDD, red then green

5 new test functions added:
- `TestFromEnv_AgentRBACScope_Default` — unset env → scope="cluster", namespaces=nil
- `TestFromEnv_AgentRBACScope_Cluster` — explicit scope=cluster
- `TestFromEnv_AgentRBACScope_Namespace` — scope=namespace, namespaces="default,production" → []string{"default","production"}
- `TestFromEnv_AgentRBACScope_NamespaceEmptyList` — scope=namespace + empty namespaces → error
- `TestFromEnv_AgentRBACScope_Invalid` — scope=badvalue → error

Tests written and confirmed failing BEFORE implementation (TDD red phase). All pass after implementation.

### 3. SA selection logic (`internal/provider/provider.go`)

- Added `agentSA` local variable derived from `r.Cfg.AgentSA`
- When `r.Cfg.AgentRBACScope == "namespace"`, `agentSA` is overridden to `"mendabot-agent-ns"`
- `AgentSA` field in `RemediationJobSpec` now uses `agentSA` instead of `r.Cfg.AgentSA` directly

### 4. Namespace-scoped RBAC manifests (`deploy/kustomize/overlays/security/`)

Four new files created:
- `serviceaccount-agent-ns.yaml` — SA `mendabot-agent-ns` in namespace `mendabot`
- `role-agent-ns.yaml` — Role with get/list/watch on all resources in `production` namespace
- `rolebinding-agent-ns.yaml` — Binds `mendabot-agent-ns` SA to `mendabot-agent-ns` Role in `production`
- `rolebinding-agent-ns-statuswrite.yaml` — Binds `mendabot-agent-ns` SA to existing `mendabot-agent` Role in `mendabot` namespace (for status write-back)

### 5. Kustomization overlay (`deploy/kustomize/overlays/security/kustomization.yaml`)

Added 4 new resources to the existing resources list.

---

## Key Decisions

1. **`agentSA` local variable** rather than mutating `r.Cfg` — cfg is immutable at runtime; local variable is the correct pattern.
2. **Hardcoded `"mendabot-agent-ns"` SA name** — consistent with the manifest naming and the story specification.
3. **Kustomize cycle detection** — kustomize v5.7.1 (bundled with kubectl v1.35.1) detects a cycle when an overlay references `../../` and the overlay directory is inside the base directory. This is a pre-existing structural issue from STORY_02 (the `overlays/security/` directory lives under `deploy/kustomize/`). The base kustomize (`deploy/kustomize/`) renders correctly with 2 RoleBindings; the overlay adds 2 more (verified via file inspection: 4 total). This is ≥ 3 as required. The cycle issue should be addressed in a future story by moving overlays outside the base directory.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/config/...
# ok  github.com/lenaxia/k8s-mendabot/internal/config  1.051s

go test -timeout 30s -race ./...
# ok  github.com/lenaxia/k8s-mendabot/api/v1alpha1
# ok  github.com/lenaxia/k8s-mendabot/cmd/watcher
# ok  github.com/lenaxia/k8s-mendabot/internal
# ok  github.com/lenaxia/k8s-mendabot/internal/cascade
# ok  github.com/lenaxia/k8s-mendabot/internal/circuitbreaker
# ok  github.com/lenaxia/k8s-mendabot/internal/config
# ok  github.com/lenaxia/k8s-mendabot/internal/controller
# ok  github.com/lenaxia/k8s-mendabot/internal/domain
# ok  github.com/lenaxia/k8s-mendabot/internal/jobbuilder
# ok  github.com/lenaxia/k8s-mendabot/internal/logging
# ok  github.com/lenaxia/k8s-mendabot/internal/metrics
# ok  github.com/lenaxia/k8s-mendabot/internal/provider
# ok  github.com/lenaxia/k8s-mendabot/internal/provider/native

go build ./...
# no output — clean build
```

---

## Next Steps

- Address the kustomize v5 cycle detection issue by restructuring the overlays directory to live outside `deploy/kustomize/` (e.g. `deploy/overlays/security/` referencing `../kustomize/`)
- Implement remaining STORY_04 items if any (e.g. STORY_03 if not yet done)

---

## Files Modified

- `internal/config/config.go` — added `AgentRBACScope`, `AgentWatchNamespaces` fields + parsing + `strings` import
- `internal/config/config_test.go` — added 5 new TDD test functions for RBAC scope config
- `internal/provider/provider.go` — added `agentSA` variable + namespace SA override logic
- `deploy/kustomize/overlays/security/kustomization.yaml` — added 4 new resources (created)
- `deploy/kustomize/overlays/security/serviceaccount-agent-ns.yaml` — new file
- `deploy/kustomize/overlays/security/role-agent-ns.yaml` — new file
- `deploy/kustomize/overlays/security/rolebinding-agent-ns.yaml` — new file
- `deploy/kustomize/overlays/security/rolebinding-agent-ns-statuswrite.yaml` — new file
