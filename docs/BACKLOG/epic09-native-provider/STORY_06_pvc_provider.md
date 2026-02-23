# Story: PVCProvider

**Epic:** [epic09-native-provider](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want mendabot to detect PersistentVolumeClaims stuck in
Pending so that storage provisioning failures trigger a remediation Job without requiring
k8sgpt-operator.

---

## Acceptance Criteria

- [ ] `pvcProvider` struct defined in `internal/provider/native/pvc.go`
  (unexported; exported constructor `NewPVCProvider(c client.Client) *pvcProvider`
  in same file; panics if `c == nil`)
- [ ] Compile-time assertion `var _ domain.SourceProvider = (*pvcProvider)(nil)` present
- [ ] `ProviderName()` returns `"native"`
- [ ] `ObjectType()` returns `&v1.PersistentVolumeClaim{}`
- [ ] `ExtractFinding` returns `(nil, nil)` for Bound or Lost PVCs
- [ ] `ExtractFinding` returns `(nil, nil)` for Pending PVCs with no `ProvisioningFailed` event
- [ ] `ExtractFinding` returns a populated `*Finding` for Pending PVCs that have a
  `ProvisioningFailed` event with a non-empty message
- [ ] `Finding.ParentObject` is `"PersistentVolumeClaim/<name>"` (PVCs have no meaningful
  workload parent in the owner chain for deduplication purposes).
  Call: `getParent(ctx, p.client, pvc.ObjectMeta, "PersistentVolumeClaim")` (returns
  `"PersistentVolumeClaim/<name>"` since PVCs have no ownerReferences)
- [ ] `Finding.Kind` is `"PersistentVolumeClaim"`, `Finding.Name` is the PVC name
- [ ] `Finding.Errors` contains the `ProvisioningFailed` event message
- [ ] `pvcProvider` holds a `client.Client` for event fetching

---

## Event fetching

Detecting the failure requires fetching the latest event for the PVC:

```
List Events where:
  involvedObject.name      == pvc.Name
  involvedObject.namespace == pvc.Namespace
  involvedObject.kind      == "PersistentVolumeClaim"
Sort by lastTimestamp descending
If the most recent event has Reason == "ProvisioningFailed", produce a finding
```

Use `client.List` with a field selector on **all three** `involvedObject.*` fields:

```go
client.MatchingFields{
    "involvedObject.name":      pvc.Name,
    "involvedObject.namespace": pvc.Namespace,
    "involvedObject.kind":      "PersistentVolumeClaim",
}
```

**Why all three fields?** Without the `involvedObject.kind` filter, events for a Pod,
Service, or other object that happens to share the same name as the PVC (in the same
namespace) would be incorrectly returned. This is a real scenario in clusters where
names are derived from a shared prefix (e.g. `database` as both a PVC and a Service).
Omitting the kind filter would cause false positives.

**Note on envtest:** The fake client used in unit tests may not support multi-field
field selector filtering. Use `fake.NewClientBuilder().WithIndex(...)` to register
the field index for `involvedObject.name`, and filter the remaining two fields
manually in the test helper that constructs fake events, or verify the kind filter
in a separate integration test. Document whichever approach is used.

Unlike `PodProvider` (which detects all failures purely from pod status fields),
`PVCProvider` must fetch events because the Kubernetes PVC status does not expose a
provisioning-failure flag directly — the `ProvisioningFailed` event is the authoritative
signal.

**Ordering with `phase != Bound` check:** The phase check (`pvc.Status.Phase != Bound`)
is performed **before** the event lookup. A successfully provisioned PVC (Phase == Bound)
is returned as `(nil, nil)` immediately, before any event is fetched. This means that
even if stale `ProvisioningFailed` events exist from a previous failed attempt (events
persist for hours), a now-Bound PVC will never produce a finding. The ordering is:
1. Check `pvc.Status.Phase != Bound` — if Bound, return `(nil, nil)`
2. Fetch events with all three field selector conditions
3. If most recent event is `ProvisioningFailed`, return Finding

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `BoundPVC` | PVC with `Phase == Bound` | `(nil, nil)` |
| `PendingNoEvent` | Pending PVC, no events | `(nil, nil)` |
| `PendingProvisioningFailed` | Pending PVC, latest event `ProvisioningFailed` with message | Finding with event message as error text |
| `PendingUnrelatedEvent` | Pending PVC, latest event `Reason == "Provisioning"` | `(nil, nil)` |
| `WrongType` | Non-PVC object passed | `(nil, error)` |

---

## Tasks

- [ ] Write all 5 tests in `internal/provider/native/pvc_test.go` (TDD — must fail first)
- [ ] Implement `PVCProvider` in `internal/provider/native/pvc.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (getParent — called for consistency; returns the PVC's own name
since PVCs have no workload owner reference)
**Blocks:** STORY_08 (main wiring)

---

## Definition of Done

- [ ] All 5 tests pass with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
