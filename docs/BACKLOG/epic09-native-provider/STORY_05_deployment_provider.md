# Story: DeploymentProvider

**Epic:** [epic09-native-provider](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want mendabot to detect degraded Deployments directly from
cluster state so that a Deployment with unavailable replicas triggers a remediation Job
without requiring k8sgpt-operator.

---

## Acceptance Criteria

- [ ] `deploymentProvider` struct defined in `internal/provider/native/deployment.go`
  (unexported; exported constructor `NewDeploymentProvider(c client.Client) *deploymentProvider`
  in same file; panics if `c == nil`)
- [ ] Compile-time assertion `var _ domain.SourceProvider = (*deploymentProvider)(nil)` present
- [ ] `ProviderName()` returns `"native"`
- [ ] `ObjectType()` returns `&appsv1.Deployment{}`
- [ ] `ExtractFinding` returns `(nil, nil)` for healthy Deployments
- [ ] `ExtractFinding` returns a populated `*Finding` when `spec.replicas != status.readyReplicas`,
  ignoring the transient scaling case described below
- [ ] `ExtractFinding` returns a populated `*Finding` when `status.conditions` contains
  a condition with `Type == "Available"` and `Status == "False"`, even if replica counts
  look healthy (e.g. a progressing rollout stuck due to image pull)
- [ ] Error text for the replica mismatch case includes both `spec.replicas` and
  `status.readyReplicas` values
- [ ] Error text for the `Available=False` case includes the condition `Reason` and
  `Message` fields (both may be empty strings; include them regardless so the agent has
  full context)
- [ ] `Finding.ParentObject` is `"Deployment/<name>"` — a Deployment is its own anchor.
  Call: `getParent(ctx, p.client, deploy.ObjectMeta, "Deployment")` (returns `"Deployment/<name>"`
  since a Deployment has no ownerReferences)
- [ ] `Finding.Kind` is `"Deployment"`, `Finding.Name` is the Deployment name
- [ ] `Finding.Errors` is a JSON array; may contain one or two entries if both replica
  mismatch and `Available=False` are present simultaneously

---

## Scaling transient exclusion

When `status.replicas > spec.replicas` the Deployment has been scaled down and the
status field has not yet caught up. This is not a failure — it is a normal scaling event.
This case must not produce a finding. Only when `status.readyReplicas < spec.replicas`
(with `status.replicas <= spec.replicas`) is the Deployment genuinely degraded.

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `HealthyDeployment` | `spec.replicas=3`, `status.readyReplicas=3`, no `Available=False` condition | `(nil, nil)` |
| `DegradedDeployment` | `spec.replicas=3`, `status.readyReplicas=1` | Finding with mismatch error text |
| `ZeroReadyReplicas` | `spec.replicas=2`, `status.readyReplicas=0` | Finding |
| `ScalingDownTransient` | `spec.replicas=2`, `status.replicas=3`, `status.readyReplicas=2` | `(nil, nil)` — scaling transient, not a failure |
| `AvailableConditionFalse` | `spec.replicas=3`, `status.readyReplicas=3`, condition `Available=False` with non-empty `Reason` and `Message` | Finding is returned (condition triggers despite healthy replica counts) |
| `ErrorTextIncludesReason` | Same setup as `AvailableConditionFalse` | The returned Finding's `Errors` JSON contains both the `Reason` and `Message` string values |
| `WrongType` | Non-Deployment object passed | `(nil, error)` |
| `ErrorTextContent` | Degraded Deployment (replica mismatch) | Error text contains both `spec.replicas` and `status.readyReplicas` values |

**Note on `AvailableConditionFalse` and `ErrorTextIncludesReason`:** These two test cases
exercise the same code path (`Available=False` condition branch) and may share identical
input setup. They are kept as separate test cases intentionally — one asserts the Finding
is non-nil (existence check), the other asserts the error text content (correctness check).
Merging them would make the failure message ambiguous if the test fails. Keep them separate.

---

## Tasks

- [ ] Write all 8 tests in `internal/provider/native/deployment_test.go` (TDD — must fail first)
- [ ] Implement `DeploymentProvider` in `internal/provider/native/deployment.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (getParent — used for consistency even though Deployments are
their own anchor; the call is still made so the pattern is uniform)
**Blocks:** STORY_08 (main wiring)

---

## Definition of Done

- [ ] All 8 tests pass with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
