# Story 02: Reconciler — Namespace Filter Gate in SourceProviderReconciler

**Epic:** [epic15-namespace-filtering](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator**, I want the mechanic reconciler to skip findings from
namespaces that are outside my configured allowlist (`WATCH_NAMESPACES`) or inside my
denylist (`EXCLUDE_NAMESPACES`), so that high-volume system namespace noise never
reaches the `RemediationJob` creation path.

---

## Background

`SourceProviderReconciler` in `internal/provider/provider.go` (line 27) is the
controller-runtime reconciler that wraps every `domain.SourceProvider`. Its `Reconcile`
method (line 60) is the single place where the decision to create a `RemediationJob` is
made. The field `Cfg config.Config` (line 31) is already a struct field on
`SourceProviderReconciler`, so `r.Cfg.WatchNamespaces` and `r.Cfg.ExcludeNamespaces`
(added in STORY_01) are immediately accessible inside `Reconcile` without any structural
changes.

### Exact integration point

After `ExtractFinding` returns a non-nil finding (lines 118–125), and after both
injection detection blocks (lines 127–157), the reconciler computes the fingerprint
(line 159) and then enters the stabilisation window logic (line 167). The namespace
filter check must be inserted **immediately after the injection detection blocks and
before the fingerprint computation** — i.e. between line 157 and line 159 in the
current file. This placement ensures:

1. The finding has been fully validated (not nil, not injected).
2. No wasted work is done computing a fingerprint for a finding that will be silently
   dropped.
3. The guard is a simple early return `ctrl.Result{}, nil` — the same pattern used by
   the injection suppress path (lines 139, 155).

### Namespace available on the finding

`domain.Finding.Namespace` (defined in `internal/domain/provider.go`, line 97) carries
the namespace of the affected resource. For namespaced objects like Pods
(`internal/provider/native/pod.go`, line 119: `Namespace: pod.Namespace`) the field is
populated by the provider. For cluster-scoped objects like Nodes
(`internal/provider/native/node.go`, line 102: `Namespace: ""`), `finding.Namespace`
is always the empty string `""`.

### NodeProvider exemption

`nodeProvider.ExtractFinding` always sets `finding.Namespace = ""` (node.go line 102).
The filter logic must not apply to cluster-scoped resources. The correct guard is:

```go
if finding.Namespace != "" {
    // apply WatchNamespaces and ExcludeNamespaces checks
}
```

This is a single branch guard — if `finding.Namespace` is empty (cluster-scoped), the
entire filter is bypassed unconditionally.

### Filter check order

When `finding.Namespace != ""`:

1. **WatchNamespaces check first:** if `r.Cfg.WatchNamespaces` is non-empty, the
   finding's namespace must be present in the list. If it is not found, return
   `ctrl.Result{}, nil` (silently skip).
2. **ExcludeNamespaces check second:** if `r.Cfg.ExcludeNamespaces` is non-empty, the
   finding's namespace must **not** be present. If it is found in the list, return
   `ctrl.Result{}, nil` (silently skip).

This order is significant: `WatchNamespaces` acts as an explicit allowlist that gates
before the denylist. An operator who sets both `WATCH_NAMESPACES=production` and
`EXCLUDE_NAMESPACES=production` would have every finding from `production` pass the
allowlist check and then be denied by the denylist — an arguably degenerate config, but
the ordered evaluation is consistent and predictable.

### Logging

Silently-dropped findings must be logged at `Debug` level (not `Info` or `Warn`) to
avoid log spam in normal operation. Each log call should carry the structured fields
`provider`, `namespace`, `kind`, and `name` so that operators can diagnose mismatches
at elevated log levels. When `r.Log == nil` (test scenarios that omit the logger), the
log call must be guarded.

---

## Acceptance Criteria

- [ ] The namespace filter block is inserted between the injection detection blocks and
  the `domain.FindingFingerprint` call in `SourceProviderReconciler.Reconcile`
  (`internal/provider/provider.go`).
- [ ] The filter is entirely guarded by `if finding.Namespace != ""` so that
  cluster-scoped providers (Node) are always exempt.
- [ ] When `r.Cfg.WatchNamespaces` is non-empty and `finding.Namespace` is not in the
  list, the reconciler returns `ctrl.Result{}, nil` without creating a `RemediationJob`.
- [ ] When `r.Cfg.ExcludeNamespaces` is non-empty and `finding.Namespace` is in the
  list, the reconciler returns `ctrl.Result{}, nil` without creating a `RemediationJob`.
- [ ] When both `r.Cfg.WatchNamespaces` and `r.Cfg.ExcludeNamespaces` are nil (the
  default), the filter block is a no-op and all existing behaviour is unchanged.
- [ ] Skipped findings are logged at `Debug` level (with `r.Log` nil-guard).
- [ ] All new reconciler tests are table-driven and pass with `-race`.
- [ ] Full test suite passes: `go test -timeout 120s -race ./...`

---

## Technical Implementation

### Filter block to insert in `Reconcile`

Insert after line 157 (end of `domain.DetectInjection(finding.Details)` block), before
the `fp, err := domain.FindingFingerprint(finding)` call at line 159:

```go
// Namespace filter: skip findings from namespaces that are outside the
// configured allowlist (WatchNamespaces) or inside the denylist
// (ExcludeNamespaces). Cluster-scoped providers (e.g. NodeProvider) always
// set finding.Namespace = "" and are unconditionally exempt.
if finding.Namespace != "" {
    if len(r.Cfg.WatchNamespaces) > 0 {
        allowed := false
        for _, ns := range r.Cfg.WatchNamespaces {
            if ns == finding.Namespace {
                allowed = true
                break
            }
        }
        if !allowed {
            if r.Log != nil {
                r.Log.Debug("namespace filter: skipping finding (not in WatchNamespaces)",
                    zap.String("provider", r.Provider.ProviderName()),
                    zap.String("namespace", finding.Namespace),
                    zap.String("kind", finding.Kind),
                    zap.String("name", finding.Name),
                )
            }
            return ctrl.Result{}, nil
        }
    }
    for _, ns := range r.Cfg.ExcludeNamespaces {
        if ns == finding.Namespace {
            if r.Log != nil {
                r.Log.Debug("namespace filter: skipping finding (in ExcludeNamespaces)",
                    zap.String("provider", r.Provider.ProviderName()),
                    zap.String("namespace", finding.Namespace),
                    zap.String("kind", finding.Kind),
                    zap.String("name", finding.Name),
                )
            }
            return ctrl.Result{}, nil
        }
    }
}
```

No changes are required to `SourceProviderReconciler`'s struct fields, constructor,
`SetupWithManager`, or `main.go` — `r.Cfg` is already populated at construction time
(see `cmd/watcher/main.go` line 147: `Cfg: cfg`).

---

## Test Cases

All cases are table-driven and belong in `internal/provider/provider_test.go`. The
existing test infrastructure already constructs a `SourceProviderReconciler` with a
fake client; new test rows follow the same shape.

For each test, the reconciler is constructed with a pod finding in namespace
`"production"`, and the test varies `Cfg.WatchNamespaces` and `Cfg.ExcludeNamespaces`.

| Test name | `WatchNamespaces` | `ExcludeNamespaces` | `finding.Namespace` | Expected |
|---|---|---|---|---|
| `NSFilter_WatchEmpty_AllowAll` | `nil` | `nil` | `"production"` | `RemediationJob` created (no filter) |
| `NSFilter_WatchListMatch_Allowed` | `["production"]` | `nil` | `"production"` | `RemediationJob` created |
| `NSFilter_WatchListNoMatch_Skipped` | `["staging"]` | `nil` | `"production"` | no `RemediationJob`; result is `{}` |
| `NSFilter_WatchListMultiMatch` | `["staging","production"]` | `nil` | `"production"` | `RemediationJob` created |
| `NSFilter_ExcludeMatch_Skipped` | `nil` | `["production"]` | `"production"` | no `RemediationJob`; result is `{}` |
| `NSFilter_ExcludeNoMatch_Allowed` | `nil` | `["kube-system"]` | `"production"` | `RemediationJob` created |
| `NSFilter_BothSet_WatchPassExcludeBlock` | `["production"]` | `["production"]` | `"production"` | no `RemediationJob` (WatchNamespaces passes, ExcludeNamespaces blocks) |
| `NSFilter_BothSet_WatchBlockShortCircuits` | `["staging"]` | `["kube-system"]` | `"production"` | no `RemediationJob` (WatchNamespaces blocks before ExcludeNamespaces is reached) |
| `NSFilter_NodeProvider_Exempt` | `["staging"]` | `["default"]` | `""` | `RemediationJob` created (cluster-scoped; namespace filter bypassed) |

The `NSFilter_NodeProvider_Exempt` test must use a finding with `Namespace: ""` to
simulate a Node finding and confirm the filter block is skipped entirely.

---

## Tasks

- [ ] Read `internal/provider/provider_test.go` in full to identify the existing test
  helper pattern for constructing `SourceProviderReconciler` with a fake client.
- [ ] Write all nine table-driven test cases above in `internal/provider/provider_test.go`
  (TDD — confirm they fail before implementation).
- [ ] Insert the namespace filter block in `SourceProviderReconciler.Reconcile`
  (`internal/provider/provider.go`) at the location described above (after the
  `DetectInjection(finding.Details)` block, before `FindingFingerprint`).
- [ ] Run `go test -race ./internal/provider/...` — all tests must pass.
- [ ] Run full test suite: `go test -timeout 120s -race ./...`
- [ ] Run `go vet ./...` — clean.

---

## Dependencies

**Depends on:** STORY_01 — `config.Config` must have `WatchNamespaces []string` and
`ExcludeNamespaces []string` before this story can be implemented.

**Depends on:** epic09-native-provider complete — `SourceProviderReconciler` and its
`Cfg config.Config` field (line 31 of `internal/provider/provider.go`) must exist.

**Blocks:** Nothing.

---

## Definition of Done

- [x] Namespace filter block inserted in `Reconcile` at the correct location
- [x] `NodeProvider` (cluster-scoped, `finding.Namespace == ""`) is unconditionally exempt
- [x] `WatchNamespaces` checked before `ExcludeNamespaces`
- [x] Skipped findings logged at `Debug` level with nil-guard on `r.Log`
- [x] All nine table-driven tests pass with `-race`
- [x] Full test suite passes: `go test -timeout 120s -race ./...`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
