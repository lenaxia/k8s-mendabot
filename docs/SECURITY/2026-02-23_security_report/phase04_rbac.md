# Phase 4: RBAC Enforcement Testing

**Date run:** 2026-02-23
**Reviewer:** OpenCode (automated review)
**Cluster:** no — all live tests SKIPPED; code review substituted where possible

---

## 4.1 Default Cluster Scope — Secret Read

**Status:** SKIPPED — reason: no cluster available

Code review evidence (from phase02_architecture.md §2.2):

```yaml
# deploy/kustomize/clusterrole-agent.yaml
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Agent can read Secret (cluster scope) | yes | yes (by code review — `resources: ["*"]` + `get`) | PASS (expected — AR-01) |

**Note:** This result is EXPECTED to be "yes" — it is accepted residual risk AR-01. The ClusterRole grants read-all-resources access cluster-wide, including Secrets. Live test deferred to next review with a cluster.

---

## 4.2 Namespace Scope — Secret Read Restriction

**Status:** SKIPPED — reason: no cluster available; namespace-scoped Role reviewed in code

Code review: `deploy/overlays/security/role-agent-ns.yaml` reviewed. The namespace-scoped Role binds only to specified namespaces. The watcher sets `AGENT_RBAC_SCOPE=namespace` to use this path. The ClusterRole is NOT supplemented — the deployment guide instructs operators to deploy either the ClusterRole OR the namespace-scoped Role, not both.

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Secret read blocked in out-of-scope namespace | no | not live-tested | SKIPPED |
| Secret read allowed in in-scope namespace | yes | not live-tested | SKIPPED |

---

## 4.3 Agent Write Restriction

**Status:** SKIPPED — reason: no cluster available

Code review evidence:

```yaml
# ClusterRole mechanic-agent
verbs: ["get", "list", "watch"]
```

No `create`, `update`, `patch`, or `delete` verbs are present. `pods/exec` and `nodes/proxy` are not listed as explicit resources with escalation verbs.

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Cannot create pods | no | no (by code review — only read verbs) | PASS |
| Cannot create deployments | no | no (by code review) | PASS |
| Cannot exec into pods | no | no (by code review — `create` on `pods/exec` not granted) | PASS |
| Cannot access nodes/proxy write | no | no (only `get` available, which is standard investigation access) | PASS |

---

## 4.4 Watcher Escalation Paths

**Status:** SKIPPED — reason: no cluster available

Code review evidence (from phase02_architecture.md §2.2):

- Watcher ClusterRole does NOT include `secrets` in the resource list — watcher cannot read Secrets
- `configmaps` write is granted at ClusterRole scope — this is finding 2026-02-23-005

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Watcher cannot read Secrets | no | no (by code review — Secrets not listed in watcher ClusterRole) | PASS |
| Watcher cannot delete RemediationJobs outside mechanic ns | no | not live-tested — `delete` is on CRD which exists in mechanic ns only | SKIPPED |

---

## Phase 4 Summary

**Total findings:** 0 (finding 2026-02-23-005 from §2.2 already recorded in Phase 2)
**Findings added to findings.md:** none new — see 2026-02-23-005 (watcher ConfigMap cluster-wide write, recorded in Phase 2)
**Tests skipped:** 4.1 live test, 4.2 full, 4.4 remote-namespace delete test — defer to next cluster-available review
