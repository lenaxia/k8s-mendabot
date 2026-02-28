# Story 04: Agent RBAC Scoping by Namespace

**Epic:** [epic12-security-review](README.md)
**Priority:** Medium
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **security-conscious operator**, I want to restrict the agent's Kubernetes access
to specific namespaces rather than cluster-wide, so that a manipulated agent cannot read
Secrets or resources in unrelated namespaces.

---

## Background

The current agent ClusterRole in `deploy/kustomize/clusterrole-agent.yaml` is:

```yaml
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
```

This includes Secrets in every namespace — a conscious accepted risk per HLD §11.
The HLD notes: "Operators who consider this unacceptable must restrict the ClusterRole
explicitly."

This story implements that restriction as a first-class feature. The mechanism:

1. A new env var `AGENT_RBAC_SCOPE` on the watcher Deployment (`cluster` | `namespace`)
2. When `namespace`, `config.FromEnv()` also reads `AGENT_WATCH_NAMESPACES` (comma-separated)
3. `JobBuilder.Build()` selects a different ServiceAccount based on scope:
   - `cluster`: `rjob.Spec.AgentSA` (current behaviour — `mechanic-agent`)
   - `namespace`: `mechanic-agent-ns` (a new SA bound to namespace-scoped Roles)
4. New namespace-scoped RBAC manifests in the deploy overlay

The `rjob.Spec.AgentSA` field is already set by `SourceProviderReconciler.Reconcile()`
from `r.Cfg.AgentSA` (line 313). This story adds a second SA name to `Config` and teaches
the reconciler which to use.

---

## Acceptance Criteria

- [ ] `config.Config` gains `AgentRBACScope string` and `AgentWatchNamespaces []string`
- [ ] `config.FromEnv()` parses `AGENT_RBAC_SCOPE` (default `"cluster"`) and
      `AGENT_WATCH_NAMESPACES` (required when scope is `"namespace"`)
- [ ] `config.FromEnv()` returns an error if `AGENT_RBAC_SCOPE=namespace` but
      `AGENT_WATCH_NAMESPACES` is empty
- [ ] `config.FromEnv()` returns an error if `AGENT_RBAC_SCOPE` is not `"cluster"` or
      `"namespace"`
- [ ] `SourceProviderReconciler.Reconcile()` sets `AgentSA` based on scope:
      `mechanic-agent` for cluster scope, `mechanic-agent-ns` for namespace scope
- [ ] `deploy/kustomize/overlays/security/` contains a `ServiceAccount`, `Role` (per
      watched namespace), and `RoleBinding` for `mechanic-agent-ns`
- [ ] `config_test.go` covers: valid cluster scope, valid namespace scope with namespaces,
      namespace scope with empty namespaces (error), invalid scope value (error)
- [ ] `go test -timeout 30s -race ./internal/config/...` passes

---

## Technical Implementation

### Changes to `internal/config/config.go`

Add two new fields to `Config`:

```go
type Config struct {
    // ... existing fields ...
    AgentRBACScope       string   // AGENT_RBAC_SCOPE — "cluster" (default) or "namespace"
    AgentWatchNamespaces []string // AGENT_WATCH_NAMESPACES — required when scope is "namespace"
}
```

Add parsing in `FromEnv()`:

```go
scope := os.Getenv("AGENT_RBAC_SCOPE")
if scope == "" {
    scope = "cluster"
}
if scope != "cluster" && scope != "namespace" {
    return Config{}, fmt.Errorf("AGENT_RBAC_SCOPE must be 'cluster' or 'namespace', got %q", scope)
}
cfg.AgentRBACScope = scope

if scope == "namespace" {
    nsStr := os.Getenv("AGENT_WATCH_NAMESPACES")
    if nsStr == "" {
        return Config{}, fmt.Errorf("AGENT_WATCH_NAMESPACES is required when AGENT_RBAC_SCOPE=namespace")
    }
    for _, ns := range strings.Split(nsStr, ",") {
        ns = strings.TrimSpace(ns)
        if ns != "" {
            cfg.AgentWatchNamespaces = append(cfg.AgentWatchNamespaces, ns)
        }
    }
    if len(cfg.AgentWatchNamespaces) == 0 {
        return Config{}, fmt.Errorf("AGENT_WATCH_NAMESPACES is empty after parsing")
    }
}
```

### Changes to `SourceProviderReconciler.Reconcile()`

In `internal/provider/provider.go`, where `AgentSA` is set on the `RemediationJob` spec
(currently line 313):

```go
agentSA := r.Cfg.AgentSA  // default: "mechanic-agent" (cluster scope)
if r.Cfg.AgentRBACScope == "namespace" {
    agentSA = "mechanic-agent-ns"
}
// ...
AgentSA: agentSA,
```

### New RBAC manifests (in `deploy/kustomize/overlays/security/`)

`serviceaccount-agent-ns.yaml`:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mechanic-agent-ns
  namespace: mechanic
```

`role-agent-ns.yaml` — one Role per watched namespace (example for `production`):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mechanic-agent-ns
  namespace: production
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
```

`rolebinding-agent-ns.yaml`:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mechanic-agent-ns
  namespace: production
subjects:
- kind: ServiceAccount
  name: mechanic-agent-ns
  namespace: mechanic
roleRef:
  kind: Role
  name: mechanic-agent-ns
  apiGroup: rbac.authorization.k8s.io
```

The agent also needs the existing `role-agent.yaml` (`remediationjobs/status` patch)
bound to `mechanic-agent-ns`:

`rolebinding-agent-ns-statuswrite.yaml`:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mechanic-agent-ns-statuswrite
  namespace: mechanic
subjects:
- kind: ServiceAccount
  name: mechanic-agent-ns
  namespace: mechanic
roleRef:
  kind: Role
  name: mechanic-agent
  apiGroup: rbac.authorization.k8s.io
```

**Note on Nodes:** Nodes are cluster-scoped. A namespace-scoped agent cannot read
`kubectl describe node`. The agent prompt should note this when `AGENT_RBAC_SCOPE=namespace`
is active. This is a known limitation that the operator accepts when choosing namespace scope.

---

## Tasks

- [ ] Write `config_test.go` cases for new fields (TDD)
- [ ] Update `internal/config/config.go` with `AgentRBACScope` and `AgentWatchNamespaces`
- [ ] Run config tests — must pass
- [ ] Update `internal/provider/provider.go` to select `mechanic-agent-ns` when
      `AgentRBACScope == "namespace"`
- [ ] Create namespace-scoped RBAC manifests in `deploy/kustomize/overlays/security/`
- [ ] Run `kubectl apply -k deploy/kustomize/overlays/security/ --dry-run=client`
- [ ] Run `go test -timeout 30s -race ./...`

---

## Dependencies

**Depends on:** epic04-deploy (base RBAC manifests), epic01-controller
(`SourceProviderReconciler` sets `AgentSA`)
**Blocks:** STORY_06 (pentest)

---

## Definition of Done

- [ ] `go test -timeout 30s -race ./internal/config/...` passes
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] `kubectl apply -k deploy/kustomize/overlays/security/ --dry-run=client` passes
- [ ] `AGENT_RBAC_SCOPE=namespace` with empty `AGENT_WATCH_NAMESPACES` returns an error
      from `config.FromEnv()`
- [ ] `AGENT_RBAC_SCOPE=badvalue` returns an error from `config.FromEnv()`
