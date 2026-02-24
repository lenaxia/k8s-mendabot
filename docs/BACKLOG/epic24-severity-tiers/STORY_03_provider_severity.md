# Story 03: Providers — Assign Severity in ExtractFinding

**Epic:** [epic24-severity-tiers](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **cluster operator**, I want each finding to carry an accurate severity rating so that
I can filter and triage investigations by impact without having to inspect every finding
manually.

---

## Background

The six native providers live in `internal/provider/native/`: `pod.go`, `deployment.go`,
`statefulset.go`, `node.go`, `job.go`, `pvc.go`. Each implements `ExtractFinding(obj
client.Object) (*domain.Finding, error)`. Currently they return a `Finding` with no
`Severity` set (zero value `""`).

After this story, every `Finding` returned by a provider has `Severity` set to one of the
four valid values. No provider may return a `Finding` with `Severity == ""`.

---

## Design

### Severity assignment rules per provider

#### pod.go

Inspect `pod.Status.ContainerStatuses[*].State.Waiting.Reason` and
`pod.Status.ContainerStatuses[*].State.Terminated.ExitCode` and the restart count.

| Condition | Severity |
|-----------|----------|
| Any container in `CrashLoopBackOff` AND restart count > 5 | `critical` |
| Any container in `CrashLoopBackOff` (restart count ≤ 5) | `high` |
| Any container in `OOMKilled` (terminated reason) | `high` |
| Any container in `ImagePullBackOff` or `ErrImagePull` | `high` |
| Pod has `Unschedulable` condition | `high` |
| Any container terminated with non-zero exit code (other reasons) | `medium` |
| Default (finding exists but none of the above match) | `medium` |

Use the highest severity found across all containers.

#### deployment.go

Inspect `deploy.Status.ReadyReplicas`, `*deploy.Spec.Replicas` (guarded: the existing
code already wraps the replica check in `if deploy.Spec.Replicas != nil`), and the
`Available=False` condition in `deploy.Status.Conditions`.

The "half ready" threshold uses `*Spec.Replicas` (not `Status.Replicas`) — consistent
with how the existing provider computes the finding text. If `Spec.Replicas == nil`, the
replica branch is skipped entirely and only the `Available=False` condition can fire.

| Condition | Severity |
|-----------|----------|
| `ReadyReplicas == 0` (and `Spec.Replicas != nil && *Spec.Replicas > 0`) | `critical` |
| `ReadyReplicas < *Spec.Replicas / 2` (less than half, non-zero) | `high` |
| `ReadyReplicas < *Spec.Replicas` (some replicas missing, but ≥ half ready) | `medium` |
| `Available=False` condition (may co-fire with replica issues) | `medium` |
| Only `Available=False` fires (no replica issue) | `medium` |

When both the replica check and `Available=False` both fire, use the higher of the two.
`computeDeploymentSeverity` takes both flags and returns the max.

#### statefulset.go

Mirrors deployment logic. The existing provider fires two independent checks:
1. **Replica mismatch** — only when `generation == observedGeneration` (not scaling)
2. **`Available=False` condition** — always, regardless of scaling state

The `computeStatefulSetSeverity` helper receives the results of both checks and returns
the highest applicable severity.

| Condition | Severity |
|-----------|----------|
| `ReadyReplicas == 0` (and `*Spec.Replicas > 0`) | `critical` |
| `ReadyReplicas < *Spec.Replicas / 2` | `high` |
| `ReadyReplicas < *Spec.Replicas` (some missing, ≥ half ready) | `medium` |
| `Available=False` condition | `medium` |

When `Spec.Replicas == nil`, only the `Available=False` path can fire.

#### node.go

The existing provider checks conditions using a switch with three branches:

1. `NodeReady == False or Unknown` → `critical`
2. `NodeMemoryPressure`, `NodeDiskPressure`, `NodePIDPressure`, `NodeNetworkUnavailable` == True → `high`
3. `default` (any other `ConditionTrue` not in `ignoredNodeConditions`) → `high`

`NetworkUnavailable=True` is in the named-condition branch (case 2) — it maps to `high`.

Unknown custom conditions falling into the `default` branch also map to `high`.

The final severity is the highest across all firing conditions: if both `NodeReady=False`
and `NodeMemoryPressure=True` fire, the result is `critical`.

| Condition | Severity |
|-----------|----------|
| `NodeReady == False` or `NodeReady == Unknown` | `critical` |
| `NodeMemoryPressure`, `NodeDiskPressure`, `NodePIDPressure`, `NodeNetworkUnavailable` == True | `high` |
| Any other `ConditionTrue` (not in `ignoredNodeConditions`) | `high` |

#### job.go

A finding is only extracted when the Job has exceeded its backoff limit. The provider
directly returns a `Finding` — there is no branching within `ExtractFinding` itself once
the failure state is confirmed.

| Condition | Severity |
|-----------|----------|
| Job failed (backoff exhausted) | `medium` |

Set `Severity: domain.SeverityMedium` directly on the returned `Finding` — no helper
function needed.

#### pvc.go

The PVC provider fires a finding only when **both** conditions are met:
1. `pvc.Status.Phase == Pending`
2. A `ProvisioningFailed` Kubernetes Event exists for the PVC (found via `latestProvisioningFailedMessage`)

There is no `pvc.Status.Conditions` involvement — the detection is purely Events-based.
If a `ProvisioningFailed` event is found, severity is always `high`. No helper function
is needed.

| Condition | Severity |
|-----------|----------|
| `Pending` + `ProvisioningFailed` event found | `high` |

---

## Implementation Pattern

Each provider computes severity locally before building the `Finding`. A local helper
function `computeSeverity(obj) domain.Severity` keeps the logic isolated and testable.
The provider test files already use table-driven tests — add severity assertions to each
existing test case and add new cases for each severity level.

---

## Acceptance Criteria

- [ ] All six providers set `Finding.Severity` to a non-empty value on every returned finding
- [ ] Pod: `critical` for CrashLoopBackOff > 5 restarts; `high` for OOMKilled, ImagePullBackOff, Unschedulable; `medium` for other non-zero exits
- [ ] Deployment: `critical` for 0 ready; `high` for < 50% ready; `medium` for Available=False
- [ ] StatefulSet: `critical` for 0 ready; `high` for < 50% ready
- [ ] Node: `critical` for NotReady; `high` for pressure conditions
- [ ] Job: `medium` for exhausted backoff
- [ ] PVC: `high` for Pending/ProvisioningFailed
- [ ] No provider returns a `Finding` with `Severity == ""`
- [ ] All provider tests updated to assert severity

---

## Tasks

- [ ] Update `internal/provider/native/pod.go` — add `computePodSeverity` and set `Finding.Severity`
- [ ] Update `internal/provider/native/pod_test.go` — add severity assertions to all test cases; add new severity-specific cases
- [ ] Update `internal/provider/native/deployment.go` — add `computeDeploymentSeverity`
- [ ] Update `internal/provider/native/deployment_test.go`
- [ ] Update `internal/provider/native/statefulset.go` — add `computeStatefulSetSeverity`
- [ ] Update `internal/provider/native/statefulset_test.go`
- [ ] Update `internal/provider/native/node.go` — add `computeNodeSeverity`
- [ ] Update `internal/provider/native/node_test.go`
- [ ] Update `internal/provider/native/job.go` — assign `SeverityMedium` directly
- [ ] Update `internal/provider/native/job_test.go`
- [ ] Update `internal/provider/native/pvc.go` — assign `Severity: domain.SeverityHigh` directly on the returned `Finding` (no helper function needed — there is only one severity outcome)
- [ ] Update `internal/provider/native/pvc_test.go`
- [ ] Run `go test -race -timeout 30s ./internal/provider/...` — all tests must pass
- [ ] Run `go build ./...` — must be clean

---

## Dependencies

**Depends on:** STORY_01 (`domain.Severity` type and constants)
**Blocks:** Nothing directly (STORY_04 reads severity from the Finding, which is already set)

---

## Definition of Done

- [ ] All six providers assign a non-empty `Severity` on every returned `Finding`
- [ ] Provider tests cover all severity levels for each provider
- [ ] Full test suite `go test -race ./...` passes
- [ ] `go vet ./...` clean
