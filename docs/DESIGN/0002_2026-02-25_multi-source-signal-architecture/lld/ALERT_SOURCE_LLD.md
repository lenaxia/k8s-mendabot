# AlertSource CRD — Low-Level Design

**Version:** 1.1
**Date:** 2026-02-25
**Status:** Proposed
**HLD Reference:** [§5, §8, §9](../HLD.md)

---

## 1. Overview

The `AlertSource` CRD represents a configured external alert integration. Each CR declares
one source: its type (Alertmanager, PagerDuty, OpsGenie, etc.), its delivery mode (webhook,
poll, or both), its priority in cross-source deduplication, and any credentials or label
mappings required.

The `AlertSourceReconciler` watches these CRDs and manages the lifecycle of webhook paths
and polling goroutines accordingly. See `ALERT_SOURCE_RECONCILER_LLD.md` for reconciler
internals.

---

## 2. CRD Schema

### 2.1 Group, Version, Kind

```
group:    mechanic.io
version:  v1alpha1
kind:     AlertSource
plural:   alertsources
scope:    Namespaced   (must be in the same namespace as the watcher)
```

**Important:** `AlertSource` lives under `mechanic.io`, which is a **different API group**
from `RemediationJob` (`remediation.mechanic.io`). Both groups must be registered separately
in the controller-runtime scheme at startup. Add to `main.go`:

```go
// Register mechanic.io/v1alpha1 (AlertSource) — separate from remediation.mechanic.io
if err := v1alpha1.AddAlertSourceToScheme(scheme); err != nil {
    logger.Fatal("failed to add alertsource scheme", zap.Error(err))
}
```

`AddAlertSourceToScheme` is defined alongside `AddRemediationToScheme`:

```go
// api/v1alpha1/alertsource_types.go
var alertSourceGroupVersion = schema.GroupVersion{
    Group:   "mechanic.io",
    Version: "v1alpha1",
}

func AddAlertSourceToScheme(s *runtime.Scheme) error {
    s.AddKnownTypes(alertSourceGroupVersion, &AlertSource{}, &AlertSourceList{})
    metav1.AddToGroupVersion(s, alertSourceGroupVersion)
    return nil
}
```

If this registration is omitted, the controller-runtime manager will panic at startup when
`AlertSourceReconciler.SetupWithManager` attempts to add an informer for `AlertSource`.

### 2.2 Go Types

```go
// api/v1alpha1/alertsource_types.go

// AlertSource configures one external alert signal source.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Priority",type=integer,JSONPath=`.spec.priority`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AlertSource struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   AlertSourceSpec   `json:"spec,omitempty"`
    Status AlertSourceStatus `json:"status,omitempty"`
}

type AlertSourceSpec struct {
    // Type identifies which adapter parses this source's payloads.
    // Must match a registered adapter name. Built-in for v2: "alertmanager".
    // Future releases will add "pagerduty" and "opsgenie" once those adapters are
    // validated against real customer payloads. See HLD §4.2 for rationale.
    // +kubebuilder:validation:Enum=alertmanager
    // +kubebuilder:validation:Required
    Type string `json:"type"`

    // Priority is used for cross-source deduplication. Higher value wins.
    // Must be >= 0. Default: 50.
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:default=50
    Priority int `json:"priority"`

    // StabilisationWindow controls whether the stabilisation window is applied.
    // Set to 0 (default) to skip stabilisation entirely — recommended for push sources
    // that have already applied their own `for:` duration before firing.
    // Set to any non-zero value to apply stabilisation (the actual duration used is the
    // global STABILISATION_WINDOW env var, not this field's value — see
    // ALERT_SOURCE_RECONCILER_LLD §4.1 for the known limitation).
    // +kubebuilder:default="0s"
    StabilisationWindow metav1.Duration `json:"stabilisationWindow,omitempty"`

    // Webhook configures push-based alert delivery.
    // +optional
    Webhook *AlertSourceWebhookSpec `json:"webhook,omitempty"`

    // Poll configures pull-based alert retrieval.
    // +optional
    Poll *AlertSourcePollSpec `json:"poll,omitempty"`

    // LabelMapping maps alert label names to Kubernetes resource identity fields.
    // Keys are fixed (see AlertSourceLabelMapping); values are the label names used
    // by this source. Default values match standard kube-state-metrics label names.
    // +optional
    LabelMapping AlertSourceLabelMapping `json:"labelMapping,omitempty"`
}

type AlertSourceWebhookSpec struct {
    // Enabled controls whether the webhook path is registered.
    // +kubebuilder:default=true
    Enabled bool `json:"enabled"`

    // Path is the HTTP path to register on the webhook server.
    // Must be unique across all AlertSource CRs.
    // Example: "/webhook/v1/alertmanager"
    // +kubebuilder:validation:Pattern=`^/.*`
    // +kubebuilder:validation:Required
    Path string `json:"path"`

    // HMACSecretRef references a Kubernetes Secret containing the webhook signing secret.
    // If set, all requests must include a valid HMAC signature header.
    // +optional
    HMACSecretRef *SecretKeyRef `json:"hmacSecretRef,omitempty"`
}

type AlertSourcePollSpec struct {
    // Enabled controls whether the polling loop runs.
    // +kubebuilder:default=false
    Enabled bool `json:"enabled"`

    // URL is the base URL of the alert source API.
    // Example: "http://alertmanager.monitoring.svc:9093"
    // +kubebuilder:validation:Required
    URL string `json:"url"`

    // Interval is how often to poll for new alerts.
    // +kubebuilder:default="60s"
    Interval metav1.Duration `json:"interval"`

    // AuthSecretRef references a Secret containing a bearer token for authentication.
    // +optional
    AuthSecretRef *SecretKeyRef `json:"authSecretRef,omitempty"`
}

type SecretKeyRef struct {
    // Name of the Secret.
    Name string `json:"name"`
    // Key within the Secret.
    Key string `json:"key"`
}

// AlertSourceLabelMapping maps semantic resource identity fields to the alert label
// names used by this specific source. All fields are optional; the values shown are
// the defaults (matching kube-state-metrics / kube-prometheus-stack conventions).
type AlertSourceLabelMapping struct {
    // +kubebuilder:default="namespace"
    Namespace string `json:"namespace,omitempty"`
    // +kubebuilder:default="deployment"
    Deployment string `json:"deployment,omitempty"`
    // +kubebuilder:default="pod"
    Pod string `json:"pod,omitempty"`
    // +kubebuilder:default="kubernetes_node"
    Node string `json:"node,omitempty"`
    // +kubebuilder:default="statefulset"
    StatefulSet string `json:"statefulset,omitempty"`
    // +kubebuilder:default="service"
    Service string `json:"service,omitempty"`
    // +kubebuilder:default="persistentvolumeclaim"
    PVC string `json:"pvc,omitempty"`
    // +kubebuilder:default="job"
    Job string `json:"job,omitempty"`
    // +kubebuilder:default="alertname"
    AlertName string `json:"alertname,omitempty"`
}

type AlertSourceStatus struct {
    // Conditions reflect the current state of the AlertSource.
    // The "Ready" condition indicates the source is configured and active.
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // LastPollTime is the timestamp of the most recent successful poll (poll mode only).
    // +optional
    LastPollTime *metav1.Time `json:"lastPollTime,omitempty"`

    // AlertsReceived is a running counter of alerts received since the CR was created.
    AlertsReceived int64 `json:"alertsReceived"`

    // AlertsDispatched is a running counter of RemediationJobs created by this source.
    AlertsDispatched int64 `json:"alertsDispatched"`

    // AlertsSuppressed is a running counter of alerts suppressed by priority dedup.
    AlertsSuppressed int64 `json:"alertsSuppressed"`
}
```

### 2.3 Validation Rules

| Rule | Enforcement |
|---|---|
| At least one of `webhook.enabled` or `poll.enabled` must be true | kubebuilder CEL validation on spec |
| `webhook.path` must be unique across all `AlertSource` CRs in the namespace | Validated by `AlertSourceReconciler` on reconcile; sets `Ready=False` with reason `PathConflict` |
| `poll.url` must be a valid HTTP/HTTPS URL | kubebuilder `format: uri` annotation |
| `type` must match a registered adapter | kubebuilder enum annotation; registration gap caught at startup |

---

## 3. AlertSourceAdapter Interface

Each source type is implemented as an adapter. The adapter is responsible for:
1. Parsing raw webhook payloads into `[]domain.Finding`
2. Fetching alerts from a poll API and returning `[]domain.Finding`
3. Identifying the K8s resource affected by each alert

**Important:** Resource identity resolution may require walking Pod owner references, which
needs a K8s API call. The `client.Client` is injected at adapter construction time (not
passed per-call) to keep the per-call methods simple.

```go
// internal/provider/alertsource/adapter.go

// AlertSourceAdapter translates raw external alert data into domain Findings.
// Each AlertSource type (alertmanager, etc.) has one implementation.
// Implementations must be safe to call concurrently.
type AlertSourceAdapter interface {
    // TypeName returns the stable lowercase type identifier.
    // Must match AlertSourceSpec.Type values registered in main.go.
    TypeName() string

    // ParseWebhook parses a raw HTTP request body into one or more Findings.
    // Called by the webhook handler on each incoming POST.
    // Must be safe to call concurrently.
    ParseWebhook(ctx context.Context, body []byte, spec v1alpha1.AlertSourceSpec) ([]domain.Finding, error)

    // FetchAlerts queries the remote API and returns current active alerts as Findings.
    // Called by the polling loop on each interval tick.
    // Returns (nil, nil) if there are no active alerts.
    FetchAlerts(ctx context.Context, spec v1alpha1.AlertSourceSpec, authToken string) ([]domain.Finding, error)
}

// NewAlertmanagerAdapter constructs an AlertmanagerAdapter with a K8s client.
// The client is used for ownerRef resolution when a Pod label is present in alert labels.
func NewAlertmanagerAdapter(client client.Client) *AlertmanagerAdapter {
    return &AlertmanagerAdapter{client: client}
}
```

Adapters that only support one mode may return `domain.ErrNotSupported` from the unused
method. The reconciler will not call `FetchAlerts` on a webhook-only adapter and will not
call `ParseWebhook` on a poll-only adapter, but implementors should return a clear error
rather than panic if called incorrectly.

---

## 4. Resource Identity Resolution

Each adapter is responsible for extracting a K8s resource identity from alert labels. The
`AlertSourceLabelMapping` in the spec provides the label name overrides; the adapter applies
them.

### 4.1 Resolution Priority Order

When resolving resource identity from alert labels, adapters apply this priority order
(first match wins):

1. `pod` label → `Kind=Pod`, walk ownerRefs to resolve `ParentObject`
2. `deployment` label → `Kind=Deployment`, `ParentObject=Deployment/<value>`
3. `statefulset` label → `Kind=StatefulSet`, `ParentObject=StatefulSet/<value>`
4. `job` label → `Kind=Job`, `ParentObject=Job/<value>`
5. `node` label → `Kind=Node`, `ParentObject=Node/<value>`, `Namespace=""`
6. `pvc` label → `Kind=PersistentVolumeClaim`, `ParentObject=PersistentVolumeClaim/<value>`
7. `service` label → `Kind=Service`, `ParentObject=Service/<value>`
8. **Fallback**: `Kind=Alert`, `ParentObject=Alert/<alertname>`, `Namespace=<namespace label or "default">`

The fallback ensures every alert produces a valid Finding even when no specific resource
can be identified. The agent receives the full `FINDING_ALERT_LABELS` and can investigate
from there.

### 4.2 Namespace Resolution

Namespace is read from the label mapped to `AlertSourceLabelMapping.Namespace` (default:
`"namespace"`). If absent, `"default"` is used. Node-level alerts have no namespace; for
those, namespace is set to `""` in the Finding.

### 4.3 Helper: `resolveResource`

A shared helper function in `internal/provider/alertsource/resource.go`.
When a `pod` label is present, the helper performs a K8s API call to retrieve the Pod and
walk its `ownerReferences` to determine the `ParentObject` (e.g. the owning Deployment).
The `client.Client` for this call comes from the adapter's injected client (§3).

```go
// resolveResource extracts a K8s resource identity from a flat label map.
// mapping is the user-configured label name overrides.
// k8sClient is required for Pod owner reference resolution; pass nil to skip ownerRef walk.
// Returns kind, namespace, parentObject strings.
func resolveResource(
    ctx context.Context,
    labels map[string]string,
    mapping v1alpha1.AlertSourceLabelMapping,
    k8sClient client.Client,
) (kind, namespace, parentObject string)
```

---

## 5. Built-in Adapter Implementations

### 5.1 Alertmanager Adapter

**File:** `internal/provider/alertsource/adapters/alertmanager.go`

Parses the standard [Alertmanager v2 webhook payload](https://prometheus.io/docs/alerting/latest/configuration/#webhook_config):

```go
type AlertmanagerPayload struct {
    Alerts []AlertmanagerAlert `json:"alerts"`
}

type AlertmanagerAlert struct {
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
    StartsAt    time.Time         `json:"startsAt"`
    EndsAt      time.Time         `json:"endsAt"`
    Status      string            `json:"status"` // "firing" | "resolved"
}
```

**`ParseWebhook`:**
- Unmarshal payload
- Filter to `status == "firing"` alerts only (resolved alerts are not remediable)
- For each firing alert, call `resolveResource(ctx, labels, mapping, a.client)`, build `domain.Finding`

**`FetchAlerts`:**
- `GET <url>/api/v2/alerts?active=true&silenced=false&inhibited=false`
- Unmarshal response (same `AlertmanagerAlert` struct)
- Filter to firing alerts only (same as `ParseWebhook`)
- Return all active alerts as Findings; do NOT attempt channel inspection for dedup

**Note on webhook + poll overlap:** When both modes are enabled for the same AlertSource,
the same alert may arrive via webhook push and then appear again on the next poll tick.
This does NOT need to be deduplicated in `FetchAlerts` — Go channels are not inspectable
and the contents cannot be read without consuming. Cross-mode deduplication is handled
naturally by `resolveDedup` in `processFinding`: if an active RJ already exists for the
resource (created from the webhook-sourced finding), the poll-sourced finding will be
suppressed at the priority resolution step. No extra logic is required in the adapter.

**HMAC validation (`webhook` mode):**
- Alertmanager does not have a built-in signing mechanism; validation is optional
- If `hmacSecretRef` is configured, the handler computes `HMAC-SHA256(secret, body)` and
  compares to `X-Alertmanager-Key` header

### 5.2 Future Adapters (out of scope for v2)

PagerDuty and OpsGenie adapters are **not implemented in v2**. See HLD §4.2 for rationale.

To add a new adapter in a future release:
1. Implement `AlertSourceAdapter` with injected `client.Client`
2. Add one entry to the registry in `main.go`
3. Add the new value to the `AlertSourceSpec.Type` kubebuilder enum annotation

No CRD schema changes are required.

---

## 6. Finding Construction from Alerts

All adapters produce `domain.Finding` objects with these fields populated:

```go
domain.Finding{
    Kind:           "<resolved kind>",              // from resolveResource
    Name:           "<resource name>",               // from label value; may be empty
    Namespace:      "<namespace>",                   // from namespace label
    ParentObject:   "<Kind>/<name>",                 // canonical parentObject form
    Errors:         `[{"text":"<alertname>: <key labels summary>"}]`,
    AlertName:      "<alertname label value>",
    AlertLabels:    map[string]string{ ... },         // full raw label set
    SourceType:     "<adapter TypeName()>",
    SourceCRName:   "<AlertSource CR name>",          // set by webhook handler / poller; used for counter attribution
    SourcePriority: spec.Priority,                   // from AlertSource CR spec
    SkipStabilisation: spec.StabilisationWindow.Duration == 0,
}
```

`Errors` is constructed as a human-readable summary rather than raw JSON for readability
in `kubectl get remediationjobs`. The full label set is in `AlertLabels`.

---

## 7. Adapter Registry

Adapters are registered at startup in `main.go`. The registry maps type names to adapter
instances. Each adapter is constructed with the K8s client injected:

```go
// internal/provider/alertsource/registry.go
type AdapterRegistry map[string]AlertSourceAdapter

// Default registry populated in main.go:
registry := alertsource.AdapterRegistry{
    "alertmanager": adapters.NewAlertmanagerAdapter(mgr.GetClient()),
    // pagerduty and opsgenie: future releases
}
```

If an `AlertSource` CR references a type not in the registry, the reconciler sets
`Ready=False` with reason `UnknownType` and logs an error.

Adding a new adapter type requires:
1. Implementing `AlertSourceAdapter` (with `client.Client` injected via constructor)
2. Adding one entry to the registry in `main.go`
3. Adding the new value to the `AlertSourceSpec.Type` kubebuilder enum

---

## 8. Webhook Server and DynamicMux

The webhook HTTP server is a `net/http.Server` started as a `manager.Runnable`. Route
management uses a custom `DynamicMux` rather than `http.ServeMux` directly.

**Why not `http.ServeMux`:** `http.ServeMux.Handle` panics if the same path pattern is
registered twice (Go 1.21+). It also provides no way to unregister a path. For the dynamic
registration/deregistration pattern required here (CRs created and deleted at runtime),
`http.ServeMux` is not suitable.

**`DynamicMux` — thread-safe handler map:**

```go
// internal/provider/alertsource/dynamic_mux.go

// DynamicMux is a thread-safe HTTP multiplexer that supports runtime registration
// and deregistration of path handlers without panicking on re-registration.
type DynamicMux struct {
    mu       sync.RWMutex
    handlers map[string]http.Handler // keyed by path
}

func NewDynamicMux() *DynamicMux {
    return &DynamicMux{handlers: make(map[string]http.Handler)}
}

// Register adds or replaces a handler for the given path.
// Safe to call concurrently with active HTTP requests.
func (m *DynamicMux) Register(path string, h http.Handler) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.handlers[path] = h
}

// Deregister removes a handler for the given path.
// Subsequent requests to that path receive 404.
func (m *DynamicMux) Deregister(path string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    delete(m.handlers, path)
}

// ServeHTTP implements http.Handler. Looks up the handler under a read lock;
// returns 404 if no handler is registered for the path.
func (m *DynamicMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    m.mu.RLock()
    h, ok := m.handlers[r.URL.Path]
    m.mu.RUnlock()
    if !ok {
        http.NotFound(w, r)
        return
    }
    h.ServeHTTP(w, r)
}

// HasPath returns whether a path is currently registered. Used for conflict detection.
func (m *DynamicMux) HasPath(path string) bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    _, ok := m.handlers[path]
    return ok
}
```

**WebhookServer:**

```go
// internal/provider/alertsource/webhook_server.go

type WebhookServer struct {
    Port int
    Mux  *DynamicMux
    Log  *zap.Logger
}

func (s *WebhookServer) Start(ctx context.Context) error {
    srv := &http.Server{
        Addr:              fmt.Sprintf(":%d", s.Port),
        Handler:           s.Mux,
        ReadHeaderTimeout: 10 * time.Second,
    }
    go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
    if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        return err
    }
    return nil
}

func (s *WebhookServer) NeedLeaderElection() bool {
    return false // starts on all replicas; see HLD §9 for multi-replica notes
}
```

Registered with the controller-runtime manager:

```go
// cmd/watcher/main.go
dynamicMux := alertsource.NewDynamicMux()
webhookServer := &alertsource.WebhookServer{Port: cfg.WebhookPort, Mux: dynamicMux, Log: logger}
if err := mgr.Add(webhookServer); err != nil {
    log.Fatal("failed to add webhook server", zap.Error(err))
}
```

Routes are dynamically added/removed by the `AlertSourceReconciler` as CRs are created
or deleted. The `AlertSourceReconciler` holds a reference to the shared `DynamicMux`:

```go
// Registration:
dynamicMux.Register(spec.Webhook.Path, newWebhookHandler(adapter, spec, findingCh, log))

// Deregistration (on CR delete or webhook.enabled=false):
dynamicMux.Deregister(spec.Webhook.Path)
```

---

## 9. Package Structure

```
internal/
└── provider/
    └── alertsource/
        ├── reconciler.go          # AlertSourceReconciler (CR lifecycle, channel drain, pending-alert handling)
        ├── reconciler_test.go
        ├── webhook_server.go      # WebhookServer (net/http, manager.Runnable)
        ├── dynamic_mux.go         # DynamicMux (thread-safe handler registry, replaces http.ServeMux)
        ├── dynamic_mux_test.go
        ├── webhook_handler.go     # Per-path HTTP handler (HMAC, parse, post to channel)
        ├── poller.go              # Polling goroutine management
        ├── resource.go            # resolveResource helper (takes client.Client for ownerRef walk)
        ├── resource_test.go
        ├── registry.go            # AdapterRegistry type
        └── adapters/
            ├── alertmanager.go    # AlertmanagerAdapter (implements AlertSourceAdapter)
            └── alertmanager_test.go
```

---

## 10. Testing Strategy

### Unit tests — adapters

| Test | File | Description |
|---|---|---|
| `TestAlertmanagerAdapter_ParseWebhook_FiringOnly` | alertmanager_test.go | Resolved alerts are filtered out |
| `TestAlertmanagerAdapter_ParseWebhook_ResolvesPodToDeployment` | alertmanager_test.go | Pod label → walks ownerRef to Deployment (uses fake client) |
| `TestAlertmanagerAdapter_ParseWebhook_FallbackKind` | alertmanager_test.go | No resource labels → `Kind=Alert` fallback |
| `TestAlertmanagerAdapter_ParseWebhook_InvalidJSON` | alertmanager_test.go | Returns error |
| `TestAlertmanagerAdapter_FetchAlerts_FiltersResolved` | alertmanager_test.go | Uses `httptest.Server` mock |
| `TestResolveResource_DeploymentLabel` | resource_test.go | `deployment=foo` → `Kind=Deployment, ParentObject=Deployment/foo` |
| `TestResolveResource_NodeLabel` | resource_test.go | `node=foo` → empty namespace |
| `TestResolveResource_Fallback` | resource_test.go | No resource labels → `Kind=Alert` |
| `TestResolveResource_CustomLabelMapping` | resource_test.go | Custom label name override respected |
| `TestResolveResource_NilClient` | resource_test.go | nil client → skips ownerRef walk, uses pod name as parentObject |

### Unit tests — DynamicMux

| Test | File | Description |
|---|---|---|
| `TestDynamicMux_Register_ServesRequest` | dynamic_mux_test.go | Registered path receives requests |
| `TestDynamicMux_Deregister_Returns404` | dynamic_mux_test.go | Deregistered path returns 404 |
| `TestDynamicMux_Register_ReplacesExisting` | dynamic_mux_test.go | Re-registering same path replaces handler; no panic |
| `TestDynamicMux_ConcurrentAccess` | dynamic_mux_test.go | Concurrent Register + ServeHTTP does not race (run with -race) |

### Integration tests — webhook handler

| Test | File | Description |
|---|---|---|
| `TestWebhookHandler_ValidHMAC` | webhook_handler_test.go | Valid signature accepted |
| `TestWebhookHandler_InvalidHMAC` | webhook_handler_test.go | Invalid signature returns 400 |
| `TestWebhookHandler_PostsToChannel` | webhook_handler_test.go | Valid payload posts Finding to channel |
| `TestWebhookHandler_ChannelFull` | webhook_handler_test.go | Channel full returns 503 |

### Integration tests — AlertSourceReconciler (envtest)

| Test | Description |
|---|---|
| `TestAlertSourceReconciler_RegistersWebhookOnCreate` | Creating a CR registers the webhook path in DynamicMux |
| `TestAlertSourceReconciler_DeregistersWebhookOnDelete` | Deleting a CR deregisters the path |
| `TestAlertSourceReconciler_PathConflictSetsNotReady` | Duplicate path sets `Ready=False` |
| `TestAlertSourceReconciler_UnknownTypeSetsNotReady` | Unknown adapter type sets `Ready=False` |
| `TestAlertSourceReconciler_UpdatesStatusCounters` | Received/dispatched/suppressed counters increment |
