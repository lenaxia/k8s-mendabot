# Story: PodProvider

**Epic:** [epic09-native-provider](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **cluster operator**, I want mendabot to detect failing pods directly from cluster
state so that CrashLoopBackOff, ImagePullBackOff, OOMKilled, and similar conditions
trigger a remediation Job without requiring k8sgpt-operator.

---

## Acceptance Criteria

- [ ] `podProvider` struct defined in `internal/provider/native/pod.go` (unexported; the
  exported constructor `NewPodProvider(c client.Client) *podProvider` is in the same file
  and panics if `c == nil`)
- [ ] Compile-time assertion `var _ domain.SourceProvider = (*podProvider)(nil)` present
- [ ] `ProviderName()` returns `"native"`
- [ ] `ObjectType()` returns `&v1.Pod{}`
- [ ] `ExtractFinding` returns `(nil, nil)` for healthy pods (no failure conditions)
- [ ] `ExtractFinding` returns a populated `*Finding` for all failure conditions in the
  table below
- [ ] `Finding.ParentObject` is set from `getParent(ctx, p.client, pod.ObjectMeta, "Pod")`
  (four-argument form — see STORY_03 for signature details)
- [ ] `Finding` is constructed as follows (illustrative — error text varies by condition):
  ```go
  errorsJSON, _ := json.Marshal([]struct{ Text string `json:"text"` }{
      {Text: "container my-app is in CrashLoopBackOff (last exit: OOMKilled)"},
  })
  finding := &domain.Finding{
      Kind:         "Pod",
      Name:         pod.Name,
      Namespace:    pod.Namespace,
      ParentObject: getParent(ctx, p.client, pod.ObjectMeta, "Pod"),
      Errors:       string(errorsJSON),
      SourceRef: domain.SourceRef{
          APIVersion: "v1",
          Kind:       "Pod",
          Name:       pod.Name,
          Namespace:  pod.Namespace,
      },
  }
  ```
  Note: `domain.FindingFingerprint` is called by `SourceProviderReconciler`, not by the
  provider itself. The provider only constructs and returns the `*domain.Finding`.
- [ ] `Finding.Errors` is a JSON array of `[{"text":"..."}]` entries, one per failing
  container
- [ ] Error text for waiting-state failures includes `containerStatus.State.Waiting.Message`
  when it is non-empty
- [ ] Error text for CrashLoopBackOff includes the last termination reason (e.g. `OOMKilled`)
  and container name
- [ ] `Finding.Kind` is `"Pod"`
- [ ] `Finding.Name` is the pod name (no namespace prefix)
- [ ] `Finding.Namespace` is the pod namespace
- [ ] `Finding.SourceRef` identifies the pod (`APIVersion: "v1"`, `Kind: "Pod"`)
- [ ] `podProvider` holds a `client.Client` field for the `getParent` call
- [ ] `PodProvider` does **not** fetch events — all detection is from pod status fields
  only

---

## Failure Conditions Detected

| Condition | Detection logic |
|---|---|
| `CrashLoopBackOff` | `containerStatus.State.Waiting.Reason == "CrashLoopBackOff"` — error text includes last termination reason (`containerStatus.LastTerminationState.Terminated.Reason`) and container name |
| `OOMKilled` | Surfaced via the CrashLoopBackOff branch when `LastTerminationState.Terminated.Reason == "OOMKilled"` |
| `ImagePullBackOff` / `ErrImagePull` | `containerStatus.State.Waiting.Reason` in the error reason set; include `Waiting.Message` in error text |
| `CreateContainerConfigError` and similar | `containerStatus.State.Waiting.Reason` in the error reason set; include `Waiting.Message` in error text |
| Non-zero exit code (terminated, not restarting) | `containerStatus.State.Terminated.ExitCode != 0` and `State.Waiting == nil` (container has terminated and is not being restarted) |
| Unschedulable (pending) | `pod.Status.Phase == Pending` and a `PodScheduled` condition with `Reason == "Unschedulable"`; include the condition `Message` in error text |

Readiness probe failures are **not** detected by this provider. Detection via events is
event-order-dependent and produces false positives for transient unhealthy states; the
stabilisation window (STORY_12) is the correct mechanism for filtering transient failures.

Error reason set (waiting reasons that produce a finding):
`CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `CreateContainerConfigError`,
`InvalidImageName`, `RunContainerError`, `CreateContainerError`.

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `HealthyRunningPod` | Running pod, all containers ready | `(nil, nil)` |
| `CrashLoopBackOffOOMKilled` | Container waiting with `CrashLoopBackOff`, last terminated `OOMKilled` | Finding with error text containing `OOMKilled` and container name |
| `CrashLoopBackOffGeneric` | Container waiting with `CrashLoopBackOff`, last terminated `Error` | Finding with error text containing container name |
| `ImagePullBackOff` | Container waiting with `ImagePullBackOff` and a non-empty `Message` | Finding; error text contains the Message |
| `ErrImagePull` | Container waiting with `ErrImagePull` | Finding with error text |
| `CreateContainerConfigError` | Container waiting with `CreateContainerConfigError` and message | Finding; error text contains the message |
| `NonZeroExitCode` | Container terminated with exit code 137, `Waiting == nil` | Finding with exit code in error text |
| `UnschedulablePending` | Pod pending, `PodScheduled` condition `Unschedulable` with message | Finding; error text contains the scheduler message |
| `NoOwnerRef` | Crashing pod with no ownerReferences | `Finding.ParentObject == "Pod/<pod-name>"` |
| `WithDeploymentParent` | Crashing pod with ReplicaSet → Deployment chain | `Finding.ParentObject == "Deployment/my-app"` |
| `WrongType` | Non-Pod object passed | `(nil, error)` |
| `MultipleContainerFailures` | Two containers both failing | `Finding.Errors` contains two entries |

---

## Tasks

- [ ] Write all 12 tests in `internal/provider/native/pod_test.go` (TDD — must fail first)
- [ ] Implement `PodProvider` in `internal/provider/native/pod.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_03 (getParent)
**Blocks:** STORY_08 (main wiring)

---

## Definition of Done

- [ ] All 12 tests pass with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
