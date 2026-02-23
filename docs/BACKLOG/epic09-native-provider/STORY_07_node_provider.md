# Story: NodeProvider

**Epic:** [epic09-native-provider](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want mendabot to detect unhealthy nodes directly from cluster
state so that NotReady nodes and abnormal node conditions trigger a remediation Job without
requiring k8sgpt-operator.

---

## Acceptance Criteria

- [x] `nodeProvider` struct defined in `internal/provider/native/node.go`
  (unexported; exported constructor `NewNodeProvider(c client.Client) *nodeProvider`
  in same file; panics if `c == nil`)
- [x] Compile-time assertion `var _ domain.SourceProvider = (*nodeProvider)(nil)` present
- [x] `ProviderName()` returns `"native"`
- [x] `ObjectType()` returns `&v1.Node{}`
- [x] `ExtractFinding` returns `(nil, nil)` for healthy nodes
- [x] `ExtractFinding` returns a populated `*Finding` for all failure conditions in the
  table below
- [x] `Finding.ParentObject` is `"Node/<name>"` — nodes have no workload parent.
  Call: `getParent(ctx, nil, node.ObjectMeta, "Node")` — note that NodeProvider does not
  need a `client.Client` for `getParent` (nodes have no ownerReferences, so the client
  is never used). Pass `nil` or a valid client — both are safe since `getParent` only
  calls the client when ownerReferences exist. For consistency with other providers,
  `nodeProvider` still holds a `client.Client` field and passes it.
- [x] `Finding.Kind` is `"Node"`, `Finding.Name` is the node name, `Finding.Namespace` is `""`
  (nodes are cluster-scoped)
- [x] `Finding.Errors` contains one entry per failing condition

---

## Failure Conditions Detected

Detection is based **exclusively on `node.status.conditions`** — no taint inspection,
no event fetching. Node taints (e.g. `node.kubernetes.io/not-ready`) are applied by the
node lifecycle controller as a consequence of the same conditions being set; checking
both would add complexity with no additional signal. Conditions are the authoritative
source and are checked in isolation.

Each condition is evaluated independently (OR logic): a finding is produced if **any one**
of the following conditions is in a failing state. A node with both `NodeReady==False` and
`MemoryPressure==True` produces a single Finding with two error entries (one per failing
condition).

| Condition | Detection logic |
|---|---|
| `NodeReady == False` or `NodeReady == Unknown` | Standard `NodeReady` condition not `True` |
| `MemoryPressure == True` | `NodeMemoryPressure` condition is `True` |
| `DiskPressure == True` | `NodeDiskPressure` condition is `True` |
| `PIDPressure == True` | `NodePIDPressure` condition is `True` |
| `NetworkUnavailable == True` | `NodeNetworkUnavailable` condition is `True` |

The k8s-specific condition type `EtcdIsVoter` (used by k3s) must be ignored — it is not
a failure condition even when `True`. Do not produce a finding for it.

Error text format per condition: `"node <name> has condition <Type> (<Reason>): <Message>"`.

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `HealthyNode` | All standard conditions `False`/`True` as expected for a healthy node | `(nil, nil)` |
| `NotReadyFalse` | `NodeReady == False` | Finding with condition error text |
| `NotReadyUnknown` | `NodeReady == Unknown` | Finding with condition error text |
| `MemoryPressure` | `NodeMemoryPressure == True` | Finding |
| `DiskPressure` | `NodeDiskPressure == True` | Finding |
| `PIDPressure` | `NodePIDPressure == True` | Finding |
| `NetworkUnavailable` | `NodeNetworkUnavailable == True` | Finding |
| `EtcdIsVoterIgnored` | `EtcdIsVoter == True` (k3s condition) | `(nil, nil)` |
| `MultipleConditions` | `NodeReady == False` and `MemoryPressure == True` | Finding with two error entries |
| `WrongType` | Non-Node object passed | `(nil, error)` |

---

## Tasks

- [x] Write all 10 tests in `internal/provider/native/node_test.go` (TDD — must fail first)
- [x] Implement `NodeProvider` in `internal/provider/native/node.go`
- [x] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (pattern consistency; `getParent` returns `"Node/<name>"` for
cluster-scoped resources with no ownerReferences)
**Blocks:** STORY_08 (main wiring)

---

## Definition of Done

- [x] All 10 tests pass with `-race`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
