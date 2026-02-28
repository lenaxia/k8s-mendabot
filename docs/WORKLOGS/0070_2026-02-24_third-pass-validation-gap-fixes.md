# Worklog: Third-Pass Validation Gap Fixes

**Date:** 2026-02-24
**Session:** Third-pass skeptical validation; 5 gaps found and fixed (2 Major, 3 Minor)
**Status:** Complete

---

## Objective

Third-pass comprehensive skeptical review. Previous two rounds fixed 15 gaps. This round
focused on deep correctness: audit log exhaustiveness, CRD/Helm completeness, reconciler
loop termination, and finalizer consistency.

---

## Work Completed

### Validation Findings (5 gaps total)

#### GAP-1 (Major) — Dead finalizer RBAC marker
- **Files:** `internal/controller/remediationjob_controller.go:24`,
  `charts/mechanic/templates/clusterrole-watcher.yaml:27-29`,
  `deploy/kustomize/clusterrole-watcher.yaml:24-26`
- **Fix:** Removed the `//+kubebuilder:rbac:groups=remediation.mechanic.io,resources=remediationjobs/finalizers,verbs=update`
  marker and the corresponding `remediationjobs/finalizers` RBAC rules from both YAML files.
  The controller never adds or removes a finalizer — the marker was dead code granting unnecessary
  permissions. The only legitimate finalizer references remaining are in integration tests where
  `batch.kubernetes.io/job-tracking` finalizers are stripped from batch Jobs (correct test infra).

#### GAP-2 (Major) — PhaseSucceeded zombie: CompletedAt nil path never requeued
- **File:** `internal/controller/remediationjob_controller.go:63-75`
- **Fix:** In the `case v1alpha1.PhaseSucceeded:` block, added a safety-net guard: when
  `CompletedAt == nil` (possible after controller restart with partially-applied status),
  deep-copy the rjob, set `CompletedAt = metav1.Now()`, patch status, and return
  `RequeueAfter: time.Second`. This ensures the TTL path always runs. Without this, a
  Succeeded rjob with nil CompletedAt would accumulate in etcd forever and permanently
  suppress dedup re-dispatch for that fingerprint.
- **Test added:** `TestRemediationJobReconciler_PhaseSucceeded_NilCompletedAt_SetsCompletedAt`

#### GAP-3 (Minor) — Dedup default case completely silent
- **File:** `internal/provider/provider.go:232-238`
- **Fix:** Added `r.Log.Debug(...)` in the switch default case with fingerprint, rjob name,
  and phase fields. Gives operators visibility into why a finding is not generating a new job.

#### GAP-4 (Minor) — IS_SELF_REMEDIATION=false hardcoded in JobBuilder
- **File:** `internal/jobbuilder/job.go:164`
- **Fix:** Removed the `{Name: "IS_SELF_REMEDIATION", Value: "false"}` env var entry. The
  self-remediation feature is not implemented; the hardcoded value created a false capability
  contract. Note: the var still appears in agent prompt/entrypoint artifacts (out of Go scope).

#### GAP-5 (Minor) — Stabilisation window suppression emits no log
- **File:** `internal/provider/provider.go:176-192`
- **Fix:** Added `r.Log.Info(...)` at both stabilisation window paths:
  1. First-seen: `"stabilisation window: first seen, deferring RemediationJob creation"` with fingerprint + window duration
  2. Not-yet-elapsed: `"stabilisation window: holding, not yet elapsed"` with fingerprint + remaining duration

---

## Tests Run

```
go build ./...                              — clean
go test -count=1 -timeout 60s -race ./...  — 12/12 packages PASS
go vet ./...                                — clean
helm lint charts/mechanic                  — 0 chart(s) failed
```

---

## Next Steps

All 3 rounds of validation are complete with zero remaining Critical or Major gaps.
Proceed with next epic per the implementation order:
1. **epic23** — structured audit log gaps
2. **epic21** — Kubernetes Events (note: 3 Eventf sites already added in round-1 gap fix)
3. **epic22** — GitHub App token expiry guard
4. **epic18** — pre-PR manifest validation
5. **epic15** — namespace filtering
6. **epic16** — annotation opt-in/out
7. **epic20** — dry-run mode

---

## Files Modified

- `internal/controller/remediationjob_controller.go`
- `internal/controller/remediationjob_controller_test.go`
- `internal/jobbuilder/job.go`
- `internal/provider/provider.go`
- `charts/mechanic/templates/clusterrole-watcher.yaml`
- `deploy/kustomize/clusterrole-watcher.yaml`
