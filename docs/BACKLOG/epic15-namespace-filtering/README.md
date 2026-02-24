# Epic 15: Namespace Filtering

**Feature Tracker:** FT-A1
**Area:** Accuracy & Precision

## Purpose

Add `WATCH_NAMESPACES` and `EXCLUDE_NAMESPACES` environment variable support so that
mendabot ignores transient events from system namespaces (`kube-system`, `cert-manager`,
`monitoring`, `flux-system`) and only generates investigations for workload namespaces
selected by the operator.

Without this, mendabot triggers investigations for every cert-manager certificate rotation,
Flux reconciliation backoff, or monitoring stack restart — high-volume transient activity
that has zero actionable fix. This is the single largest source of noise on a real cluster.

## Status: Not Started

## Deep-Dive Findings (2026-02-23)

### Config (STORY_01)
- `internal/config/config.go` struct at line 13; `FromEnv` at line 34.
- The existing comma-split/trim/skip-empty pattern is established by `AgentWatchNamespaces`
  (line 29) parsed at lines 145–158. The two new fields follow the same pattern exactly.
- No cross-field validation needed. Both fields default to `nil` (not empty slice).
- Test insertion point: after `TestFromEnv_AgentWatchNamespacesWhitespaceOnly` (line 656
  of `config_test.go`).
- Deployment docs: two commented-out env var entries in
  `deploy/kustomize/deployment-watcher.yaml`.

### Reconciler Filter (STORY_02)
- `SourceProviderReconciler` at `internal/provider/provider.go:27`; `Cfg config.Config`
  field already at line 31 — no structural changes needed.
- Exact insertion point: **after** the `DetectInjection(finding.Details)` block
  (line 157) and **before** `domain.FindingFingerprint` (line 159).
- `domain.Finding.Namespace` is populated for namespaced resources (e.g. `pod.Namespace`
  at `pod.go:119`) and is `""` for Nodes (`node.go:102`).
- Cluster-scoped providers are exempted by `if finding.Namespace != ""` guard.
- Filter evaluation order: WatchNamespaces allowlist first, ExcludeNamespaces denylist second.
- Skipped findings logged at `Debug` level with nil-guard on `r.Log`.
- `main.go` and `SetupWithManager` require **no changes** — `r.Cfg` is already populated
  at construction time (`main.go:147: Cfg: cfg`).
- 9 table-driven test cases required (including `NSFilter_NodeProvider_Exempt` with
  `Namespace: ""`).

## Dependencies

- epic09-native-provider complete (`SourceProviderReconciler` in `internal/provider/provider.go`)
- epic00-foundation complete (`internal/config/config.go`)

## Blocks

- epic16 (annotation constants share `internal/domain/` — parallel once both are started)
- epic23 (new namespace-suppression path requires audit log coverage)

## Stories

| Story | File | Status |
|-------|------|--------|
| Config — WATCH_NAMESPACES and EXCLUDE_NAMESPACES env vars | [STORY_01_config.md](STORY_01_config.md) | Not Started |
| Reconciler — namespace filter gate in SourceProviderReconciler | [STORY_02_reconciler_filter.md](STORY_02_reconciler_filter.md) | Not Started |

## Implementation Order

```
STORY_01 (config) ──> STORY_02 (reconciler filter)
```

## Definition of Done

- [ ] `config.Config` gains `WatchNamespaces []string` and `ExcludeNamespaces []string`
- [ ] `config.FromEnv` parses both env vars (comma-separated); both default to nil
- [ ] `SourceProviderReconciler.Reconcile` rejects findings outside `WatchNamespaces` (when non-empty)
- [ ] `SourceProviderReconciler.Reconcile` rejects findings in `ExcludeNamespaces`
- [ ] `NodeProvider` is explicitly exempt from namespace filtering (cluster-scoped, `finding.Namespace == ""`)
- [ ] All unit and integration tests pass with `-race`
- [ ] `WATCH_NAMESPACES` and `EXCLUDE_NAMESPACES` documented in `deploy/kustomize/deployment-watcher.yaml` as commented-out env vars
- [ ] Worklog written
