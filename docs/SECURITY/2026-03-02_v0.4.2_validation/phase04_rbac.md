# Phase 4: RBAC Enforcement Testing

**Date run:** 2026-03-02
**Reviewer:** automated (k8s-mechanic security process)
**Cluster:** yes — v1.33.2 (Talos, 6-node HA)
**mechanic version:** v0.4.2 (chart 0.4.2, namespace: default)

---

## 4.1 Default Cluster Scope — Secret Read

**Status:** PASS

```bash
kubectl create secret generic rbac-test-secret --from-literal=key=supersecretvalue -n default
kubectl auth can-i get secret/rbac-test-secret -n default \
  --as=system:serviceaccount:default:mechanic-agent
# → no

kubectl get secret rbac-test-secret -n default \
  --as=system:serviceaccount:default:mechanic-agent
# → Error from server (Forbidden): secrets "rbac-test-secret" is forbidden:
#   User "system:serviceaccount:default:mechanic-agent" cannot get resource
#   "secrets" in API group "" in the namespace "default"
```

**Result:** Agent SA cannot read Secrets. ClusterRole `mechanic-agent` does not
include `secrets` in any resource rule. This is a strengthening from the original
AR-01 acceptance (old v0.3.x ClusterRole granted secrets). AR-01 is no longer
applicable — secret read is denied at RBAC level.

---

## 4.2 Namespace Scope — Not applicable

The v0.4.2 chart does not deploy a namespace-scoped agent mode by default.
`agent.rbac.scope` defaults to `cluster`. Namespace-scope overlay test skipped —
no namespace-scoped ServiceAccount deployed.

---

## 4.3 Agent Write Restriction

**Status:** PASS (with one expected exception — see note)

```bash
kubectl auth can-i create pod -n default        --as=system:serviceaccount:default:mechanic-agent  → no  ✓
kubectl auth can-i create deployment -n default --as=system:serviceaccount:default:mechanic-agent  → no  ✓
kubectl auth can-i create configmap -n default  --as=system:serviceaccount:default:mechanic-agent  → yes (expected — see note)
kubectl auth can-i create pods/exec -n default  --as=system:serviceaccount:default:mechanic-agent  → no  ✓
kubectl auth can-i create nodes/proxy           --as=system:serviceaccount:default:mechanic-agent  → no  ✓

# nodes/proxy false-positive investigation:
kubectl auth can-i get nodes/proxy --as=system:serviceaccount:default:mechanic-agent
# → yes (WARNING: false positive from kubectl auth can-i)
kubectl get --raw /api/v1/nodes/cp-00/proxy/ --as=system:serviceaccount:default:mechanic-agent
# → Error from server (Forbidden): nodes "cp-00" is forbidden: User
#   "system:serviceaccount:default:mechanic-agent" cannot get resource
#   "nodes/proxy" in API group "" at the cluster scope
```

**ConfigMap create (expected):** `Role/mechanic-agent` in namespace `default`
explicitly grants `create` on `configmaps`. This is by design — the dry-run report
channel writes investigation reports to ConfigMaps when `watcher.dryRun: true`.
The permission is correctly namespace-scoped to `default` only:

```bash
kubectl auth can-i create configmap -n kube-system --as=system:serviceaccount:default:mechanic-agent → no  ✓
kubectl auth can-i create configmap -n monitoring   --as=system:serviceaccount:default:mechanic-agent → no  ✓
```

**`nodes/proxy` false positive:** `kubectl auth can-i` reports `yes` for
`get nodes/proxy` but the actual API call returns `Forbidden`. This is a known
`kubectl auth can-i` quirk for non-namespaced subresources in k8s 1.33. The
`mechanic-agent` ClusterRole grants `get/list/watch` on `nodes` but does NOT
include `nodes/proxy` as a subresource — confirmed by direct API test above.

**No new findings.**

---

## 4.4 Watcher Escalation Paths

**Status:** PASS with accepted residual risks (unchanged from prior reports)

```bash
kubectl auth can-i get secret -n default \
  --as=system:serviceaccount:default:mechanic-watcher
# → yes  (accepted — AR-08, see below)

kubectl auth can-i delete remediationjob -n kube-system \
  --as=system:serviceaccount:default:mechanic-watcher
# → yes  (ClusterRole scope — expected; RemediationJobs don't exist in kube-system)

kubectl auth can-i create pod -n default \
  --as=system:serviceaccount:default:mechanic-watcher
# → no  ✓
```

**Watcher secret read (AR-08):** The `mechanic-watcher` ClusterRole grants
`get/list/watch` on `secrets`. This is required for `watcher.prAutoClose: true`
to read the `github-app` Secret for GitHub App token exchange. Accepted residual
risk, unchanged from prior runs.

**Watcher remediationjob delete cluster-wide:** The watcher's ClusterRole grants
`delete` on `remediationjobs` across all namespaces. In practice RemediationJobs
only exist in the mechanic deployment namespace (`default`). This is broader than
strictly necessary but acceptable — the watcher needs delete to GC completed jobs
and mechanic's design is single-namespace. Logged as informational, not a finding.

**No new findings.**

---

## Phase 4 Summary

| Test | Result | Notes |
|------|--------|-------|
| 4.1 Agent secret read | **PASS** | Forbidden at RBAC level — AR-01 no longer applicable in v0.4.2 |
| 4.2 Namespace scope | SKIPPED | cluster scope only in default install |
| 4.3 Agent write restrictions | **PASS** | ConfigMap create expected + correctly namespace-scoped; nodes/proxy false-positive confirmed blocked by actual API test |
| 4.4 Watcher escalation | **PASS** | AR-08 accepted (watcher secret read); rjob delete cluster-wide informational |

**Total new findings:** 0
**Accepted residual risks re-confirmed:** AR-08 (watcher secret read)
