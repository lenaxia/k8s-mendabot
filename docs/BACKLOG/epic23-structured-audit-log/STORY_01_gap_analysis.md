# Story 01: Gap Analysis — Structured Audit Log

**Epic:** [epic23-structured-audit-log](README.md)
**Priority:** High
**Status:** Not Started

---

## Objective

Verify that epic12 STORY_03 was fully implemented and covers all key decision points in
`internal/provider/provider.go` and `internal/controller/remediationjob_controller.go`.
Document any gaps, including new decision points introduced by epics 15–22 that do not
yet have audit log coverage.

---

## Audit Coverage Findings

### internal/provider/provider.go

Epic12 STORY_03 planned audit calls at six decision points. Findings against the current
file (293 lines total):

---

**Decision point: finding.suppressed.cascade**
- Expected: `zap.Bool("audit", true)`, `event=finding.suppressed.cascade`
- Actual log call found: not present — no cascade suppression logic exists in this file
- Gap: **yes** — epic12 STORY_03 described this point at "line 156" but that logic is
  absent from the current file. No cascade checker code path exists.

---

**Decision point: finding.suppressed.circuit_breaker**
- Expected: `zap.Bool("audit", true)`, `event=finding.suppressed.circuit_breaker`
- Actual log call found: not present — no circuit breaker logic exists in this file
- Gap: **yes** — same as cascade; STORY_03 referenced "line 191" which is not present.

---

**Decision point: finding.suppressed.max_depth**
- Expected: `zap.Bool("audit", true)`, `event=finding.suppressed.max_depth`
- Actual log call found: not present — no chain-depth / self-remediation depth check
  exists in this file
- Gap: **yes** — STORY_03 referenced "line 219" which is not present.

---

**Decision point: finding.suppressed.stabilisation_window (first-seen)**
- Expected: `zap.Bool("audit", true)`, `event=finding.suppressed.stabilisation_window`
- Actual log call found: not present — the stabilisation window block at lines 167–181
  silently returns `ctrl.Result{RequeueAfter: r.Cfg.StabilisationWindow}` with no log
  call of any kind
- Gap: **yes** — the requeue on first-seen (line 170) and the requeue while window is
  still open (line 175) both emit nothing.

---

**Decision point: finding.injection_detected (errors field)**
- Expected: not in epic12 STORY_03 event table; present as a Warn call
- Actual log call found: `r.Log.Warn("potential prompt injection detected in finding errors", zap.Bool("audit", true), zap.String("event", "finding.injection_detected"), ...)` — **lines 129–137**
- Gap: **no** — audit=true is present. Note: this event name is not in the epic23 README
  decision-point table; it should be added to that table.

---

**Decision point: finding.injection_detected_in_details**
- Expected: not in epic12 STORY_03 event table; present as a Warn call
- Actual log call found: `r.Log.Warn("potential prompt injection detected in finding details", zap.Bool("audit", true), zap.String("event", "finding.injection_detected_in_details"), ...)` — **lines 145–153**
- Gap: **no** — audit=true is present.

---

**Decision point: remediationjob.created**
- Expected: `zap.Bool("audit", true)`, `event=remediationjob.created`, fields: provider,
  fingerprint, kind, namespace, parentObject, remediationJob
- Actual log call found: `r.Log.Info("RemediationJob created", zap.Bool("audit", true), zap.String("event", "remediationjob.created"), zap.String("provider", ...), zap.String("fingerprint", fp[:12]), zap.String("kind", ...), zap.String("namespace", ...), zap.String("parentObject", ...), zap.String("remediationJob", rjob.Name))` — **lines 272–282**
- Gap: **no** — fully covered.

---

**Decision point: remediationjob.cancelled (source deleted)**
- Expected: `zap.Bool("audit", true)`, `event=remediationjob.cancelled`,
  fields: remediationJob, reason=source_deleted, sourceRef
- Actual log call found: `r.Log.Info("RemediationJob cancelled", zap.Bool("audit", true), zap.String("event", "remediationjob.cancelled"), zap.String("remediationJob", rjob.Name), zap.String("reason", "source_deleted"), zap.String("sourceRef", req.Name))` — **lines 103–110**
- Gap: **no** — fully covered.

---

**Decision point: finding_detected (new in epic23 README table)**
- Expected: `event=finding_detected`, fields: provider, kind, namespace, name, fingerprint
- Actual log call found: not present — no audit log is emitted immediately after
  `ExtractFinding` returns a non-nil finding (lines 118–125). The fingerprint is not
  computed until line 159, and no log call sits between the finding extraction and the
  stabilisation window block.
- Gap: **yes** — this event is in the epic23 README decision-point table but was not in
  epic12 STORY_03 and was never implemented.

---

**Decision point: duplicate_fingerprint (new in epic23 README table)**
- Expected: `event=duplicate_fingerprint`, fields: fingerprint, existing_rjob_name
- Actual log call found: not present — the dedup loop at lines 190–203 finds an existing
  non-Failed RemediationJob and returns `ctrl.Result{}, nil` (line 196) with no log call
  at all.
- Gap: **yes** — this event is in the epic23 README decision-point table and was never
  implemented.

---

**Decision point: readiness gate suppression**
- Expected: not in epic12 STORY_03 table or epic23 README table
- Actual log call found: `r.Log.Error("readiness check failed, suppressing RemediationJob creation", zap.Error(err), zap.String("checker", ...), zap.String("fingerprint", fp[:12]), ...)` — **lines 211–220** — but **without** `zap.Bool("audit", true)`
- Gap: **yes** — the log call exists but is not marked as an audit event.

---

### internal/controller/remediationjob_controller.go

Epic12 STORY_03 planned audit calls at four decision points. Findings against the current
file (246 lines total):

---

**Decision point: job.dispatched**
- Expected: `zap.Bool("audit", true)`, `event=job.dispatched`,
  fields: remediationJob, job, namespace, fingerprint
- Actual log call found: `r.Log.Info("dispatched agent job", zap.String("remediationJob", rjob.Name), zap.String("job", job.Name), zap.String("namespace", job.Namespace))` — **lines 230–234** — **without** `zap.Bool("audit", true)` and without `event=` or `fingerprint=`
- Gap: **yes** — the log call exists but is missing audit=true, event field, and
  fingerprint field.

---

**Decision point: job.succeeded / job.failed**
- Expected: `zap.Bool("audit", true)`, `event=job.succeeded` or `event=job.failed`,
  fields: remediationJob, job, namespace, prRef
- Actual log call found: `r.Log.Info("agent job terminal", zap.Bool("audit", true), zap.String("event", event), zap.String("remediationJob", rjob.Name), zap.String("job", job.Name), zap.String("namespace", rjob.Namespace), zap.String("prRef", rjob.Status.PRRef))` — **lines 132–140** — where `event` is `"job.succeeded"` or `"job.failed"`
- Gap: **no** — fully covered for both terminal states.

---

**Decision point: remediationjob.deleted_ttl**
- Expected: `zap.Bool("audit", true)`, `event=remediationjob.deleted_ttl`,
  fields: remediationJob, namespace, prRef
- Actual log call found: `r.Log.Info("RemediationJob deleted by TTL", zap.Bool("audit", true), zap.String("event", "remediationjob.deleted_ttl"), zap.String("remediationJob", rjob.Name), zap.String("namespace", rjob.Namespace), zap.String("prRef", rjob.Status.PRRef))` — **lines 73–80**
- Gap: **no** — fully covered.

---

**Decision point: permanently_failed (epic17, not yet implemented)**
- Expected: `event=permanently_failed`, fields: rjob_name, retry_count
- Actual log call found: not present — epics 15–22 do not exist yet; this path is not
  implemented
- Gap: **deferred** — must be added as part of epic17.

---

**Decision point: dry_run_report_stored (epic20, not yet implemented)**
- Expected: `event=dry_run_report_stored`, fields: rjob_name
- Actual log call found: not present — epic20 does not exist yet
- Gap: **deferred** — must be added as part of epic20.

---

**Decision point: max_concurrent_jobs gate**
- Expected: not in epic12 STORY_03 table or epic23 README table
- Actual log call found: not present — the `MaxConcurrentJobs` gate at lines 159–161
  silently requeues after 30 seconds with no log call of any kind.
- Gap: **informational** — not in any decision-point table; worth adding as a non-audit
  debug log at minimum.

---

## New Decision Points from v1 Epics (require audit coverage)

The following decision points are listed in the epic23 README table or will be introduced
by epics 15–22. None of those epics exist yet as story files.

| Epic | Event name | Location | Required fields |
|------|-----------|----------|-----------------|
| epic15 | `finding_suppressed` with `reason=namespace_excluded` | `SourceProviderReconciler.Reconcile` — after namespace filter check, before stabilisation window | provider, kind, namespace, name, reason |
| epic16 | `finding_suppressed` with `reason=annotation_disabled` or `reason=skip_until` | `SourceProviderReconciler.Reconcile` — after annotation gate check | provider, kind, namespace, name, reason, annotation_value |
| epic17 | `permanently_failed` | `RemediationJobReconciler.Reconcile` — terminal path after dead-letter threshold exceeded | rjob_name, retry_count |
| epic20 | `dry_run_report_stored` | `RemediationJobReconciler.Reconcile` — after dry-run report is persisted | rjob_name |

Each implementing story for epics 15–22 must include audit log coverage as part of its
own Definition of Done.

---

## Gaps Found

1. **`internal/provider/provider.go` line 170 / line 175 — stabilisation window, no audit log.**
   The first-seen requeue and the still-within-window requeue both return silently. No
   `zap.Bool("audit", true)` call with `event=finding.suppressed.stabilisation_window`
   exists. Epic12 STORY_03 specified this call but it was not added.

2. **`internal/provider/provider.go` — cascade suppression logic absent entirely.**
   Epic12 STORY_03 planned calls for `event=finding.suppressed.cascade` and
   `event=finding.suppressed.circuit_breaker` and `event=finding.suppressed.max_depth`,
   referencing lines 156, 191, and 219 respectively. None of those code paths exist in
   the current file. Either these features were never built or the code was removed. No
   audit calls can be added until the corresponding logic exists.

3. **`internal/provider/provider.go` — no `finding_detected` audit call.**
   The epic23 README decision-point table requires `event=finding_detected` immediately
   after a non-nil finding is returned from `ExtractFinding`. No such call exists anywhere
   in the file.

4. **`internal/provider/provider.go` — no `duplicate_fingerprint` audit call.**
   The epic23 README decision-point table requires `event=duplicate_fingerprint` when the
   dedup loop (lines 190–203) finds an existing non-Failed RemediationJob and suppresses
   creation. The current code returns `ctrl.Result{}, nil` at line 196 with no log.

5. **`internal/provider/provider.go` lines 211–220 — readiness gate log missing audit=true.**
   The `r.Log.Error(...)` call for a failed readiness check does not include
   `zap.Bool("audit", true)`. This is a suppression decision and should be auditable.

6. **`internal/controller/remediationjob_controller.go` lines 230–234 — dispatch log missing audit=true and event field.**
   The existing `r.Log.Info("dispatched agent job", ...)` call is missing
   `zap.Bool("audit", true)`, `zap.String("event", "job.dispatched")`, and
   `zap.String("fingerprint", rjob.Spec.Fingerprint[:12])`. Epic12 STORY_03 specified
   these fields but they were not added.

---

## Tasks

- [ ] **Gap 1:** Add `zap.Bool("audit", true)` + `event=finding.suppressed.stabilisation_window`
      + `reason=first_seen` to `internal/provider/provider.go` line 170 (first-seen path)
      and a separate call with `reason=window_open` at line 175 (still-within-window path)
- [ ] **Gap 2:** Defer — do not add cascade/circuit-breaker/max-depth audit calls until
      the corresponding suppression logic is implemented; create a note in the implementing
      epic's story
- [ ] **Gap 3:** Add `zap.Bool("audit", true)` + `event=finding_detected` call after
      `ExtractFinding` returns a non-nil finding in `internal/provider/provider.go`
      (after line 122), once the fingerprint is available (after line 159 — emit the call
      there so fingerprint can be included)
- [ ] **Gap 4:** Add `zap.Bool("audit", true)` + `event=duplicate_fingerprint` call at
      the early return at `internal/provider/provider.go` line 196, logging
      `fingerprint=fp[:12]` and `existingRemediationJob=rjob.Name`
- [ ] **Gap 5:** Add `zap.Bool("audit", true)` to the existing `r.Log.Error(...)` call
      at `internal/provider/provider.go` lines 211–220
- [ ] **Gap 6:** Add `zap.Bool("audit", true)`, `zap.String("event", "job.dispatched")`,
      and `zap.String("fingerprint", rjob.Spec.Fingerprint[:12])` to the existing
      `r.Log.Info("dispatched agent job", ...)` call at
      `internal/controller/remediationjob_controller.go` lines 230–234
- [ ] For each of epics 15–22: ensure the implementing story's Definition of Done
      explicitly requires audit log coverage for its new suppression/dispatch path

---

## Definition of Done

- [ ] All decision points in the table in epic23 README emit `audit=true` log lines
- [ ] Stabilisation window first-seen and window-open paths both emit audit lines
- [ ] Duplicate fingerprint dedup path emits an audit line
- [ ] Finding detected path emits an audit line (after fingerprint is computed)
- [ ] Readiness gate suppression path includes `audit=true`
- [ ] Dispatch log in `RemediationJobReconciler` includes `audit=true`, `event`, and `fingerprint`
- [ ] `go test -timeout 30s -race ./...` passes
