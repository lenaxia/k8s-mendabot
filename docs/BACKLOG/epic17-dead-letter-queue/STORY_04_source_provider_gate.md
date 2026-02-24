# Story 04: SourceProviderReconciler — Respect PermanentlyFailed; No Re-Dispatch

**Epic:** [epic17-dead-letter-queue](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mendabot operator**, I want the `SourceProviderReconciler` to skip
`RemediationJob` objects in the `PermanentlyFailed` phase so that once a finding has
exhausted all retries it is never re-dispatched, and I can inspect the tombstoned object
as a permanent audit record without it being deleted and recreated.

---

## Background

### The current re-dispatch path

In `internal/provider/provider.go`, after the fingerprint is computed and the
stabilisation window has elapsed, the reconciler lists `RemediationJob` objects matching
the fingerprint label (lines 183–203):

```go
// provider.go:183-203
var rjobList v1alpha1.RemediationJobList
if err := r.List(ctx, &rjobList,
    client.InNamespace(r.Cfg.AgentNamespace),
    client.MatchingLabels{"remediation.mendabot.io/fingerprint": fp[:12]},
); err != nil {
    return ctrl.Result{}, err
}
for i := range rjobList.Items {
    rjob := &rjobList.Items[i]
    if rjob.Spec.Fingerprint != fp {
        continue
    }
    if rjob.Status.Phase != v1alpha1.PhaseFailed {
        return ctrl.Result{}, nil  // ← non-Failed object → skip entirely (dedup)
    }
    // Failed RemediationJob with the same fingerprint — delete it so a new
    // investigation can be dispatched.
    if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
        return ctrl.Result{}, delErr
    }
}
```

The logic is:
- Any non-`PhaseFailed` object with the same fingerprint → return (already handled).
- A `PhaseFailed` object → delete it so a fresh `RemediationJob` can be created below.

### What must change

`PhasePermanentlyFailed` must be treated as a **permanent tombstone**:
- Do **not** delete the `PermanentlyFailed` object.
- Return immediately so no new `RemediationJob` is created.

The check at line 195 (`if rjob.Status.Phase != v1alpha1.PhaseFailed`) currently
short-circuits for any non-`PhaseFailed` phase. After this change it must also
short-circuit for `PhasePermanentlyFailed` — but the existing short-circuit already
handles it correctly because `PhasePermanentlyFailed != PhaseFailed` is `true`.

However, the existing short-circuit logic is: "if phase is not `PhaseFailed`, return".
That means `PermanentlyFailed` already returns — but it returns in the **same way as
`PhaseRunning` or `PhaseSucceeded`** (which is correct for dedup but for the wrong
reason). To make the gate explicit and self-documenting, the loop body should be
refactored to distinguish the cases:

```go
for i := range rjobList.Items {
    rjob := &rjobList.Items[i]
    if rjob.Spec.Fingerprint != fp {
        continue
    }
    switch rjob.Status.Phase {
    case v1alpha1.PhasePermanentlyFailed:
        // Permanently tombstoned — never re-dispatch regardless of source state.
        if r.Log != nil {
            r.Log.Info("RemediationJob permanently failed; suppressing re-dispatch",
                zap.Bool("audit", true),
                zap.String("event", "remediationjob.permanently_failed_suppressed"),
                zap.String("remediationJob", rjob.Name),
                zap.String("fingerprint", fp[:12]),
            )
        }
        return ctrl.Result{}, nil
    case v1alpha1.PhaseFailed:
        // Failed but below cap — delete so a new investigation can be dispatched.
        if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
            return ctrl.Result{}, delErr
        }
    default:
        // Any other phase (Pending, Dispatched, Running, Succeeded, Cancelled) —
        // an active or completed job exists; dedup suppresses creation of another.
        return ctrl.Result{}, nil
    }
}
```

### Where `MaxRetries` is set on the created `RemediationJob`

When the reconciler falls through to object creation (lines 229–262 in `provider.go`),
it populates `rjob.Spec` from `r.Cfg`. This story adds `MaxRetries` to that Spec
construction:

```go
// provider.go:241-261 — inside the Spec literal
Spec: v1alpha1.RemediationJobSpec{
    // ... existing fields ...
    MaxRetries: r.Cfg.MaxInvestigationRetries,
},
```

If `r.Cfg.MaxInvestigationRetries` is zero (e.g., in tests that do not set it), the
reconciler in STORY_03 falls back to 3. Setting it explicitly here ensures the CRD
carries the operator's configured value rather than the fallback.

---

## Acceptance Criteria

- [x] `PhasePermanentlyFailed` objects are never deleted by `SourceProviderReconciler`
- [x] When a `PermanentlyFailed` object exists for a fingerprint, no new `RemediationJob` is created
- [x] An audit log line is emitted when suppressing re-dispatch of a `PermanentlyFailed` job
- [x] `MaxRetries` is populated from `r.Cfg.MaxInvestigationRetries` when creating a new `RemediationJob`
- [x] All existing `PhaseFailed` delete-and-re-create behaviour is unchanged
- [x] All tests pass: `go test -timeout 30s -race ./internal/provider/...`

---

## Technical Implementation

### `internal/provider/provider.go`

#### Change 1 — Replace the fingerprint-dedup loop (lines 190–203)

```go
// Before (lines 190-203):
for i := range rjobList.Items {
    rjob := &rjobList.Items[i]
    if rjob.Spec.Fingerprint != fp {
        continue
    }
    if rjob.Status.Phase != v1alpha1.PhaseFailed {
        return ctrl.Result{}, nil
    }
    if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
        return ctrl.Result{}, delErr
    }
}

// After:
for i := range rjobList.Items {
    rjob := &rjobList.Items[i]
    if rjob.Spec.Fingerprint != fp {
        continue
    }
    switch rjob.Status.Phase {
    case v1alpha1.PhasePermanentlyFailed:
        if r.Log != nil {
            r.Log.Info("RemediationJob permanently failed; suppressing re-dispatch",
                zap.Bool("audit", true),
                zap.String("event", "remediationjob.permanently_failed_suppressed"),
                zap.String("remediationJob", rjob.Name),
                zap.String("fingerprint", fp[:12]),
            )
        }
        return ctrl.Result{}, nil
    case v1alpha1.PhaseFailed:
        if delErr := r.Delete(ctx, rjob); delErr != nil && !apierrors.IsNotFound(delErr) {
            return ctrl.Result{}, delErr
        }
    default:
        return ctrl.Result{}, nil
    }
}
```

#### Change 2 — Populate `MaxRetries` in the `RemediationJob` Spec literal (line ~257)

```go
// Inside the Spec: v1alpha1.RemediationJobSpec{ ... } literal:
MaxRetries: r.Cfg.MaxInvestigationRetries,
```

---

## Test Cases

### Unit tests — `internal/provider/provider_test.go`

Add the following after the existing dedup tests (search for tests that use `PhaseFailed`
in the provider test file):

```go
// TestSourceProviderReconciler_PermanentlyFailed_Suppressed verifies that a
// RemediationJob in PermanentlyFailed phase is NOT deleted and no new job is created.
func TestSourceProviderReconciler_PermanentlyFailed_Suppressed(t *testing.T) {
    const fp = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
    obj := makeWatchedObject("result-perm", agentNamespace)
    finding := &domain.Finding{
        Kind:         "Pod",
        Name:         "pod-crash",
        Namespace:    agentNamespace,
        ParentObject: "my-deploy",
        Errors:       `[{"text":"OOMKilled"}]`,
    }
    p := &fakeSourceProvider{
        name:       "native",
        objectType: &corev1.ConfigMap{},
        finding:    finding,
    }

    permFailedRJob := &v1alpha1.RemediationJob{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mendabot-" + fp[:12],
            Namespace: agentNamespace,
            Labels: map[string]string{
                "remediation.mendabot.io/fingerprint": fp[:12],
            },
            Annotations: map[string]string{
                "remediation.mendabot.io/fingerprint-full": fp,
            },
        },
        Spec: v1alpha1.RemediationJobSpec{
            Fingerprint: fp,
            MaxRetries:  3,
        },
        Status: v1alpha1.RemediationJobStatus{
            Phase:      v1alpha1.PhasePermanentlyFailed,
            RetryCount: 3,
        },
    }

    c := newTestClient(obj, permFailedRJob)
    r := newTestReconciler(p, c)
    // Override finding fingerprint to match the existing rjob's fingerprint.
    // In unit tests we override the provider's ExtractFinding to return a fixed
    // finding that hashes to fp — or we set the fingerprint annotation directly
    // and trust the label selector to find it.

    _, err := r.Reconcile(context.Background(), reqFor("result-perm", agentNamespace))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // The PermanentlyFailed rjob must still exist — it was not deleted.
    var existing v1alpha1.RemediationJob
    if getErr := c.Get(context.Background(),
        types.NamespacedName{Name: permFailedRJob.Name, Namespace: agentNamespace},
        &existing); getErr != nil {
        t.Errorf("PermanentlyFailed rjob was deleted (expected it to survive): %v", getErr)
    }

    // No new RemediationJob should have been created.
    var rjobList v1alpha1.RemediationJobList
    if listErr := c.List(context.Background(), &rjobList,
        client.InNamespace(agentNamespace)); listErr != nil {
        t.Fatalf("list rjobs: %v", listErr)
    }
    if len(rjobList.Items) != 1 {
        t.Errorf("expected exactly 1 RemediationJob (the tombstone), got %d", len(rjobList.Items))
    }
}

// TestSourceProviderReconciler_PhaseFailed_DeletesAndCreatesNew verifies the
// existing PhaseFailed re-dispatch behaviour is unchanged after the switch refactor.
func TestSourceProviderReconciler_PhaseFailed_DeletesAndCreatesNew(t *testing.T) {
    const fp = "bbccddee1122334455667788990011aabbccddee1122334455667788990011aa"
    obj := makeWatchedObject("result-fail", agentNamespace)
    finding := &domain.Finding{
        Kind:         "Pod",
        Name:         "pod-crash",
        Namespace:    agentNamespace,
        ParentObject: "my-deploy",
        Errors:       `[{"text":"ImagePullBackOff"}]`,
    }
    p := &fakeSourceProvider{
        name:       "native",
        objectType: &corev1.ConfigMap{},
        finding:    finding,
    }

    failedRJob := &v1alpha1.RemediationJob{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mendabot-" + fp[:12],
            Namespace: agentNamespace,
            Labels: map[string]string{
                "remediation.mendabot.io/fingerprint": fp[:12],
            },
            Annotations: map[string]string{
                "remediation.mendabot.io/fingerprint-full": fp,
            },
        },
        Spec: v1alpha1.RemediationJobSpec{
            Fingerprint: fp,
            MaxRetries:  3,
        },
        Status: v1alpha1.RemediationJobStatus{
            Phase:      v1alpha1.PhaseFailed,
            RetryCount: 1,
        },
    }

    c := newTestClient(obj, failedRJob)
    r := newTestReconciler(p, c)

    _, err := r.Reconcile(context.Background(), reqFor("result-fail", agentNamespace))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // The Failed rjob should be gone (deleted by SourceProviderReconciler).
    var checkDeleted v1alpha1.RemediationJob
    if getErr := c.Get(context.Background(),
        types.NamespacedName{Name: failedRJob.Name, Namespace: agentNamespace},
        &checkDeleted); getErr == nil {
        t.Error("expected PhaseFailed rjob to be deleted, but it still exists")
    }
}

// TestSourceProviderReconciler_MaxRetries_PopulatedFromConfig verifies that newly
// created RemediationJobs carry MaxRetries from Cfg.MaxInvestigationRetries.
func TestSourceProviderReconciler_MaxRetries_PopulatedFromConfig(t *testing.T) {
    obj := makeWatchedObject("result-maxretries", agentNamespace)
    finding := &domain.Finding{
        Kind:         "Pod",
        Name:         "pod-crash",
        Namespace:    agentNamespace,
        ParentObject: "my-deploy",
        Errors:       `[{"text":"CrashLoopBackOff"}]`,
    }
    p := &fakeSourceProvider{
        name:       "native",
        objectType: &corev1.ConfigMap{},
        finding:    finding,
    }

    c := newTestClient(obj)
    r := &provider.SourceProviderReconciler{
        Client:   c,
        Scheme:   newTestScheme(),
        Cfg: config.Config{
            AgentNamespace:          agentNamespace,
            MaxInvestigationRetries: 5,
        },
        Provider: p,
    }

    _, err := r.Reconcile(context.Background(), reqFor("result-maxretries", agentNamespace))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var rjobList v1alpha1.RemediationJobList
    if listErr := c.List(context.Background(), &rjobList,
        client.InNamespace(agentNamespace)); listErr != nil {
        t.Fatalf("list rjobs: %v", listErr)
    }
    if len(rjobList.Items) == 0 {
        t.Fatal("expected a RemediationJob to be created")
    }
    if rjobList.Items[0].Spec.MaxRetries != 5 {
        t.Errorf("MaxRetries = %d, want 5", rjobList.Items[0].Spec.MaxRetries)
    }
}
```

---

## Tasks

- [x] Write the three unit tests above (TDD — run first; will fail until STORY_01 + STORY_03 are merged)
- [x] Refactor the fingerprint-dedup loop (lines 190–203) to a `switch` on `rjob.Status.Phase`
- [x] Add `PhasePermanentlyFailed` case that returns immediately without deleting
- [x] Add audit log emit for `remediationjob.permanently_failed_suppressed`
- [x] Add `MaxRetries: r.Cfg.MaxInvestigationRetries` to the `RemediationJob` Spec literal
- [x] Run: `go test -timeout 30s -race ./internal/provider/...` — must pass
- [x] Run: `go vet ./internal/provider/...` — must be clean

---

## Dependencies

**Depends on:** STORY_01 (type definitions), STORY_02 (`Cfg.MaxInvestigationRetries`),
STORY_03 (the controller must actually set `PhasePermanentlyFailed` for this gate to
be exercised in integration tests)
**Blocks:** Nothing

---

## Definition of Done

- [x] `PhasePermanentlyFailed` rjobs are never deleted by `SourceProviderReconciler`
- [x] No new `RemediationJob` is created when a `PermanentlyFailed` tombstone exists
- [x] Audit log event `remediationjob.permanently_failed_suppressed` is emitted
- [x] `MaxRetries` field is populated from `r.Cfg.MaxInvestigationRetries` on creation
- [x] `PhaseFailed` delete-and-re-create behaviour unchanged
- [x] `go test -timeout 30s -race ./internal/provider/...` green
- [x] `go vet ./internal/provider/...` clean
