# Epic 23: Structured Audit Log

**Feature Tracker:** FT-S3
**Area:** Security

## Purpose

Ensure every security-relevant decision point in `SourceProviderReconciler` and
`RemediationJobReconciler` emits a structured log line with `zap.Bool("audit", true)`.
This creates a queryable audit trail for compliance and post-incident investigation.

Epic 12 (security review) delivered an initial audit log implementation (STORY_03).
This epic tracks gaps found during the 2026-02-23 deep dive and extensions required
by epics 15–22.

## Status: Not Started (Gap Analysis Complete)

## Deep-Dive Findings (2026-02-23)

A gap analysis of the current codebase against the epic12 STORY_03 spec and the
epic23 decision-point table found **6 actionable gaps** (see details below).

Three additional gaps were found but are **deferred** (dependent on epics not yet
implemented): cascade suppression, circuit breaker suppression, max-depth suppression.

### Gap 1 — Stabilisation window: no audit log (HIGH)

`internal/provider/provider.go` lines 167–181: the stabilisation window block
returns silently on both the first-seen path (line 170) and the still-within-window
path (line 175). No `zap.Bool("audit", true)` call with
`event=finding.suppressed.stabilisation_window` exists.

Epic12 STORY_03 specified this call but it was never added.

**Fix:** Add audit log calls at lines 170 and 175 with `reason=first_seen` and
`reason=window_open` respectively.

### Gap 2 — `finding_detected`: no audit log (HIGH)

`internal/provider/provider.go`: no audit log is emitted after `ExtractFinding`
returns a non-nil finding (lines 118–125). The fingerprint is computed at line 159.

**Fix:** Add `event=finding_detected` audit call after line 159 (so fingerprint can
be included). Required fields: provider, kind, namespace, name, fingerprint.

### Gap 3 — `duplicate_fingerprint`: no audit log (HIGH)

`internal/provider/provider.go` lines 190–203: the dedup loop finds an existing
non-Failed `RemediationJob` and returns `ctrl.Result{}, nil` (line 196) with no log
call of any kind.

**Fix:** Add `event=duplicate_fingerprint` audit call at line 196.
Required fields: fingerprint, existingRemediationJob.

### Gap 4 — Readiness gate: missing `audit=true` (MEDIUM)

`internal/provider/provider.go` lines 211–220: a `r.Log.Error(...)` call for a
failed readiness check exists but does **not** include `zap.Bool("audit", true)`.
This is a suppression decision and should be auditable.

**Fix:** Add `zap.Bool("audit", true)` to the existing log call.

### Gap 5 — Dispatch log: missing `audit=true`, `event=`, `fingerprint=` (HIGH)

`internal/controller/remediationjob_controller.go` lines 230–234:
```go
r.Log.Info("dispatched agent job",
    zap.String("remediationJob", rjob.Name),
    zap.String("job", job.Name),
    zap.String("namespace", job.Namespace))
```
Missing: `zap.Bool("audit", true)`, `zap.String("event", "job.dispatched")`,
`zap.String("fingerprint", rjob.Spec.Fingerprint[:12])`.

Epic12 STORY_03 specified these fields but they were not added.

**Fix:** Add the three missing fields to the existing log call.

### Gap 6 — Max concurrent jobs gate: no log at all (LOW/INFORMATIONAL)

`internal/controller/remediationjob_controller.go` lines 159–161: the
`MaxConcurrentJobs` gate silently requeues after 30 seconds with no log call.

**Fix (non-audit):** Add a `Debug` level log so operators can diagnose throttling.

### Deferred gaps (require unimplemented epics)

| Gap | Event | Deferred to |
|-----|-------|-------------|
| Cascade suppression | `finding.suppressed.cascade` | epic13 / future |
| Circuit breaker suppression | `finding.suppressed.circuit_breaker` | future |
| Max-depth suppression | `finding.suppressed.max_depth` | future |
| `permanently_failed` | `job.permanently_failed` | epic17 STORY_03 |
| `dry_run_report_stored` | `dry_run_report_stored` | epic20 STORY_04 |

## New Decision Points from v1 Epics

These decision points will be introduced by epics 15–22. Each implementing story's
Definition of Done must include audit coverage.

| Epic | Event | Location | Required fields |
|------|-------|----------|-----------------|
| epic15 | `finding_suppressed` with `reason=namespace_excluded` | SourceProviderReconciler after namespace filter | provider, kind, namespace, name, reason |
| epic16 | `finding_suppressed` with `reason=annotation_disabled` or `reason=skip_until` | SourceProviderReconciler after annotation gate | provider, kind, namespace, name, reason, annotation_value |
| epic17 | `permanently_failed` | RemediationJobReconciler terminal path | rjob_name, retry_count |
| epic20 | `dry_run_report_stored` | RemediationJobReconciler after report persisted | rjob_name |

## Known-Good Audit Coverage (confirmed by gap analysis)

| Event | File | Lines |
|-------|------|-------|
| `finding.injection_detected` | `provider.go` | 129–137 |
| `finding.injection_detected_in_details` | `provider.go` | 145–153 |
| `remediationjob.created` | `provider.go` | 272–282 |
| `remediationjob.cancelled` (source deleted) | `provider.go` | 103–110 |
| `job.succeeded` | `remediationjob_controller.go` | 132–140 |
| `job.failed` | `remediationjob_controller.go` | 132–140 |
| `remediationjob.deleted_ttl` | `remediationjob_controller.go` | 73–80 |

## Dependencies

- epic12-security-review complete (STORY_03_audit_log — initial implementation)
- epic15 through epic22 (new decision points must have audit coverage in their implementing stories)

## Blocks

- Nothing

## Stories

| Story | File | Status |
|-------|------|--------|
| Gap analysis — verify epic12 STORY_03 completeness | [STORY_01_gap_analysis.md](STORY_01_gap_analysis.md) | **Complete — 6 Gaps Found** |

## Implementation Order

Gaps 1–5 can be fixed in a single PR. Gap 6 (max concurrent jobs) is optional.
Deferred gaps are tracked in their respective implementing epics.

## Key Decision Points (must all emit audit log lines)

| Location | Event | Required Fields |
|---|---|---|
| `SourceProviderReconciler.Reconcile` | `finding_detected` | provider, kind, namespace, name, fingerprint |
| `SourceProviderReconciler.Reconcile` | `finding.suppressed.stabilisation_window` | provider, kind, namespace, reason (first_seen / window_open) |
| `SourceProviderReconciler.Reconcile` | `finding_suppressed` (namespace, annotation) | provider, kind, namespace, name, reason |
| `SourceProviderReconciler.Reconcile` | `duplicate_fingerprint` | fingerprint, existing_rjob_name |
| `SourceProviderReconciler.Reconcile` | `remediationjob_created` | provider, kind, namespace, fingerprint, rjob_name |
| `RemediationJobReconciler.Reconcile` | `job_dispatched` | rjob_name, agent_job_name, fingerprint |
| `RemediationJobReconciler.Reconcile` | `job_succeeded` | rjob_name, agent_job_name |
| `RemediationJobReconciler.Reconcile` | `job_failed` | rjob_name, agent_job_name, retry_count |
| `RemediationJobReconciler.Reconcile` | `permanently_failed` | rjob_name, retry_count (epic17) |
| `RemediationJobReconciler.Reconcile` | `dry_run_report_stored` | rjob_name (epic20) |

## Definition of Done

- [ ] Gap 1: Stabilisation window first-seen and window-open paths emit audit lines
- [ ] Gap 2: `finding_detected` audit call added after fingerprint computation (line 159)
- [ ] Gap 3: `duplicate_fingerprint` audit call added at dedup early return (line 196)
- [ ] Gap 4: `audit=true` added to readiness gate error log (lines 211–220)
- [ ] Gap 5: `audit=true`, `event=job.dispatched`, `fingerprint=` added to dispatch log (lines 230–234)
- [ ] Epics 15–22 implementing stories each include audit coverage in their Definition of Done
- [ ] All tests pass with `-race`
- [ ] Worklog written
