# Worklog: Epic 13 Design Review — 4 Issues Fixed

**Date:** 2026-02-23
**Session:** Fix 4 issues identified in external design review of epic13 correlation implementation
**Status:** Complete

---

## Objective

An external design review identified 1 bug, 1 architectural concern, 1 nondeterminism
issue, and 1 minor correctness issue in the epic13 correlation implementation. Fix all
four and restore deterministic `-count=3` test runs.

---

## Work Completed

### 1. Bug — `FINDING_CORRELATED_FINDINGS` threshold off-by-one (`internal/jobbuilder/job.go`)

The guard `len(correlatedFindings) > 1` was semantically wrong. The intent is "inject if
any correlated findings exist." The current rules always produce ≥ 2 entries in
`AllFindings` so the bug had no practical effect, but a future rule producing a
single-entry group would silently fail to inject the env var.

**Fix:** Changed `> 1` to `> 0`.
**Test:** Renamed `TestBuild_SingleElementSlice_NoCorrelatedEnvVar` →
`TestBuild_SingleElementSlice_SetsCorrelatedEnvVar` and inverted the assertion. The
nil-slice test continues to assert no env var is set.

### 2. Nondeterminism — `MultiPodSameNodeRule` on simultaneous multi-node failures (`internal/correlator/rules.go`)

When two nodes simultaneously hit the threshold, Go map iteration over the `nodePods`
map was random — the "winning" node and its primary pod could flip between reconcile
cycles.

**Fix:** After identifying all qualifying nodes, collect them into `[]string`, call
`sort.Strings()`, and always select index 0 (lexicographically first node name).
**Test:** `TestMultiPodSameNodeRule_TwoNodesBothAtThreshold_DeterministicWinner` — 3 pods
on `node-aaa`, 3 on `node-zzz`, threshold=3; run 5 times confirming `node-aaa` always wins.

### 3. Minor — `SameNamespaceParentRule` prefix over-matching (`internal/correlator/rules.go`)

`strings.HasPrefix("application", "app")` is true — two unrelated apps with a string
prefix relationship would be spuriously correlated.

**Fix:** Introduced `isParentPrefix(a, b string) bool` helper returning
`a == b || strings.HasPrefix(a, b+"-")`. This requires a dash separator, preventing
`"app"` / `"application"` false positives. Added a code comment documenting the
known remaining limitation: sibling apps sharing a dash-prefix (e.g. `cert-manager` /
`cert-manager-cainjector`) still match.
**Test:** `TestSameNamespaceParentRule_NoSpuriousPrefixMatch` — `"app"` and
`"application"` do not correlate.

### 4. Design documentation — `SameNamespaceParentRule` scope clarification

The rule is designed for cross-provider correlation. In single-provider deployments,
same-provider findings for the same parent are fingerprint-deduplicated before the
correlator runs — the rule rarely fires in practice for those scenarios.

**Fix:** Added a doc comment to the `SameNamespaceParentRule` struct in `rules.go`
and a note to `STORY_01_builtin_rules.md`. No logic changed.

### 5. TC01 timing fragility fix (`internal/controller/correlation_integration_test.go`)

Under `-count=3` CPU load, TC01's window-hold assertion (`RequeueAfter > 0`) flaked
because the 1-second window had already elapsed by the time the reconcile ran. Changed
the assertion to: if `RequeueAfter > 0` hold the window; if `RequeueAfter == 0` the
window already elapsed and the dispatch happened inline — fall through to the phase
assertion. This makes TC01 correct under both fast and slow environments.

---

## Key Decisions

`isParentPrefix` still allows `cert-manager`/`cert-manager-cainjector` correlation. A
fully correct fix would require knowing the Kubernetes resource ownership graph — not
available to a pure string-matching rule. The limitation is documented. Operators with
predictable naming can disable `SameNamespaceParentRule` by setting
`DISABLE_CORRELATION=true` or by not deploying it (rule list is configurable in
`buildCorrelator()`).

---

## Tests Run

```
go build ./...                                             → CLEAN
go vet ./...                                               → CLEAN
go test -count=1 -timeout 90s -race ./...                 → 17/17 PASS
go test -count=3 -timeout 300s -race ./internal/controller → PASS (31s)
```

---

## Blockers

None.

---

## Next Steps

Epic 13 and 14 are fully reviewed, fixed, and validated. The next session should:
1. Merge `feature/epic13-multi-signal-correlation` to `main`
2. Tag `v0.4.0`
3. Start the next epic (FT-A1 namespace filtering, FT-A2 annotation opt-out, or FT-R1
   dead-letter queue — all ★★★/● from FEATURE_TRACKER.md)

---

## Files Modified

- `internal/jobbuilder/job.go` — `> 1` → `> 0`
- `internal/jobbuilder/job_test.go` — inverted single-element test
- `internal/correlator/rules.go` — deterministic node selection, `isParentPrefix` helper, doc comment
- `internal/correlator/rules_test.go` — 2 new tests
- `internal/controller/correlation_integration_test.go` — TC01 timing-resilient assertion
- `docs/BACKLOG/epic13-multi-signal-correlation/STORY_01_builtin_rules.md` — scope note added
