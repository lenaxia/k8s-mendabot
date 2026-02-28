# Worklog: Epic 11 Story 02 — jobProvider Depth-Aware Detection

**Date:** 2026-02-25
**Session:** Replace unconditional mechanic-job guard with depth-aware detection in jobProvider
**Status:** Complete

---

## Objective

Replace the unconditional `(nil, nil)` guard for `app.kubernetes.io/managed-by: mechanic-watcher`
jobs in `internal/provider/native/job.go` with depth-aware logic that produces a `Finding` with
`ChainDepth` computed from the owning `RemediationJob`. This enables self-remediation cascade
investigations up to the configured depth limit (enforcement in STORY_03).

---

## Work Completed

### 1. Test-first implementation (TDD)

- Deleted `TestJobProvider_ExtractFinding_ExcludesMechanicManagedJobs` (lines 398–431 in
  the pre-change file) — that test asserted the old `(nil, nil)` behaviour.
- Added import `v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"` to `job_test.go`.
- Added 9 new test cases covering all branches of the new logic:
  1. `TestJobChainDepth_NonMechanicJob` — non-mechanic job → ChainDepth = 0
  2. `TestJobChainDepth_MechanicOwnerDepth0` — owner RJob.ChainDepth=0 → ChainDepth = 1
  3. `TestJobChainDepth_MechanicOwnerDepth1` — owner RJob.ChainDepth=1 → ChainDepth = 2
  4. `TestJobChainDepth_MechanicOwnerNotFound` — no RJob in fake client → ChainDepth = 1
  5. `TestJobChainDepth_MechanicNoOwnerRef` — no OwnerReferences → ChainDepth = 1
  6. `TestJobChainDepth_MechanicStillActive` — Active=1, Failed=0 → (nil, nil)
  7. `TestJobChainDepth_MechanicSucceeded` — CompletionTime != nil → (nil, nil)
  8. `TestJobChainDepth_MechanicCronJobOwned` — CronJob owner ref → (nil, nil)
  9. `TestJobChainDepth_MechanicSuspended` — JobSuspended=True condition → (nil, nil)
- Confirmed all 9 new tests failed before implementation.

### 2. Production code changes (`internal/provider/native/job.go`)

- Added imports: `apierrors "k8s.io/apimachinery/pkg/api/errors"` and
  `v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"`.
- Replaced the unconditional guard block (3 lines) with:
  `isMechanicJob := job.Labels["app.kubernetes.io/managed-by"] == "mechanic-watcher"`
- Added `chainDepth` computation after all existing guards and before building the finding:
  calls `p.getChainDepthFromOwner()` when `isMechanicJob` is true, defaults to 0 otherwise.
- Set `ChainDepth: chainDepth` in the `domain.Finding` struct literal.
- Added `getChainDepthFromOwner` private helper (lines 136–154) matching the exact spec
  from STORY_02: iterates `OwnerReferences`, looks up RemediationJob via `p.client.Get`,
  returns `rjob.Spec.Finding.ChainDepth + 1` on success, `1` on not-found or no owner,
  `(0, err)` on unexpected API errors.

---

## Key Decisions

- `chainDepth` computation is placed after all existing skip guards (CronJob, Suspended,
  failure detection) so `getChainDepthFromOwner` is only called for jobs that will actually
  produce a Finding. This avoids unnecessary API calls for skipped jobs.
- Used `context.Background()` in `getChainDepthFromOwner` call, consistent with the pattern
  already used in `ExtractFinding` for `getParent`.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/provider/native/...
# ok (1.312s, 9 new tests passing)

go build ./...
# clean

go vet ./...
# clean

go test -timeout 30s -race ./...
# all 15 packages pass
```

---

## Next Steps

STORY_03: Enforce `SELF_REMEDIATION_MAX_DEPTH` in the reconciler — use `Finding.ChainDepth`
to decide whether to create a `RemediationJob` or drop the finding with a logged warning.

---

## Files Modified

- `internal/provider/native/job.go` — replaced guard, added `getChainDepthFromOwner`, set `ChainDepth`
- `internal/provider/native/job_test.go` — deleted old test, added 9 new tests, added v1alpha1 import
