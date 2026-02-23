# Worklog: Epic 09 STORY_06 — PVCProvider

**Date:** 2026-02-22
**Session:** Implement PVCProvider with ProvisioningFailed event detection (STORY_06)
**Status:** Complete

---

## Objective

Implement `internal/provider/native/pvc.go` — a `SourceProvider` that detects
PersistentVolumeClaims stuck in Pending with a `ProvisioningFailed` event.

---

## Work Completed

### 1. Test file (TDD — tests written before implementation)

Wrote `internal/provider/native/pvc_test.go` with 11 test cases:

- `TestPVCProviderName_IsNative` — ProviderName() returns "native"
- `TestPVCObjectType_IsPVC` — ObjectType() returns *corev1.PersistentVolumeClaim
- `TestBoundPVC_ReturnsNil` — Phase=Bound → (nil, nil)
- `TestPendingPVC_NoEvents_ReturnsNil` — Phase=Pending, no events → (nil, nil)
- `TestPendingPVC_WithProvisioningFailed_ReturnsFinding` — Phase=Pending, ProvisioningFailed event → finding with all fields
- `TestPendingPVC_WithOtherEvent_ReturnsNil` — Phase=Pending, reason "Provisioning" → (nil, nil)
- `TestPVCWrongType_ReturnsError` — pass a Pod → (nil, error)
- `TestPVCFindingErrors_IsValidJSON` — Errors field is valid JSON array
- `TestPVCErrorText_IncludesEventMessage` — error text contains event message
- `TestPVCBoundWithStaleEvents_ReturnsNil` — Bound PVC with stale ProvisioningFailed events → (nil, nil); confirms phase check runs before event lookup
- `TestPVCEventForDifferentKind_ReturnsNil` — event with matching name but Kind="Pod" is ignored (kind filter correctness)

Confirmed tests failed to compile before implementation (TDD red phase).

### 2. Implementation

Wrote `internal/provider/native/pvc.go`:

- `pvcProvider` struct with `client.Client` field
- `NewPVCProvider(c client.Client) domain.SourceProvider` — panics if nil
- Compile-time assertion `var _ domain.SourceProvider = (*pvcProvider)(nil)`
- `ProviderName()` → `"native"`
- `ObjectType()` → `&corev1.PersistentVolumeClaim{}`
- `ExtractFinding`: type-asserts, checks `Phase != Pending` first, lists events
- `latestProvisioningFailedMessage`: lists events with `client.InNamespace` only,
  filters in-process for `InvolvedObject.Name == pvc.Name` and `InvolvedObject.Kind == "PersistentVolumeClaim"`,
  then filters `Reason == "ProvisioningFailed"`, sorts by `LastTimestamp` descending,
  returns the most recent message
- Error text format: `"pvc <name>: ProvisioningFailed: <event message>"`
- `ParentObject`: calls `getParent(...)` which returns `"PersistentVolumeClaim/<name>"` since PVCs have no ownerReferences
- `SourceRef`: APIVersion "v1", Kind "PersistentVolumeClaim"

---

## Key Decisions

1. **No field selectors in List call.** The fake client from controller-runtime does not
   support multi-field field selector filtering natively. Production code uses
   `client.InNamespace` only, then filters all three conditions in Go. This is slightly
   less efficient than server-side field selectors but correct, testable with the fake
   client, and consistent with the story's guidance.

2. **Kind filter added.** Events are filtered for `InvolvedObject.Kind == "PersistentVolumeClaim"`
   to prevent false positives from Pods or Services that share the same name as the PVC.
   This is the scenario documented in the story spec.

3. **Phase check before event lookup.** Bound PVCs return (nil, nil) immediately without
   any event list call. Stale `ProvisioningFailed` events from previous attempts on a
   now-Bound PVC are correctly ignored.

4. **Sort by LastTimestamp descending.** When multiple ProvisioningFailed events exist,
   the most recent message is used. This matches the story spec.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./internal/provider/native/... -v
# 49 tests, all PASS

go test -timeout 60s -race ./...
# All 10 packages pass

go vet ./...
# Clean

go build ./...
# Clean
```

---

## Next Steps

STORY_07 (NodeProvider): detect NotReady nodes and non-standard conditions.
Depends on STORY_03 (getParent — complete). Independent of STORY_06.

---

## Files Modified

- `internal/provider/native/pvc.go` — new file (implementation)
- `internal/provider/native/pvc_test.go` — new file (11 tests)
- `docs/WORKLOGS/0026_2026-02-22_epic09-story06-pvc-provider.md` — this file
- `docs/WORKLOGS/README.md` — index updated
