# Story 12: README — Helm Install Instructions

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Medium (user-facing documentation)
**Status:** Not Started
**Estimated Effort:** 20 minutes

---

## User Story

As an **external operator**, I want the README to show me how to install mechanic
with Helm in under 5 minutes.

---

## Acceptance Criteria

- [ ] `README.md` Quick Start section is updated with Helm-based instructions
- [ ] Instructions cover:
  1. Adding the Helm repository (note: requires GitHub Pages + Chart Releaser to be
     set up; for now, use `helm install` directly from source or OCI reference)
  2. Creating the two required Secrets (exact key names)
  3. Running `helm install` with the two required values
  4. Verifying with `kubectl get rjob`
- [ ] A separate "Kustomize" section notes that the raw Kustomize manifests remain
  in `deploy/kustomize/` for operators who prefer them
- [ ] All configuration values documented in a table with descriptions and defaults

---

## Tasks

- [ ] Update `README.md` Quick Start section
- [ ] Add a Configuration Reference table for all top-level values keys
- [ ] Add a note that both the Helm chart and Kustomize manifests are available
- [ ] Review rendered Markdown locally (e.g. GitHub preview)

---

## Quick Start content (reference)

```markdown
## Quick Start

### Prerequisites

- Kubernetes ≥ 1.28
- Helm ≥ 3.14
- A GitHub App with permissions: Contents (write), Pull Requests (write),
  Metadata (read)

### 1. Create required Secrets

```sh
kubectl create namespace mechanic

kubectl create secret generic github-app \
  --namespace mechanic \
  --from-literal=app-id=<your-app-id> \
  --from-literal=installation-id=<your-installation-id> \
  --from-file=private-key=<path-to-private-key.pem>

kubectl create secret generic llm-credentials \
  --namespace mechanic \
  --from-literal=api-key=<your-api-key> \
  --from-literal=base-url=https://api.openai.com/v1 \
  --from-literal=model=gpt-4o \
  --from-literal=kube-api-server=https://<cluster-api-server>:6443
```

### 2. Install the chart

```sh
helm install mechanic oci://ghcr.io/lenaxia/charts/mechanic \
  --namespace mechanic \
  --set gitops.repo=myorg/my-gitops-repo \
  --set gitops.manifestRoot=kubernetes
```

Or from source:

```sh
helm install mechanic charts/mechanic/ \
  --namespace mechanic \
  --set gitops.repo=myorg/my-gitops-repo \
  --set gitops.manifestRoot=kubernetes
```

### 3. Verify

```sh
kubectl get rjob -n mechanic
```
```

---

## Notes

- OCI registry push is not in scope for this epic — the `oci://` install example
  is aspirational and should be marked as "coming soon" until Chart Releaser is
  configured.
- The "from source" instruction is accurate immediately after this epic is complete.
- Keep the existing Kustomize section in README.md — do not remove it. Add Helm as
  an additional option.

---

## Dependencies

**Depends on:** STORY_11 (CI must pass before documentation claims helm install works)
**Blocks:** nothing

---

## Definition of Done

- [ ] README.md Quick Start section updated
- [ ] Configuration Reference table covers all values keys
- [ ] Kustomize section preserved
