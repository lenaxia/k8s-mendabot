# Worklog: README Accuracy Update

**Date:** 2026-03-02
**Session:** Bring README-LLM.md and README.md up to date with the current state of the codebase
**Status:** Complete

---

## Objective

Ensure `README-LLM.md` and `README.md` accurately reflect the current project state,
including all packages, correct feature statuses, metrics system description, and
deferred epics — so future AI-assisted sessions start from correct context.

---

## Work Completed

### 1. README-LLM.md — directory tree

The tree was missing entire packages that exist on disk. Updated to include:

- `internal/circuitbreaker/` — self-remediation cascade circuit breaker
- `internal/correlator/` — multi-signal correlation engine
- `internal/domain/` — expanded from 2 files to full list (annotations, correlation,
  delimiter, injection, interfaces, provider, redact, severity, sink)
- `internal/github/` — GitHub App token exchange
- `internal/metrics/` — all custom Prometheus metrics
- `internal/provider/bounded_map.go` — fixed-capacity LRU dedup map
- `internal/provider/native/` — all 7 native providers + parent/truncate helpers
- `internal/provider/export_test.go` — black-box test helpers
- `internal/readiness/llm/` — bedrock (stub), openai, vertex (stub) readiness checkers
- `internal/readiness/sink/github.go` — GitHub sink readiness checker
- `internal/sink/github/` — closer.go + merge_checker.go
- `internal/testutil/` — fake event helpers + recorder

Removed stale `internal/provider/k8sgpt/` subtree (replaced by native provider).

### 2. README-LLM.md — epic statuses

- Epic 26 (auto-close-resolved): corrected from `not started` to `complete`
- Epic 27 (pr-feedback-iteration): corrected from `not started` to `deferred`
- Epic 28 (manual-trigger): corrected from `not started` to `deferred`

### 3. README-LLM.md — Metrics System section

Added a new "Metrics System" section before "Technology Stack" documenting:

- Registration mechanism (`ctrlmetrics.Registry` via `init()`)
- All 8 metrics with type, labels, and description
- Suppression reason and job outcome constants
- Helm metrics gate behaviour (watcher always binds `:8080/metrics`; flag only controls Service)
- `BoundedMap` description

### 4. README-LLM.md — version and date

- Version bumped to 1.2
- Last Updated set to 2026-03-02
- Version History table updated with entry for 1.2

### 5. README.md — Roadmap

- Multi-signal correlation: `Planned` → `Deferred`
- Added PR feedback iteration row: `Deferred`
- Added on-demand manual trigger row: `Deferred`

---

## Key Decisions

- `README-LLM.md` directory tree now mirrors the actual `internal/` layout exactly.
  Future sessions must keep these in sync when adding packages.
- Bedrock and Vertex readiness checkers are noted as non-functional stubs in the tree
  comments — this is intentional so future agents don't assume they work.

---

## Blockers

None.

---

## Tests Run

No code was changed in this session — documentation only. Full test suite was passing
at the end of the previous session.

---

## Next Steps

No immediate next steps. The next session should:
1. Read `README-LLM.md` (this file) — now accurate as of 2026-03-02
2. Check `docs/WORKLOGS/` for the latest entries
3. Review `docs/BACKLOG/FEATURE_TRACKER.md` for the next epic to implement

---

## Files Modified

- `README-LLM.md`
- `README.md`
- `docs/WORKLOGS/0106_2026-03-02_readme-accuracy-update.md` (this file)
