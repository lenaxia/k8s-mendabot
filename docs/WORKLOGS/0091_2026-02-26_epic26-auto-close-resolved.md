# Worklog 0091 — Epic 26: Auto-Close Resolved Findings

**Date:** 2026-02-26
**Branch:** `feature/epic26-auto-close-resolved`
**Version:** v0.3.24 → v0.3.25

## Summary

Implemented Epic 26 end-to-end across 6 stories. When a Kubernetes finding clears
(deployment recovers, PVC is provisioned, node returns Ready), the watcher now
automatically closes the GitHub PR that the agent opened — both for in-flight jobs
(Pending/Dispatched/Running) and for already-succeeded jobs whose PR became stale after
the cluster self-healed.

## Motivation

Production clusters with frequent transient failures accumulated a backlog of stale PRs
that obscured genuinely actionable ones. Humans were closing them manually. The
`PhaseSucceeded` path (job already done, finding later clears) was the most common
scenario and was entirely unhandled.

## Changes

### STORY_00 — SinkRef domain type + SinkCloser interface
- Added `SinkRef` struct to `api/v1alpha1/remediationjob_types.go` with fields
  `Type`, `URL`, `Number`, `Repo`
- Added `SinkCloser` interface to `internal/domain/sink.go`
- Added `NoopSinkCloser` for use when `PR_AUTO_CLOSE=false`
- Updated `DeepCopyInto` in `zz_generated.deepcopy.go`
- Added `sinkRef` to CRD testdata YAML

### STORY_01 — GitHub App token provider
- `internal/github/token_provider.go` — RS256 JWT → installation token exchange
- 55-minute in-memory cache (refreshes before the 1-hour GitHub App expiry)
- Mock HTTP server tests covering cache hit, cache miss, and error paths

### STORY_02 — GitHubSinkCloser (REST API)
- `internal/sink/github/closer.go` — calls GitHub REST API directly (`net/http`)
- No `gh` CLI subprocess — watcher container does not have `gh` installed
- `postComment` posts a human-readable closure reason; failure is a warning (non-fatal)
- `closeItem` closes the PR/issue; HTTP 422 treated as success (already closed)
- Validates `SinkRef.Number > 0` and `SinkRef.Repo != ""` before any API call

### STORY_03 — Wire SinkCloser into SourceProviderReconciler
- **Path A** — finding cleared, in-flight jobs: calls `SinkCloser.Close` before
  cancelling Pending/Dispatched/Running/Suppressed jobs that have a non-empty `SinkRef.URL`
- **Path B** — finding cleared, already-Succeeded jobs: `autoCloseSucceededSinks()`
  helper iterates `PhaseSucceeded` rjobs matching the same `SourceResultRef` and closes
  their sinks; does NOT delete the rjob (it remains as the dedup tombstone)
- `SinkCloser` injected as a field on `SourceProviderReconciler`; closure failure is
  logged and never blocks cancellation or deletion

### STORY_04 — Helm chart + config
- `internal/config/config.go` — added `PRAutoClose bool` (default `true`)
- `charts/mechanic/values.yaml` — added `prAutoClose: true`
- `charts/mechanic/templates/deployment-watcher.yaml` — added `PR_AUTO_CLOSE` env var
  and `envFrom` for GitHub App Secret; Secret is **required** (no `optional: true`)
- `charts/mechanic/values.schema.json` — added `prAutoClose` boolean field
- `cmd/watcher/main.go` — constructs `GitHubAppTokenProvider` + `GitHubSinkCloser`
  when `PRAutoClose=true`; falls back to `NoopSinkCloser` otherwise

### STORY_05 — Agent: write SinkRef after gh pr create
- Added STEP 9 to `charts/mechanic/files/prompts/core.txt`
- Agent captures `PR_URL` and `PR_NUMBER` from `gh pr create --json url,number` output
  (or `gh pr view` if finding an existing PR in STEP 1)
- Agent runs `kubectl patch remediationjob --subresource=status` to populate
  `status.sinkRef` and `status.prRef`; patch failure is a warning, never aborts the job
- Agent RBAC already had `remediationjobs/status` with `patch` verb (verified in
  `charts/mechanic/templates/role-agent.yaml`)

## Test Coverage

All new code covered by unit tests with mock HTTP servers. No fake environment or
cluster required.

```
ok  github.com/lenaxia/k8s-mechanic/internal/github       (cached)
ok  github.com/lenaxia/k8s-mechanic/internal/sink/github  (cached)
ok  github.com/lenaxia/k8s-mechanic/internal/provider     9.917s
ok  github.com/lenaxia/k8s-mechanic/internal/config       1.210s
```

Full suite: `go test -timeout 30s -race ./...` — all pass.

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| REST API not `gh` CLI | `gh` is absent from the watcher container |
| 422 = success | GitHub returns 422 on already-closed PR; idempotency is required |
| `postComment` failure is non-fatal | PR closure must not be gated on comment success |
| `PhaseSucceeded` rjobs NOT deleted after auto-close | They serve as dedup tombstones |
| Secret mount is required (not optional) | Pod fails fast at startup if Secret is missing |
| `PR_AUTO_CLOSE=false` → `NoopSinkCloser` | Binary opt-out at operator level |
| Agent writes SinkRef via prompt STEP 9 | Agent calls `gh` as AI tool; no shell hook available |

## Backwards Compatibility

Old `RemediationJob` objects (pre-epic26) have `status.prRef` set but
`status.sinkRef` empty. The auto-close logic checks `SinkRef.URL != ""` — old objects
are silently skipped. Migration is natural: old rjobs expire via TTL; new ones populate
`SinkRef`.

## Verification (post-deploy)

```bash
# After agent completes on a new finding:
kubectl get rjob -n mechanic mechanic-<fp12> -o jsonpath='{.status.sinkRef}'

# After the finding resolves, within a few reconcile cycles the PR should be closed.
# Check for auto-close comment on the PR.
```

## Commits

- `88b070e` STORY_00 — SinkRef domain type + SinkCloser interface
- `41711e5` STORY_01 — GitHub App token provider
- `e6903cc` STORY_02 — GitHubSinkCloser REST API implementation
- `068847a` STORY_03 — wire SinkCloser into SourceProviderReconciler
- `b98edd7` STORY_04 — Helm chart Secret mount + main.go SinkCloser wiring
- `9aa90d5` STORY_05 — agent writes SinkRef after gh pr create
