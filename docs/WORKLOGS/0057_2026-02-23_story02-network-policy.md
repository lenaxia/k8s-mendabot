# Worklog: STORY_02 Network Policy for Agent Jobs

**Date:** 2026-02-23
**Session:** Epic 12 STORY_02 — NetworkPolicy egress restriction for agent Jobs, security overlay
**Status:** Complete

---

## Objective

Implement STORY_02 from epic12-security-review: create a Kustomize `NetworkPolicy` manifest
restricting egress on agent Job Pods to DNS (53 UDP/TCP), the Kubernetes API server (6443 TCP),
and HTTPS (443 TCP). The policy must live in an opt-in overlay, not in the base kustomization.

---

## Work Completed

### 1. NetworkPolicy manifest

Created `deploy/kustomize/network-policy-agent.yaml`:
- Kind: `NetworkPolicy`, apiVersion: `networking.k8s.io/v1`
- Selector: `app.kubernetes.io/managed-by: mechanic-watcher` (the label applied by
  `JobBuilder.Build()` at `internal/jobbuilder/job.go:229`)
- `policyTypes: [Egress]` only — no ingress restriction
- Three egress rules: DNS (53 UDP+TCP), kube-apiserver (6443 TCP), HTTPS (443 TCP)
- In-line comments explain CNI requirement and operator customisation for non-standard
  API server ports or known LLM CIDR ranges

### 2. Security overlay

Created `deploy/overlays/security/` directory and:
- `deploy/overlays/security/kustomization.yaml` — references `../../kustomize/` as base
  and `network-policy-agent.yaml` (local copy) as the additional resource
- `deploy/overlays/security/network-policy-agent.yaml` — copy of the policy file

**Key deviation from story specification:** The story specified
`deploy/kustomize/overlays/security/` as the overlay path. Kustomize v5 (v5.7.1, shipped
with kubectl v1.35.1) enforces `LoadRestrictionsRootOnly` by default. An overlay that is
a subdirectory of its own base triggers cycle detection and fails unconditionally:

```
cycle detected: candidate root '…/deploy/kustomize' contains visited root
'…/deploy/kustomize/overlays/security'
```

The canonical Kustomize fix is to place overlays as siblings of the base, not children.
The overlay was placed at `deploy/overlays/security/` (alongside `deploy/kustomize/`),
which is the documented Kustomize best practice and passes all verifications.

### 3. Story and backlog updated

- `docs/BACKLOG/epic12-security-review/STORY_02_network_policy.md` — status changed to
  Complete; checklist items marked; path deviation documented in acceptance criteria notes

---

## Key Decisions

1. **Overlay outside base directory:** Kustomize v5 `LoadRestrictionsRootOnly` makes
   subdirectory overlays invalid. `deploy/overlays/security/` (sibling to `deploy/kustomize/`)
   is the correct structure. The story's originally specified path was architecturally
   incompatible with Kustomize v5.

2. **NetworkPolicy file copied into overlay:** Kustomize v5 also prohibits loading files
   that are outside the overlay root (`../../file.yaml` style). The policy file was copied
   into `deploy/overlays/security/` so the overlay is self-contained. The canonical file
   in `deploy/kustomize/network-policy-agent.yaml` remains the source of truth.

3. **No namespace.yaml in base kustomization:** The actual `kustomization.yaml` does not
   list `namespace.yaml` (it was removed in a prior session; the DEPLOY_LLD shows a stale
   reference). This was not modified — staying within story scope.

---

## Blockers

None.

---

## Tests Run

```
kubectl kustomize deploy/kustomize/ | grep -c "kind: NetworkPolicy"
→ 0  (policy absent from base — correct)

kubectl kustomize deploy/overlays/security/ | grep "kind: NetworkPolicy"
→ kind: NetworkPolicy  (policy present in overlay — correct)

go test -timeout 30s -race ./...
→ ok  github.com/lenaxia/k8s-mechanic/api/v1alpha1         (cached)
→ ok  github.com/lenaxia/k8s-mechanic/cmd/watcher          (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal             (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/cascade     (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/circuitbreaker (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/config      (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/controller  9.978s
→ ok  github.com/lenaxia/k8s-mechanic/internal/domain      (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/jobbuilder  (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/logging     (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/metrics     (cached)
→ ok  github.com/lenaxia/k8s-mechanic/internal/provider    10.737s
→ ok  github.com/lenaxia/k8s-mechanic/internal/provider/native (cached)
All 13 packages pass.
```

---

## Next Steps

STORY_03 (structured audit log) and STORY_04 (agent RBAC scoping) are independent and
can proceed. STORY_06 (pentest) requires STORY_02 complete — this story satisfies that
dependency. Start STORY_03: add `zap.Bool("audit", true)` structured log lines to
`internal/provider/provider.go` and `internal/controller/remediationjob_controller.go`
at all key remediation decision points.

---

## Files Modified

- `deploy/kustomize/network-policy-agent.yaml` (new)
- `deploy/overlays/security/kustomization.yaml` (new)
- `deploy/overlays/security/network-policy-agent.yaml` (new — copy for overlay self-containment)
- `docs/BACKLOG/epic12-security-review/STORY_02_network_policy.md` (status + checklist updated)
- `docs/WORKLOGS/0045_2026-02-23_story02-network-policy.md` (this file)
