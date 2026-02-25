# Phase 5: Network Egress Testing

**Date run:** 2026-02-24
**Cluster:** yes (v0.3.9, default namespace)
**CNI:** Cilium (running, confirmed)

---

## 5.1 CNI Prerequisite Check

**Status:** Executed

```
kubectl get pods -n kube-system | grep cilium
→ cilium-9hjrp   1/1 Running
  cilium-d4pcd   1/1 Running
  cilium-dcx9d   1/1 Running
  ... (6 nodes, all Running)
  cilium-operator-c6fb7b4bc-mzj6m  1/1 Running
```

**CNI found:** Cilium — NetworkPolicy enforcement available.

---

## 5.2 Security Overlay Deploy

**Status:** NOT executed — the security overlay (`deploy/overlays/security/`) was not applied to this cluster.

**Verification:**
```
kubectl get networkpolicies -A
→ No resources found
```

No NetworkPolicy objects exist in any namespace. The security overlay has never been applied to this deployment.

**Finding:** This is the documented **AR-03** (NetworkPolicy requires CNI operator action). Cilium is deployed and capable of enforcing NetworkPolicy, but no NetworkPolicy has been applied to restrict agent egress. This is the same state as the 2026-02-23 and 2026-02-24 reviews.

**New finding:** 2026-02-24-P-009 (MEDIUM) — Cilium is present and capable but no NetworkPolicy applied; agent egress to all destinations is unrestricted.

---

## 5.3 Egress Restriction Tests

**Status:** SKIPPED — no running agent pod available at time of testing (all jobs Completed and pods cannot be exec'd). No NetworkPolicy is deployed, so all tests would fail in the expected failure mode (all egress permitted).

**Analysis without live test:**
Since no NetworkPolicy is present, the following would be observed if tests were run:
- Test 1 (DNS): PASS (allowed — no policy to block it)
- Test 2 (GitHub API 443): PASS (allowed)
- Test 3 (Arbitrary external): FAIL — would CONNECT (no egress restriction)
- Test 4 (Kubernetes API): PASS
- Test 5 (Internal cluster service): FAIL — would CONNECT (no egress restriction)

---

## Phase 5 Summary

| Test | Result |
|------|--------|
| CNI check | Cilium present — PASS |
| NetworkPolicy deployed | No — FAIL |
| DNS | SKIPPED |
| GitHub API | SKIPPED |
| Arbitrary external endpoint | SKIPPED (expected FAIL — no policy) |
| Kubernetes API | SKIPPED |
| Non-API cluster service | SKIPPED (expected FAIL — no policy) |

**Total findings:** 1 (P-009)
**Findings added to findings.md:** 2026-02-24-P-009
