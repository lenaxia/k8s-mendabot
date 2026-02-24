# Story: RemediationJobReconciler — read investigation report from Job logs

**Epic:** [epic20-dry-run-mode](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **cluster operator** using dry-run mode, I want `kubectl get rjob <name> -o yaml` to
show the agent's investigation report in `status.message`, so that I can review what
mendabot would have done without leaving the Kubernetes API.

---

## Background

### `RemediationJobStatus.Message` — already exists

`api/v1alpha1/remediationjob_types.go` line 173 shows that `Message string` is **already
present** in `RemediationJobStatus`:

```go
// Message is a human-readable description of the current state,
// e.g. an error message if Phase is Failed.
Message string `json:"message,omitempty"`
```

`DeepCopyInto` already copies it (line 212). No CRD type changes are required by this story.

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

The agent writes the report to `/workspace/investigation-report.txt` on the shared-workspace
`EmptyDir` volume. The entrypoint script (`agent-entrypoint.sh`) writes to that path and
then exits. The report content appears in the pod's stdout only if the entrypoint explicitly
`cat`s it before `exec opencode`. However, the **investigation-report.txt** file exists only
on the pod's ephemeral volume — it is not part of container stdout.

The correct approach is to use the Kubernetes **CoreV1 Pods GetLogs API**. After the Job
succeeds, the controller calls:

```go
logReq := r.CoreV1Client.CoreV1().Pods(r.Cfg.AgentNamespace).GetLogs(podName, &corev1.PodLogOptions{
    Container: "mendabot-agent",
})
```

This returns the `mendabot-agent` container's stdout. To make the report available via logs,
**STORY_03 must additionally update `agent-entrypoint.sh`** to cat the report file to stdout
after `opencode` exits in dry-run mode (see Coordination note below).

Alternatively — and more reliably — the entrypoint can print the report to stdout immediately
before `exec opencode` exits. The simplest implementation is:

```bash
# In dry-run mode, after opencode writes the report:
if [ "${DRY_RUN:-false}" = "true" ]; then
    echo "=== DRY_RUN INVESTIGATION REPORT ==="
    cat /workspace/investigation-report.txt
fi
```

This ensures the report appears in the pod logs regardless of how `opencode` exits.

> **Coordination note:** The `cat` call above belongs in `docker/scripts/agent-entrypoint.sh`
> and should be implemented as part of STORY_03 (prompt), not this story. STORY_04 assumes
> the report is present in the `mendabot-agent` container logs when the Job has succeeded and
> the `mendabot.io/dry-run: "true"` annotation is set on the Job.

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
| `internal/controller/remediationjob_controller.go` | new private method `fetchDryRunReport` |
| `docker/scripts/agent-entrypoint.sh` | add `cat /workspace/investigation-report.txt` to stdout after opencode exits in dry-run mode (see Coordination note — this may be implemented as part of STORY_03) |
| `cmd/watcher/main.go` | populate `KubeClient` on the reconciler |

---

## Acceptance Criteria

- [ ] `RemediationJobReconciler` gains a `KubeClient kubernetes.Interface` field
- [ ] When a Job succeeds (`job.Status.Succeeded > 0`) **and** has annotation
  `mendabot.io/dry-run: "true"`, the reconciler:
  1. Lists pods with label `batch.kubernetes.io/job-name: <job-name>` in `cfg.AgentNamespace`
  2. Selects the first pod with phase `Succeeded`
  3. Fetches logs for container `"mendabot-agent"` via `KubeClient.CoreV1().Pods(...).GetLogs(...).Stream(ctx)`
  4. Reads up to **10,000 bytes** of log output
  5. Stores the truncated content in `rjob.Status.Message`
  6. Patches `rjob.Status` via `r.Status().Patch(...)`
- [ ] If no succeeded pod is found, or if the log fetch fails, the reconciler logs a warning
  and sets `rjob.Status.Message` to a descriptive error string — it does **not** return an
  error that would trigger a retry loop, since the Job itself succeeded
- [ ] When `DRY_RUN` is not set (normal mode), `status.message` is never populated by this
  code path — it remains whatever it was (empty by default)
- [ ] `docker/scripts/agent-entrypoint.sh` cats the report to stdout before exiting in
  dry-run mode (either implemented here or confirmed implemented in STORY_03)
- [ ] `cmd/watcher/main.go` creates a `kubernetes.Clientset` from in-cluster config and
  passes it as `KubeClient`
- [ ] Unit tests cover the new log-fetch path using a fake `kubernetes.Interface`
- [ ] `go test -race ./internal/controller/...` passes

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
// container logs of the first succeeded pod owned by job. Returns the report
// truncated to maxReportBytes, or an error description string on failure.
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

    buf := make([]byte, maxReportBytes)
    n, _ := io.ReadFull(stream, buf)
    return string(buf[:n])
}
```

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

### 4. Update `agent-entrypoint.sh` (coordination with STORY_03)

Add the following block **after** `exec opencode run "$(cat /tmp/rendered-prompt.txt)"`:

```bash
# In dry-run mode, emit the investigation report to stdout so the
# watcher can read it via the Kubernetes pod logs API.
if [ "${DRY_RUN:-false}" = "true" ] && [ -f /workspace/investigation-report.txt ]; then
    echo "=== DRY_RUN INVESTIGATION REPORT ==="
    cat /workspace/investigation-report.txt
fi
```

> **Note:** `exec` replaces the shell process, so the block above must appear *before*
> the `exec opencode` line, not after. The entrypoint should be restructured to run
> `opencode` without `exec` in dry-run mode, then cat the report:
>
> ```bash
> if [ "${DRY_RUN:-false}" = "true" ]; then
>     opencode run "$(cat /tmp/rendered-prompt.txt)"
>     echo "=== DRY_RUN INVESTIGATION REPORT ==="
>     cat /workspace/investigation-report.txt
> else
>     exec opencode run "$(cat /tmp/rendered-prompt.txt)"
> fi
> ```

### 5. Update `cmd/watcher/main.go`

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

| Test Name | Setup | Assertion |
|-----------|-------|-----------|
| `TestReconcile_DryRunSucceeded_ReportStored` | Job with `mendabot.io/dry-run: "true"` and `Succeeded: 1`; fake pod with `Phase: Succeeded`; fake log stream returns report text | `rjob.Status.Message` contains the report text |
| `TestReconcile_DryRunSucceeded_ReportTruncated` | Same as above but log stream returns > 10,000 bytes | `rjob.Status.Message` is exactly 10,000 bytes |
| `TestReconcile_DryRunSucceeded_NoPodFound` | Job succeeded, dry-run annotated, but no pods in list | `rjob.Status.Message` starts with `"dry-run report unavailable"` |
| `TestReconcile_NoDryRun_MessageNotPopulated` | Job succeeded, no dry-run annotation | `rjob.Status.Message == ""` |
| `TestFetchDryRunReport_NilKubeClient` | `KubeClient == nil` | returns `"dry-run report unavailable: KubeClient not configured"` |

---

## Tasks

- [ ] Add `KubeClient kubernetes.Interface` to `RemediationJobReconciler` struct
- [ ] Add `//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get` marker
- [ ] Implement `fetchDryRunReport` method
- [ ] Add `maxReportBytes = 10_000` constant
- [ ] Wire dry-run branch in reconcile loop (detect `mendabot.io/dry-run` annotation)
- [ ] Restructure `agent-entrypoint.sh` to cat report in dry-run mode (or confirm
  STORY_03 covers this — must not be done twice)
- [ ] Add `KubeClient` wiring in `cmd/watcher/main.go`
- [ ] Write the five controller tests
- [ ] Run `go test -race ./internal/controller/...` — all pass
- [ ] Run `go build ./...` — clean

---

## Dependencies

**Depends on:** STORY_01 (`cfg.DryRun` field)
**Depends on:** STORY_02 (`mendabot.io/dry-run` annotation on the Job)
**Depends on:** STORY_03 (the `agent-entrypoint.sh` must emit the report to stdout in dry-run
mode; if STORY_03 does not include the entrypoint change, it must be done here)

---

## Definition of Done

- [ ] All five new controller tests pass with `-race`
- [ ] Existing controller tests unchanged and still pass
- [ ] Full test suite passes: `go test -timeout 120s -race ./...`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
- [ ] `kubectl get rjob <name> -o jsonpath='{.status.message}'` shows the investigation
  report after a dry-run Job succeeds (manual verification in a test cluster)
