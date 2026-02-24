# Worklog: Cross-Epic Validation Gap Fixes

**Date:** 2026-02-23
**Session:** Skeptical validation of all implemented epics (epic00–epic17); 11 gaps found and fixed
**Status:** Complete

---

## Objective

Run a comprehensive skeptical review of all implemented epics to find integration gaps, dead code,
contract violations, and anything that could silently break at runtime.

---

## Work Completed

### Validation Findings (11 gaps total)

#### GAP-1 (Major) — CRD YAML schema forward-declared deferred-epic fields
- **Files:** `charts/mendabot/crds/remediationjob.yaml`, `deploy/kustomize/crd-remediationjob.yaml`
- **Fix:** Removed `isSelfRemediation`, `chainDepth` (from spec), `correlationGroupID` (from status), and `Suppressed` (from phase enum) — these fields belong to deferred epics 11/13 and have no Go backing types.
- `testdata/crds/remediationjob_crd.yaml` was already clean; no change needed there.

#### GAP-2 (Major) — Helm chart injected 7 env vars not parsed by the binary
- **Files:** `charts/mendabot/templates/deployment-watcher.yaml`, `charts/mendabot/values.yaml`
- **Fix:** Removed the entire `selfRemediation:` values block (7 knobs: maxDepth, cooldownSeconds, upstreamRepo, disableUpstreamContributions, disableCascadeCheck, cascadeNamespaceThreshold, cascadeNodeCacheTTLSeconds) and their corresponding env var injections. These are from the deferred cascade-prevention feature; the binary silently ignored them.

#### GAP-3 (Major) — AgentWatchNamespaces parsed but never used to scope informer cache
- **File:** `cmd/watcher/main.go`
- **Fix:** Refactored `ctrl.NewManager` to build `ctrl.Options` in a local var; when `cfg.AgentWatchNamespaces` is non-empty, set `opts.Cache = cache.Options{DefaultNamespaces: ...}` to restrict controller-runtime informer to those namespaces. The namespace RBAC boundary is now actually enforced.

#### GAP-4 (Major) — JobProvider had no self-exclusion for mendabot-agent-* jobs
- **File:** `internal/provider/native/job.go`
- **Fix:** Added early-return guard in `ExtractFinding`: `if job.Labels["app.kubernetes.io/managed-by"] == "mendabot-watcher" { return nil, nil }`. Removed now-unused `cfg config.Config` field and `config` import from the struct. Updated call site in `main.go`. Added test `TestJobProvider_ExtractFinding_ExcludesMendabotManagedJobs`.

#### GAP-5 (Minor) — `SinkConfig` type was dead code
- **File:** `internal/domain/provider.go`
- **Fix:** Deleted `SinkConfig` struct and its two tests (`TestSinkConfig_FieldsExist`, `TestSinkConfig_ZeroValue`).

#### GAP-6 (Minor) — `Finding.SourceRef` populated but never consumed
- **Files:** `internal/domain/provider.go`, all 6 native provider files, test files
- **Fix:** Removed `SourceRef` field from `Finding` struct, removed the `SourceRef` type definition, removed population in all 6 native providers, removed test assertions for SourceRef.

#### GAP-7 (Minor) — `dispatch()` log missing `audit=true` and `event` field
- **File:** `internal/controller/remediationjob_controller.go`
- **Fix:** Added `zap.Bool("audit", true)` and `zap.String("event", "job.dispatched")` to the `r.Log.Info("dispatched agent job", ...)` call.

#### GAP-8 (Minor) — `EventRecorder` wired but never called in provider.go
- **File:** `internal/provider/provider.go`
- **Fix:** Added `EventRecorder.Eventf` calls at three sites (nil-guarded):
  1. After successful RemediationJob creation: `RemediationJobCreated` (Normal)
  2. Cancel path: `RemediationJobCancelled` (Warning)
  3. PermanentlyFailed suppression: `RemediationJobPermanentlyFailed` (Warning)
- Added test `TestReconcile_EventRecorder_EmitsRemediationJobCreated`.

#### GAP-9 (Minor) — Stale `e.g. k8sgpt` in testdata CRD description
- **File:** `testdata/crds/remediationjob_crd.yaml:58`
- **Fix:** Changed `e.g. k8sgpt` → `e.g. native`.

#### GAP-10 (Minor) — No test for MaxRetries=0 fallback path
- **File:** `internal/controller/remediationjob_controller_test.go`
- **Fix:** Added `TestRemediationJobReconciler_PhaseFailed_ZeroMaxRetries_UsesDefault` — creates rjob with MaxRetries=0, RetryCount=2; asserts phase reaches PermanentlyFailed (proving fallback to 3 works).

#### GAP-11 (Minor) — LLM_PROVIDER, MAX_INVESTIGATION_RETRIES, INJECTION_DETECTION_ACTION, AGENT_RBAC_SCOPE absent from Helm chart
- **Files:** `charts/mendabot/values.yaml`, `charts/mendabot/templates/deployment-watcher.yaml`
- **Fix:** Added the four missing env vars to the watcher section of values.yaml (with correct defaults) and their injections in the deployment template.

---

## Key Decisions

- **GAP-6 (SourceRef removed entirely):** Rather than marking it `// reserved for future use`, it was deleted per the zero-technical-debt rule. If a future story needs it, it can be reintroduced with a concrete consumer.
- **GAP-3 (namespace scoping):** The fix uses `cache.Options.DefaultNamespaces` which restricts informers to the listed namespaces. When the list is empty (cluster scope), the zero value of `cache.Options` means no restriction — existing behaviour is preserved exactly.
- **GAP-4 (cfg field removal):** The `cfg` field was removed from `jobProvider` since it was the only reason for the field's existence. `NewJobProvider` now takes only `client.Client`.

---

## Blockers

None.

---

## Tests Run

```
go build ./...                   — clean
go test -timeout 60s -race ./... — 12/12 packages PASS
go vet ./...                     — clean
helm lint charts/mendabot        — 0 chart(s) failed
```

---

## Next Steps

All implemented epics (epic00–epic17) are now validated clean. Resume implementing remaining epics per the ordered backlog:
1. **epic23** — structured audit log gaps (additive, low risk)
2. **epic21** — Kubernetes Events (EventRecorder wiring — GAP-8 is now done, so epic21 scope may be reduced)
3. **epic22** — GitHub App token expiry guard
4. **epic18** — pre-PR manifest validation
5. **epic15** — namespace filtering
6. **epic16** — annotation opt-in/out
7. **epic20** — dry-run mode

---

## Files Modified

- `charts/mendabot/crds/remediationjob.yaml`
- `deploy/kustomize/crd-remediationjob.yaml`
- `charts/mendabot/templates/deployment-watcher.yaml`
- `charts/mendabot/values.yaml`
- `cmd/watcher/main.go`
- `internal/provider/native/job.go`
- `internal/provider/native/job_test.go`
- `internal/provider/native/pod.go`
- `internal/provider/native/pod_test.go`
- `internal/provider/native/deployment.go`
- `internal/provider/native/statefulset.go`
- `internal/provider/native/node.go`
- `internal/provider/native/node_test.go`
- `internal/provider/native/pvc.go`
- `internal/domain/provider.go`
- `internal/domain/provider_test.go`
- `internal/controller/remediationjob_controller.go`
- `internal/controller/remediationjob_controller_test.go`
- `internal/provider/provider.go`
- `internal/provider/provider_test.go`
- `internal/provider/provider_integration_test.go`
- `testdata/crds/remediationjob_crd.yaml`
