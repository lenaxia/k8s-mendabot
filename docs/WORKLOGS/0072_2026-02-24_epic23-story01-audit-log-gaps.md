# Worklog: Epic 23 STORY_01 — Audit Log Gaps Fixed in provider.go

**Date:** 2026-02-24
**Session:** Fix 5 audit log gaps in provider.go so every decision point emits audit=true + stable event name
**Status:** Complete

---

## Objective

Fix all remaining audit log gaps in `internal/provider/provider.go` so that every key
decision point in `SourceProviderReconciler` emits a structured zap log line with
`zap.Bool("audit", true)` and a stable `zap.String("event", "<name>")` field. Security
teams can then filter on `audit=true` in any log aggregator to reconstruct the complete
remediation decision audit trail.

---

## Work Completed

### 1. finding.detected — new audit log added

After `ExtractFinding` returns a non-nil finding and the fingerprint is computed (after
the `len(fp) < 12` guard), added a new `Info` log:

- `event=finding.detected`
- Fields: `audit`, `event`, `provider`, `kind`, `namespace`, `name`, `fingerprint`

This was the most critical gap — the detection event had no trace at all.

### 2. finding.suppressed.stabilisation_window (reason=first_seen) — audit=true added

Replaced the existing `Info` log at the first-seen stabilisation window path with a
structured log including `audit=true`, stable `event=finding.suppressed.stabilisation_window`,
and `reason=first_seen`.

### 3. finding.suppressed.stabilisation_window (reason=window_open) — audit=true added

Replaced the existing `Info` log at the still-within-window stabilisation path with a
structured log including `audit=true`, stable `event=finding.suppressed.stabilisation_window`,
and `reason=window_open`.

### 4. finding.suppressed.duplicate — Debug promoted to Info with audit=true

The dedup default case was logging at `Debug` level with no `audit=true` and no stable
event name. Promoted to `Info`, added `audit=true`, and stable `event=finding.suppressed.duplicate`.

### 5. readiness.check_failed — missing event field added

The existing `readiness.check_failed` log was missing the `event` field entirely. Added
`zap.String("event", "readiness.check_failed")` to complete the structured log line.

### 6. Backlog documents created

- `docs/BACKLOG/epic23-structured-audit-log/README.md`
- `docs/BACKLOG/epic23-structured-audit-log/STORY_01_provider_audit_gaps.md`

---

## Key Decisions

- All changes are log-only — no logic was modified. This was deliberate: epic23 is purely
  about audit observability, not behavioural changes.
- The `Debug` → `Info` promotion for `finding.suppressed.duplicate` was intentional:
  suppression decisions are security-relevant and must appear at `Info` level so they
  are captured in production log pipelines that filter out `Debug`.
- `readiness.check_failed` was counted as gap #5 (noted in the context as an `event`
  field gap, not a new log) and fixed as part of this story.

---

## Blockers

None.

---

## Tests Run

```
go test -count=1 -timeout 30s -race ./...
```

All 12 packages pass:

```
ok  github.com/lenaxia/k8s-mechanic/api/v1alpha1             1.243s
ok  github.com/lenaxia/k8s-mechanic/cmd/watcher              1.285s
ok  github.com/lenaxia/k8s-mechanic/internal/config          1.060s
ok  github.com/lenaxia/k8s-mechanic/internal/controller      12.593s
ok  github.com/lenaxia/k8s-mechanic/internal/domain          1.312s
ok  github.com/lenaxia/k8s-mechanic/internal/jobbuilder      1.235s
ok  github.com/lenaxia/k8s-mechanic/internal/logging         1.069s
ok  github.com/lenaxia/k8s-mechanic/internal/provider        12.173s
ok  github.com/lenaxia/k8s-mechanic/internal/provider/native 1.684s
ok  github.com/lenaxia/k8s-mechanic/internal/readiness       1.097s
ok  github.com/lenaxia/k8s-mechanic/internal/readiness/llm   1.654s
ok  github.com/lenaxia/k8s-mechanic/internal/readiness/sink  1.524s
```

---

## Next Steps

Epic 23 STORY_01 is complete. Epic 23 has only one story — the epic is ready to merge
to `main` via PR #7. No further implementation work required for epic 23.

---

## Files Modified

- `internal/provider/provider.go` — 5 audit log fixes
- `docs/BACKLOG/epic23-structured-audit-log/README.md` — epic backlog created
- `docs/BACKLOG/epic23-structured-audit-log/STORY_01_provider_audit_gaps.md` — story created
