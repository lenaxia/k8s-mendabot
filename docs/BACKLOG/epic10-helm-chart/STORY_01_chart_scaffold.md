# Story 01: Chart Scaffold and Chart.yaml

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Critical (blocks all other stories)
**Status:** Not Started
**Estimated Effort:** 15 minutes

---

## User Story

As a **chart maintainer**, I want a minimal but valid Helm chart scaffold so I can
iterate on templates and run `helm lint` from the very first commit.

---

## Acceptance Criteria

- [ ] `charts/mechanic/Chart.yaml` exists with all required fields:
  - `apiVersion: v2`
  - `name: mechanic`
  - `description: Kubernetes-native SRE remediation bot`
  - `type: application`
  - `version: 0.1.0`
  - `appVersion: v0.3.0`
  - `kubeVersion: ">=1.28.0-0"`
  - `keywords`, `home`, `sources`, `maintainers` populated
- [ ] `charts/mechanic/values.yaml` exists with at minimum `{}` (populated fully in STORY_02)
- [ ] `charts/mechanic/templates/` directory exists with `.gitkeep` or first template
- [ ] `charts/mechanic/crds/remediationjob.yaml` is a copy of
  `deploy/kustomize/crd-remediationjob.yaml` — identical content
- [ ] `helm lint charts/mechanic/` passes (may warn about empty templates; that is acceptable)

---

## Tasks

- [ ] Create `charts/mechanic/` directory structure
- [ ] Write `Chart.yaml`
- [ ] Create stub `values.yaml`
- [ ] Create `templates/` directory
- [ ] Copy CRD to `charts/mechanic/crds/remediationjob.yaml`
- [ ] Run `helm lint charts/mechanic/` and confirm it passes

---

## Notes

- No `dependencies:` section in Chart.yaml — the chart has zero library dependencies
- The `crds/` directory is a Helm-native mechanism: Helm installs CRDs from this
  directory during `helm install` but deliberately skips them on `helm upgrade`.
  The upgrade gap is covered by the pre-upgrade hook in STORY_08.
- `appVersion` tracks the mechanic release version, not the chart version.
  Chart version (`version`) increments independently when chart-only changes ship.

---

## Dependencies

**Depends on:** nothing
**Blocks:** STORY_02 and all downstream stories

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] CRD file in `crds/` is byte-for-byte identical to the Kustomize original
