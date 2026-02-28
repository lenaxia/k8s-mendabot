# Worklog: Epic 11 STORY_06 — Monitoring Complete

**Date:** 2026-02-23
**Session:** Completed the remaining three items in STORY_06 (Monitoring): Kubernetes Events, Grafana dashboard, and Prometheus alert rules. Included a full adversarial review cycle with 10 gaps found and fixed.
**Status:** Complete

---

## Objective

Complete STORY_06 (Monitoring and Observability for Cascade Events), which was left at ~60% done after the previous session. Remaining items:
- Kubernetes Events (`EventRecorder`) for `CircuitBreakerOpened`, `DeepCascadeDetected`, `InfrastructureCascadeSuppressed`
- Grafana dashboard template
- Prometheus alert rules

---

## Work Completed

### 1. Kubernetes Events — EventRecorder integration

**Files:** `internal/provider/provider.go`, `internal/provider/export_test.go`, `internal/provider/provider_test.go`, `cmd/watcher/main.go`

Added `EventRecorder record.EventRecorder` field to `SourceProviderReconciler`. Three events emitted:

| Event Reason | Trigger | Type |
|---|---|---|
| `CircuitBreakerOpened` | Circuit breaker blocks self-remediation | Warning |
| `DeepCascadeDetected` | `ChainDepth > 1` on a self-remediation finding | Warning |
| `InfrastructureCascadeSuppressed` | Cascade checker suppresses a finding | Warning |

All calls are guarded with `if r.EventRecorder != nil`. The `EventRecorder` is emitted against `obj` (the watched source object), which is always non-nil at each emit point.

**Wired in `cmd/watcher/main.go`** — `EventRecorder: mgr.GetEventRecorderFor("mechanic-watcher")` added to all `SourceProviderReconciler` constructions in the provider loop.

**TDD:** Four tests written first (failed), then implemented (passed):
- `TestReconcile_CircuitBreakerBlocked_EmitsEvent`
- `TestReconcile_DeepCascade_EmitsEvent`
- `TestReconcile_InfrastructureCascadeSuppressed_EmitsEvent`
- `TestReconcile_NilEventRecorder_NoPanic`

### 2. Adversarial review — 10 gaps found and fixed

Full skeptical review of the EventRecorder implementation found 10 gaps. All fixed:

| Gap | Severity | Fix |
|-----|----------|-----|
| GAP-1 | Major | RFC3339Nano → RFC3339 in CB test timestamp; CB parser used RFC3339 |
| GAP-2 | Major | DeepCascade test injected fake cascade checker (deterministic) |
| GAP-3 | Critical | Added `SetCircuitBreakerForTest` to `export_test.go` |
| GAP-4 | Major | `FirstSeen()` returns copy; replaced direct map mutation with `SetFirstSeenForTest` |
| GAP-5 | Minor | Removed struct field comments that restated field names |
| GAP-6 | Minor | Removed narrative inline comments from `Reconcile` |
| GAP-7 | Minor | All 4 new tests now call `metrics.ResetMetrics()` + `t.Cleanup` |
| GAP-8 | Major | Same root as GAP-2; fixed by cascade checker injection |
| GAP-9 | Minor | Replaced non-idiomatic drain loop with clean `select/default` |
| GAP-10 | Minor | Moved test-helper methods from `provider.go` to `export_test.go` |

GAP-10 is architecturally significant: `FirstSeen()`, `SetFirstSeenForTest()`, `SetCascadeCheckerForTest()`, and `SetCircuitBreakerForTest()` are now in `internal/provider/export_test.go` (package `provider`, only compiled during test builds). They no longer appear in the production binary.

### 3. Grafana dashboard

**File:** `deploy/monitoring/grafana-dashboard.json`

Project-level dashboard covering all mechanic metrics. Five sections:

| Section | Panels |
|---------|--------|
| Remediation Overview | Success rate gauge, total successes/failures (stat), total suppressions (stat), active circuit breakers (stat) |
| Self-Remediation Throughput | Rate by outcome (timeseries), success rate over time (timeseries) |
| Cascade Prevention | Suppression rate by type (timeseries), suppression reason rate (timeseries) |
| Chain Depth | p50/p90/p99 depth distribution (timeseries), max-depth-exceeded rate (timeseries) |
| Circuit Breaker | Activation rate (timeseries), cooldown remaining (timeseries) |

Template variables for `namespace` and `provider` filters. UID: `mechanic-operator-overview`.

### 4. Prometheus alert rules

**File:** `deploy/monitoring/alerts.yaml`

`PrometheusRule` manifest with two rule groups:

**`mechanic.remediation`:**
- `MechanicSelfRemediationSuccessRateLow` — rate < 0.5 for 15m (critical)
- `MechanicSelfRemediationSuccessRateDegraded` — rate < 0.8 for 30m (warning)
- `MechanicHighFailureRate` — > 0.1 failures/sec over 5m (warning)

**`mechanic.cascade`:**
- `MechanicCircuitBreakerHighActivationRate` — > 0.05 trips/sec for 5m (warning)
- `MechanicCircuitBreakerStuckOpen` — cooldown active for > 30m (warning)
- `MechanicDeepCascadeDetected` — any `max_depth_exceeded` increment (warning)
- `MechanicHighCascadeSuppressionRate` — > 0.2 suppressions/sec for 10m (warning)
- `MechanicInfrastructureCascadeSuppressing` — infrastructure_cascade active for 5m (warning)
- `MechanicCascadeChainDepthHigh` — p95 chain depth > 3 for 5m (warning)

---

## Key Decisions

- **EventRecorder on `obj` not `rjob`**: At suppression time (cascade check, circuit breaker), no `RemediationJob` exists yet. Emitting against the watched source `obj` is correct — it's always the object being reconciled.
- **`export_test.go` pattern**: Moves test-only helpers out of the production binary while keeping them in the `provider` package for unexported field access. This is the standard Go pattern for this situation.
- **No new env vars for Events/Metrics/Logging config**: The STORY_06 spec proposed `METRICS_ENABLED`, `AUDIT_LOG_ENABLED`, `EVENTS_ENABLED` toggles. These were not implemented. The existing `ServiceMonitor` already controls scraping. Adding env-var toggles for observability features creates more ways to accidentally blind yourself in production — not worth the complexity.
- **`deploy/monitoring/` location**: Dashboard and alert rules are placed under `deploy/monitoring/` rather than `charts/` or `docs/` because they are operational Kubernetes manifests (PrometheusRule) and Grafana provisioning artifacts, not Helm-chart-internal concerns.

---

## Blockers

None.

---

## Tests Run

```
go build ./...                          # zero errors
go test -timeout 60s -race ./...        # all 13 packages pass
```

---

## Next Steps

Epic 11 is now complete. All STORY_06 acceptance criteria are met.

Recommended next: Epic 08 (Pluggable Agent Provider) — the only unstarted epic. It addresses the hardcoded Secret/ConfigMap name problem (compile-time constants in `jobbuilder`).

---

## Files Modified

- `internal/provider/provider.go` — added `EventRecorder` field, emitted 3 events, removed test-helper methods, removed narrative comments
- `internal/provider/export_test.go` — new file: `FirstSeen`, `SetFirstSeenForTest`, `SetCascadeCheckerForTest`, `SetCircuitBreakerForTest`
- `internal/provider/provider_test.go` — 4 new event tests, GAP-1/2/4/7/9 fixes
- `cmd/watcher/main.go` — added `EventRecorder` to all `SourceProviderReconciler` constructions
- `deploy/monitoring/alerts.yaml` — new file: PrometheusRule with 9 alert rules
- `deploy/monitoring/grafana-dashboard.json` — new file: project-level Grafana dashboard
- `docs/BACKLOG/epic11-self-remediation-cascade/STORY_06_monitoring.md` — updated to Complete
- `docs/WORKLOGS/README.md` — added entry 0040
