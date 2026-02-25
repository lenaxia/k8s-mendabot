# Story 02: jobProvider — Detect Mendabot Agent Jobs and Compute Chain Depth

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mendabot operator**, I want failed mendabot agent jobs to produce a
Finding (rather than being silently dropped), so that the watcher can trigger
a self-remediation investigation up to the configured depth limit.

---

## Problem

`internal/provider/native/job.go:53-55` currently returns `(nil, nil)` for any
job whose `app.kubernetes.io/managed-by` label equals `mendabot-watcher`. This
prevents a cascade loop but also prevents any investigation of a legitimately
failing agent job.

This story replaces that unconditional guard with depth-aware logic: a mendabot
agent job produces a Finding with `ChainDepth` computed from its owning
`RemediationJob`. Depth enforcement (whether to proceed) is handled in
STORY_03; this story only computes and surfaces the depth.

---

## Acceptance Criteria

- [ ] A failed `batch/v1` Job labelled `app.kubernetes.io/managed-by: mendabot-watcher`
  returns a non-nil `Finding` from `jobProvider.ExtractFinding`.
- [ ] `Finding.ChainDepth` equals the owning `RemediationJob`'s
  `Spec.Finding.ChainDepth` plus one.
- [ ] If the job has no owning `RemediationJob` (owner reference missing or
  object not found), `ChainDepth` defaults to `1`.
- [ ] All existing skip conditions are preserved and unchanged:
  - CronJob-owned jobs → `(nil, nil)`
  - Suspended jobs → `(nil, nil)`
  - Not-yet-failed jobs (failed == 0 || active != 0 || completionTime != nil) → `(nil, nil)`
- [ ] Non-mendabot jobs are unaffected; `ChainDepth` is always `0` for them.
- [ ] `getChainDepthFromOwner` is a private helper on `jobProvider` that:
  - Looks up the owning `RemediationJob` via `job.OwnerReferences`
  - Reads `RemediationJob.Spec.Finding.ChainDepth` and returns it + 1
  - Returns `(1, nil)` if no owner reference exists
  - Returns `(1, nil)` if the owner `RemediationJob` is not found (API 404)
  - Returns `(0, err)` only for unexpected API errors

---

## Technical Implementation

### Location: `internal/provider/native/job.go`

**Replace the unconditional mendabot guard at line 53-55:**

```go
// Before (unconditional guard — blocks all self-remediations):
if job.Labels["app.kubernetes.io/managed-by"] == "mendabot-watcher" {
    return nil, nil
}

// After (depth-aware — produces a Finding with ChainDepth):
isMendabotJob := job.Labels["app.kubernetes.io/managed-by"] == "mendabot-watcher"
```

The `isMendabotJob` flag is used at the end of `ExtractFinding` to populate
`Finding.ChainDepth` via `getChainDepthFromOwner`.

**New private helper:**

```go
// getChainDepthFromOwner reads the ChainDepth of the RemediationJob that owns
// this batch/v1 Job and returns depth+1 as the child chain depth.
// Returns 1 if no owning RemediationJob is found or if the owner is not present
// in the cluster — this is the safe default for a first-level self-remediation.
// Returns (0, err) only for unexpected API errors.
func (p *jobProvider) getChainDepthFromOwner(ctx context.Context, job *batchv1.Job) (int, error) {
    for _, ref := range job.OwnerReferences {
        if ref.Kind != "RemediationJob" {
            continue
        }
        var rjob v1alpha1.RemediationJob
        if err := p.client.Get(ctx, client.ObjectKey{
            Namespace: job.Namespace,
            Name:      ref.Name,
        }, &rjob); err != nil {
            if apierrors.IsNotFound(err) {
                return 1, nil
            }
            return 0, fmt.Errorf("jobProvider: reading owner RemediationJob %s: %w", ref.Name, err)
        }
        return int(rjob.Spec.Finding.ChainDepth) + 1, nil
    }
    return 1, nil
}
```

**Updated `ExtractFinding` skeleton (depth path only):**

```go
isMendabotJob := job.Labels["app.kubernetes.io/managed-by"] == "mendabot-watcher"

// ... existing CronJob, suspended, failure-detection guards unchanged ...

var chainDepth int
if isMendabotJob {
    var err error
    chainDepth, err = p.getChainDepthFromOwner(context.Background(), job)
    if err != nil {
        return nil, err
    }
}

// ... build errors slice unchanged ...

finding := &domain.Finding{
    Kind:         "Job",
    Name:         job.Name,
    Namespace:    job.Namespace,
    ParentObject: parent,
    Errors:       string(errorsJSON),
    ChainDepth:   chainDepth,
}
return finding, nil
```

### Import additions

`job.go` must import:
- `apierrors "k8s.io/apimachinery/pkg/api/errors"` (for `IsNotFound`)
- `"github.com/lenaxia/k8s-mendabot/api/v1alpha1"` (for `RemediationJob`)

The file already imports `"sigs.k8s.io/controller-runtime/pkg/client"` and
`"context"`.

---

## What this story does NOT do

- Does not enforce `SELF_REMEDIATION_MAX_DEPTH` — that is STORY_03.
- Does not call the circuit breaker — that is STORY_03 / STORY_04.
- Does not add config fields — that is STORY_03.
- Does not change any non-mendabot-job code paths.

---

## Files to modify

| File | Change |
|------|--------|
| `internal/provider/native/job.go` | Replace unconditional guard; add `getChainDepthFromOwner`; populate `Finding.ChainDepth` |
| `internal/provider/native/job_test.go` | Add tests (see below) |

---

## Testing Requirements

**Unit tests** (`internal/provider/native/job_test.go`):

**First:** Delete `TestJobProvider_ExtractFinding_ExcludesMendabotManagedJobs`
(lines 398–431). That test asserts `(nil, nil)` for a failed mendabot-labelled
job, which is the exact behaviour this story replaces. It will fail after the
change and must not be left in the file.

Use a fake client (`sigs.k8s.io/controller-runtime/pkg/client/fake`) to
stub `RemediationJob` objects. The `newTestScheme()` helper defined in
`internal/provider/native/parent_test.go` already registers `v1alpha1` types,
`corev1`, `appsv1`, and `batchv1` — use it directly. Do not create a separate
scheme function.

Helper functions `assertErrorTextContains`, `assertErrorsJSON`, `contains`, and
`ownerRef` are already defined in `package native` test files and are available
without redefinition.

| Test case | Setup | Expected `ChainDepth` | Expected nil? |
|---|---|---|---|
| Non-mendabot failed job | No label | `0` | no |
| Mendabot job, owner RJob has depth 0 | `RJob.Spec.Finding.ChainDepth = 0` | `1` | no |
| Mendabot job, owner RJob has depth 1 | `RJob.Spec.Finding.ChainDepth = 1` | `2` | no |
| Mendabot job, owner not found (404) | No RJob in fake client | `1` | no |
| Mendabot job, no owner reference | Empty `OwnerReferences` | `1` | no |
| Mendabot job, still active | `Active=1, Failed=0` | — | yes (nil) |
| Mendabot job, succeeded | `CompletionTime != nil` | — | yes (nil) |
| CronJob-owned mendabot job | CronJob owner ref | — | yes (nil) |
| Suspended mendabot job | `JobSuspended=True` | — | yes (nil) |

Multiple happy path and unhappy path tests are required per the project TDD
standards.

---

## Dependencies

**Depends on:** STORY_01 (`domain.Finding.ChainDepth` must exist)
**Blocks:** STORY_03 (reconciler needs depth in the Finding)

---

## Definition of Done

- [ ] All tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
- [ ] `getChainDepthFromOwner` tested for all owner-lookup branches
- [ ] Non-mendabot job paths unchanged and covered by existing tests
