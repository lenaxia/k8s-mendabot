# Phase 4: RBAC Enforcement Testing

**Date run:**
**Reviewer:**
**Cluster:** yes / no — if no, mark all tests SKIPPED

---

## 4.1 Default Cluster Scope — Secret Read

**Status:** Executed / SKIPPED — reason: ______

```bash
kubectl auth can-i get secret/rbac-test-secret -n default \
  --as=system:serviceaccount:mendabot:mendabot-agent
```
```
<!-- paste output -->
```

```bash
kubectl get secret rbac-test-secret -n default \
  --as=system:serviceaccount:mendabot:mendabot-agent -o yaml
```
```
<!-- paste output -->
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Agent can read Secret (cluster scope) | yes | | |

**Note:** This result is EXPECTED to be "yes" — it is accepted residual risk AR-01.
Record it as a confirmed accepted risk, not a defect.

---

## 4.2 Namespace Scope — Secret Read Restriction

**Status:** Executed / SKIPPED — reason: ______

```bash
# Out-of-scope namespace — should be forbidden
kubectl auth can-i get secret -n production \
  --as=system:serviceaccount:mendabot:mendabot-agent-ns
```
```
<!-- paste output -->
```

```bash
# In-scope namespace — should be allowed
kubectl auth can-i get secret -n default \
  --as=system:serviceaccount:mendabot:mendabot-agent-ns
```
```
<!-- paste output -->
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Secret read blocked in out-of-scope namespace | no | | |
| Secret read allowed in in-scope namespace | yes | | |

---

## 4.3 Agent Write Restriction

**Status:** Executed / SKIPPED — reason: ______

```bash
kubectl auth can-i create pod -n default \
  --as=system:serviceaccount:mendabot:mendabot-agent
kubectl auth can-i create deployment -n default \
  --as=system:serviceaccount:mendabot:mendabot-agent
kubectl auth can-i create pods/exec -n default \
  --as=system:serviceaccount:mendabot:mendabot-agent
kubectl auth can-i get nodes/proxy \
  --as=system:serviceaccount:mendabot:mendabot-agent
```
```
<!-- paste output -->
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Cannot create pods | no | | |
| Cannot create deployments | no | | |
| Cannot exec into pods | no | | |
| Cannot access nodes/proxy | no | | |

---

## 4.4 Watcher Escalation Paths

**Status:** Executed / SKIPPED — reason: ______

```bash
kubectl auth can-i get secret -n default \
  --as=system:serviceaccount:mendabot:mendabot-watcher
kubectl auth can-i delete remediationjob -n kube-system \
  --as=system:serviceaccount:mendabot:mendabot-watcher
```
```
<!-- paste output -->
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| Watcher cannot read Secrets | no | | |
| Watcher cannot delete RemediationJobs outside mendabot ns | no | | |

---

## Phase 4 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
