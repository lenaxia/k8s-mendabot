# Story 03: Wire SinkCloser into SourceProviderReconciler

## Status: Complete

## Goal

Add the `SinkCloser` and `PRAutoClose` fields to `SourceProviderReconciler` and
`config.Config`, then add the two auto-close trigger paths described in the README:

- **Path A** — finding-cleared while the job is in a non-terminal phase
  (Pending/Dispatched/Running/Suppressed): call `SinkCloser.Close` before cancelling
- **Path B** — finding-cleared for a job already in `PhaseSucceeded`: call
  `SinkCloser.Close` but do **not** delete the rjob (it is the dedup tombstone)

This story is the integration point. STORY_00, 01, and 02 must be complete first.

## Background

See [README.md](README.md) for the full trigger design and the flapping-finding /
dedup interaction. The key design constraints:

1. Sink closure failure must **never** block a `RemediationJob` cancellation or
   prevent the reconciler from returning. Log and continue.
2. `PhaseSucceeded` rjobs must NOT be deleted after auto-close — they are the dedup
   tombstone. Deleting them would allow re-dispatch before TTL.
3. Old rjobs with `SinkRef.URL == ""` are silently skipped.
4. `PR_AUTO_CLOSE=false` skips closure entirely but does not change cancellation logic.

## Acceptance Criteria

- [x] `config.Config` has `PRAutoClose bool` populated from `PR_AUTO_CLOSE` env var
      (default `true`)
- [x] `SourceProviderReconciler` has a `SinkCloser domain.SinkCloser` field
- [x] Path A: `SinkCloser.Close` called for in-flight jobs with `SinkRef.URL != ""`
      before cancellation; failure logged, not returned
- [x] Path B: `SinkCloser.Close` called for `PhaseSucceeded` jobs with
      `SinkRef.URL != ""`; rjob is NOT deleted afterward; failure logged, not returned
- [x] `PR_AUTO_CLOSE=false` → neither Path A nor Path B calls `Close`
- [x] `SinkCloser == nil` → gracefully skipped (do not panic; nil check before call)
- [x] `go test -timeout 30s -race ./...` passes
- [x] `go build ./...` succeeds

## Implementation Notes

### internal/config/config.go

Add to `Config` struct:

```go
// PRAutoClose controls whether open sinks are auto-closed when a finding resolves.
// Default: true. Set PR_AUTO_CLOSE=false to disable.
PRAutoClose bool // PR_AUTO_CLOSE — default true
```

Add to `FromEnv()`:

```go
prAutoCloseStr := os.Getenv("PR_AUTO_CLOSE")
switch prAutoCloseStr {
case "", "true", "1":
    cfg.PRAutoClose = true
case "false", "0":
    cfg.PRAutoClose = false
default:
    return Config{}, fmt.Errorf("PR_AUTO_CLOSE must be 'true', 'false', '1', or '0', got %q", prAutoCloseStr)
}
```

### internal/provider/provider.go

Add field to `SourceProviderReconciler`:

```go
// SinkCloser closes open GitHub sinks when a finding resolves.
// nil disables auto-close (equivalent to PR_AUTO_CLOSE=false).
SinkCloser domain.SinkCloser
```

**Path A** — in the `!apierrors.IsNotFound(err)` → finding-cleared block, inside the
loop over `rjobList.Items`, before the existing phase check that guards cancellation:

```go
// Auto-close: call before transitioning to Cancelled so we have the full
// rjob status available (SinkRef is cleared after deletion).
if r.Cfg.PRAutoClose && r.SinkCloser != nil && rjob.Status.SinkRef.URL != "" {
    reason := fmt.Sprintf(
        "Closing automatically: the underlying issue (%s/%s %s) has resolved. No manual fix is required.",
        rjob.Spec.Finding.Kind, rjob.Spec.Finding.Name, rjob.Spec.Finding.Namespace)
    if closeErr := r.SinkCloser.Close(ctx, rjob, reason); closeErr != nil {
        if r.Log != nil {
            r.Log.Error(closeErr, "auto-close sink failed; continuing with cancellation",
                zap.Bool("audit", true),
                zap.String("event", "sink.close_failed"),
                zap.String("remediationJob", rjob.Name),
                zap.String("sinkURL", rjob.Status.SinkRef.URL),
            )
        }
    }
}
// existing phase guard and cancellation logic unchanged
```

**Path B** — in the same finding-cleared block, add a second pass after the existing
cancellation loop completes:

```go
// Path B: auto-close Succeeded jobs whose sink is still open.
// Do NOT delete these rjobs — they are the dedup tombstone.
if r.Cfg.PRAutoClose && r.SinkCloser != nil {
    for i := range rjobList.Items {
        rjob := &rjobList.Items[i]
        if rjob.Spec.SourceResultRef.Name != req.Name ||
            rjob.Spec.SourceResultRef.Namespace != req.Namespace {
            continue
        }
        if rjob.Status.Phase != v1alpha1.PhaseSucceeded {
            continue
        }
        if rjob.Status.SinkRef.URL == "" {
            continue
        }
        reason := fmt.Sprintf(
            "Closing automatically: the underlying issue (%s/%s %s) has resolved. No manual fix is required.",
            rjob.Spec.Finding.Kind, rjob.Spec.Finding.Name, rjob.Spec.Finding.Namespace)
        if closeErr := r.SinkCloser.Close(ctx, rjob, reason); closeErr != nil {
            if r.Log != nil {
                r.Log.Error(closeErr, "auto-close succeeded sink failed",
                    zap.Bool("audit", true),
                    zap.String("event", "sink.close_succeeded_failed"),
                    zap.String("remediationJob", rjob.Name),
                    zap.String("sinkURL", rjob.Status.SinkRef.URL),
                )
            }
        }
        // Intentionally no r.Delete(ctx, rjob) — tombstone must remain.
    }
}
```

**Path B also fires when `finding == nil`** (the object exists but has no active
error). The current code at `provider.go:137-143` returns early on `finding == nil`
**without fetching any rjobs**. The `finding == nil` path currently has no list call
at all — it just clears `firstSeen` and returns. Path B requires a list fetch here.

The change to `provider.go` for the `finding == nil` path is:

```go
if finding == nil {
    r.firstSeen.Clear()
    if r.EventRecorder != nil {
        r.EventRecorder.Event(obj, corev1.EventTypeNormal, "FindingCleared", "Finding cleared; no active finding on this object")
    }
    // Path B: auto-close Succeeded sinks for this source ref.
    // Only pay the List cost when auto-close is enabled.
    if r.Cfg.PRAutoClose && r.SinkCloser != nil {
        var rjobList v1alpha1.RemediationJobList
        if listErr := r.List(ctx, &rjobList, client.InNamespace(r.Cfg.AgentNamespace)); listErr == nil {
            r.autoCloseSucceededSinks(ctx, req.Name, req.Namespace, &rjobList)
        } else if r.Log != nil {
            r.Log.Error(listErr, "auto-close: failed to list rjobs for finding-cleared path")
        }
    }
    return ctrl.Result{}, nil
}
```

The `r.List()` is guarded by `r.Cfg.PRAutoClose && r.SinkCloser != nil` so it is a
no-op when auto-close is disabled — the happy path cost is zero. List errors are
logged but do not block the return; the finding is cleared regardless.

To avoid code duplication, extract a helper:

```go
// autoCloseSucceededSinks iterates rjobList (which must cover the full AgentNamespace
// — do not scope it to a single source ref before passing it in, the helper filters
// internally) and calls SinkCloser.Close on every PhaseSucceeded job whose
// SourceResultRef matches sourceRefName/sourceRefNamespace and whose SinkRef.URL is
// non-empty. The caller is responsible for guarding with PRAutoClose and SinkCloser
// nil checks before invoking this method.
//
// Called on every reconcile where the source finding is absent. Successive calls after
// the first are no-ops because GitHubSinkCloser treats GitHub's 422 (already-closed)
// response as success — the sink is already in the desired state and no further action
// is taken. This is the mechanism that prevents API spam on repeated reconciles.
func (r *SourceProviderReconciler) autoCloseSucceededSinks(
    ctx context.Context,
    sourceRefName, sourceRefNamespace string,
    rjobList *v1alpha1.RemediationJobList,
) {
    for i := range rjobList.Items {
        rjob := &rjobList.Items[i]
        if rjob.Spec.SourceResultRef.Name != sourceRefName ||
            rjob.Spec.SourceResultRef.Namespace != sourceRefNamespace {
            continue
        }
        if rjob.Status.Phase != v1alpha1.PhaseSucceeded || rjob.Status.SinkRef.URL == "" {
            continue
        }
        reason := fmt.Sprintf(
            "Closing automatically: the underlying issue (%s/%s %s) has resolved. No manual fix is required.",
            rjob.Spec.Finding.Kind, rjob.Spec.Finding.Name, rjob.Spec.Finding.Namespace)
        if err := r.SinkCloser.Close(ctx, rjob, reason); err != nil {
            if r.Log != nil {
                r.Log.Error(err, "auto-close succeeded sink failed",
                    zap.Bool("audit", true),
                    zap.String("event", "sink.close_succeeded_failed"),
                    zap.String("remediationJob", rjob.Name),
                    zap.String("sinkURL", rjob.Status.SinkRef.URL),
                )
            }
        }
    }
}
```

**List scoping contract:** `autoCloseSucceededSinks` always receives the full
`AgentNamespace`-scoped list. Do not pre-filter the list by `SourceResultRef` before
passing it in — the helper filters internally. If a caller passes a narrower list (e.g.
already filtered by fingerprint label), some Succeeded rjobs may be missed and their
PRs never closed.

Call `r.autoCloseSucceededSinks(...)`:
- In the `IsNotFound` path: after the existing cancellation loop completes, passing
  the same `rjobList` already fetched by that path.
- In the `finding == nil` path: after the new conditional `r.List()` call above.

### Test scenarios

All tests use a fake `SinkCloser` that records calls and can be configured to return
an error.

**Path A — in-flight job with SinkRef:**
- Pending rjob with `SinkRef.URL` set → `SinkCloser.Close` called once before
  cancellation; rjob ends in `PhaseCancelled` and is deleted

**Path A — in-flight job without SinkRef:**
- Pending rjob with `SinkRef.URL == ""` → `SinkCloser.Close` NOT called

**Path A — SinkCloser returns error:**
- `SinkCloser.Close` returns error → cancellation still proceeds; error is logged;
  no error returned from `Reconcile`

**Path A — PRAutoClose=false:**
- `cfg.PRAutoClose = false` → `SinkCloser.Close` NOT called even with `SinkRef` set

**Path B — Succeeded job with SinkRef, finding cleared:**
- `PhaseSucceeded` rjob with `SinkRef.URL` set → `SinkCloser.Close` called once;
  rjob is NOT deleted (still exists after reconcile)

**Path B — Succeeded job without SinkRef:**
- `PhaseSucceeded` rjob with `SinkRef.URL == ""` → `SinkCloser.Close` NOT called

**Path B — SinkCloser returns error:**
- `SinkCloser.Close` returns error → rjob not deleted; error logged; reconcile
  returns nil

**Path B — nil SinkCloser:**
- `r.SinkCloser = nil` → no panic; reconcile returns nil

**finding == nil path triggers Path B:**
- Object exists but `ExtractFinding` returns `(nil, nil)` + `PhaseSucceeded` rjob with
  `SinkRef.URL` set → `SinkCloser.Close` is called (a new `r.List()` is performed
  inside the `finding == nil` branch specifically for this case)

**Old rjob (prRef only, no sinkRef):**
- `PhaseSucceeded` rjob with `Status.PRRef = "https://..."` but `SinkRef.URL == ""` →
  `SinkCloser.Close` NOT called

**Path B idempotency across reconciles:**
- Source object is deleted; first reconcile closes the PR (Close returns nil) ✓
- If the reconciler fires again for the same source ref (e.g. cache resync),
  `GitHubSinkCloser.Close` calls the GitHub API again; GitHub returns 422 (already
  closed); closer treats 422 as success and returns nil — no spam, no error logged.
  This is the mechanism that prevents redundant API calls across multiple reconciles.
  Test: fake `SinkCloser` that returns nil on all calls; verify `Close` is called each
  time but `Reconcile` returns nil and rjob is never deleted.

## Files Touched

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `PRAutoClose bool` |
| `internal/provider/provider.go` | Add `SinkCloser` field; add Path A + Path B logic; extract `autoCloseSucceededSinks` helper |
| `internal/provider/provider_test.go` | Add test scenarios above |

## TDD Sequence

1. Add `PRAutoClose` to `config.Config` and `FromEnv()`; write config tests first
2. Write provider tests for Path A and Path B — all fail
3. Add `SinkCloser` field and Path A logic to `provider.go`
4. Add `autoCloseSucceededSinks` and Path B call sites
5. All provider tests pass
6. Run full suite: `go test -timeout 30s -race ./...`
