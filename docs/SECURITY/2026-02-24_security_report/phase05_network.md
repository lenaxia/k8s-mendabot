# Phase 5: Network Egress Testing

**Date run:**
**Reviewer:**
**CNI available:** yes / no — if no, mark all live tests SKIPPED

---

## 5.1 CNI Prerequisite Check

**Status:** Executed / SKIPPED — reason: ______

```bash
kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.containerRuntimeVersion}'
kubectl get pods -n kube-system | grep -E '(cilium|calico|canal)'
kubectl get networkpolicies -A
```
```
<!-- paste output -->
```

**CNI found:** cilium / calico / canal / none / other: ______

---

## 5.2 Security Overlay Deploy

**Status:** Executed / SKIPPED — reason: ______

```bash
kubectl apply -k deploy/overlays/security/ --dry-run=client
```
```
<!-- paste output — should show all resources including NetworkPolicy -->
```

```bash
kubectl apply -k deploy/overlays/security/
kubectl get networkpolicies -n mendabot
```
```
<!-- paste output -->
```

---

## 5.3 Egress Restriction Tests

**Agent pod used:**
```bash
AGENT_POD=$(kubectl get pod -n mendabot -l app.kubernetes.io/managed-by=mendabot-watcher \
  -o jsonpath='{.items[0].metadata.name}')
echo $AGENT_POD
```
```
<!-- paste pod name -->
```

### Test 1: DNS resolution (should succeed)

**Status:** Executed / SKIPPED

```bash
kubectl exec -n mendabot "$AGENT_POD" -- nslookup github.com
```
```
<!-- paste output -->
```

**Result:** PASS (resolved) / FAIL (failed to resolve)

---

### Test 2: GitHub API port 443 (should succeed)

**Status:** Executed / SKIPPED

```bash
kubectl exec -n mendabot "$AGENT_POD" -- \
  curl -sS --max-time 10 https://api.github.com/zen
```
```
<!-- paste output -->
```

**Result:** PASS (returned response) / FAIL

---

### Test 3: Arbitrary external endpoint (should be blocked)

**Status:** Executed / SKIPPED

```bash
kubectl exec -n mendabot "$AGENT_POD" -- \
  curl -sS --max-time 5 https://example.com
```
```
<!-- paste output — should time out or be reset -->
```

**Result:** PASS (blocked/timed out) / FAIL (connected)

---

### Test 4: Kubernetes API server (should succeed)

**Status:** Executed / SKIPPED

```bash
kubectl exec -n mendabot "$AGENT_POD" -- \
  curl -sS --max-time 5 -k https://kubernetes.default.svc.cluster.local/api
```
```
<!-- paste output -->
```

**Result:** PASS (received API response) / FAIL

---

### Test 5: Non-API-server internal cluster service (should be blocked)

**Status:** Executed / SKIPPED

```bash
# Deploy test server
kubectl run test-server -n default --image=hashicorp/http-echo -- -text=hello
kubectl expose pod test-server -n default --port=5678
TEST_SERVER_IP=$(kubectl get svc test-server -n default -o jsonpath='{.spec.clusterIP}')
kubectl exec -n mendabot "$AGENT_POD" -- \
  curl -sS --max-time 5 "http://$TEST_SERVER_IP:5678"
```
```
<!-- paste output — should time out -->
```

**Result:** PASS (blocked) / FAIL (connected)

---

## Phase 5 Summary

| Test | Result |
|------|--------|
| DNS resolution | PASS / FAIL / SKIPPED |
| GitHub API (443) | PASS / FAIL / SKIPPED |
| Arbitrary external endpoint | PASS / FAIL / SKIPPED |
| Kubernetes API server | PASS / FAIL / SKIPPED |
| Non-API cluster service | PASS / FAIL / SKIPPED |

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
