# Story 01: Config — WATCH_NAMESPACES and EXCLUDE_NAMESPACES

**Epic:** [epic15-namespace-filtering](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want to configure mechanic with an allowlist
(`WATCH_NAMESPACES`) and a denylist (`EXCLUDE_NAMESPACES`) of namespaces, so that
investigation jobs are only generated for workload namespaces I care about and not
for system namespaces like `kube-system`, `cert-manager`, or `flux-system`.

---

## Background

`internal/config/config.go` defines `Config` (line 13) and `FromEnv` (line 34). All
configuration is read from environment variables at startup; no field may be changed
at runtime.

The existing pattern for optional multi-value fields is established by
`AgentWatchNamespaces []string` (line 29), which is parsed from
`AGENT_WATCH_NAMESPACES` (a comma-separated list) in the `scope == "namespace"` branch
(lines 145–158). The `FromEnv` parser uses `strings.Split(nsStr, ",")` followed by
`strings.TrimSpace` on each element and discards empty tokens.

Two new optional fields are needed:

| Env var | `Config` field | Type | Default |
|---|---|---|---|
| `WATCH_NAMESPACES` | `WatchNamespaces` | `[]string` | `nil` (allow all) |
| `EXCLUDE_NAMESPACES` | `ExcludeNamespaces` | `[]string` | `nil` (deny none) |

Both fields must default to `nil` (empty slice, not set) when the env var is absent or
empty. Neither is required: an empty `WATCH_NAMESPACES` means "watch all namespaces";
an empty `EXCLUDE_NAMESPACES` means "exclude none."

There is no cross-field validation required: an operator is permitted to set both
simultaneously (allowlist is evaluated first, denylist second — see STORY_02).

---

## Acceptance Criteria

- [ ] `config.Config` (in `internal/config/config.go`) gains two new fields:
  ```go
  WatchNamespaces   []string // WATCH_NAMESPACES — default nil (allow all)
  ExcludeNamespaces []string // EXCLUDE_NAMESPACES — default nil (deny none)
  ```
  Fields are placed after `AgentWatchNamespaces` to maintain declaration order.
- [ ] `config.FromEnv` parses `WATCH_NAMESPACES` using the same comma-split/trim/skip-empty
  pattern as `AgentWatchNamespaces` (lines 150–155 of `config.go`); when the env var is
  absent or contains only whitespace/commas, `cfg.WatchNamespaces` is `nil`.
- [ ] `config.FromEnv` parses `EXCLUDE_NAMESPACES` identically; `cfg.ExcludeNamespaces`
  is `nil` when absent or blank.
- [ ] Neither field is required: `FromEnv` does **not** return an error when either var
  is unset.
- [ ] `config_test.go` gains tests for all cases listed in the Test Cases section below.
- [ ] `go test -race ./internal/config/...` passes with no failures.

---

## Technical Implementation

### Field additions to `config.Config`

```go
// WatchNamespaces limits reconciliation to the listed namespaces.
// When nil (WATCH_NAMESPACES not set), all namespaces are watched.
// Cluster-scoped resources (e.g. Nodes) are always exempt from this filter.
WatchNamespaces []string // WATCH_NAMESPACES — default nil (allow all)

// ExcludeNamespaces suppresses RemediationJob creation for findings in these
// namespaces. Applied after WatchNamespaces. When nil (EXCLUDE_NAMESPACES not
// set), no namespaces are excluded.
// Cluster-scoped resources (e.g. Nodes) are always exempt from this filter.
ExcludeNamespaces []string // EXCLUDE_NAMESPACES — default nil (deny none)
```

### `FromEnv` parsing logic

Add after the existing `AgentWatchNamespaces` block (after line 158):

```go
if nsStr := os.Getenv("WATCH_NAMESPACES"); nsStr != "" {
    for _, ns := range strings.Split(nsStr, ",") {
        ns = strings.TrimSpace(ns)
        if ns != "" {
            cfg.WatchNamespaces = append(cfg.WatchNamespaces, ns)
        }
    }
}

if nsStr := os.Getenv("EXCLUDE_NAMESPACES"); nsStr != "" {
    for _, ns := range strings.Split(nsStr, ",") {
        ns = strings.TrimSpace(ns)
        if ns != "" {
            cfg.ExcludeNamespaces = append(cfg.ExcludeNamespaces, ns)
        }
    }
}
```

Note: unlike `AgentWatchNamespaces`, neither new field returns an error when the list
parses to zero entries — an empty result is silently treated as nil (the default).

### Test cases for `config_test.go`

Add the following tests, following the `setRequiredEnv(t)` helper pattern established
at line 250.

| Test function | Input | Expected |
|---|---|---|
| `TestFromEnv_WatchNamespacesDefault` | `WATCH_NAMESPACES` unset | `cfg.WatchNamespaces == nil` |
| `TestFromEnv_WatchNamespacesBlank` | `WATCH_NAMESPACES=""` | `cfg.WatchNamespaces == nil` |
| `TestFromEnv_WatchNamespacesSingle` | `WATCH_NAMESPACES="production"` | `cfg.WatchNamespaces == []string{"production"}` |
| `TestFromEnv_WatchNamespacesMultiple` | `WATCH_NAMESPACES="default,production,staging"` | `cfg.WatchNamespaces == []string{"default","production","staging"}` |
| `TestFromEnv_WatchNamespacesWhitespace` | `WATCH_NAMESPACES=" default , staging "` | `cfg.WatchNamespaces == []string{"default","staging"}` (trimmed) |
| `TestFromEnv_WatchNamespacesWhitespaceOnly` | `WATCH_NAMESPACES="  ,  "` | `cfg.WatchNamespaces == nil` |
| `TestFromEnv_ExcludeNamespacesDefault` | `EXCLUDE_NAMESPACES` unset | `cfg.ExcludeNamespaces == nil` |
| `TestFromEnv_ExcludeNamespacesBlank` | `EXCLUDE_NAMESPACES=""` | `cfg.ExcludeNamespaces == nil` |
| `TestFromEnv_ExcludeNamespacesSingle` | `EXCLUDE_NAMESPACES="kube-system"` | `cfg.ExcludeNamespaces == []string{"kube-system"}` |
| `TestFromEnv_ExcludeNamespacesMultiple` | `EXCLUDE_NAMESPACES="kube-system,cert-manager,flux-system"` | three elements in order |
| `TestFromEnv_ExcludeNamespacesWhitespaceOnly` | `EXCLUDE_NAMESPACES="  ,  "` | `cfg.ExcludeNamespaces == nil` |
| `TestFromEnv_BothFiltersCoexist` | `WATCH_NAMESPACES="production"` and `EXCLUDE_NAMESPACES="kube-system"` | both fields populated, no error |

---

## Tasks

- [ ] Read `internal/config/config_test.go` in full and identify the correct insertion
  point for new tests (after `TestFromEnv_AgentWatchNamespacesWhitespaceOnly`, line 656).
- [ ] Write all test cases from the table above in `internal/config/config_test.go` (TDD
  — run `go test ./internal/config/...` and confirm they fail before any code changes).
- [ ] Add `WatchNamespaces []string` and `ExcludeNamespaces []string` to `config.Config`
  in `internal/config/config.go` with inline doc comments.
- [ ] Add parsing blocks for `WATCH_NAMESPACES` and `EXCLUDE_NAMESPACES` in
  `config.FromEnv`, following the comma-split/trim/skip-empty pattern.
- [ ] Run `go test -race ./internal/config/...` — all tests must pass.
- [ ] Add commented-out env var documentation to
  `charts/mechanic/templates/deployment-watcher.yaml` (see Definition of Done).

---

## Dependencies

**Depends on:** epic00-foundation complete (`internal/config/config.go` exists and
`FromEnv` is the sole config entry point).

**Blocks:** STORY_02 (the reconciler filter reads `cfg.WatchNamespaces` and
`cfg.ExcludeNamespaces` from the `Config` struct passed to `SourceProviderReconciler.Cfg`).

---

## Definition of Done

- [x] `config.Config` has `WatchNamespaces []string` and `ExcludeNamespaces []string`
- [x] `config.FromEnv` parses both env vars; both default to `nil`
- [x] All new config tests pass with `-race`
- [x] Full test suite passes: `go test -timeout 120s -race ./...`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
- [x] `charts/mechanic/templates/deployment-watcher.yaml` has two commented-out env var entries:
  ```yaml
  # - name: WATCH_NAMESPACES
  #   value: ""  # comma-separated; empty = watch all namespaces
  # - name: EXCLUDE_NAMESPACES
  #   value: ""  # comma-separated; empty = exclude no namespaces
  ```
