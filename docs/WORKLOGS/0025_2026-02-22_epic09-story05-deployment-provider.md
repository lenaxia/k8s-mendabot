# Worklog: Epic 09 STORY_05 — DeploymentProvider

**Date:** 2026-02-22
**Session:** Implement DeploymentProvider with replica mismatch and Available=False detection
**Status:** Complete

---

## Objective

Implement `internal/provider/native/deployment.go` — a `SourceProvider` that watches
Deployment objects and produces findings for:
1. Replica mismatch: `status.readyReplicas < spec.replicas` (excluding scaling-down transients)
2. `Available=False` condition: always reported regardless of replica counts

---

## Work Completed

### 1. Test file (TDD — written and confirmed failing before implementation)

Created `internal/provider/native/deployment_test.go` with 13 tests:

- `TestDeploymentProviderName_IsNative` — ProviderName() == "native"
- `TestDeploymentObjectType_IsDeployment` — ObjectType() returns *appsv1.Deployment
- `TestHealthyDeployment` — 3/3 replicas, Available=True → (nil, nil)
- `TestDegradedDeployment` — 3 spec, 1 ready → finding
- `TestZeroReadyReplicas` — 2 spec, 0 ready → finding
- `TestScalingDownTransient` — status.replicas(3) > spec.replicas(2), readyReplicas=2 → (nil, nil)
- `TestAvailableConditionFalse` — 3/3 replicas but Available=False → finding
- `TestErrorTextIncludesReason` — Available=False finding includes Reason and Message
- `TestDeploymentWrongType` — Pod passed → (nil, error)
- `TestErrorTextContent` — replica mismatch error text includes both replica counts
- `TestDeploymentFindingErrors_IsValidJSON` — Errors is valid JSON array
- `TestDeploymentParentObject_IsSelf` — no ownerRefs → "Deployment/my-deploy"
- `TestBothConditions_TwoEntries` — both failures present → 2 error entries

### 2. Implementation

Created `internal/provider/native/deployment.go`:

- `deploymentProvider` struct with `client client.Client`
- `NewDeploymentProvider(c client.Client) domain.SourceProvider` — panics on nil client
- Compile-time interface assertion
- `ProviderName()` → `"native"`
- `ObjectType()` → `&appsv1.Deployment{}`
- `ExtractFinding()`:
  - Type-asserts to `*appsv1.Deployment`
  - Scaling-down transient guard: skips replica check when `status.replicas > spec.replicas`
  - Replica mismatch: reports when `status.replicas <= spec.replicas && readyReplicas < specReplicas`
  - Available=False: always reported, includes Reason and Message in error text
  - Calls `getParent()` for ParentObject (returns "Deployment/name" for root deployments)
  - Errors serialised via `json.Marshal`

### 3. Story and worklog maintenance

- Updated STORY_05 status to Complete, all checkboxes ticked
- Created this worklog entry

---

## Key Decisions

- **Scaling transient detection via `status.replicas > spec.replicas`** (not generation check):
  The authoritative story doc specifies this exact condition. When status.replicas exceeds
  spec.replicas, old pods are still terminating after a scale-down — this is normal, not a
  failure. The generation-based check in the delegation prompt was an alternative, but the
  story file takes precedence.

- **Available=False always reported**: Even when replica counts look healthy (e.g. 3/3 ready
  but Available=False from a stuck rollout). This matches the story's explicit requirement.

- **Error text format**: `"deployment <name>: Available=False reason=<reason> message=<message>"`
  includes both Reason and Message so the remediation agent has full diagnostic context.

- **Two error entries when both conditions fire**: Finding.Errors is a JSON array and can
  contain both a replica mismatch entry and an Available=False entry simultaneously.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./internal/provider/native/...
# PASS — 38 tests (13 new + 25 pre-existing)

go test -timeout 60s -race ./...
# PASS — all 10 packages

go vet ./...
# Clean

go build ./...
# Clean
```

---

## Next Steps

STORY_06 is next in the epic. Read
`docs/BACKLOG/epic09-native-provider/STORY_06_*.md` and implement accordingly.

---

## Files Modified

- `internal/provider/native/deployment.go` — created (implementation)
- `internal/provider/native/deployment_test.go` — created (13 tests)
- `docs/BACKLOG/epic09-native-provider/STORY_05_deployment_provider.md` — status updated to Complete
- `docs/WORKLOGS/0025_2026-02-22_epic09-story05-deployment-provider.md` — this file
- `docs/WORKLOGS/README.md` — index updated
