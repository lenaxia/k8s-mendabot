# Story 03: Structured Audit Log for Remediation Decisions

**Epic:** [epic12-security-review](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **security auditor**, I want every mendabot decision (finding suppressed, job
dispatched, job cancelled, PR recorded) to produce a structured zap log line with
`"audit": true`, so that I can filter logs on that field and reconstruct a complete
decision timeline for post-incident analysis.

---

## Background

Both reconcilers currently log decisions but without a consistent `audit` field:

- `SourceProviderReconciler.Reconcile()` in `internal/provider/provider.go` logs cascade
  suppressions (line 156), circuit breaker activations (line 191), and RemediationJob
  creation (line 327) at `Info` or `Warn` level, but uses varied field names and none
  include a stable `audit=true` marker.
- `RemediationJobReconciler.Reconcile()` in `internal/controller/remediationjob_controller.go`
  logs job dispatch but without a stable audit field.

Without a consistent field, security teams cannot reliably extract a decision log via
`jq 'select(.audit == true)'` or equivalent log query.

The zap logger is already threaded through both reconcilers via the `Log *zap.Logger`
field, so no plumbing changes are needed — only log statement additions and updates.

---

## Acceptance Criteria

- [ ] Every suppression decision in `SourceProviderReconciler.Reconcile()` emits a zap
      `Info` line with `zap.Bool("audit", true)` and `zap.String("event", "<event-name>")`
- [ ] Every `RemediationJob` creation emits a zap `Info` line with `audit=true`
- [ ] Every `RemediationJob` cancellation (source deleted path) emits an audit line
- [ ] Every agent Job dispatch (`PhaseDispatched`) in `RemediationJobReconciler` emits
      an audit line
- [ ] Every phase transition to a terminal state (Succeeded, Failed, Cancelled) emits
      an audit line
- [ ] `go test -timeout 30s -race ./...` continues to pass (no logic changes, log-only)

---

## Technical Implementation

### Audit event names

Stable, dot-separated event names to use as the `event` field value:

| Event | Description |
|-------|-------------|
| `finding.suppressed.cascade` | Cascade checker suppressed a finding |
| `finding.suppressed.circuit_breaker` | Circuit breaker blocked a self-remediation |
| `finding.suppressed.max_depth` | Chain depth exceeded `SelfRemediationMaxDepth` |
| `finding.suppressed.stabilisation_window` | Stabilisation window not yet elapsed |
| `remediationjob.created` | New `RemediationJob` created by `SourceProviderReconciler` |
| `remediationjob.cancelled` | `RemediationJob` cancelled because source was deleted |
| `job.dispatched` | Agent `batch/v1 Job` created by `RemediationJobReconciler` |
| `job.succeeded` | Agent Job transitioned to Succeeded |
| `job.failed` | Agent Job transitioned to Failed |
| `remediationjob.deleted_ttl` | `RemediationJob` deleted by TTL in Succeeded phase |

### Changes to `internal/provider/provider.go`

**Cascade suppression** (around line 156, after `r.Log.Info("suppressing finding due to cascade"...)`):
```go
r.Log.Info("finding suppressed",
    zap.Bool("audit", true),
    zap.String("event", "finding.suppressed.cascade"),
    zap.String("provider", r.Provider.ProviderName()),
    zap.String("kind", finding.Kind),
    zap.String("namespace", finding.Namespace),
    zap.String("reason", reason),
)
```

**Circuit breaker block** (around line 191):
```go
r.Log.Info("finding suppressed",
    zap.Bool("audit", true),
    zap.String("event", "finding.suppressed.circuit_breaker"),
    zap.String("provider", r.Provider.ProviderName()),
    zap.String("namespace", finding.Namespace),
    zap.Duration("cooldownRemaining", remaining),
    zap.Int("chainDepth", finding.ChainDepth),
)
```

**Max depth** (around line 219):
```go
r.Log.Info("finding suppressed",
    zap.Bool("audit", true),
    zap.String("event", "finding.suppressed.max_depth"),
    zap.String("provider", r.Provider.ProviderName()),
    zap.String("namespace", finding.Namespace),
    zap.Int("chainDepth", finding.ChainDepth),
    zap.Int("maxDepth", r.Cfg.SelfRemediationMaxDepth),
)
```

**Stabilisation window — first-seen** (line 241):
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

**RemediationJob created** (replace/augment existing log at line 327):
```go
r.Log.Info("RemediationJob created",
    zap.Bool("audit", true),
    zap.String("event", "remediationjob.created"),
    zap.String("provider", r.Provider.ProviderName()),
    zap.String("fingerprint", fp[:12]),
    zap.String("kind", finding.Kind),
    zap.String("namespace", finding.Namespace),
    zap.String("parentObject", finding.ParentObject),
    zap.String("remediationJob", rjob.Name),
    zap.Bool("isSelfRemediation", finding.IsSelfRemediation),
)
```

**RemediationJob cancelled** (source-deleted path, after each successful delete):
```go
r.Log.Info("RemediationJob cancelled",
    zap.Bool("audit", true),
    zap.String("event", "remediationjob.cancelled"),
    zap.String("remediationJob", rjob.Name),
    zap.String("reason", "source_deleted"),
    zap.String("sourceRef", req.Name),
)
```

### Changes to `internal/controller/remediationjob_controller.go`

**Agent Job dispatched** (after patching status to Dispatched, around step 7):
```go
r.Log.Info("agent Job dispatched",
    zap.Bool("audit", true),
    zap.String("event", "job.dispatched"),
    zap.String("remediationJob", rjob.Name),
    zap.String("job", job.Name),
    zap.String("namespace", job.Namespace),
    zap.String("fingerprint", rjob.Spec.Fingerprint[:12]),
)
```

**Phase transition to Succeeded**:
```go
r.Log.Info("agent Job succeeded",
    zap.Bool("audit", true),
    zap.String("event", "job.succeeded"),
    zap.String("remediationJob", rjob.Name),
    zap.String("job", rjob.Status.JobRef),
    zap.String("prRef", rjob.Status.PRRef),
)
```

**Phase transition to Failed**:
```go
r.Log.Info("agent Job failed",
    zap.Bool("audit", true),
    zap.String("event", "job.failed"),
    zap.String("remediationJob", rjob.Name),
    zap.String("job", rjob.Status.JobRef),
)
```

**TTL deletion**:
```go
r.Log.Info("RemediationJob deleted by TTL",
    zap.Bool("audit", true),
    zap.String("event", "remediationjob.deleted_ttl"),
    zap.String("remediationJob", rjob.Name),
)
```

---

## Tasks

- [ ] Add audit log lines to `internal/provider/provider.go` at all suppression and
      creation decision points
- [ ] Add audit log lines to `internal/controller/remediationjob_controller.go` at all
      dispatch and phase-transition points
- [ ] Verify `go test -timeout 30s -race ./...` passes (no logic changes)
- [ ] Spot-check that nil-guard on `r.Log` is in place before every new log call
      (pattern from existing code: `if r.Log != nil {`)

---

## Dependencies

**Depends on:** epic01-controller, epic11 (circuit breaker and cascade checker log points
are already present — this story adds the `audit=true` field)
**Blocks:** STORY_06 (pentest)

---

## Definition of Done

- [ ] `go test -timeout 30s -race ./...` passes
- [ ] Every key decision in both reconcilers emits a line with `"audit": true`
- [ ] Log lines include a stable `event` string for machine filtering
- [ ] No logic changes — log additions only
