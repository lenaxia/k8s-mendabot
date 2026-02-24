# Story 01: Fix Remaining Audit Log Gaps in provider.go

**Epic:** [epic23-structured-audit-log](README.md)
**Priority:** High
**Status:** Pending

---

## User Story

As a **security auditor**, I want every decision in `SourceProviderReconciler`
to emit a structured log line with `zap.Bool("audit", true)` and a stable `event`
string, so that I can reconstruct the full decision timeline from logs alone.

---

## Background

Epic 12 STORY_03 implemented the initial audit log lines. Three gaps remain in
`internal/provider/provider.go` after the epic17/12 cross-validation rounds:

1. **Stabilisation window** — emits `Info` log at first-seen and window-open paths
   but without `audit=true` or a stable `event` name.
2. **finding.detected** — no audit log emitted after `ExtractFinding` returns a
   non-nil finding (the most important detection event has no trace).
3. **duplicate_fingerprint** — dedup default case logs at `Debug` level without
   `audit=true`; should be `Info` with `audit=true`.

No logic changes are required — these are log-only additions.

---

## Acceptance Criteria

- [ ] After `ExtractFinding` returns non-nil and the fingerprint is computed, emit:
      `event=finding.detected` at `Info` level with `audit=true`
- [ ] Stabilisation window first-seen path emits `audit=true` with
      `event=finding.suppressed.stabilisation_window` and `reason=first_seen`
- [ ] Stabilisation window still-within-window path emits `audit=true` with
      `event=finding.suppressed.stabilisation_window` and `reason=window_open`
- [ ] Dedup default case (existing non-Failed RemediationJob) emits `Info` level with
      `audit=true` and `event=finding.suppressed.duplicate`
- [ ] All existing tests continue to pass (`go test -timeout 30s -race ./...`)
- [ ] No logic changes — log additions only

---

## Technical Implementation

All changes are in `internal/provider/provider.go`.

### Gap 1 — finding.detected (after line 171, where `fp` is computed)

After the `if len(fp) < 12` guard, add:

```go
if r.Log != nil {
    r.Log.Info("finding detected",
        zap.Bool("audit", true),
        zap.String("event", "finding.detected"),
        zap.String("provider", r.Provider.ProviderName()),
        zap.String("kind", finding.Kind),
        zap.String("namespace", finding.Namespace),
        zap.String("name", finding.Name),
        zap.String("fingerprint", fp[:12]),
    )
}
```

### Gap 2 — Stabilisation window first-seen path (currently ~line 176)

Replace:
```go
r.Log.Info("stabilisation window: first seen, deferring RemediationJob creation",
    zap.String("fingerprint", fp[:12]),
    zap.Duration("window", r.Cfg.StabilisationWindow),
)
```
With:
```go
r.Log.Info("finding suppressed",
    zap.Bool("audit", true),
    zap.String("event", "finding.suppressed.stabilisation_window"),
    zap.String("provider", r.Provider.ProviderName()),
    zap.String("fingerprint", fp[:12]),
    zap.String("reason", "first_seen"),
    zap.Duration("window", r.Cfg.StabilisationWindow),
)
```

### Gap 3 — Stabilisation window still-within-window path (currently ~line 188)

Replace:
```go
r.Log.Info("stabilisation window: holding, not yet elapsed",
    zap.String("fingerprint", fp[:12]),
    zap.Duration("remaining", remaining),
)
```
With:
```go
r.Log.Info("finding suppressed",
    zap.Bool("audit", true),
    zap.String("event", "finding.suppressed.stabilisation_window"),
    zap.String("provider", r.Provider.ProviderName()),
    zap.String("fingerprint", fp[:12]),
    zap.String("reason", "window_open"),
    zap.Duration("remaining", remaining),
)
```

### Gap 4 — Dedup default case (currently ~line 232)

Replace:
```go
r.Log.Debug("dedup: suppressing re-dispatch, existing RemediationJob in active or terminal phase",
    zap.String("fingerprint", fp[:12]),
    zap.String("remediationJob", rjob.Name),
    zap.String("phase", string(rjob.Status.Phase)),
)
```
With:
```go
r.Log.Info("finding suppressed",
    zap.Bool("audit", true),
    zap.String("event", "finding.suppressed.duplicate"),
    zap.String("provider", r.Provider.ProviderName()),
    zap.String("fingerprint", fp[:12]),
    zap.String("remediationJob", rjob.Name),
    zap.String("phase", string(rjob.Status.Phase)),
)
```

---

## Out of Scope

- Cascade suppression events (require epic11 cascade checker integration)
- Circuit breaker suppression (future epic)
- Max-depth suppression (future epic)
- `max_concurrent_jobs` throttle log (see STORY_02 if desired)

---

## Dependencies

None. All required infrastructure is already in place.

---

## Definition of Done

- [ ] `go test -timeout 30s -race ./...` passes
- [ ] All 4 gaps fixed in `internal/provider/provider.go`
- [ ] No logic changes — log additions and updates only
- [ ] Code reviewed with zero gaps
