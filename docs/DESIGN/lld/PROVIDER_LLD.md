# Domain: Provider Interfaces — Low-Level Design

**Version:** 1.1
**Date:** 2026-02-20
**Status:** Authoritative Specification
**HLD Reference:** [Section 5](../HLD.md)

---

## 1. Overview

### 1.1 Motivation

The initial design hard-codes two specific integrations:

- **Input:** k8sgpt `Result` CRDs
- **Output:** GitHub pull requests via the `gh` CLI

Both are the right choice for v1, but tying them to the core at the structural level
makes future extension unnecessarily expensive. Introducing `SourceProvider` and
`SinkProvider` interfaces isolates what varies (the integrations) from what stays fixed
(the `RemediationJob` CRD, the dispatch reconciler, the fingerprint algorithm).

### 1.2 The Fixed Core

The following are **not** provider concerns — they are part of the fixed core and do not
vary across integrations:

| Component | Role |
|---|---|
| `RemediationJob` CRD | Normalised internal representation of any finding from any source |
| `RemediationJobReconciler` | Dispatch, concurrency enforcement, Job creation, status sync |
| `batch/v1 Job` + agent image | Execution environment — unchanged regardless of source or sink |
| Fingerprint algorithm | Deduplication key — computed by the source provider from its native type |

### 1.3 What Varies

| Concern | Interface | v1 Implementation | Future examples |
|---|---|---|---|
| How findings enter the system | `SourceProvider` | `K8sGPTProvider` (watches k8sgpt `Result` CRDs) | `PrometheusProvider`, `DatadogProvider`, `KubeEventProvider` |
| How fixes are delivered | `SinkProvider` | `GitHubSinkProvider` (prompt + `gh` CLI in agent) | `GitLabSinkProvider`, `GiteaSinkProvider`, `JiraSinkProvider` |

---

## 2. Package Structure

```
internal/
├── domain/
│   ├── interfaces.go       # JobBuilder interface (existing)
│   └── provider.go         # SourceProvider, SinkProvider, Finding interfaces
│
└── provider/
    ├── provider.go         # SourceProviderReconciler: wraps any SourceProvider as a ctrl.Reconciler
    ├── k8sgpt/
    │   ├── provider.go     # K8sGPTProvider — implements SourceProvider for Result CRDs
    │   ├── provider_test.go
    │   ├── reconciler.go      # fingerprintFor() package-level function (no struct)
    │   └── reconciler_test.go # fingerprintFor unit tests + envtest integration tests
    └── github/
        ├── config.go       # GitHubSinkConfig (env vars, prompt template path)
        └── README.md       # Documents the GitHub sink: prompt conventions, gh CLI usage
```

The `github/` package is intentionally minimal in Go — the sink is implemented primarily
in the agent entrypoint and prompt, not in Go controller code. The package exists to hold
configuration types and documentation, and to make the boundary explicit.

---

## 3. SourceProvider Interface

```go
// SourceProvider is implemented by any component that watches an external resource
// and can produce a normalised Finding from it.
//
// The SourceProvider does NOT create RemediationJob objects directly — that is the
// responsibility of SourceProviderReconciler, which calls ExtractFinding() and owns
// the creation logic.
type SourceProvider interface {
    // ProviderName returns a stable, lowercase identifier for this provider.
    // Used as the value of RemediationJobSpec.SourceType (e.g. "k8sgpt", "prometheus").
    // Must be unique across all registered providers.
    ProviderName() string

    // ObjectType returns a pointer to the runtime.Object type this provider watches.
    // Used by SourceProviderReconciler to register the correct informer.
    // Example: return &k8sgptv1alpha1.Result{}
    ObjectType() client.Object

    // ExtractFinding converts a watched object into a Finding.
    // Returns (nil, nil) if the object should be skipped (e.g. no errors present).
    // Returns (nil, err) for transient errors that should trigger a requeue.
    ExtractFinding(obj client.Object) (*Finding, error)

    // Fingerprint computes the deduplication key for the given Finding.
    // The algorithm may vary per provider (e.g. k8sgpt uses namespace+kind+parent+errors;
    // a Prometheus provider might use alertname+labels).
    // Must be deterministic: same logical finding always produces the same fingerprint.
    Fingerprint(f *Finding) string
}
```

### 3.1 Finding — the normalised input type

```go
// Finding is the provider-agnostic representation of a cluster problem.
// All source providers must map their native type to this struct.
// The RemediationJob spec is populated directly from this struct.
type Finding struct {
    // Kind is the Kubernetes resource kind affected (e.g. "Pod", "Deployment").
    Kind string

    // Name is the plain resource name (no namespace prefix).
    Name string

    // Namespace is the namespace of the affected resource.
    Namespace string

    // ParentObject is the logical owner (e.g. "my-deployment" for a crashing pod).
    // Used as the deduplication anchor. If there is no meaningful parent, use Name.
    ParentObject string

    // Errors is a pre-serialised, redacted JSON string of error descriptions.
    // Format: [{"text":"..."},{"text":"..."}]
    // Sensitive fields must be stripped by the provider before populating this field.
    Errors string

    // Details is a human-readable explanation of the finding (e.g. k8sgpt LLM analysis).
    // May be empty.
    Details string

    // SourceRef identifies the native object that produced this Finding.
    SourceRef SourceRef
}

// SourceRef is a back-reference to the native object that produced a Finding.
type SourceRef struct {
    // APIVersion of the native object (e.g. "core.k8sgpt.ai/v1alpha1").
    APIVersion string

    // Kind of the native object (e.g. "Result").
    Kind string

    // Name of the native object.
    Name string

    // Namespace of the native object.
    Namespace string
}
```

---

## 4. SinkProvider Interface

The sink is where a fix is delivered. In v1, this is a GitHub PR opened by the
`mechanic-agent` running inside a `batch/v1 Job`. Future sinks could be GitLab MRs,
Gitea PRs, or Jira tickets.

The sink is **not a Go controller interface** — it is an abstraction at the agent level:

```
agent entrypoint → reads SINK_TYPE env var → executes appropriate sink logic
```

For v1, `SINK_TYPE=github` is the only value. The agent entrypoint selects the sink
implementation based on this variable. Future sink implementations would add a new
entrypoint branch and a new prompt variant.

```go
// SinkConfig holds the configuration passed to the agent for a specific sink.
// Populated from watcher env vars and mounted Secrets; injected as Job env vars.
type SinkConfig struct {
    // Type identifies the sink implementation (e.g. "github", "gitlab", "gitea").
    // Injected as SINK_TYPE env var into the agent Job.
    Type string

    // AdditionalEnv holds sink-specific env vars (e.g. GITLAB_HOST for a GitLab sink).
    // These are injected alongside the standard FINDING_* vars.
    AdditionalEnv map[string]string
}
```

This design keeps the sink extension point entirely in configuration and agent code,
with no Go interface to implement in the controller. Adding a new sink requires:
1. A new agent entrypoint branch for `SINK_TYPE=<newsink>`
2. A new prompt template variant
3. A new `SinkConfig.Type` value registered in the watcher config

This is the right boundary: Go interfaces for things that affect controller behaviour;
configuration + agent code for things that only affect what the agent does at runtime.

---

## 5. SourceProviderReconciler

`SourceProviderReconciler` is a generic controller-runtime reconciler that wraps any
`SourceProvider`. It eliminates the need to write boilerplate reconcile logic for each
new source.

```go
// SourceProviderReconciler is a controller-runtime Reconciler that wraps a SourceProvider.
// It handles fetch, skip-if-not-found, ExtractFinding, Fingerprint, dedup-by-CRD, and
// RemediationJob creation. Source-specific logic is entirely in the SourceProvider.
type SourceProviderReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Log      *zap.Logger
    Cfg      config.Config
    Provider SourceProvider
}

func (r *SourceProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the watched object using the provider's ObjectType().
    // 2. If NotFound: find and cancel pending RemediationJobs for this source ref. Return nil.
    // 3. Call Provider.ExtractFinding(obj).
    //    If nil, nil: skip. Return nil.
    //    If nil, err: return err (requeue).
    // 4. fp = Provider.Fingerprint(finding)
    // 5. List RemediationJobs with label remediation.mechanic.io/fingerprint=fp[:12]
    //    Skip if non-Failed match with matching full fingerprint exists.
    // 6. Build and create RemediationJob from finding + fp + provider name.
    //    If AlreadyExists: return nil.
    //    If other error: return error.
    // 7. Return nil.
}

func (r *SourceProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(r.Provider.ObjectType()).
        Complete(r)
}
```

`main.go` registers one `SourceProviderReconciler` per enabled provider:

```go
for _, p := range enabledProviders {
    if err := (&provider.SourceProviderReconciler{
        Client:   mgr.GetClient(),
        Scheme:   mgr.GetScheme(),
        Log:      logger,
        Cfg:      cfg,
        Provider: p,
    }).SetupWithManager(mgr); err != nil {
        log.Fatal("provider setup failed", zap.Error(err))
    }
}
```

Adding a new source provider in the future requires only:
1. Implementing `SourceProvider` for the new type
2. Adding it to `enabledProviders` in `main.go`
3. Vendoring the new type's Go API (or using `unstructured.Unstructured`)

---

## 6. K8sGPTProvider

The v1 implementation of `SourceProvider`.

```go
// K8sGPTProvider watches k8sgpt Result CRDs and extracts Findings from them.
type K8sGPTProvider struct{}

func (p *K8sGPTProvider) ProviderName() string { return v1alpha1.SourceTypeK8sGPT }

func (p *K8sGPTProvider) ObjectType() client.Object { return &v1alpha1.Result{} }

func (p *K8sGPTProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
    result, ok := obj.(*v1alpha1.Result)
    if !ok {
        return nil, fmt.Errorf("K8sGPTProvider: expected *Result, got %T", obj)
    }
    if len(result.Spec.Error) == 0 {
        return nil, nil // skip
    }

    // Redact Sensitive fields before serialisation.
    redacted := make([]v1alpha1.Failure, len(result.Spec.Error))
    for i, f := range result.Spec.Error {
        redacted[i] = v1alpha1.Failure{Text: f.Text}
    }
    errorsJSON, err := json.Marshal(redacted)
    if err != nil {
        return nil, fmt.Errorf("K8sGPTProvider: serialising errors: %w", err)
    }

    return &domain.Finding{
        Kind:         result.Spec.Kind,
        Name:         result.Spec.Name,
        Namespace:    result.Namespace,
        ParentObject: result.Spec.ParentObject,
        Errors:       string(errorsJSON),
        Details:      result.Spec.Details,
        SourceRef: domain.SourceRef{
            APIVersion: "core.k8sgpt.ai/v1alpha1",
            Kind:       "Result",
            Name:       result.Name,
            Namespace:  result.Namespace,
        },
    }, nil
}

func (p *K8sGPTProvider) Fingerprint(f *domain.Finding) string {
    // Re-parse error texts from the pre-serialised JSON string in f.Errors so they
    // can be sorted before hashing. This mirrors fingerprintFor() in CONTROLLER_LLD §4
    // which operates on the original []Failure slice — both produce identical output
    // because f.Errors is produced by the same serialisation (redacted Failure structs).
    var failures []struct {
        Text string `json:"text"`
    }
    _ = json.Unmarshal([]byte(f.Errors), &failures) // empty slice on error → still deterministic

    texts := make([]string, 0, len(failures))
    for _, fv := range failures {
        texts = append(texts, fv.Text)
    }
    sort.Strings(texts)

    payload := struct {
        Namespace    string   `json:"namespace"`
        Kind         string   `json:"kind"`
        ParentObject string   `json:"parentObject"`
        ErrorTexts   []string `json:"errorTexts"`
    }{
        Namespace:    f.Namespace,
        Kind:         f.Kind,
        ParentObject: f.ParentObject,
        ErrorTexts:   texts,
    }
    b, err := json.Marshal(payload)
    if err != nil {
        panic(fmt.Sprintf("K8sGPTProvider.Fingerprint: json.Marshal failed: %v", err))
    }
    return fmt.Sprintf("%x", sha256.Sum256(b))
}
```

---

## 7. RemediationJob Changes

`RemediationJobSpec` gains two fields to record the provider context:

```go
type RemediationJobSpec struct {
    // ... existing fields ...

    // SourceType identifies which provider created this RemediationJob.
    // Set to the value of SourceProvider.ProviderName().
    // Allows future tooling to filter RemediationJobs by source.
    SourceType string `json:"sourceType"`

    // SinkType identifies which sink the agent should use for output.
    // Defaults to "github" in v1. Injected as SINK_TYPE env var into the agent Job.
    SinkType string `json:"sinkType"`
}
```

The `sourceType` field also serves as documentation in `kubectl get remediationjobs`:

```
NAME                    PHASE      KIND    PARENT         SOURCE    AGE
mechanic-a3f9c2b14d8e  Succeeded  Pod     my-deployment  k8sgpt    2h
```

---

## 8. Event Filter Predicate

With the `SourceProviderReconciler`, the skip-if-no-errors logic moves into
`ExtractFinding()` returning `nil, nil`. No predicate is needed at the manager level —
the filtering is provider-specific and belongs in the provider.

---

## 9. Testing Strategy

### Unit tests (`internal/provider/k8sgpt/`)

These tests live in `provider_test.go` and `reconciler_test.go`.

| Test | File | Description |
|---|---|---|
| `TestK8sGPTProvider_ProviderName` | provider_test.go | Returns "k8sgpt" |
| `TestK8sGPTProvider_ExtractFinding_NoErrors` | provider_test.go | Returns nil, nil |
| `TestK8sGPTProvider_ExtractFinding_WithErrors` | provider_test.go | Returns populated Finding |
| `TestK8sGPTProvider_ExtractFinding_SensitiveRedacted` | provider_test.go | Sensitive fields absent from Finding.Errors |
| `TestK8sGPTProvider_ExtractFinding_WrongType` | provider_test.go | Non-Result object returns error |
| `TestFingerprintFor_Deterministic` | reconciler_test.go | Same ResultSpec input twice → same output |
| `TestFingerprintFor_ErrorOrderIndependent` | reconciler_test.go | Reversed errors → same fingerprint |
| `TestFingerprintFor_SameParentDifferentPods` | reconciler_test.go | Same namespace/parent/errors → same fingerprint |
| `TestFingerprintFor_DifferentErrors` | reconciler_test.go | Different error texts → different fingerprint |
| `TestFingerprintFor_DifferentParents` | reconciler_test.go | Same errors, different parent → different fingerprint |
| `TestFingerprintFor_DifferentNamespaces` | reconciler_test.go | Same parent/errors, different namespace → different fingerprint |
| `TestFingerprintFor_EmptyErrors` | reconciler_test.go | nil vs empty slice → same fingerprint |
| `TestFingerprintEquivalence` | reconciler_test.go | Cross-function equivalence: `fingerprintFor` and `K8sGPTProvider.Fingerprint` must produce identical output for the same logical finding. Table-driven; **must include a case with `<`, `>`, and `&` in error text** to guard against json.Marshal HTML-escaping divergence. Steps: (1) build a `*v1alpha1.Result`, (2) call `fingerprintFor(result.Namespace, result.Spec)`, (3) call `provider.ExtractFinding(result)` to get a `*domain.Finding`, (4) call `provider.Fingerprint(finding)`, (5) assert both outputs are equal. |

Note: `fingerprintFor()` (standalone function in `reconciler.go`) and `K8sGPTProvider.Fingerprint()`
(method in `provider.go`) implement the same algorithm at different abstraction levels.
`fingerprintFor` operates on `v1alpha1.ResultSpec`; `Fingerprint()` operates on `*domain.Finding`
by re-parsing the pre-serialised `Finding.Errors` JSON. Both must produce identical output for
the same logical finding. See CONTROLLER_LLD.md §4 and PROVIDER_LLD.md §6.

### Unit tests (`internal/provider/`)

These tests live in `internal/provider/provider_test.go`.

| Test | Description |
|---|---|
| `TestSourceProviderReconciler_CallsExtractFinding` | Reconciler calls provider.ExtractFinding |
| `TestSourceProviderReconciler_SkipsOnNilFinding` | nil finding → no RemediationJob created |
| `TestSourceProviderReconciler_CreatesRemediationJob` | Valid finding → RemediationJob created |
| `TestSourceProviderReconciler_SkipsDuplicateFingerprint` | Non-Failed RemediationJob exists → skip |
| `TestSourceProviderReconciler_ReDispatchesFailedRemediationJob` | Failed RemediationJob → new one |
