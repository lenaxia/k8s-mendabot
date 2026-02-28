# AlertSourceReconciler — Low-Level Design

**Version:** 1.1
**Date:** 2026-02-25
**Status:** Proposed
**HLD Reference:** [§7, §8, §11](../HLD.md)

---

## 1. Overview

The `AlertSourceReconciler` manages the full lifecycle of external alert sources. It operates
as a controller-runtime reconciler with two reconcile targets:

1. **`AlertSource` CRs** — handles create, update, and delete events: registers/deregisters
   webhook paths on the `DynamicMux`, starts/stops polling goroutines, updates status.

2. **`RemediationJob` objects** (via `Watches`) — when a RJ that carries a
   `mechanic.io/pending-alert` annotation reaches a terminal phase (Succeeded or Failed),
   the reconciler reads the annotation, clears it atomically, and creates a new RJ from the
   pending finding. This requires **no `FindingCh` wired into `RemediationJobReconciler`**.

A separate goroutine drains the `FindingCh` channel (findings from webhook handlers and
pollers) and runs the priority resolution + dedup + create pipeline.

---

## 2. Struct and Dependencies

```go
// internal/provider/alertsource/reconciler.go

type AlertSourceReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Log      *zap.Logger
    Cfg      config.Config

    // Registry maps adapter type names to adapter instances.
    Registry alertsource.AdapterRegistry

    // DynamicMux is the shared webhook handler map.
    // Paths are added/removed by this reconciler using Register/Deregister.
    DynamicMux *alertsource.DynamicMux

    // FindingCh is the shared buffered channel all external findings flow through.
    // Capacity is configured by FINDING_CHANNEL_BUFFER (default: 500).
    // NOTE: this channel is NOT shared with RemediationJobReconciler.
    FindingCh chan domain.Finding

    // Pollers holds active polling goroutines keyed by AlertSource namespaced name.
    // Guarded by pollersMu.
    Pollers   map[string]context.CancelFunc
    PollersMu sync.Mutex

    // webhookPaths maps registered path → owner AlertSource namespaced name.
    // Guarded by webhooksMu.
    webhookPaths map[string]string
    webhooksMu   sync.Mutex

    // statusCounters holds in-memory counts per AlertSource name.
    // Flushed asynchronously to K8s status via flushStatusLoop.
    // Initialized lazily by the increment methods; NOT initialized externally from main.go
    // (sourceCounters is unexported and must not be referenced outside this package).
    statusCounters map[string]*sourceCounters
    statusMu       sync.Mutex

    // CascadeChecker, FirstSeen, and ReadinessChecker are independent instances —
    // not shared with SourceProviderReconciler. Each reconciler owns its own
    // stabilisation state and readiness cache.
    //
    // CircuitBreaker is intentionally absent: it guards self-remediation chains
    // (finding.IsSelfRemediation == true), which alert-sourced findings never trigger.
    CascadeChecker   cascade.Checker
    FirstSeen        *provider.BoundedMap
    ReadinessChecker readiness.Checker

    // managerCtx is the parent context for all poller goroutines. It is initialised
    // to context.Background() at construction time (see §2.1 main.go wiring) and
    // replaced with the manager's context in Start(). It is NEVER nil.
    //
    // CRITICAL: Reconcile() can be invoked by controller-runtime before Start() is
    // called (if AlertSource CRs already exist in the cluster at startup). Using
    // context.Background() as the initial value ensures that ensurePollerRunning()
    // does not panic on a nil context during that window. Pollers started before
    // Start() fires will be cancelled and restarted by ensurePollerRunning() when
    // the real manager context arrives via Start().
    //
    // MUST NOT use the per-reconcile ctx from Reconcile() for pollers — that context
    // is cancelled when the reconcile request completes, which would terminate pollers
    // immediately after each CR reconcile.
    managerCtx context.Context
}

type sourceCounters struct {
    received   int64
    dispatched int64
    suppressed int64
}
```

### 2.1 Registration in main.go

```go
// cmd/watcher/main.go

findingCh := make(chan domain.Finding, cfg.FindingChannelBuffer)
dynamicMux := alertsource.NewDynamicMux()

alertSourceReconciler := &alertsource.AlertSourceReconciler{
    Client:           mgr.GetClient(),
    Scheme:           mgr.GetScheme(),
    Log:              logger,
    Cfg:              cfg,
    Registry:         buildAdapterRegistry(mgr.GetClient()),
    DynamicMux:       dynamicMux,
    FindingCh:        findingCh,
    Pollers:          make(map[string]context.CancelFunc),
    webhookPaths:     make(map[string]string),
    // NOTE: statusCounters is NOT initialized here. It uses an unexported type
    // (sourceCounters) and is initialized lazily inside the reconciler via the
    // incrementReceived/incrementDispatched/incrementSuppressed methods, each of
    // which creates a new entry when the key is absent. This avoids exposing the
    // internal counter type to main.go.
    CascadeChecker:   cascadeChecker,
    ReadinessChecker: combinedChecker,
    FirstSeen:        provider.NewBoundedMap(1000, 0, 0),
    managerCtx:       context.Background(), // safe initial value; replaced by Start()
}
```

---

## 3. CR Lifecycle Reconciler

### 3.1 SetupWithManager

The reconciler watches two resource types:

```go
func (r *AlertSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Initialize internal maps that cannot be set from main.go (unexported types).
    // Pollers and webhookPaths are exported-type maps initialized in main.go wiring.
    // statusCounters uses the unexported sourceCounters type and must be initialized here.
    if r.statusCounters == nil {
        r.statusCounters = make(map[string]*sourceCounters)
    }

    // Predicate for RemediationJob watch: only fire when an RJ with the
    // pending-alert annotation transitions to a terminal phase.
    //
    // Terminal phases: Succeeded, Failed, Suppressed, Cancelled.
    // Suppressed: the RJ was absorbed into a correlation group — a primary RJ handles
    //   the investigation, so the pending finding should be dispatched now.
    // Cancelled: the native source object was deleted while the RJ was in-flight;
    //   the pending finding should still be dispatched (the alert source signal
    //   is independent of the native object's lifecycle).
    // If these phases are omitted the annotation persists on a terminal RJ forever
    // and the pending finding is silently dropped.
    pendingAlertPredicate := predicate.Funcs{
        UpdateFunc: func(e event.UpdateEvent) bool {
            newRJ, ok := e.ObjectNew.(*v1alpha1.RemediationJob)
            if !ok {
                return false
            }
            _, hasPending := newRJ.GetAnnotations()[v1alpha1.AnnotationPendingAlert]
            isTerminal := newRJ.Status.Phase == v1alpha1.PhaseSucceeded ||
                newRJ.Status.Phase == v1alpha1.PhaseFailed ||
                newRJ.Status.Phase == v1alpha1.PhaseSuppressed ||
                newRJ.Status.Phase == v1alpha1.PhaseCancelled
            return hasPending && isTerminal
        },
        CreateFunc:  func(e event.CreateEvent) bool { return false },
        DeleteFunc:  func(e event.DeleteEvent) bool { return false },
        GenericFunc: func(e event.GenericEvent) bool { return false },
    }

    return ctrl.NewControllerManagedBy(mgr).
        // GenerationChangedPredicate is required here. Without it, every
        // r.Status().Update() call at the end of Reconcile increments resourceVersion,
        // the informer fires an Update event, and Reconcile is re-queued. That re-queue
        // calls ensurePollerRunning, which unconditionally cancels and restarts the
        // poller — resetting its ticker. If the status flush interval (30s) is shorter
        // than the poll interval, the poller never actually fires. Generation only
        // increments on spec changes, so filtering to generation changes prevents
        // status-update-triggered reconcile loops.
        For(&v1alpha1.AlertSource{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
        Watches(
            &v1alpha1.RemediationJob{},
            handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
                // Map the RJ event to a synthetic reconcile request using the RJ's
                // namespace/name. The Reconcile function checks if it's an RJ request
                // vs. an AlertSource request by attempting to Get each type.
                return []reconcile.Request{{
                    NamespacedName: types.NamespacedName{
                        Namespace: obj.GetNamespace(),
                        Name:      obj.GetName(),
                    },
                }}
            }),
            builder.WithPredicates(pendingAlertPredicate),
        ).
        Complete(r)
}
```

### 3.2 Reconcile Loop

```go
func (r *AlertSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Determine request type: AlertSource CR or RemediationJob (from Watch)
    var rj v1alpha1.RemediationJob
    if err := r.Get(ctx, req.NamespacedName, &rj); err == nil {
        // It's a RemediationJob event from the pending-alert Watch
        return ctrl.Result{}, r.handlePendingAlert(ctx, &rj)
    }

    // Otherwise it's an AlertSource event
    var as v1alpha1.AlertSource
    if err := r.Get(ctx, req.NamespacedName, &as); err != nil {
        if apierrors.IsNotFound(err) {
            // CR deleted — clean up webhook, poller, and alert-sourced RJs
            r.deregisterWebhook(req.NamespacedName.String())
            r.stopPoller(req.NamespacedName.String())
            r.cancelAlertSourceRJs(ctx, req.NamespacedName.String())
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // Resolve adapter
    adapter, ok := r.Registry[as.Spec.Type]
    if !ok {
        r.setCondition(&as, "Ready", metav1.ConditionFalse, "UnknownType",
            fmt.Sprintf("no adapter registered for type %q", as.Spec.Type))
        return ctrl.Result{}, r.Status().Update(ctx, &as)
    }

    // Webhook management
    if as.Spec.Webhook != nil && as.Spec.Webhook.Enabled {
        if err := r.ensureWebhookRegistered(as, adapter); err != nil {
            r.setCondition(&as, "Ready", metav1.ConditionFalse, "PathConflict", err.Error())
            return ctrl.Result{}, r.Status().Update(ctx, &as)
        }
    } else {
        r.deregisterWebhook(req.NamespacedName.String())
    }

    // Poll management
    if as.Spec.Poll != nil && as.Spec.Poll.Enabled {
        r.ensurePollerRunning(ctx, as, adapter)
    } else {
        r.stopPoller(req.NamespacedName.String())
    }

    r.setCondition(&as, "Ready", metav1.ConditionTrue, "Configured", "")
    return ctrl.Result{}, r.Status().Update(ctx, &as)
}
```

### 3.3 Webhook Registration (uses DynamicMux)

```go
func (r *AlertSourceReconciler) ensureWebhookRegistered(
    as v1alpha1.AlertSource,
    adapter AlertSourceAdapter,
) error {
    path := as.Spec.Webhook.Path
    key := namespacedName(&as)

    r.webhooksMu.Lock()
    defer r.webhooksMu.Unlock()

    // Check for path conflicts with other AlertSources
    if existing, ok := r.webhookPaths[path]; ok && existing != key {
        return fmt.Errorf("path %q already registered by %s", path, existing)
    }

    handler := newWebhookHandler(adapter, as.Spec, r.FindingCh, r.Log)
    r.DynamicMux.Register(path, handler)      // thread-safe; no panic on re-registration
    r.webhookPaths[path] = key
    return nil
}

func (r *AlertSourceReconciler) deregisterWebhook(key string) {
    r.webhooksMu.Lock()
    defer r.webhooksMu.Unlock()

    for path, owner := range r.webhookPaths {
        if owner == key {
            r.DynamicMux.Deregister(path)    // removes handler; subsequent requests → 404
            delete(r.webhookPaths, path)
        }
    }
}
```

### 3.4 AlertSource Deletion: Cleanup of Active RJs

When an `AlertSource` CR is deleted, active RJs created by that source continue running to
completion (the remediation is still valid). Only new RJ creation stops. The webhook path is
deregistered so no further alerts from this source are processed. In-flight RJs are not
cancelled — this is intentional (unlike native source deletion, which cancels RJs because
the native object no longer exists).

```go
// cancelAlertSourceRJs is intentionally a no-op: alert-sourced RJs are not
// cancelled when their AlertSource CR is deleted. The ongoing investigation
// remains valid even if the source configuration is removed.
// This function exists as a documentation hook and for future use.
func (r *AlertSourceReconciler) cancelAlertSourceRJs(ctx context.Context, key string) {
    // no-op: alert-sourced RJs outlive their AlertSource CR
}
```

### 3.5 Poller Management

```go
func (r *AlertSourceReconciler) ensurePollerRunning(
    ctx context.Context,
    as v1alpha1.AlertSource,
    adapter AlertSourceAdapter,
) {
    key := namespacedName(&as)

    r.PollersMu.Lock()
    defer r.PollersMu.Unlock()

    // Cancel existing poller if spec changed (interval, URL, etc.)
    if cancel, ok := r.Pollers[key]; ok {
        cancel()
    }

    // Use the manager context (r.managerCtx), NOT the per-reconcile ctx parameter.
    // See the managerCtx field comment in the struct definition for the full rationale.
    // r.managerCtx is never nil: it is initialised to context.Background() at
    // construction and replaced with the real manager context in Start().
    pollCtx, cancel := context.WithCancel(r.managerCtx)
    r.Pollers[key] = cancel

    go r.runPoller(pollCtx, as, adapter)
}

func (r *AlertSourceReconciler) runPoller(
    ctx context.Context,
    as v1alpha1.AlertSource,
    adapter AlertSourceAdapter,
) {
    ticker := time.NewTicker(as.Spec.Poll.Interval.Duration)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            authToken := r.resolveAuthToken(ctx, as)
            findings, err := adapter.FetchAlerts(ctx, as.Spec, authToken)
            if err != nil {
                r.Log.Error("poll failed", zap.String("source", as.Name), zap.Error(err))
                r.incrementReceived(as.Name, 0) // count error in status
                continue
            }
            for _, f := range findings {
                f.SourceCRName = as.Name // set CR name for counter attribution
                select {
                case r.FindingCh <- f:
                    r.incrementReceived(as.Name, 1)
                default:
                    r.Log.Warn("finding channel full, dropping polled alert",
                        zap.String("alertName", f.AlertName))
                }
            }
        }
    }
}
```

---

## 4. Finding Drain Loop

The drain loop is a `manager.Runnable` (implements `Start(ctx context.Context) error`).
It runs continuously on the leader, consuming findings from the channel.

```go
// Start implements manager.Runnable
func (r *AlertSourceReconciler) Start(ctx context.Context) error {
    // Replace the context.Background() placeholder set at construction time.
    // From this point all new poller goroutines inherit the manager lifetime.
    // Pollers started before Start() was called (during the startup window) used
    // context.Background() as their parent; they are cancelled and recreated on
    // the next AlertSource CR reconcile, which is acceptable.
    r.managerCtx = ctx
    r.Log.Info("alert source finding drain loop started")
    // Start async status flush loop
    go r.flushStatusLoop(ctx)

    for {
        select {
        case <-ctx.Done():
            r.Log.Info("alert source finding drain loop stopped")
            return nil
        case f := <-r.FindingCh:
            if err := r.processFinding(ctx, f); err != nil {
                r.Log.Error("error processing finding", zap.Error(err),
                    zap.String("alertName", f.AlertName))
                // Do not requeue — alert sources are push/pull; the source will retry
            }
        }
    }
}

func (r *AlertSourceReconciler) NeedLeaderElection() bool {
    return true // only the leader creates RemediationJobs
}
```

### 4.1 processFinding

```go
func (r *AlertSourceReconciler) processFinding(ctx context.Context, f domain.Finding) error {
    // 1. Cascade check
    // NOTE: cascade.Checker.ShouldSuppress requires three arguments including client.Client.
    // Alert-sourced findings for Deployment/StatefulSet/Node kind are not checked by the
    // cascade checker (cascade.go:72 only processes Kind==Pod). This is intentional.
    if f.Kind == "Pod" && r.CascadeChecker != nil {
        suppress, reason, err := r.CascadeChecker.ShouldSuppress(ctx, &f, r.Client)
        if err != nil {
            r.Log.Error("cascade check error", zap.Error(err))
            // Non-fatal — continue processing
        } else if suppress {
            r.Log.Info("suppressing alert finding due to cascade",
                zap.String("reason", reason), zap.String("alertName", f.AlertName))
            return nil
        }
    }

    // 2. Resource fingerprint (v2 — no error texts)
    fp, err := domain.FindingFingerprint(&f)
    if err != nil {
        return fmt.Errorf("processFinding: fingerprint: %w", err)
    }

    // 3. Stabilisation window
    // External alert sources set SkipStabilisation=true; this block is a no-op for them.
    // For poll-mode sources with SkipStabilisation=false: the first poll occurrence is
    // recorded in FirstSeen. The finding is only created on the NEXT poll tick after the
    // stabilisation window has elapsed. If the alert clears between polls, it is silently
    // dropped — this is the expected behaviour for transient issues that self-resolved.
    //
    // IMPORTANT: The duration used is r.Cfg.StabilisationWindow (the global native
    // stabilisation window from STABILISATION_WINDOW env var), NOT the per-AlertSource
    // spec.StabilisationWindow field. The spec field only controls whether stabilisation
    // is skipped entirely (Duration == 0 → SkipStabilisation = true). If stabilisation
    // is not skipped, the global duration applies. This is a known simplification: per-source
    // duration overrides would require adding a StabilisationWindow field to domain.Finding.
    // A user wanting per-source duration must either (a) set SkipStabilisation=true and rely
    // on Alertmanager's own `for:` duration, or (b) set the global STABILISATION_WINDOW to
    // the desired value.
    if !f.SkipStabilisation && r.Cfg.StabilisationWindow > 0 {
        if first, seen := r.FirstSeen.Get(fp); !seen {
            r.FirstSeen.Set(fp)
            return nil // wait for stabilisation; re-processing on next poll
        } else if time.Since(first) < r.Cfg.StabilisationWindow {
            return nil
        }
    }

    // 4. Priority resolution + dedup
    action, existingRJ, err := r.resolveDedup(ctx, fp, f.SourcePriority)
    if err != nil {
        return err
    }

    switch action {
    case dedupActionSuppress:
        r.incrementSuppressed(f.SourceCRName)
        return nil

    case dedupActionAnnotatePending:
        return r.annotatePendingAlert(ctx, existingRJ, &f)

    case dedupActionCreateWithPreviousPR:
        f.PreviousPRURL = existingRJ.Status.PRRef
        fallthrough

    case dedupActionCreate:
        // 5. Readiness gate
        if err := r.ReadinessChecker.Check(ctx); err != nil {
            r.Log.Warn("readiness gate not passed, dropping finding",
                zap.String("alertName", f.AlertName), zap.Error(err))
            return nil // drop; alert source will retry
        }
        // 6. Create RemediationJob
        return r.createRemediationJob(ctx, &f, fp)
    }

    return nil
}
```

### 4.2 createRemediationJob

Alert-sourced RJs use the AlertSource CR name as the `SourceResultRef` sentinel value.
This is safe because AlertSource names are in the mechanic namespace and will never collide
with workload object names used by native providers. See HLD §14.2.

```go
func (r *AlertSourceReconciler) createRemediationJob(
    ctx context.Context,
    f *domain.Finding,
    fp string,
) error {
    // Compute v1 fingerprint for migration compatibility.
    // f.Errors is always valid JSON (constructed by the adapter as [{"text":"..."}]).
    // If FindingFingerprintV1 fails for any reason, fall back to using the v2 fp
    // for the legacy Spec.Fingerprint field — this is safe because the v1 dedup
    // check (in SourceProviderReconciler) uses the remediation.mechanic.io/fingerprint
    // label which is also set to fp[:12] as the fallback. The only consequence is
    // that the migration-window v1 fallback check becomes a no-op for this RJ.
    v1fp, err := domain.FindingFingerprintV1(f)
    if err != nil {
        r.Log.Warn("FindingFingerprintV1 failed; using v2 fp for legacy field",
            zap.String("alertName", f.AlertName), zap.Error(err))
        v1fp = fp // safe fallback; len(fp) == 64 so fp[:12] is always valid
    }

    rj := &v1alpha1.RemediationJob{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mechanic-" + fp[:12],
            Namespace: r.Cfg.AgentNamespace,
            Labels: map[string]string{
                v1alpha1.LabelResourceFingerprint:           fp[:12],
                v1alpha1.LabelSourcePriority:                strconv.Itoa(f.SourcePriority),
                "remediation.mechanic.io/fingerprint":       v1fp[:12], // migration compat
                "app.kubernetes.io/managed-by":              "mechanic-watcher",
            },
            Annotations: map[string]string{
                v1alpha1.AnnotationResourceFingerprintFull:  fp,
                v1alpha1.AnnotationSourceType:               f.SourceType,
                v1alpha1.AnnotationErrorSummary:             buildErrorSummary(f),
                "remediation.mechanic.io/fingerprint-full":  v1fp, // migration compat
            },
        },
        Spec: v1alpha1.RemediationJobSpec{
            SourceType:          f.SourceType,
            SourcePriority:      f.SourcePriority,
            SinkType:            r.Cfg.SinkType,
            Fingerprint:         v1fp,         // legacy field
            ResourceFingerprint: fp,           // v2 field
            SourceResultRef: v1alpha1.ResultRef{
                // Sentinel: AlertSource CR name/namespace (never matches a native object).
                // Use f.SourceCRName (the CR name) NOT f.SourceType (the adapter type name).
                // These are the same by convention, but using the CR name is correct.
                // SourceProviderReconciler cancellation logic will not accidentally fire.
                Name:      f.SourceCRName,         // AlertSource CR name, e.g. "alertmanager"
                Namespace: r.Cfg.AgentNamespace,
            },
            Finding: v1alpha1.FindingSpec{
                Kind:          f.Kind,
                Name:          f.Name,
                Namespace:     f.Namespace,
                ParentObject:  f.ParentObject,
                Errors:        f.Errors,
                AlertName:     f.AlertName,
                AlertLabels:   f.AlertLabels,
                PreviousPRURL: f.PreviousPRURL,
            },
            GitOpsRepo:         r.Cfg.GitOpsRepo,
            GitOpsManifestRoot: r.Cfg.GitOpsManifestRoot,
            AgentImage:         r.Cfg.AgentImage,
            AgentSA:            r.Cfg.AgentSA,
        },
    }

    if err := r.Create(ctx, rj); err != nil {
        if apierrors.IsAlreadyExists(err) {
            return nil // already exists — idempotent
        }
        return fmt.Errorf("createRemediationJob: %w", err)
    }
    r.incrementDispatched(f.SourceCRName)
    return nil
}

// buildErrorSummary constructs the value for the mechanic.io/error-summary annotation.
// It is a plain-text, single-line summary for quick human inspection via kubectl.
// Format: "<alertname>: <key>=<value> <key>=<value> ..."
// Key labels included (in order, if present): namespace, the resource label that resolved
// the kind (deployment/pod/node/etc.), severity, reason.
// Example: "KubeDeploymentReplicasMismatch: namespace=default deployment=test-broken-image severity=warning"
//
// For native findings (AlertName empty), returns a truncated version of f.Errors
// (first error text, max 120 chars).
func buildErrorSummary(f *domain.Finding) string {
    if f.AlertName == "" {
        // Native finding: extract first error text
        var errs []struct{ Text string `json:"text"` }
        if err := json.Unmarshal([]byte(f.Errors), &errs); err == nil && len(errs) > 0 {
            t := errs[0].Text
            if len(t) > 120 {
                t = t[:117] + "..."
            }
            return t
        }
        return f.Errors
    }
    // Alert-sourced finding: alertname + key labels
    parts := []string{f.AlertName + ":"}
    for _, key := range []string{"namespace", "deployment", "pod", "node", "statefulset", "service", "pvc", "severity", "reason"} {
        if v, ok := f.AlertLabels[key]; ok && v != "" {
            parts = append(parts, key+"="+v)
        }
    }
    return strings.Join(parts, " ")
}
```

---

## 5. Pending Alert Annotation

### 5.1 Writing the Annotation

```go
func (r *AlertSourceReconciler) annotatePendingAlert(
    ctx context.Context,
    rj *v1alpha1.RemediationJob,
    f *domain.Finding,
) error {
    encoded, err := json.Marshal(f)
    if err != nil {
        return fmt.Errorf("annotatePendingAlert: marshal: %w", err)
    }

    patch := client.MergeFrom(rj.DeepCopy())
    if rj.Annotations == nil {
        rj.Annotations = make(map[string]string)
    }
    rj.Annotations[v1alpha1.AnnotationPendingAlert] = string(encoded)

    return r.Patch(ctx, rj, patch)
}
```

Constants:
```go
// api/v1alpha1/annotations.go
const AnnotationPendingAlert = "mechanic.io/pending-alert"
```

### 5.2 Processing on Terminal State — `handlePendingAlert`

This is called by `Reconcile` when the RJ Watch predicate fires (terminal RJ with
pending-alert annotation). The annotation is cleared atomically before the new RJ is
created — this is the idempotency guarantee.

```go
func (r *AlertSourceReconciler) handlePendingAlert(
    ctx context.Context,
    rj *v1alpha1.RemediationJob,
) error {
    pendingJSON, hasPending := rj.Annotations[v1alpha1.AnnotationPendingAlert]
    if !hasPending {
        return nil // watch predicate should prevent this, but be defensive
    }
    isTerminal := rj.Status.Phase == v1alpha1.PhaseSucceeded ||
        rj.Status.Phase == v1alpha1.PhaseFailed ||
        rj.Status.Phase == v1alpha1.PhaseSuppressed ||
        rj.Status.Phase == v1alpha1.PhaseCancelled
    if !isTerminal {
        return nil // not yet terminal — predicate should prevent this too
    }

    var pendingFinding domain.Finding
    if err := json.Unmarshal([]byte(pendingJSON), &pendingFinding); err != nil {
        r.Log.Error("failed to unmarshal pending alert annotation — dropping",
            zap.String("rj", rj.Name), zap.Error(err))
        // Clear the corrupt annotation to prevent infinite reconcile loop
        return r.clearPendingAlertAnnotation(ctx, rj)
    }

    // STEP 1: Atomically clear the annotation BEFORE creating the new RJ.
    // If the process restarts after this patch but before creation, the pending
    // finding is lost. This is acceptable — Alertmanager will re-deliver on retry.
    if err := r.clearPendingAlertAnnotation(ctx, rj); err != nil {
        return fmt.Errorf("handlePendingAlert: clear annotation: %w", err)
    }

    // STEP 2: Propagate previous PR URL from the completed RJ.
    if rj.Status.Phase == v1alpha1.PhaseSucceeded && rj.Status.PRRef != "" {
        pendingFinding.PreviousPRURL = rj.Status.PRRef
    }

    // STEP 3: Compute resource fingerprint and run the create pipeline.
    fp, err := domain.FindingFingerprint(&pendingFinding)
    if err != nil {
        return fmt.Errorf("handlePendingAlert: fingerprint: %w", err)
    }

    if err := r.ReadinessChecker.Check(ctx); err != nil {
        r.Log.Warn("readiness gate not passed in handlePendingAlert; re-annotating for retry",
            zap.String("rj", rj.Name), zap.Error(err))
        // The annotation was already cleared above. Re-annotate the (now terminal)
        // RJ so the Watch predicate fires again on the next Update event and retries
        // this path when readiness is restored.
        //
        // Do NOT return nil here — that silently drops the pending finding permanently,
        // because the annotation has been removed and Alertmanager will not re-deliver
        // it (the original alert may have resolved by then). Re-annotating ensures
        // the pending finding survives a transient readiness failure.
        return r.annotatePendingAlert(ctx, rj, &pendingFinding)
    }

    return r.createRemediationJob(ctx, &pendingFinding, fp)
}

func (r *AlertSourceReconciler) clearPendingAlertAnnotation(
    ctx context.Context,
    rj *v1alpha1.RemediationJob,
) error {
    patch := client.MergeFrom(rj.DeepCopy())
    delete(rj.Annotations, v1alpha1.AnnotationPendingAlert)
    return r.Patch(ctx, rj, patch)
}
```

**Why `RemediationJobReconciler` is NOT involved:**
The `RemediationJobReconciler` remains provider-agnostic. It has no `FindingCh` parameter
and no knowledge of pending-alert annotations. The `AlertSourceReconciler` handles the full
pending-alert lifecycle via its own Watch on `RemediationJob` objects.

---

## 6. Priority Resolution — dedupAction

```go
// internal/provider/alertsource/dedup.go

type dedupAction int

const (
    dedupActionCreate              dedupAction = iota // no active RJ → create
    dedupActionSuppress                               // active higher-priority RJ → drop
    dedupActionAnnotatePending                        // active lower-priority RJ → annotate
    dedupActionCreateWithPreviousPR                   // prior Succeeded RJ → create with PR URL
)

func (r *AlertSourceReconciler) resolveDedup(
    ctx context.Context,
    fp string,
    incomingPriority int,
) (dedupAction, *v1alpha1.RemediationJob, error) {
    var rjList v1alpha1.RemediationJobList
    if err := r.List(ctx, &rjList,
        client.InNamespace(r.Cfg.AgentNamespace),
        client.MatchingLabels{v1alpha1.LabelResourceFingerprint: fp[:12]},
    ); err != nil {
        return dedupActionCreate, nil, err
    }

    for i := range rjList.Items {
        rj := &rjList.Items[i]
        fullFP := rj.Annotations[v1alpha1.AnnotationResourceFingerprintFull]
        if fullFP != fp {
            continue
        }

        switch rj.Status.Phase {
        case v1alpha1.PhaseFailed:
            // Delete the failed RJ so a fresh investigation can start.
            // Handle delete errors explicitly — do NOT ignore them.
            if err := r.Delete(ctx, rj); err != nil && !apierrors.IsNotFound(err) {
                return dedupActionCreate, nil, fmt.Errorf("resolveDedup: delete failed RJ: %w", err)
            }
            // Deletion succeeded (or already gone) — fall through to create
            return dedupActionCreate, nil, nil

        case v1alpha1.PhaseSucceeded:
            // Delete the succeeded RJ before signalling create. With v2 resource-level
            // fingerprinting, the new RJ would have the exact same name
            // ("mechanic-" + rfp[:12]). If the old Succeeded RJ is not deleted first,
            // r.Create returns AlreadyExists and the new RJ is silently never created.
            // This is the key difference from the v1 behaviour: v1 fingerprints included
            // error texts, so the new finding (with fresh errors) produced a different
            // fingerprint and a different name. v2 resource fingerprints are stable —
            // same resource always maps to the same name.
            if err := r.Delete(ctx, rj); err != nil && !apierrors.IsNotFound(err) {
                return dedupActionCreate, nil, fmt.Errorf("resolveDedup: delete succeeded RJ: %w", err)
            }
            return dedupActionCreateWithPreviousPR, rj, nil

        case v1alpha1.PhaseSuppressed:
            // The RJ was absorbed by the correlator — a primary RJ covers the
            // investigation group. The suppressed RJ itself will never produce output.
            // A new finding for the same resource should start a fresh investigation.
            // Delete the suppressed RJ (same name as any new RJ would use) and create.
            if err := r.Delete(ctx, rj); err != nil && !apierrors.IsNotFound(err) {
                return dedupActionCreate, nil, fmt.Errorf("resolveDedup: delete suppressed RJ: %w", err)
            }
            return dedupActionCreate, nil, nil

        case v1alpha1.PhaseCancelled:
            // The RJ was cancelled because its native source object was deleted while
            // in-flight. It is terminal. A new finding should start a fresh investigation.
            // Delete the cancelled RJ and create a new one.
            if err := r.Delete(ctx, rj); err != nil && !apierrors.IsNotFound(err) {
                return dedupActionCreate, nil, fmt.Errorf("resolveDedup: delete cancelled RJ: %w", err)
            }
            return dedupActionCreate, nil, nil

        default: // Pending, Dispatched, Running — genuinely active
            existingPriority, _ := strconv.Atoi(rj.Labels[v1alpha1.LabelSourcePriority])
            if incomingPriority <= existingPriority {
                return dedupActionSuppress, rj, nil
            }
            return dedupActionAnnotatePending, rj, nil
        }
    }

    // No v2 RJ found — check v1 RJs to prevent duplicate during migration window
    return r.resolveDedup_v1Fallback(ctx, fp, incomingPriority)
}

// resolveDedup_v1Fallback is called when no v2-labeled RJ is found for the
// incoming resource fingerprint. It checks whether an active v1-created RJ
// exists for the same resource, to prevent a duplicate investigation during
// the 7-day migration window after v2 is deployed.
//
// KNOWN LIMITATION — this fallback is a best-effort approximation, not a
// guarantee. A full v1-to-v2 fingerprint cross-match requires the original
// error texts from the v1 Finding, which are not available here. Computing
// the v1 fingerprint from a synthetic Finding with empty errors would only
// match v1 RJs that were themselves created from zero-error findings — an
// edge case that does not occur in practice.
//
// Practical consequence: during the migration window (up to 7 days), an
// alert-source finding CAN create a second concurrent RJ alongside an active
// v1 native-source RJ for the same resource. Two parallel investigations are
// harmless (the agent will open or update the same PR) but wasteful.
//
// Operators who want to eliminate migration-window duplicates entirely should
// drain all active RemediationJobs before deploying v2.
//
// The collision resolves naturally: once v1 RJs expire (7-day TTL) and all
// new RJs are created with v2 labels, the primary v2 dedup path handles
// everything correctly and this fallback becomes unreachable dead code.
func (r *AlertSourceReconciler) resolveDedup_v1Fallback(
    ctx context.Context,
    fp string,        // v2 resource fingerprint (for computing v1 fingerprint)
    incomingPriority int,
) (dedupAction, *v1alpha1.RemediationJob, error) {
    return dedupActionCreate, nil, nil
}
```

---

## 7. Status Counter Updates (Async)

The `AlertSource` status counters (`alertsReceived`, `alertsDispatched`, `alertsSuppressed`)
are updated asynchronously to avoid blocking the drain loop hot path on K8s API writes.

**Counter key:** Counters are keyed by the **AlertSource CR name** (not by `SourceType`).
This is important: multiple AlertSource CRs can share the same type (e.g. two CRs both with
`type: alertmanager` but different paths/polling URLs). Using `SourceType` as the key would
merge their counters and cause the wrong CR to be updated in `flushCountersForSource`.

The Finding must carry the AlertSource CR name to enable correct counter attribution.
Add `SourceCRName string` to `domain.Finding` alongside the other new v2 fields (see
`FINGERPRINT_LLD.md §0`). Adapters set this from the AlertSource CR name passed to them
at webhook/poll time. The drain loop uses `f.SourceCRName` as the counter key.

```go
// flushStatusLoop runs as a background goroutine alongside the drain loop.
// It periodically flushes in-memory counters to K8s AlertSource status.
func (r *AlertSourceReconciler) flushStatusLoop(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            r.flushAllCounters(ctx) // best-effort final flush on shutdown
            return
        case <-ticker.C:
            r.flushAllCounters(ctx)
        }
    }
}

func (r *AlertSourceReconciler) flushAllCounters(ctx context.Context) {
    r.statusMu.Lock()
    // Snapshot the current cumulative totals and zero them out atomically.
    // Zeroing here (under the lock, before releasing) means any increments that
    // arrive while flushing are counted in the NEXT flush cycle, not lost.
    // If the K8s patch below fails, we lose the delta for this cycle — acceptable
    // given the flush is best-effort and the comment in flushCountersForSource documents this.
    snapshot := make(map[string]*sourceCounters, len(r.statusCounters))
    for k, v := range r.statusCounters {
        snapshot[k] = &sourceCounters{
            received:   v.received,
            dispatched: v.dispatched,
            suppressed: v.suppressed,
        }
        // Zero out the in-memory counters. Without this, the next flush would re-add
        // the same cumulative total to the K8s status values, doubling every 30 seconds.
        v.received = 0
        v.dispatched = 0
        v.suppressed = 0
    }
    r.statusMu.Unlock()

    for crName, counts := range snapshot {
        flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
        r.flushCountersForSource(flushCtx, crName, counts)
        cancel()
    }
}

func (r *AlertSourceReconciler) flushCountersForSource(
    ctx context.Context,
    crName string,  // AlertSource CR name (not SourceType)
    counts *sourceCounters,
) {
    var as v1alpha1.AlertSource
    if err := r.Get(ctx, types.NamespacedName{
        Name:      crName,
        Namespace: r.Cfg.AgentNamespace,
    }, &as); err != nil {
        return // CR may have been deleted; skip silently
    }
    patch := client.MergeFrom(as.DeepCopy())
    as.Status.AlertsReceived += counts.received
    as.Status.AlertsDispatched += counts.dispatched
    as.Status.AlertsSuppressed += counts.suppressed
    if err := r.Status().Patch(ctx, &as, patch); err != nil {
        r.Log.Warn("failed to flush status counters",
            zap.String("alertSource", crName), zap.Error(err))
        // The in-memory counters were already zeroed in flushAllCounters before this
        // patch was attempted. A patch failure means the delta for this flush cycle is
        // lost. This is acceptable: status counters are best-effort observability data,
        // not load-bearing correctness data. The drain loop and RJ creation are unaffected.
    }
}

// incrementReceived, incrementDispatched, incrementSuppressed all use the
// AlertSource CR name as the key — NOT SourceType.
func (r *AlertSourceReconciler) incrementReceived(crName string, n int64) {
    r.statusMu.Lock()
    defer r.statusMu.Unlock()
    if _, ok := r.statusCounters[crName]; !ok {
        r.statusCounters[crName] = &sourceCounters{}
    }
    r.statusCounters[crName].received += n
}

func (r *AlertSourceReconciler) incrementDispatched(crName string) {
    r.statusMu.Lock()
    defer r.statusMu.Unlock()
    if _, ok := r.statusCounters[crName]; !ok {
        r.statusCounters[crName] = &sourceCounters{}
    }
    r.statusCounters[crName].dispatched++
}

func (r *AlertSourceReconciler) incrementSuppressed(crName string) {
    r.statusMu.Lock()
    defer r.statusMu.Unlock()
    if _, ok := r.statusCounters[crName]; !ok {
        r.statusCounters[crName] = &sourceCounters{}
    }
    r.statusCounters[crName].suppressed++
}
```

---

## 8. Concurrency and Safety

| Concern | Mitigation |
|---|---|
| Multiple webhook goroutines writing to `FindingCh` concurrently | Channel is goroutine-safe by design |
| Multiple poller goroutines writing to `FindingCh` concurrently | Same — channel is safe |
| `Pollers` map accessed from reconciler and pollers | `PollersMu` mutex guards all access |
| `webhookPaths` map accessed from reconciler and webhook handlers | `webhooksMu` mutex guards all access |
| `DynamicMux` route table accessed concurrently by reconciler and HTTP server | `DynamicMux` uses `sync.RWMutex` internally; safe for concurrent reads and writes |
| Drain loop and CR reconciler both calling `r.List` / `r.Create` | Both go through the controller-runtime client which is thread-safe |
| `FindingCh` full (back-pressure) | Webhook handler returns `503`; poller logs warning and drops; no panic |
| Pending-alert annotation written by drain loop, read by `handlePendingAlert` | Both use K8s API server as coordination point; MergePatch semantics prevent lost updates |
| `statusCounters` map accessed from drain loop and flush goroutine | `statusMu` mutex guards all access |
| Flush goroutine makes K8s API calls with 5s timeout | Slow API server cannot stall the drain loop |

---

## 9. Testing Strategy

### Unit tests

| Test | Description |
|---|---|
| `TestResolveDedup_NoActiveRJ` | Returns `dedupActionCreate` |
| `TestResolveDedup_ActiveHigherPriority` | Returns `dedupActionSuppress` |
| `TestResolveDedup_ActiveLowerPriority` | Returns `dedupActionAnnotatePending` |
| `TestResolveDedup_FailedRJ_DeleteSucceeds` | Deletes failed RJ, returns `dedupActionCreate` |
| `TestResolveDedup_FailedRJ_DeleteFails` | Delete error is returned (not swallowed) |
| `TestResolveDedup_SucceededRJ` | Returns `dedupActionCreateWithPreviousPR` with PR URL |
| `TestAnnotatePendingAlert_Writes` | Annotation JSON is written correctly |
| `TestAnnotatePendingAlert_Overwrites` | Second pending alert overwrites first |
| `TestHandlePendingAlert_NoPendingAnnotation` | No-op |
| `TestHandlePendingAlert_ClearsAnnotationBeforeCreate` | Annotation removed BEFORE new RJ is created (order matters) |
| `TestHandlePendingAlert_WithPendingAnnotation_Succeeded` | New RJ has PreviousPRURL set |
| `TestHandlePendingAlert_WithPendingAnnotation_Failed` | New RJ has PreviousPRURL empty |
| `TestHandlePendingAlert_CorruptAnnotation` | Annotation cleared, no panic, no new RJ |
| `TestHandlePendingAlert_Idempotent` | Second reconcile after annotation cleared is a no-op |
| `TestProcessFinding_CascadeSignature` | cascade.ShouldSuppress called with (ctx, finding, r.Client) — three args |

### Integration tests (envtest)

| Test | Description |
|---|---|
| `TestAlertSourceReconciler_FullWebhookFlow` | CR created → webhook POST → RJ created |
| `TestAlertSourceReconciler_PrioritySuppressionFlow` | Native RJ active → alert at lower priority → suppressed |
| `TestAlertSourceReconciler_PendingAlertFlow` | Native RJ at priority 10 → alert at priority 90 → annotation set → native RJ terminal → Watch fires → annotation cleared → new RJ created with PreviousPRURL |
| `TestAlertSourceReconciler_PendingAlertIdempotency` | Process restart after annotation clear: no duplicate RJ |
| `TestAlertSourceReconciler_PendingAlertSinglePR` | Only one RJ active for a given resource throughout the pending alert flow |
| `TestAlertSourceReconciler_PollerFlow` | CR with poll enabled → mock server polled → RJ created |
| `TestAlertSourceReconciler_CRDeleteStopsPoller` | CR deleted → poller goroutine stops |
| `TestAlertSourceReconciler_CRDeleteKeepsActiveRJs` | CR deleted → active RJs continue running |
| `TestAlertSourceReconciler_ChannelBackPressure` | Full channel → 503 from webhook handler |
| `TestAlertSourceReconciler_StatusCounterAsync` | Counters updated after flush interval, not per-event |
| `TestAlertSourceReconciler_SourceResultRefSentinel` | Alert-sourced RJs use AlertSource name as SourceResultRef; native cancellation logic is not triggered |
