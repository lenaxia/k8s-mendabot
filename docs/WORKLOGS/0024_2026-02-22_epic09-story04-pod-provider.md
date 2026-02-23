# Worklog: Epic 09 STORY_04 — PodProvider with Failure Detection

**Date:** 2026-02-22
**Session:** TDD implementation of PodProvider (native source provider for pod failure detection)
**Status:** Complete

---

## Objective

Implement `internal/provider/native/pod.go` with an unexported `podProvider` struct that
implements `domain.SourceProvider`, detecting pod failure conditions directly from pod
status fields without requiring k8sgpt-operator. Write all 16 tests first (TDD), then
implement.

---

## Work Completed

### 1. Test file: `internal/provider/native/pod_test.go`

Wrote 16 tests before any implementation. Confirmed build failure with `go test` (tests
referenced undefined `NewPodProvider`). Tests cover:

- `TestProviderName_IsNative` — ProviderName() == "native"
- `TestObjectType_IsPod` — ObjectType() returns *corev1.Pod
- `TestHealthyRunningPod` — running pod, no failures → (nil, nil)
- `TestWrongType` — non-Pod object → (nil, error)
- `TestCrashLoopBackOffOOMKilled` — CrashLoopBackOff, last terminated OOMKilled → finding
- `TestCrashLoopBackOffGeneric` — CrashLoopBackOff, last terminated "Error" → finding
- `TestImagePullBackOff` — ImagePullBackOff with message → finding with message
- `TestErrImagePull` — ErrImagePull → finding
- `TestCreateContainerConfigError` — CreateContainerConfigError with message → finding
- `TestNonZeroExitCode` — terminated exit code 137, Waiting==nil → finding with code
- `TestUnschedulablePending` — pending pod, Unschedulable condition → finding with message
- `TestNoOwnerRef` — crashing pod, no ownerRefs → ParentObject == "Pod/<name>"
- `TestWithDeploymentParent` — Pod → ReplicaSet → Deployment → ParentObject == "Deployment/my-app"
- `TestMultipleContainerFailures` — two failing containers → two error entries
- `TestFindingErrors_IsValidJSON` — Errors field is valid JSON `[{"text":"..."}]`
- `TestSourceRef_IsPodV1` — SourceRef.APIVersion=="v1", Kind=="Pod"

### 2. Implementation: `internal/provider/native/pod.go`

- `podProvider` struct (unexported) holding `client.Client`
- `NewPodProvider(c client.Client) domain.SourceProvider` — exported constructor, panics if nil
- Compile-time assertion `var _ domain.SourceProvider = (*podProvider)(nil)`
- `ProviderName()` → "native"
- `ObjectType()` → `&corev1.Pod{}`
- `ExtractFinding()`:
  - Type-asserts to `*corev1.Pod`, returns error on wrong type
  - Iterates over `ContainerStatuses` and `InitContainerStatuses`
  - Detects waiting failures: CrashLoopBackOff, ImagePullBackOff, ErrImagePull,
    CreateContainerConfigError, InvalidImageName, RunContainerError, CreateContainerError
  - CrashLoopBackOff error text includes last termination reason and container name
  - Other waiting failures include `Waiting.Message` when non-empty
  - Detects non-zero exit code in terminated containers (Waiting==nil guard applied)
  - Detects unschedulable pending pods via PodScheduled condition
  - Calls `getParent(context.Background(), p.client, pod.ObjectMeta, "Pod")`
  - Marshals errors via `json.Marshal` (not string concatenation)
  - Returns `(nil, nil)` when no failures detected

### 3. Bug found and fixed during TDD

Initial implementation used `"pod unschedulable: ..."` as error text for the Unschedulable
condition. The test checked for the string "Unschedulable" (capitalised, from the condition
Reason field). Fixed by using `fmt.Sprintf("pod %s: %s", cond.Reason, cond.Message)` so the
Reason value is explicitly included in the error text.

---

## Key Decisions

- **`context.Background()` in ExtractFinding**: `ExtractFinding` has no ctx parameter in the
  interface. Using `context.Background()` for the `getParent` call is documented in the story
  spec and is the correct approach until a ctx-aware interface is designed.
- **Error text format for Unschedulable**: Uses `"pod Unschedulable: <message>"` (reason
  from the condition object) rather than a hardcoded string. This is more accurate and
  self-documenting.
- **Waiting state guard for terminated**: When `cs.State.Waiting != nil`, we `continue` to
  avoid also checking `cs.State.Terminated`. This prevents double-reporting when a container
  is being restarted (it shows as Waiting but may also have a previous Terminated state).

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 60s -race ./internal/provider/native/...
```
Result: **PASS** — 25 tests total (9 from parent_test.go, 16 from pod_test.go), 0 failures.

```
go build ./...
```
Result: clean.

```
go vet ./...
```
Result: clean.

```
go test -timeout 60s -race ./...
```
Result: all 10 packages pass.

---

## Next Steps

STORY_04 is complete. Next: STORY_05 (NodeProvider) or STORY_08 (main wiring) per epic
backlog priority. Check `docs/BACKLOG/epic09-native-provider/README.md` for current story
sequencing.

---

## Files Modified

- `internal/provider/native/pod_test.go` — created (16 tests)
- `internal/provider/native/pod.go` — created (podProvider implementation)
- `docs/WORKLOGS/0024_2026-02-22_epic09-story04-pod-provider.md` — created (this file)
- `docs/WORKLOGS/README.md` — updated index
- `docs/BACKLOG/epic09-native-provider/STORY_04_pod_provider.md` — status updated to Complete
