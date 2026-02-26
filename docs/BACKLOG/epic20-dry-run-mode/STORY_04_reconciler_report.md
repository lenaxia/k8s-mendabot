# Story: RemediationJobReconciler — read investigation report from Job logs

**Epic:** [epic20-dry-run-mode](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **cluster operator** using dry-run mode, I want `kubectl get rjob <name> -o yaml` to
show the agent's investigation report in `status.message`, so that I can review what
mendabot would have done without leaving the Kubernetes API.

---

## Background

### `RemediationJobStatus.Message` — already exists

`api/v1alpha1/remediationjob_types.go` line 206 shows that `Message string` is **already
present** in `RemediationJobStatus`:

```go
// Message is a human-readable description of the current state,
// e.g. an error message if Phase is Failed.
Message string `json:"message,omitempty"`
```

`DeepCopyInto` already copies it (line 251). No CRD type changes are required by this story.

### How the controller currently detects Job completion

In `internal/controller/remediationjob_controller.go`, `syncPhaseFromJob` (lines 167–182)
maps `batchv1.Job` status to `RemediationJobPhase`:

```go
if job.Status.Succeeded > 0 {
    return v1alpha1.PhaseSucceeded
}
```

The reconciler detects completion at lines 100–142: it lists owned Jobs by label, calls
`syncPhaseFromJob`, and when `newPhase == v1alpha1.PhaseSucceeded` patches the status.

### How the report is read — Kubernetes CoreV1 Logs API

The agent writes the report to `/workspace/investigation-report.txt`. The entrypoint
script (`entrypoint-common.sh`) defines `emit_dry_run_report()`, which is called by the
per-agent entrypoint (`entrypoint-opencode.sh`, `entrypoint-claude.sh`) after the agent
binary returns. This function prints the sentinel `=== DRY_RUN INVESTIGATION REPORT ===`
followed by the file contents to stdout — making the report available via the Kubernetes
pod logs API.

The reconciler reads the pod logs, finds the sentinel line, and extracts only the text
after it as the report. If the sentinel is absent (e.g. opencode exited before writing the
report), the reconciler stores the raw truncated logs with a note.

> **Coordination note:** The `emit_dry_run_report` function and the per-agent entrypoint
> restructuring are implemented in STORY_03 (`entrypoint-common.sh`,
> `entrypoint-opencode.sh`, `entrypoint-claude.sh`). STORY_04 assumes the sentinel and
> report text are present in the `mendabot-agent` container logs when the Job has succeeded
> and the `mendabot.io/dry-run: "true"` annotation is set on the Job.

### How the controller identifies a dry-run Job

The Job built in STORY_02 carries the annotation `mendabot.io/dry-run: "true"`. The
reconciler reads this from the Job object it already has in hand — no additional API call
is needed:

```go
isDryRun := job.Annotations["mendabot.io/dry-run"] == "true"
```

### Log fetch — not a shell exec

The report is read via `k8s.io/client-go/kubernetes.Interface` (the typed Kubernetes client),
specifically `CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)`. This is the
standard in-cluster approach and requires no `kubectl` exec or shell subprocess.

The pod name is obtained by listing pods owned by the Job:

```go
var pods corev1.PodList
r.Client.List(ctx, &pods,
    client.InNamespace(r.Cfg.AgentNamespace),
    client.MatchingLabels{"batch.kubernetes.io/job-name": job.Name},
)
```

The first succeeded pod is used.

---

## Exact Code Locations

| File | Change |
|------|--------|
| `internal/controller/remediationjob_controller.go` | add log-fetch logic in the `PhaseSucceeded` branch; add `KubeClient kubernetes.Interface` field to reconciler struct |
| `internal/controller/remediationjob_controller.go` | new private method `fetchDryRunReport` with sentinel extraction |
| `cmd/watcher/main.go` | populate `KubeClient` on the reconciler |

---

## Acceptance Criteria

- [x] `RemediationJobReconciler` gains a `KubeClient kubernetes.Interface` field
- [x] When a Job succeeds (`job.Status.Succeeded > 0`) **and** has annotation
  `mendabot.io/dry-run: "true"`, the reconciler:
  1. Lists pods with label `batch.kubernetes.io/job-name: <job-name>` in `cfg.AgentNamespace`
  2. Selects the first pod with phase `Succeeded`
  3. Fetches logs for container `"mendabot-agent"` via `KubeClient.CoreV1().Pods(...).GetLogs(...).Stream(ctx)`
  4. Reads up to **10,000 bytes** of log output via `io.LimitReader` + `io.ReadAll`
  5. Extracts text after the sentinel `=== DRY_RUN INVESTIGATION REPORT ===`; falls back to raw log with note if sentinel absent
  6. Stores the result in `rjob.Status.Message`
  7. Patches `rjob.Status` via `r.Status().Patch(...)`
- [x] If no succeeded pod is found, or if the log fetch fails, the reconciler logs a warning
  and sets `rjob.Status.Message` to a descriptive error string — it does **not** return an
  error that would trigger a retry loop, since the Job itself succeeded
- [x] When `DRY_RUN` is not set (normal mode), `status.message` is never populated by this
  code path — it remains whatever it was (empty by default)
- [x] `entrypoint-common.sh` emits the sentinel and report to stdout in dry-run mode
  (confirmed implemented in STORY_03 — not implemented here)
- [x] `cmd/watcher/main.go` creates a `kubernetes.Clientset` and passes it as `KubeClient`
- [x] Unit tests cover the new log-fetch path using a fake `kubernetes.Interface`
- [x] `go test -race ./internal/controller/...` passes

---

## Implementation

### 1. Add `KubeClient` to the reconciler struct

```go
import "k8s.io/client-go/kubernetes"

type RemediationJobReconciler struct {
    client.Client
    Scheme     *runtime.Scheme
    Log        *zap.Logger
    JobBuilder domain.JobBuilder
    Cfg        config.Config
    // KubeClient is the typed Kubernetes client used to fetch pod logs.
    // Required when Cfg.DryRun is true; may be nil otherwise (log fetch is skipped).
    KubeClient kubernetes.Interface
}
```

### 2. Add `fetchDryRunReport` helper method

```go
// fetchDryRunReport retrieves the investigation report from the mendabot-agent
// container logs of the first succeeded pod owned by job. It finds the sentinel
// line "=== DRY_RUN INVESTIGATION REPORT ===" and returns only the text after it,
// truncated to maxReportBytes. If the sentinel is absent, returns the raw
// truncated logs with a prefix note. Returns an error description string on
// any infrastructure failure.
const maxReportBytes = 10_000

func (r *RemediationJobReconciler) fetchDryRunReport(ctx context.Context, job *batchv1.Job) string {
    if r.KubeClient == nil {
        return "dry-run report unavailable: KubeClient not configured"
    }

    var pods corev1.PodList
    if err := r.Client.List(ctx, &pods,
        client.InNamespace(r.Cfg.AgentNamespace),
        client.MatchingLabels{"batch.kubernetes.io/job-name": job.Name},
    ); err != nil {
        return fmt.Sprintf("dry-run report unavailable: list pods: %v", err)
    }

    var podName string
    for i := range pods.Items {
        if pods.Items[i].Status.Phase == corev1.PodSucceeded {
            podName = pods.Items[i].Name
            break
        }
    }
    if podName == "" {
        return "dry-run report unavailable: no succeeded pod found"
    }

    logOpts := &corev1.PodLogOptions{Container: "mendabot-agent"}
    req := r.KubeClient.CoreV1().Pods(r.Cfg.AgentNamespace).GetLogs(podName, logOpts)
    stream, err := req.Stream(ctx)
    if err != nil {
        return fmt.Sprintf("dry-run report unavailable: get logs: %v", err)
    }
    defer stream.Close()

    limited := io.LimitReader(stream, maxReportBytes)
    raw, err := io.ReadAll(limited)
    if err != nil {
        return fmt.Sprintf("dry-run report unavailable: read logs: %v", err)
    }

    // Extract only the content after the sentinel line.
    const sentinel = "=== DRY_RUN INVESTIGATION REPORT ==="
    logs := string(raw)
    if idx := strings.Index(logs, sentinel); idx >= 0 {
        return strings.TrimSpace(logs[idx+len(sentinel):])
    }
    // Sentinel absent — store raw logs with a note so the operator has something useful.
    return "(sentinel not found — raw log follows)\n" + logs
}
```

> **Imports required:** `"io"`, `"strings"`, `"fmt"`, `corev1 "k8s.io/api/core/v1"`,
> `"k8s.io/client-go/kubernetes"`.

### 3. Call `fetchDryRunReport` in the reconcile loop

In the `if len(ownedJobs.Items) > 0` block (around line 100), after `newPhase` is computed
and found to be `PhaseSucceeded`, add:

```go
if newPhase == v1alpha1.PhaseSucceeded &&
    job.Annotations["mendabot.io/dry-run"] == "true" &&
    rjob.Status.Message == "" {
    rjob.Status.Message = r.fetchDryRunReport(ctx, job)
}
```

Place this before the `r.Status().Patch(...)` call so the message is included in the same
patch that sets `Phase = Succeeded`.

### 4. Update `cmd/watcher/main.go`

```go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)

restCfg, err := rest.InClusterConfig()
if err != nil {
    setupLog.Fatal("failed to get in-cluster config", zap.Error(err))
}
kubeClient, err := kubernetes.NewForConfig(restCfg)
if err != nil {
    setupLog.Fatal("failed to create kube client", zap.Error(err))
}

// When registering the reconciler:
if err := (&controller.RemediationJobReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jb,
    Cfg:        cfg,
    KubeClient: kubeClient,
}).SetupWithManager(mgr); err != nil {
    setupLog.Fatal("unable to create controller", zap.Error(err))
}
```

Add the necessary RBAC marker to `remediationjob_controller.go`:

```go
//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get
```

---

## Test Cases

Tests live in `internal/controller/` alongside the existing controller tests. Use
`k8s.io/client-go/kubernetes/fake` for the `KubeClient`.

The fake log stream must include the sentinel line so tests verify extraction:

```
=== DRY_RUN INVESTIGATION REPORT ===
## Root Cause
ImagePullBackOff — image not found.
```

| Test Name | Setup | Assertion |
|-----------|-------|-----------|
| `TestReconcile_DryRunSucceeded_ReportStored` | Job with `mendabot.io/dry-run: "true"` and `Succeeded: 1`; fake pod with `Phase: Succeeded`; fake log stream contains sentinel + report text | `rjob.Status.Message` contains the post-sentinel report text, not the sentinel line itself |
| `TestReconcile_DryRunSucceeded_ReportTruncated` | Same as above but post-sentinel log content is > 10,000 bytes | `rjob.Status.Message` is at most 10,000 bytes |
| `TestReconcile_DryRunSucceeded_SentinelAbsent` | Same setup but log stream contains no sentinel line | `rjob.Status.Message` starts with `"(sentinel not found"` |
| `TestReconcile_DryRunSucceeded_NoPodFound` | Job succeeded, dry-run annotated, but no pods in list | `rjob.Status.Message` starts with `"dry-run report unavailable"` |
| `TestReconcile_NoDryRun_MessageNotPopulated` | Job succeeded, no dry-run annotation | `rjob.Status.Message == ""` |
| `TestFetchDryRunReport_NilKubeClient` | `KubeClient == nil` | returns `"dry-run report unavailable: KubeClient not configured"` |

---

## Tasks

- [x] Add `KubeClient kubernetes.Interface` to `RemediationJobReconciler` struct
- [x] Add `//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get` marker
- [x] Implement `fetchDryRunReport` method with `io.LimitReader` and sentinel extraction
- [x] Add `maxReportBytes = 10_000` constant
- [x] Wire dry-run branch in reconcile loop (detect `mendabot.io/dry-run` annotation)
- [x] Add `KubeClient` wiring in `cmd/watcher/main.go`
- [x] Write the six controller tests
- [x] Run `go test -race ./internal/controller/...` — all pass
- [x] Run `go build ./...` — clean

---

## Dependencies

**Depends on:** STORY_01 (`cfg.DryRun` field)
**Depends on:** STORY_02 (`mendabot.io/dry-run` annotation on the Job)
**Depends on:** STORY_03 (`entrypoint-common.sh` must emit the sentinel and report to stdout
in dry-run mode via `emit_dry_run_report`; implemented in STORY_03, not here)

---

## Definition of Done

- [x] All six new controller tests pass with `-race`
- [x] Existing controller tests unchanged and still pass
- [x] Full test suite passes: `go test -timeout 120s -race ./...`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
- [x] `kubectl get rjob <name> -o jsonpath='{.status.message}'` shows the investigation
  report after a dry-run Job succeeds (manual verification in a test cluster)
