# Epic 23: Structured Audit Log

**Feature:** FT-S3 — Structured audit log for all remediation decisions
**Branch:** `feature/epic23-structured-audit-log`
**Status:** Complete

---

## Goal

Every key decision point in `SourceProviderReconciler` and `RemediationJobReconciler`
must emit a structured zap log line with `zap.Bool("audit", true)` and a stable
`zap.String("event", "<name>")` field. Security teams can then filter on `audit=true`
in any log aggregator (Loki, Elasticsearch, Datadog) to reconstruct a complete
decision audit trail.

---

## Background

Epic 12 (STORY_03) delivered an initial set of audit log lines. After cross-epic
validation rounds (epics 17, 12, 9), four gaps remain in `internal/provider/provider.go`:

| # | Gap | Severity |
|---|-----|----------|
| 1 | `finding.detected` event missing entirely | High |
| 2 | Stabilisation window first-seen path has no `audit=true` | High |
| 3 | Stabilisation window still-open path has no `audit=true` | High |
| 4 | Dedup default case is `Debug` with no `audit=true` | High |

Note: cascade and self-remediation logic has been removed from the codebase. Those
deferred gaps no longer apply.

---

## Current Audit Coverage

### provider.go — confirmed present

| Event | Line |
|-------|------|
| `remediationjob.cancelled` | ~105 |
| `finding.injection_detected` | ~137 |
| `finding.injection_detected_in_details` | ~152 |
| `remediationjob.permanently_failed_suppressed` | ~218 |
| `readiness.check_failed` (audit=true) | ~250 |
| `remediationjob.created` | ~314 |

### controller.go — confirmed present

| Event | Line |
|-------|------|
| `remediationjob.deleted_ttl` | ~87 |
| `job.succeeded` | ~174 |
| `job.permanently_failed` | ~183 |
| `job.failed` | ~191 |
| `job.dispatched` | ~292 |

### Gaps (to fix in this epic)

| Event | Location | Issue |
|-------|----------|-------|
| `finding.detected` | provider.go after fp computed | entirely missing |
| `finding.suppressed.stabilisation_window` (first_seen) | provider.go ~177 | no `audit=true` |
| `finding.suppressed.stabilisation_window` (window_open) | provider.go ~188 | no `audit=true` |
| `finding.suppressed.duplicate` | provider.go ~233 | `Debug` level, no `audit=true` |
| `readiness.check_failed` | provider.go ~250 | missing `event` field string |

---

## Stories

| Story | Description | Status |
|-------|-------------|--------|
| [STORY_01](STORY_01_provider_audit_gaps.md) | Fix 4 gaps in provider.go | Complete |

---

## Definition of Done

- [x] All 4 gaps fixed in `internal/provider/provider.go`
- [x] `go test -timeout 30s -race ./...` passes
- [x] Code reviewed with zero gaps
- [x] Worklog written and index updated
