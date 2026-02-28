# Phase 4: RBAC Enforcement Testing

**Date run:** 2026-02-24
**Cluster:** yes (v0.3.9, default namespace)

All tests run against the live deployed ServiceAccounts.

---

## 4.1 Default Cluster Scope — Secret Read

**Status:** Executed

```
kubectl auth can-i get secret -n kube-system --as=system:serviceaccount:default:mechanic-agent
→ yes

kubectl get secret -n kube-system --as=system:serviceaccount:default:mechanic-agent
→ NAME ... bootstrap-token-m1akoo ... sh.helm.release.v1.cilium.v22 ... (SUCCESS)
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Agent can read Secret cluster-wide | yes (AR-01) | yes | PASS (confirmed accepted risk) |

---

## 4.2 Namespace Scope — Secret Read Restriction

**Status:** SKIPPED — `mechanic-agent-ns` ServiceAccount not deployed (default scope deployment)

The `role-agent-ns.yaml` and `rolebinding-agent-ns.yaml` templates exist in the chart for namespace scope but are only deployed when `watcher.agentRBACScope=namespace`. Current deployment uses default cluster scope.

---

## 4.3 Agent Write Restriction

**Status:** Executed

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Cannot create pods | no | no | PASS |
| Cannot create deployments | no | no | PASS |
| Cannot exec into pods (pods/exec create) | no | no | PASS |
| Cannot access nodes/proxy (get) | no | **yes** | **FAIL** — see finding P-004 |
| Cannot delete remediationjobs | no | no | PASS |
| Cannot update remediationjob spec | no | no | PASS |
| Cannot create remediationjobs | no | no | PASS |
| Cannot create batch/jobs | no | no | PASS |

**nodes/proxy escalation confirmed:**

```
kubectl get --raw "/api/v1/nodes/cp-00/proxy/metrics" --as=...mechanic-agent
→ # HELP aggregator_discovery_aggregation_count_total [ALPHA] ... (SUCCESS)

kubectl get --raw "/api/v1/nodes/cp-00/proxy/logs/" --as=...mechanic-agent
→ <listing: alternatives.log, containers/, etc.> (SUCCESS)
```

The agent can read node-level metrics and kubelet-exposed log listings via the API server proxy. The wildcard `resources: ["*"]` in the agent ClusterRole implicitly includes `nodes/proxy`.

---

## 4.4 Watcher Escalation Paths

**Status:** Executed

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Watcher cannot read Secrets | no | **yes** | **FAIL** — see finding P-005 |
| Watcher cannot create pods | no | no | PASS |
| Watcher cannot delete RemediationJobs in kube-system | no | yes | INFO (RemediationJob CRD does not exist in kube-system; delete would be a no-op in practice but permission exists) |
| Watcher cannot write ConfigMaps cluster-wide | no | no | PASS |
| Watcher cannot write ConfigMaps in own namespace | no | no | PASS (no configmap write in any ClusterRole or Role) |

```
kubectl get secret -n kube-system --as=...mechanic-watcher
→ NAME ... bootstrap-token-m1akoo ... (SUCCESS)
```

**Root cause:** The live deployed `mechanic-watcher` ClusterRole includes `"secrets"` in the resource list. The Helm chart source (`charts/mechanic/templates/clusterrole-watcher.yaml`) was remediated in the 2026-02-24 security review (finding 2026-02-24-002), but the cluster was never re-deployed with the fixed chart. The live RBAC state is the **pre-fix version**.

---

## 4.5 Agent Status Patch Verification

**Status:** Executed

`kubectl auth can-i patch remediationjobs/status -n default --as=...mechanic-agent` returns `no` — this is a known false negative with custom resource subresources in `kubectl auth can-i`. Actual impersonation test:

```
kubectl patch remediationjob mechanic-0cd2345e0966 -n default \
  --type=merge --subresource=status --patch='{"status":{"phase":"Running"}}' \
  --as=system:serviceaccount:default:mechanic-agent
→ remediationjob.remediation.mechanic.io/mechanic-0cd2345e0966 patched  (SUCCESS)

kubectl patch remediationjob mechanic-0cd2345e0966 -n default \
  --type=merge --patch='{"spec":{"maxRetries":99}}' \
  --as=system:serviceaccount:default:mechanic-agent
→ Forbidden  (PASS — spec write blocked)
```

**Result:** RBAC isolation of `remediationjobs/status` vs full spec is correctly enforced.

---

## Phase 4 Summary

**Total new findings:** 2 (P-004, P-005) — already documented in Phase 2
**Carry-over confirmed:** AR-01 (accepted)
**Notes:** P-004 (nodes/proxy) and P-005 (watcher secrets) are the two active HIGH findings requiring remediation.
