# Story 02: CorrelationWindow in RemediationJobReconciler

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 4 hours

---

## User Story

As a **mendabot operator**, I want the `RemediationJobReconciler` to hold newly-created
`RemediationJob` objects in `Pending` phase for a configurable window before dispatching,
so that correlated findings created within the same window are grouped rather than
dispatched independently.

---

## Background

This is the most significant change in the epic. The reconciler today transitions a
`Pending` `RemediationJob` to `Dispatched` as soon as it sees it. After this story, it
holds the object for `CORRELATION_WINDOW_SECONDS` after creation, then runs the
correlator before deciding whether to dispatch or suppress.

The hold is implemented with `ctrl.Result{RequeueAfter: remaining}`, not a goroutine
sleep. This preserves idempotency and survives watcher restarts without any additional
durable state.

---

## Acceptance Criteria

- [ ] `RemediationJobReconciler` holds any new `Pending` job for `CORRELATION_WINDOW_SECONDS`
      (default: 30) using `ctrl.Result{RequeueAfter: remaining}` before proceeding
- [ ] After the window, the reconciler lists all `Pending` `RemediationJob` objects in the
      same namespace and passes them to `Correlator.Evaluate`
- [ ] `Correlator` struct exists in `internal/correlator/correlator.go` with method
      `Evaluate(ctx, candidate, peers, client) (CorrelationGroup, bool, error)`
      (the `bool` is `true` when a match was found; idiomatic Go "found" return)
- [ ] When `Correlator.Evaluate` returns `found=true` and the candidate **is the primary**:
  - All non-primary correlated peers (from `group.CorrelatedUIDs`) are transitioned to
    `Suppressed` phase with `CorrelationGroupID` set — by the primary's reconcile call
  - `mendabot.io/correlation-group-id` and `mendabot.io/correlation-role=primary` labels
    are patched onto the primary before `dispatch` is called
  - `mendabot.io/correlation-group-id` and `mendabot.io/correlation-role=correlated` labels
    are patched onto each suppressed peer
  - Primary is dispatched with correlated peer findings (excluding the primary's own
    finding, which is already in `rjob.Spec.Finding`) via `dispatch(ctx, rjob, group.AllFindings)`
- [ ] When `Correlator.Evaluate` returns `found=true` and the candidate **is not the primary**:
  - The candidate does **not** self-suppress
  - Returns `ctrl.Result{RequeueAfter: 5 * time.Second}, nil` to give the primary time
    to run its own reconcile and suppress this job
  - On the next reconcile, if the candidate is now `Suppressed` (primary acted), the
    `case v1alpha1.PhaseSuppressed` returns immediately
  - On the next reconcile, if the candidate is still `Pending` and still non-primary,
    it requeues again — this is safe; the primary will suppress it when its window elapses
- [ ] The `switch` statement in `Reconcile()` has an explicit `case v1alpha1.PhaseSuppressed`
      that returns `ctrl.Result{}, nil` immediately, preventing suppressed jobs from
      ever being re-dispatched on subsequent reconcile events
- [ ] When `r.Correlator == nil` (set when `DISABLE_CORRELATION=true` in `main.go`):
  - Window hold is skipped entirely
  - No correlator is called
  - Existing dispatch behaviour is unchanged
- [ ] `internal/controller/remediationjob_controller_test.go` covers:
  - Window hold: job created, reconcile returns `RequeueAfter`, job still `Pending`
  - Window elapsed, candidate is primary: primary dispatched with correlated findings,
    peer transitioned to `Suppressed` by primary's reconcile
  - Window elapsed, candidate is not primary: reconcile returns `RequeueAfter: 5s`,
    job still `Pending`; subsequent reconcile (after primary acts) finds `Suppressed`
    and returns immediately
  - Window elapsed, no correlation found: job dispatched normally
  - `r.Correlator == nil`: job dispatched immediately without hold
- [ ] `go test -timeout 30s -race ./internal/controller/...` passes

---

## Technical Implementation

### Why non-primaries must not self-suppress

If a non-primary self-suppresses before the primary has reconciled, the primary's
subsequent `pendingPeers` call filters by `Phase == Pending` only, so the now-Suppressed
non-primary is invisible. The primary dispatches as a solo job and the non-primary's
finding is permanently lost from the investigation context.

By having non-primaries requeue instead of self-suppress, all correlated findings remain
Pending until the primary's window elapses. The primary then atomically suppresses all
non-primary peers and dispatches with the full group context in a single reconcile call.

### `PhaseSuppressed` case in the reconciler switch

The `switch` in `Reconcile()` at line 64 currently handles `PhaseSucceeded`,
`PhaseFailed`, `PhasePermanentlyFailed`, and `PhaseCancelled`. Any unmatched phase falls
through to the owned-jobs list and eventually to dispatch. Add the `Suppressed` case
immediately after `PhaseCancelled` at line 104:

```go
case v1alpha1.PhaseSuppressed:
    return ctrl.Result{}, nil
```

Without this, every requeue event for a `Suppressed` job will proceed toward dispatch,
defeating suppression.

### Window hold insertion point

The window hold is inserted **after** the owned-jobs list block. If owned jobs exist,
the reconciler syncs status and returns at line 225. The window hold must be placed
between line 226 (end of the owned-jobs block) and line 228 (`// Check MAX_CONCURRENT_JOBS`).
This ensures the hold only applies to jobs that have no batch/v1 Job yet (pre-dispatch).
A job that already has an owned batch/v1 Job is in post-dispatch status-sync mode and
must never be held.

```go
// Window hold — inserted between line 226 and line 228:
if r.Correlator != nil {
    window := time.Duration(r.Cfg.CorrelationWindowSeconds) * time.Second
    age := time.Since(rjob.CreationTimestamp.Time)
    if age < window {
        return ctrl.Result{RequeueAfter: window - age}, nil
    }

    peers := r.pendingPeers(ctx, &rjob)
    group, found, err := r.Correlator.Evaluate(ctx, &rjob, peers, r.Client)
    if err != nil {
        return ctrl.Result{}, err
    }
    if found {
        isPrimary := group.PrimaryUID == rjob.UID
        if !isPrimary {
            // Requeue — do NOT self-suppress. The primary will suppress this job
            // when its own window elapses and it runs its reconcile.
            return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
        }
        // Primary path: suppress all correlated peers, then dispatch with correlated
        // peer findings. labelAsPrimary patches the group ID label onto rjob before
        // Build() runs — Build() reads it from rjob.Labels directly.
        if err := r.suppressCorrelatedPeers(ctx, peers, group); err != nil {
            return ctrl.Result{}, err
        }
        if err := r.labelAsPrimary(ctx, &rjob, group.GroupID); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, r.dispatch(ctx, &rjob, group.AllFindings)
    }
}
// No correlation (or correlator disabled): dispatch immediately with no correlated findings
return ctrl.Result{}, r.dispatch(ctx, &rjob, nil)
```

### `suppressCorrelatedPeers` helper

Iterates the original `peers` slice and suppresses any whose UID is in
`group.CorrelatedUIDs`. This runs within the primary's reconcile call.

```go
func (r *RemediationJobReconciler) suppressCorrelatedPeers(
    ctx context.Context,
    peers []*v1alpha1.RemediationJob,
    group correlator.CorrelationGroup,
) error {
    correlated := make(map[types.UID]struct{}, len(group.CorrelatedUIDs))
    for _, uid := range group.CorrelatedUIDs {
        correlated[uid] = struct{}{}
    }
    for _, peer := range peers {
        if _, ok := correlated[peer.UID]; !ok {
            continue
        }
        if err := r.transitionSuppressed(ctx, peer, group.GroupID, group.PrimaryUID); err != nil {
            return err
        }
    }
    return nil
}
```

### `labelAsPrimary` helper

Patches the `mendabot.io/correlation-group-id` and `mendabot.io/correlation-role=primary`
labels onto the primary RJob **before** `dispatch` is called, so that `Build()` can read
the group ID label when injecting `FINDING_CORRELATION_GROUP_ID` into the Job env.

```go
func (r *RemediationJobReconciler) labelAsPrimary(
    ctx context.Context,
    rjob *v1alpha1.RemediationJob,
    groupID string,
) error {
    rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
    if rjob.Labels == nil {
        rjob.Labels = map[string]string{}
    }
    rjob.Labels[domain.CorrelationGroupIDLabel] = groupID
    rjob.Labels[domain.CorrelationGroupRoleLabel] = domain.CorrelationRolePrimary
    return r.Patch(ctx, rjob, client.MergeFrom(rjobCopy))
}
```

### `transitionSuppressed` helper

Patches the `RemediationJob` status phase to `Suppressed`, sets
`status.correlationGroupID`, adds a `ConditionCorrelationSuppressed` Condition, and
patches the correlation labels onto the object metadata.
Uses two separate patches: one for status (`r.Status().Patch`) and one for labels
(`r.Patch`) to avoid overwriting other status fields.

```go
func (r *RemediationJobReconciler) transitionSuppressed(
    ctx context.Context,
    rjob *v1alpha1.RemediationJob,
    groupID string,
    primaryUID types.UID,
) error {
    rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
    rjob.Status.Phase = v1alpha1.PhaseSuppressed
    rjob.Status.CorrelationGroupID = groupID
    apimeta.SetStatusCondition(&rjob.Status.Conditions, metav1.Condition{
        Type:               v1alpha1.ConditionCorrelationSuppressed,
        Status:             metav1.ConditionTrue,
        Reason:             "CorrelatedGroupFound",
        Message:            fmt.Sprintf("suppressed: primary job UID %s handles investigation", string(primaryUID)),
        LastTransitionTime: metav1.Now(),
    })
    if err := r.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
        return err
    }
    // Label patch (separate to avoid clobbering status)
    rjobCopy2 := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
    if rjob.Labels == nil {
        rjob.Labels = map[string]string{}
    }
    rjob.Labels[domain.CorrelationGroupIDLabel] = groupID
    rjob.Labels[domain.CorrelationGroupRoleLabel] = domain.CorrelationRoleCorrelated
    return r.Patch(ctx, rjob, client.MergeFrom(rjobCopy2))
}
```

### `dispatch` helper — signature extension

The existing `dispatch` helper at line 270 currently has signature:
```go
func (r *RemediationJobReconciler) dispatch(ctx context.Context, rjob *v1alpha1.RemediationJob) error
```

Extend it to accept the correlated findings slice:
```go
func (r *RemediationJobReconciler) dispatch(
    ctx context.Context,
    rjob *v1alpha1.RemediationJob,
    correlatedFindings []v1alpha1.FindingSpec,
) error
```

The `groupID` is **not** passed as a parameter. `labelAsPrimary` patches the
`mendabot.io/correlation-group-id` label onto the primary before `dispatch` is called,
so `Build()` reads it directly from `rjob.Labels[domain.CorrelationGroupIDLabel]`. There
is no need to thread it through `dispatch`. This keeps the dispatch signature minimal and
avoids two sources of truth for the group ID.

Inside `dispatch`, replace the existing `r.JobBuilder.Build(rjob, nil)` call at line 298
with `r.JobBuilder.Build(rjob, correlatedFindings)`.

### `pendingPeers` helper

Lists all `Pending` `RemediationJob` objects in `r.Cfg.AgentNamespace`, excluding the
candidate itself:

```go
func (r *RemediationJobReconciler) pendingPeers(ctx context.Context, candidate *v1alpha1.RemediationJob) []*v1alpha1.RemediationJob {
    var list v1alpha1.RemediationJobList
    if err := r.List(ctx, &list, client.InNamespace(r.Cfg.AgentNamespace)); err != nil {
        return nil
    }
    var peers []*v1alpha1.RemediationJob
    for i := range list.Items {
        p := &list.Items[i]
        if p.UID == candidate.UID || p.Status.Phase != v1alpha1.PhasePending {
            continue
        }
        peers = append(peers, p)
    }
    return peers
}
```

### `Correlator` struct and `Evaluate` signature

```go
// internal/correlator/correlator.go

type CorrelationGroup struct {
    GroupID        string
    PrimaryUID     types.UID
    CorrelatedUIDs []types.UID
    Rule           string
    // AllFindings collects rjob.Spec.Finding from the primary and all correlated peers.
    // Populated by the Correlator after a rule match; passed to dispatch().
    AllFindings    []v1alpha1.FindingSpec
}

type Correlator struct {
    Rules []domain.CorrelationRule
}

// Evaluate applies rules in order, returning the first match.
// Returns (CorrelationGroup{}, false, nil) when no rule matches.
func (c *Correlator) Evaluate(
    ctx context.Context,
    candidate *v1alpha1.RemediationJob,
    peers []*v1alpha1.RemediationJob,
    cl client.Client,
) (CorrelationGroup, bool, error)
```

The correlator iterates `c.Rules` in order. On the first match it assembles
`CorrelationGroup.AllFindings` by collecting `rjob.Spec.Finding` from the candidate and
all matched peers (those whose UIDs are in `CorrelatedUIDs`), then returns
`(group, true, nil)`. If no rule matches, it returns `(CorrelationGroup{}, false, nil)`.

**AllFindings population:** This must happen inside `Correlator.Evaluate`, not in the
reconciler. After a rule returns `CorrelationResult{Matched: true}`, the correlator
resolves the primary job and the list of correlated peers, then populates `AllFindings`
with **only the non-primary findings**. The primary's own finding is already in
`rjob.Spec.Finding` at dispatch time — including it in `AllFindings` would duplicate
it in the `FINDING_CORRELATED_FINDINGS` env var and potentially confuse the agent:

```go
// Collect all jobs in the group (candidate + matched peers).
allJobs := make([]*v1alpha1.RemediationJob, 0, len(matchedPeers)+1)
allJobs = append(allJobs, candidate)
allJobs = append(allJobs, matchedPeers...)

// AllFindings contains only the non-primary findings.
// The primary's finding is already in rjob.Spec.Finding at dispatch time.
group.AllFindings = make([]v1alpha1.FindingSpec, 0, len(allJobs)-1)
for _, j := range allJobs {
    if j.UID != group.PrimaryUID {
        group.AllFindings = append(group.AllFindings, j.Spec.Finding)
    }
}
```

**AllFindings ordering is non-deterministic:** The `matchedPeers` slice is populated from
a `client.List` call whose return order is not guaranteed. Tests that assert on the
`FINDING_CORRELATED_FINDINGS` env var value **must** sort both the expected and actual
`FindingSpec` slices (e.g. by `finding.Name`) before comparing.

### Wiring the `Correlator` in `cmd/watcher/main.go`

The `Correlator` is an optional field on the reconciler. When `cfg.DisableCorrelation`
is true, the field is left nil and the reconciler's `r.Correlator != nil` guard skips all
correlation logic:

```go
// cmd/watcher/main.go — construct Correlator conditionally
var corr *correlator.Correlator
if !cfg.DisableCorrelation {
    corr = &correlator.Correlator{
        Rules: []domain.CorrelationRule{
            correlator.SameNamespaceParentRule{},
            correlator.PVCPodRule{},
            correlator.MultiPodSameNodeRule{Threshold: cfg.MultiPodThreshold},
        },
    }
}

if err := (&controller.RemediationJobReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jb,
    Cfg:        cfg,
    Recorder:   recorder,
    Correlator: corr,  // nil when DisableCorrelation=true
}).SetupWithManager(mgr); err != nil {
    ...
}
```

This requires adding `import "github.com/lenaxia/k8s-mendabot/internal/correlator"` to
`cmd/watcher/main.go`.

The escape hatch check in the reconciler is `if r.Correlator != nil` — do not use
`r.Cfg.DisableCorrelation` inside the reconciler. The nil check is the single source of
truth and avoids the reconciler needing to know about the config field name.

### Config additions (`internal/config/config.go`)

Add three fields to the `Config` struct (after `SelfRemediationCooldown` at line 60):

```go
// CorrelationWindowSeconds is how long to hold Pending jobs before dispatching.
// Default: 30. Set to 0 to skip the hold period — the correlator evaluates on the
// very first reconcile after phase initialisation, with no delay. This is distinct
// from DisableCorrelation: with 0-second window the correlator still runs and may
// group findings; with DisableCorrelation=true no correlation runs at all.
CorrelationWindowSeconds int // CORRELATION_WINDOW_SECONDS — default 30

// DisableCorrelation bypasses all correlation logic and dispatches immediately.
// When true, Correlator is never constructed; reconciler's nil guard handles the rest.
DisableCorrelation bool // DISABLE_CORRELATION — default false

// MultiPodThreshold is the minimum count of pod findings on the same node
// required to trigger MultiPodSameNodeRule.
MultiPodThreshold int // CORRELATION_MULTI_POD_THRESHOLD — default 3
```

Add the corresponding parsing to `FromEnv()`:

```go
// CORRELATION_WINDOW_SECONDS
corrWindowStr := os.Getenv("CORRELATION_WINDOW_SECONDS")
if corrWindowStr == "" {
    cfg.CorrelationWindowSeconds = 30
} else {
    n, err := strconv.Atoi(corrWindowStr)
    if err != nil {
        return Config{}, fmt.Errorf("CORRELATION_WINDOW_SECONDS must be an integer: %w", err)
    }
    if n < 0 {
        return Config{}, fmt.Errorf("CORRELATION_WINDOW_SECONDS must be >= 0, got %d", n)
    }
    // 0 is valid: the hold period is zero, so the correlator evaluates on the
    // first reconcile without waiting. Not the same as DISABLE_CORRELATION=true
    // (which skips correlation entirely).
    cfg.CorrelationWindowSeconds = n
}

// DISABLE_CORRELATION
cfg.DisableCorrelation = os.Getenv("DISABLE_CORRELATION") == "true"

// CORRELATION_MULTI_POD_THRESHOLD
threshStr := os.Getenv("CORRELATION_MULTI_POD_THRESHOLD")
if threshStr == "" {
    cfg.MultiPodThreshold = 3
} else {
    n, err := strconv.Atoi(threshStr)
    if err != nil {
        return Config{}, fmt.Errorf("CORRELATION_MULTI_POD_THRESHOLD must be an integer: %w", err)
    }
    if n < 1 {
        return Config{}, fmt.Errorf("CORRELATION_MULTI_POD_THRESHOLD must be >= 1, got %d", n)
    }
    cfg.MultiPodThreshold = n
}
```

### Helm chart wiring

The three new config fields must flow through the Helm chart.

**`charts/mendabot/values.yaml`** — add under the `watcher:` section (after
`stabilisationWindowSeconds`):

```yaml
  # -- Seconds to hold Pending jobs before dispatching (correlation window). Set to 0 to
  #    dispatch immediately without correlation. Default is 30.
  correlationWindowSeconds: 30
  # -- Disable all correlation logic and dispatch immediately. Default is false.
  disableCorrelation: false
  # -- Minimum number of pods failing on the same node to trigger MultiPodSameNodeRule.
  #    Default is 3.
  multiPodThreshold: 3
```

**`charts/mendabot/templates/deployment-watcher.yaml`** — add three env entries after the
`STABILISATION_WINDOW_SECONDS` entry at line 51:

```yaml
        - name: CORRELATION_WINDOW_SECONDS
          value: {{ .Values.watcher.correlationWindowSeconds | quote }}
        - name: DISABLE_CORRELATION
          value: {{ .Values.watcher.disableCorrelation | quote }}
        - name: CORRELATION_MULTI_POD_THRESHOLD
          value: {{ .Values.watcher.multiPodThreshold | quote }}
```

### Known Limitations

**Source deletion during the correlation window.** `SourceProviderReconciler` cancels and
deletes any `Pending` `RemediationJob` when its source object is deleted
(lines 95–96 of `internal/provider/provider.go`). If a correlated peer's source object is
deleted while the peer is still inside the correlation window, the peer is cancelled and
removed from etcd before the primary's window elapses. The primary's subsequent
`pendingPeers` call will not find the cancelled peer. The primary dispatches with whatever
peers remain.

This is correct behaviour: if the finding's source is gone, the finding is resolved.
Implementers should be aware that `AllFindings` may therefore contain fewer entries than
the group had at window start. Tests should not rely on a fixed `AllFindings` count in
scenarios where source deletion is possible.

---

## Tasks

- [ ] Write reconciler tests for window hold and correlation paths (TDD — must fail first)
- [ ] Add `case v1alpha1.PhaseSuppressed: return ctrl.Result{}, nil` to the `switch` at
      line 64, immediately after the `PhaseCancelled` case (line 104–105)
- [ ] Write `internal/correlator/correlator.go` with `Correlator.Evaluate` and `CorrelationGroup`
      (including `AllFindings []v1alpha1.FindingSpec` and `CorrelatedUIDs []types.UID`)
- [ ] Write `internal/correlator/correlator_test.go`
- [ ] Add `Correlator *correlator.Correlator` field to `RemediationJobReconciler` struct
- [ ] Extend `dispatch()` signature at line 270 to accept `correlatedFindings []v1alpha1.FindingSpec`;
      update the existing fall-through call at line 247 to pass `nil`; do NOT add a `groupID`
      parameter — `Build()` reads it from `rjob.Labels[domain.CorrelationGroupIDLabel]` which
      is already set by `labelAsPrimary` before `dispatch` is called
- [ ] Implement `suppressCorrelatedPeers` helper
- [ ] Implement `labelAsPrimary` helper
- [ ] Implement `transitionSuppressed` helper (status patch + condition + separate label patch)
- [ ] Insert window hold block between line 226 and line 228 with the logic described above
      (non-primary requeues after 5s; primary calls `suppressCorrelatedPeers`, `labelAsPrimary`,
      then `dispatch` with full group context)
- [ ] Implement `pendingPeers` helper
- [ ] Add `CorrelationWindowSeconds`, `DisableCorrelation`, `MultiPodThreshold` to
      `config.Config` and parse them in `config.FromEnv()`
- [ ] Add `config_test.go` cases for the three new fields
- [ ] Add conditional `Correlator` construction in `cmd/watcher/main.go`
- [ ] Add three env var entries to `charts/mendabot/templates/deployment-watcher.yaml`
- [ ] Add three fields to `charts/mendabot/values.yaml` under `watcher:`
- [ ] Run `go test -timeout 30s -race ./...` — must pass

---

## Dependencies

**Depends on:** STORY_00 (domain types), STORY_01 (built-in rules)
**Blocks:** STORY_05 (integration tests validate this story end-to-end)

---

## Definition of Done

- [ ] Reconciler holds `Pending` jobs for the window duration
- [ ] Primary suppresses all correlated peers in its own reconcile call
- [ ] Non-primary requeues after 5s rather than self-suppressing
- [ ] `PhaseSuppressed` is a terminal case in the reconciler switch
- [ ] `DISABLE_CORRELATION=true` restores original behaviour
- [ ] Config fields parse correctly
- [ ] Helm chart wires the three new env vars
- [ ] All tests pass
