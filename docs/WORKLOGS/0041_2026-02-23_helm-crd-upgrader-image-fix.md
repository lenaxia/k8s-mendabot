# Worklog: Helm CRD Upgrader Image Fix

**Date:** 2026-02-23
**Session:** Debugging helm install failure caused by nonexistent kubectl image tag in the CRD upgrade hook
**Status:** Complete

---

## Objective

Diagnose and fix a helm install failure where the `mendabot-crd-upgrader` pre-install hook job
was stuck in `ImagePullBackOff`, causing helm to time out and mark the release as failed.

---

## Work Completed

### 1. Root cause identification

- Initial assumption (from operator observation) was that `registry.k8s.io` was inaccessible
- Queried the registry directly: `https://registry.k8s.io/v2/kubectl/tags/list`
- Confirmed that `registry.k8s.io/kubectl:v1.28.16` does not exist — the 1.28 line ends at `v1.28.15`
- The chart was referencing a tag that was never published; this is a bad default, not a network issue

### 2. Chart fix

- Corrected `crdUpgrader.image.tag` default from `v1.28.16` to `v1.28.15` in `values.yaml`
- Made the entire image configurable via a `crdUpgrader.image.{repository,tag,pullPolicy}` block
  in `values.yaml` so operators can override for internal registry mirrors if needed
- Templated `job-crd-upgrade.yaml` to reference `{{ .Values.crdUpgrader.image.repository }}`,
  `{{ .Values.crdUpgrader.image.tag }}`, and `{{ .Values.crdUpgrader.image.pullPolicy }}`
  instead of the hardcoded string
- Verified rendering with `helm template` — default path and override path both render correctly

---

## Key Decisions

- **Keep `registry.k8s.io/kubectl` as the default** rather than switching to an alternative image.
  The registry is publicly accessible; the only problem was the bad tag. No reason to change the
  image source.
- **Make the image configurable regardless.** Even though the immediate bug is a bad tag, hardcoding
  any image in a hook job is poor chart hygiene. The `crdUpgrader.image` block costs nothing and
  prevents the same class of problem in environments with internal mirrors.
- **Do not bump to a newer kubectl minor version** in this fix. The chart's `kubeVersion` constraint
  is `>=1.28.0-0` so `v1.28.15` is the correct conservative default. A separate chart version bump
  can revisit this when the minimum supported version is raised.

---

## Blockers

None.

---

## Tests Run

- `helm template mendabot charts/mendabot --set gitops.repo=test/repo --set gitops.manifestRoot=/`
  — rendered correctly, image field shows `registry.k8s.io/kubectl:v1.28.15`
- Override path tested with `--set crdUpgrader.image.repository=my-registry.internal/kubectl
  --set crdUpgrader.image.tag=1.29.0 --set crdUpgrader.image.pullPolicy=Always`
  — rendered correctly

No Go tests affected (chart-only change).

---

## Next Steps

No immediate follow-up required. If the minimum supported Kubernetes version is raised above 1.28
in a future chart version, update `crdUpgrader.image.tag` to match the new minimum minor.

---

## Files Modified

- `charts/mendabot/values.yaml` — added `crdUpgrader.image` block, corrected tag to `v1.28.15`
- `charts/mendabot/templates/job-crd-upgrade.yaml` — templated image and imagePullPolicy fields
