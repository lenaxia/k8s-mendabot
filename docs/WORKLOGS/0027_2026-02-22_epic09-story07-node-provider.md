# Worklog: Epic 09 STORY_07 — NodeProvider

**Date:** 2026-02-22
**Session:** Implement NodeProvider with condition-based failure detection
**Status:** Complete

---

## Objective

Implement `internal/provider/native/node.go` — the `NodeProvider` that detects unhealthy
Kubernetes nodes directly from `node.status.conditions`, without requiring k8sgpt-operator.

---

## Work Completed

### 1. Test file written first (TDD)

- Created `internal/provider/native/node_test.go` with 16 test cases covering all
  acceptance criteria from STORY_07.
- Confirmed tests fail before implementation (compile error: `undefined: NewNodeProvider`).

### 2. Implementation

- Created `internal/provider/native/node.go` implementing `domain.SourceProvider`.
- `nodeProvider` struct holds a `client.Client` field (passed to `getParent` for
  consistency with other providers; nodes have no ownerReferences so client is never used).
- `NewNodeProvider(c client.Client)` constructor — panics if `c` is nil.
- Compile-time assertion: `var _ domain.SourceProvider = (*nodeProvider)(nil)`.
- `ProviderName()` returns `"native"`.
- `ObjectType()` returns `&corev1.Node{}`.
- `ExtractFinding` checks conditions using a switch on condition type:
  - `NodeReady` == False or Unknown → failure
  - `NodeMemoryPressure`, `NodeDiskPressure`, `NodePIDPressure`, `NodeNetworkUnavailable`
    == True → failure
  - `EtcdIsVoter` (k3s) explicitly ignored via `ignoredNodeConditions` map
- Error text format: `"node <name> has condition <Type> (<Reason>): <Message>"`
- Single finding with all bad conditions in errors array (OR logic, one entry per condition).
- `ParentObject` = `"Node/<name>"` (via `getParent`, nodes have no ownerRefs).
- `Namespace` = `""` (cluster-scoped resource).
- `SourceRef.APIVersion` = `"v1"`, `Kind` = `"Node"`.

### 3. Test bug fixed

Discovered ordering bug in three pressure-condition tests: `append` before `filterNodeConditions`
caused the filter to remove the just-appended True condition. Fixed by filtering first, then appending.

---

## Key Decisions

- `EtcdIsVoter` is excluded via an explicit `ignoredNodeConditions` map rather than an inline
  comment, making it easy to add future exceptions (e.g. other k3s-specific conditions).
- Standard condition set is checked via a `switch` case, not a set lookup, so the behaviour
  for each type is clear and type-safe.
- No taint inspection — conditions are the authoritative source per STORY_07 spec.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race -run TestNodeProvider ./internal/provider/native/ -v
# All 16 tests PASS

go test -timeout 60s -race ./...
# All packages PASS

go build ./...  # clean
go vet ./...    # clean
```

---

## Next Steps

STORY_08 (main wiring) — wire all native providers into `cmd/watcher/main.go`. Depends on
STORY_04–07 and STORY_10–11 being complete. Check backlog for which providers remain.

---

## Files Modified

- `internal/provider/native/node.go` — created
- `internal/provider/native/node_test.go` — created
- `docs/BACKLOG/epic09-native-provider/STORY_07_node_provider.md` — status updated
- `docs/WORKLOGS/0027_2026-02-22_epic09-story07-node-provider.md` — created
- `docs/WORKLOGS/README.md` — index updated
