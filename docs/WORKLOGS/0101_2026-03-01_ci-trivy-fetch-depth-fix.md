# Worklog: CI Fix — trivy-action fetch-depth for .trivyignore

**Date:** 2026-03-01
**Session:** Fix broken CI on v0.3.36 caused by trivy-action 0.34.1 failing to fetch .trivyignore from refs/heads/main on shallow checkout
**Status:** Complete

---

## Objective

Re-tag and re-run CI for v0.3.36 after the trivy-action Renovate bump broke the watcher and agent build workflows. Both workflows did a shallow checkout (`fetch-depth: 1` default) but trivy-action 0.34.1 tries to fetch `.trivyignore` from `refs/heads/main` on the remote, which fails on a shallow clone.

---

## Work Completed

### 1. Diagnosis

- Identified that `build-watcher.yaml` already had the `fetch-depth: 0` fix as an unstaged local change from the previous session.
- Identified that `build-agent.yaml` also uses `aquasecurity/trivy-action@0.34.1` with `trivyignores: .trivyignore` but was missing the same fix.

### 2. Fix applied to build-agent.yaml

Added `fetch-depth: 0` under the `actions/checkout` step in `.github/workflows/build-agent.yaml`, matching the fix already present in `build-watcher.yaml`.

### 3. Commit

Committed both workflow files together:
```
e98d723 fix(ci): add fetch-depth: 0 to checkout for trivy-action 0.34.1 trivyignore fetch
```

### 4. Rebase

Remote had new commits from the previous session (epic29 agent hardening merge + chart bump):
- `fea70f6 feat(epic29): agent hardening — kubectl write blocking, hardened mode, redaction improvements, Kyverno policy`
- `6137442 chore(chart): bump chart version to v0.3.36 [skip ci]`

Rebased with `git pull --rebase origin main` — clean rebase, no conflicts.

### 5. Tag re-push

- Deleted local and remote `v0.3.36` tag (which pointed to `f1d8b54`, before the fix)
- Created new local `v0.3.36` tag pointing to `e98d723`
- Pushed `main` (succeeded) and `v0.3.36` tag (succeeded after initial network timeout)

---

## Key Decisions

- **Re-tagged v0.3.36 rather than creating v0.3.37:** The fix is a CI-only change with no functional impact. Re-tagging avoids a spurious version bump for a non-code change.
- **Fixed both build-agent.yaml and build-watcher.yaml:** Both workflows use trivy-action 0.34.1 with `.trivyignore`, so both needed the fix. Fixing only watcher would have left agent broken on the next tag.

---

## Blockers

None.

---

## Tests Run

No Go tests required — change is CI workflow YAML only.

---

## Next Steps

1. Monitor CI run triggered by the new `v0.3.36` tag — confirm both `Build Watcher` and `Build Agent` workflows pass the Trivy scan step.
2. Once CI is green, deploy v0.3.36 to the cluster:
   ```bash
   helm upgrade mechanic charts/mechanic/ \
     --reuse-values \
     --set watcher.image.tag=v0.3.36 \
     --set agent.image.tag=v0.3.36
   ```
3. Verify in cluster:
   - Existing Succeeded tombstones are re-evaluated for PR merge state after the 24h short TTL
   - Merged PRs use the full 7-day tombstone TTL from `prMergedAt`
   - Unmerged PRs past 24h cause tombstone deletion and a fresh RJob

---

## Files Modified

- `.github/workflows/build-agent.yaml` — added `fetch-depth: 0` under `actions/checkout`
- `.github/workflows/build-watcher.yaml` — committed pre-existing `fetch-depth: 0` fix (was unstaged)
