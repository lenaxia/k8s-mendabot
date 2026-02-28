# Worklog: Second-Pass Validation Gap Fixes

**Date:** 2026-02-23
**Session:** Second-pass skeptical validation (round 2); 4 gaps found and fixed
**Status:** Complete

---

## Objective

Run a second-pass comprehensive skeptical review after the 11-gap fix round to catch anything
missed or introduced by those fixes.

---

## Work Completed

### Validation Findings (4 gaps total)

#### GAP-1 (Critical) — Helm values.yaml: injectionDetectionAction default crashes watcher
- **File:** `charts/mechanic/values.yaml:44`
- **Fix:** Changed `injectionDetectionAction: "warn"` → `"log"`. The value `"warn"` is rejected
  by `config.go` which only accepts `"log"` or `"suppress"` — any fresh Helm deployment would
  have crashed at startup immediately. Also corrected the stale comment on line 43 (`warn or block`
  → `log or suppress`).

#### GAP-2 (Major) — Helm chart: AGENT_WATCH_NAMESPACES never injected
- **Files:** `charts/mechanic/values.yaml`, `charts/mechanic/templates/deployment-watcher.yaml`
- **Fix:** Added `agentWatchNamespaces: ""` to the watcher section in values.yaml (empty = cluster
  scope, the safe default). Added `AGENT_WATCH_NAMESPACES` env var injection to deployment template
  immediately after `AGENT_RBAC_SCOPE`. Without this, any operator using `agentRBACScope: "namespace"`
  would get a startup crash because `config.go` requires `AGENT_WATCH_NAMESPACES` when scope is namespace.

#### GAP-3 (Minor) — provider.go: readiness-check suppression missing audit=true
- **File:** `internal/provider/provider.go:231`
- **Fix:** Added `zap.Bool("audit", true)` as the first field in the `r.Log.Error("readiness check
  failed, suppressing RemediationJob creation", ...)` call. This is a security-relevant suppression
  decision that was not captured by audit log filtering.

#### GAP-4 (Minor) — SourceProvider interface doc: ProviderName uniqueness contract inaccurate
- **File:** `internal/domain/provider.go:75`
- **Fix:** Updated comment from "Must be unique across all registered providers" to "Returns the
  provider type identifier. Multiple providers of the same type (e.g. native Pod, Deployment) may
  return the same value." This matches actual behaviour — all 6 native providers return `"native"`.

---

## Key Decisions

- **GAP-1 root cause:** The `injectionDetectionAction` value was added to values.yaml in the previous
  gap-fix round (GAP-11 of round 1) without cross-checking the accepted values in config.go. The
  correct values are `"log"` and `"suppress"`; `"warn"` was incorrectly assumed to be valid.
- **GAP-2 root cause:** `AGENT_WATCH_NAMESPACES` was also added in round 1 (GAP-11) but the
  template injection was missed — only the values key was needed but was forgotten entirely.

---

## Blockers

None.

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

All implemented epics (epic00–epic17) are now validated with zero Critical or Major gaps.
Proceed with next epic per the implementation order:
1. **epic23** — structured audit log gaps (additive, low risk)
2. **epic21** — Kubernetes Events (partial: EventRecorder.Eventf at 3 sites already done in round 1 gap fixes; review remaining scope)
3. **epic22** — GitHub App token expiry guard
4. **epic18** — pre-PR manifest validation
5. **epic15** — namespace filtering
6. **epic16** — annotation opt-in/out
7. **epic20** — dry-run mode

---

## Files Modified

- `charts/mechanic/values.yaml`
- `charts/mechanic/templates/deployment-watcher.yaml`
- `internal/provider/provider.go`
- `internal/domain/provider.go`
