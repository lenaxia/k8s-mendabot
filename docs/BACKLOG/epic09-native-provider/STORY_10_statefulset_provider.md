# Story: StatefulSetProvider

**Epic:** [epic09-native-provider](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want mendabot to detect degraded StatefulSets directly from
cluster state so that a StatefulSet with unavailable replicas triggers a remediation Job
without requiring k8sgpt-operator.

---

## Acceptance Criteria

- [ ] `statefulSetProvider` struct defined in `internal/provider/native/statefulset.go`
  (unexported; exported constructor `NewStatefulSetProvider(c client.Client) *statefulSetProvider`
  in same file; panics if `c == nil`)
- [ ] Compile-time assertion `var _ domain.SourceProvider = (*statefulSetProvider)(nil)` present
- [ ] `ProviderName()` returns `"native"`
- [ ] `ObjectType()` returns `&appsv1.StatefulSet{}`
- [ ] `ExtractFinding` returns `(nil, nil)` for healthy StatefulSets
- [ ] `ExtractFinding` returns a populated `*Finding` when `status.readyReplicas < spec.replicas`
  (using the same scaling transient exclusion as `DeploymentProvider`)
- [ ] `ExtractFinding` returns a populated `*Finding` when `status.conditions` contains
  a condition with `Type == "Available"` and `Status == "False"` (Kubernetes 1.26+)
- [ ] Error text for the replica mismatch case includes both `spec.replicas` and
  `status.readyReplicas` values
- [ ] Error text for the `Available=False` case includes the condition `Reason` and `Message`
  fields
- [ ] `Finding.ParentObject` is `"StatefulSet/<name>"` — a StatefulSet is its own anchor.
  Call: `getParent(ctx, p.client, sts.ObjectMeta, "StatefulSet")` (returns
  `"StatefulSet/<name>"` since StatefulSets have no ownerReferences)
- [ ] `Finding.Kind` is `"StatefulSet"`, `Finding.Name` is the StatefulSet name
- [ ] `Finding.Namespace` is the StatefulSet namespace
- [ ] `Finding.SourceRef` identifies the StatefulSet (`APIVersion: "apps/v1"`,
  `Kind: "StatefulSet"`)
- [ ] `Finding.Errors` is a JSON array; may contain one or two entries if both replica
  mismatch and `Available=False` are present simultaneously
- [ ] `statefulSetProvider` holds a `client.Client` field (consistent with other providers;
  required for `getParent` even though StatefulSets are their own anchor)

---

## Scaling transient exclusion

Identical to `DeploymentProvider`: when `status.replicas > spec.replicas` the StatefulSet
has been scaled down and status has not yet caught up — this is not a failure. Only when
`status.readyReplicas < spec.replicas` (with `status.replicas <= spec.replicas`) is the
StatefulSet genuinely degraded.

---

## Note on Available condition

The `Available` condition type on StatefulSet was added in Kubernetes 1.26. On older
clusters it will never be present. The provider must handle the absence of this condition
gracefully (no error, no finding produced from a missing condition).

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `HealthyStatefulSet` | `spec.replicas=3`, `status.readyReplicas=3`, no `Available=False` condition | `(nil, nil)` |
| `DegradedStatefulSet` | `spec.replicas=3`, `status.readyReplicas=1` | Finding with mismatch error text containing both values |
| `ZeroReadyReplicas` | `spec.replicas=2`, `status.readyReplicas=0` | Finding |
| `ScalingDownTransient` | `spec.replicas=2`, `status.replicas=3`, `status.readyReplicas=2` | `(nil, nil)` — scaling transient |
| `AvailableConditionFalse` | `spec.replicas=3`, `status.readyReplicas=3`, condition `Available=False` with `Reason` and `Message` | Finding; error text contains `Reason` and `Message` |
| `NoAvailableCondition` | Healthy StatefulSet on a pre-1.26 cluster, no conditions present | `(nil, nil)` |
| `ErrorTextIncludesReason` | Degraded StatefulSet with `Available=False`, non-empty `Reason` and `Message` | Error text contains both the `Reason` and `Message` values |
| `WrongType` | Non-StatefulSet object passed | `(nil, error)` |
| `ErrorTextContent` | Degraded StatefulSet (replica mismatch) | Error text contains both `spec.replicas` and `status.readyReplicas` values |

---

## Tasks

- [ ] Write all 9 tests in `internal/provider/native/statefulset_test.go` (TDD — must fail first)
- [ ] Implement `StatefulSetProvider` in `internal/provider/native/statefulset.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (getParent)
**Blocks:** STORY_08 (main wiring)

---

## Definition of Done

- [ ] All 9 tests pass with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
