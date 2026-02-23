# Story: getParent Owner-Reference Traversal

**Epic:** [epic09-native-provider](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **developer**, I want a shared `getParent` function that walks `ownerReferences`
up to the workload root so that every native provider can resolve the correct
deduplication anchor without duplicating traversal logic.

---

## Background

The fingerprint uses `ParentObject` as the deduplication anchor — a Pod named
`my-app-7d9f8b-abc12` fingerprints to its parent Deployment `my-app`, not to the
individual pod name. Without this traversal, every pod restart from the same broken
Deployment would produce a different fingerprint and spawn a separate remediation Job.

The traversal must handle multi-level chains:
- `Pod → ReplicaSet → Deployment` (most common)
- `Pod → StatefulSet`
- `Pod → DaemonSet`
- `Pod → Job → CronJob`
- Any resource with no owner references (returns the resource's own `Kind/name`)

---

## Acceptance Criteria

- [ ] `getParent(ctx context.Context, c client.Client, meta metav1.ObjectMeta, kind string) string`
  defined in `internal/provider/native/parent.go` as a package-level function
  (unexported — only native providers use it)
- [ ] The `kind string` parameter is the Kubernetes Kind of the resource whose ObjectMeta
  is passed (e.g. `"Pod"`, `"Deployment"`). It is required because `metav1.ObjectMeta`
  does not carry Kind — that is in `TypeMeta`, which is separate. The fallback return
  value `"Kind/name"` cannot be constructed without it.
- [ ] Traversal handles all chains in the test table below
- [ ] **Maximum traversal depth is 10 levels.** This guards against pathological or
  circular owner reference chains. Once 10 levels have been traversed without finding a
  root (a resource with no `ownerReferences`), the function returns the deepest node it
  has successfully resolved.
- [ ] **Circular reference guard:** the function tracks all UIDs it has visited during a
  single traversal. If a UID is seen twice, traversal stops immediately and returns the
  last successfully resolved `"Kind/name"`. This prevents infinite loops in the event of
  malformed owner chains.
- [ ] **Returns the root owner** (the resource at the top of the chain that has no
  `ownerReferences` of its own), not the immediate parent. Example: for
  `Pod → ReplicaSet → Deployment`, the function returns `"Deployment/my-app"`, not
  `"ReplicaSet/my-app-7d9f8b"`.
- [ ] **Error handling:** when a `client.Get` call fails (network error, API unavailable,
  or RBAC denied), the function **logs the error at debug level and falls back** to
  returning the deepest node successfully resolved before the error. It does **not** return
  an error to callers — the function signature is `string` only. A degraded fingerprint
  (pointing to a ReplicaSet instead of its Deployment) is a better outcome than a failed
  reconciliation. The log message must include the owner UID and the error.
- [ ] **Not-found is handled silently:** if a `client.Get` call returns a not-found error
  (the owner object no longer exists), this is treated the same as any other API error —
  fall back to the deepest resolved node, log at debug level.
- [ ] When there are no `ownerReferences`, returns `"Kind/name"` of the input resource
- [ ] Return format is `"Kind/name"` (e.g. `"Deployment/my-app"`, `"StatefulSet/redis"`)
- [ ] Function signature takes `client.Client` (controller-runtime), not a raw `k8s.io/client-go`
  typed client — consistent with the rest of the codebase
- [ ] Unit tests written before implementation using a `fake.NewClientBuilder()` client
  with pre-populated objects

---

## Test Cases (all must be written before implementation)

| Test Name | Setup | Expected return |
|-----------|-------|-----------------|
| `NoOwnerRefs` | Pod with no `ownerReferences` | `"Pod/my-pod"` |
| `PodToDeployment` | Pod → ReplicaSet → Deployment chain in fake client | `"Deployment/my-app"` |
| `PodToStatefulSet` | Pod → StatefulSet | `"StatefulSet/redis"` |
| `PodToDaemonSet` | Pod → DaemonSet | `"DaemonSet/fluentd"` |
| `PodToJob` | Pod → Job (no further owner) | `"Job/batch-processor"` |
| `PodToCronJob` | Pod → Job → CronJob | `"CronJob/nightly-cleanup"` |
| `ReplicaSetNotFound` | Pod → ReplicaSet that does not exist in fake client | `"Pod/my-pod"` (fallback) |
| `DeploymentNotFound` | Pod → ReplicaSet (found) → Deployment not found | `"ReplicaSet/my-app-7d9f8b"` (fallback at deepest found level) |
| `CircularOwnerRefs` | Object A owns object B which owns object A (circular) | Returns `"Kind/name"` of A or B (whichever was reached first after the cycle is detected); does not infinite-loop |

---

## Tasks

- [x] Write all 9 tests in `internal/provider/native/parent_test.go` (TDD — must fail before
  implementation)
- [x] Implement `getParent` in `internal/provider/native/parent.go` with signature
  `getParent(ctx context.Context, c client.Client, meta metav1.ObjectMeta, kind string) string`
- [x] Run tests — all must pass

---

## Note for provider implementers

Every native provider that calls `getParent` must pass the resource's Kind as the fourth
argument. For example:

```go
// In PodProvider.ExtractFinding:
parentObject := getParent(ctx, p.client, pod.ObjectMeta, "Pod")

// In DeploymentProvider.ExtractFinding:
parentObject := getParent(ctx, p.client, deploy.ObjectMeta, "Deployment")

// In NodeProvider.ExtractFinding (nodes have no owners, so getParent returns "Node/<name>"):
parentObject := getParent(ctx, p.client, node.ObjectMeta, "Node")
```

The `ctx` passed to `getParent` must be the `context.Context` received by `ExtractFinding`.

---

## Dependencies

**Depends on:** STORY_02 (interface settled)
**Blocks:** STORY_04, STORY_05, STORY_06, STORY_07, STORY_10, STORY_11

---

## Definition of Done

- [x] All 9 tests pass with `-race`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
