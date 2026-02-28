# Domain: Controller — Low-Level Design

**Version:** 3.1
**Date:** 2026-02-20
**Status:** Implementation Ready
**HLD Reference:** [Sections 4.1, 5, 6, 7, 12](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

The controller layer contains two distinct concerns:

1. **SourceProviders** (`internal/provider/`) — translate external signals into
   `RemediationJob` objects. v1 has one: `K8sGPTProvider`, registered with a
   `SourceProviderReconciler` that owns the full reconcile loop.
2. **RemediationJobReconciler** (`internal/controller/`) — provider-agnostic reconciler
   that watches all `RemediationJob` objects and drives the Job lifecycle.

### 1.2 Design Principles

- **CRD as state** — no in-memory map; all deduplication state lives in `RemediationJob` objects
- **Single responsibility** — `SourceProviderReconciler` only creates RemediationJobs; `RemediationJobReconciler` only dispatches Jobs and tracks status
- **Safe under restart** — watcher restart loses no state; everything is reconstructed from the API server- **Owner references** — batch/v1 Jobs are owned by RemediationJobs; deletion cascades
- **Fail loud** — errors are returned so controller-runtime requeues; never swallowed

---

## 2. Package Structure

```
api/
└── v1alpha1/
    ├── result_types.go            # vendored k8sgpt-operator CRD types
    └── remediationjob_types.go    # our own CRD types (includes sourceType field)

internal/
├── domain/
│   ├── interfaces.go          # JobBuilder interface
│   └── provider.go            # SourceProvider interface + Finding + SourceRef types
├── provider/
│   ├── provider.go            # SourceProviderReconciler (generic, wraps any SourceProvider)
│   ├── provider_test.go       # SourceProviderReconciler unit tests
│   └── k8sgpt/
│       ├── provider.go        # K8sGPTProvider — implements SourceProvider
│       ├── provider_test.go   # K8sGPTProvider unit tests (ExtractFinding, Fingerprint)
│       ├── reconciler.go      # fingerprintFor() package-level function
│       ├── reconciler_test.go # fingerprintFor unit tests + envtest integration tests
│       └── suite_test.go      # envtest bootstrap for this package
└── controller/
    ├── remediationjob_controller.go   # RemediationJobReconciler
    ├── remediationjob_controller_test.go
    └── suite_test.go                  # envtest bootstrap

cmd/
└── watcher/
    └── main.go                    # scheme registration, provider loop, manager
```

---

## 3. Data Models

### 3.1 Vendored CRD types (`api/v1alpha1/result_types.go`)

Unchanged from v1.1. Minimal vendored subset of k8sgpt-operator types:

```go
type ResultSpec struct {
    Backend      string    `json:"backend"`
    Kind         string    `json:"kind"`
    Name         string    `json:"name"`
    Error        []Failure `json:"error"`
    Details      string    `json:"details"`
    ParentObject string    `json:"parentObject"`
}

type Failure struct {
    Text      string      `json:"text,omitempty"`
    Sensitive []Sensitive `json:"sensitive,omitempty"`
}

type Sensitive struct {
    Unmasked string `json:"unmasked,omitempty"`
    Masked   string `json:"masked,omitempty"`
}
```

`AutoRemediationStatus` is intentionally omitted. Both `Result` and `ResultList` implement
`runtime.Object` via hand-written `DeepCopyObject()` and `DeepCopyInto()`.

### 3.2 RemediationJob types (`api/v1alpha1/remediationjob_types.go`)

See [REMEDIATIONJOB_LLD.md](REMEDIATIONJOB_LLD.md) for the full type definitions.
`RemediationJob` and `RemediationJobList` also implement `runtime.Object` via hand-written
deep copy methods.

---

## 4. Fingerprint Algorithm

Unchanged. The fingerprint is computed from the k8sgpt `Result`, not from the
`RemediationJob`. `SourceProviderReconciler.Reconcile()` computes it via `fingerprintFor`;
the `RemediationJobReconciler` reads it from `spec.fingerprint`.

```go
// fingerprintFor is a package-level function in internal/provider/k8sgpt/reconciler.go.
// It is called by SourceProviderReconciler.Reconcile() after ExtractFinding returns a
// non-nil Finding. Uses json.NewEncoder with SetEscapeHTML(false) so that error texts
// containing <, >, & hash identically to K8sGPTProvider.Fingerprint() which operates
// on the same bytes after the round-trip through Finding.Errors JSON.
func fingerprintFor(namespace string, spec v1alpha1.ResultSpec) string {
    texts := make([]string, 0, len(spec.Error))
    for _, f := range spec.Error {
        texts = append(texts, f.Text)
    }
    sort.Strings(texts)

    payload := struct {
        Namespace    string   `json:"namespace"`
        Kind         string   `json:"kind"`
        ParentObject string   `json:"parentObject"`
        ErrorTexts   []string `json:"errorTexts"`
    }{
        Namespace:    namespace,
        Kind:         spec.Kind,
        ParentObject: spec.ParentObject,
        ErrorTexts:   texts,
    }

    var buf bytes.Buffer
    enc := json.NewEncoder(&buf)
    enc.SetEscapeHTML(false)
    if err := enc.Encode(payload); err != nil {
        panic(fmt.Sprintf("fingerprintFor: json.Encode failed: %v", err))
    }
    return fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
}
```

---

## 5. K8sGPTProvider

### 5.0 Provider Struct (`internal/provider/k8sgpt/provider.go`)

See [PROVIDER_LLD.md](PROVIDER_LLD.md) §6 for the full implementation. The provider is a
plain struct with no fields:

```go
type K8sGPTProvider struct{}

func (p *K8sGPTProvider) ProviderName() string { return v1alpha1.SourceTypeK8sGPT }
func (p *K8sGPTProvider) ObjectType() client.Object { return &v1alpha1.Result{} }
func (p *K8sGPTProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) { ... }
func (p *K8sGPTProvider) Fingerprint(f *domain.Finding) string { ... }
```

`K8sGPTProvider` is the only exported symbol from this package that `main.go` needs. It
satisfies `domain.SourceProvider` and is registered with a `provider.SourceProviderReconciler`
in `main.go`. All reconcile boilerplate lives in `SourceProviderReconciler`; provider-specific
logic (`ExtractFinding`, `Fingerprint`) lives in `K8sGPTProvider`.

There is no separate `ResultReconciler` type. `SourceProviderReconciler` is itself the
`ctrl.Reconciler` registered with the manager. The file
`internal/provider/k8sgpt/reconciler.go` contains only the `fingerprintFor()` package-level
function — no struct, no `Reconcile` method.

### 5.1 SourceProviderReconciler Reconcile Loop (`internal/provider/provider.go`)

```
Reconcile(ctx, req):
  1. Fetch Result.
     If NotFound:
       - List RemediationJobs in cfg.AgentNamespace.
         For each where rjob.Spec.SourceResultRef.Name == req.Name
         AND rjob.Spec.SourceResultRef.Namespace == req.Namespace
         AND rjob.Status.Phase is Pending or Dispatched:
           delete the RemediationJob.
         (A deleted Result means the problem is resolved — cancel pending work.)
       - Return nil.

  2. fingerprintFor(result.Namespace, result.Spec) → fp

  3. List RemediationJobs in cfg.AgentNamespace with label
     remediation.mechanic.io/fingerprint=fp[:12]
     For each match:
       if rjob.Spec.Fingerprint == fp AND rjob.Status.Phase != PhaseFailed:
         return nil  // already handled, not failed

   4. Build RemediationJob from result + fp:
      name: "mechanic-" + fp[:12]
      namespace: cfg.AgentNamespace
       labels:
         remediation.mechanic.io/fingerprint: fp[:12]
       annotations:
         remediation.mechanic.io/fingerprint-full: fp
      spec:
        sourceType: "k8sgpt"
        sourceResultRef: {name: result.Name, namespace: result.Namespace}
        fingerprint: fp
        finding: {kind, name, namespace, parentObject, errors(redacted JSON), details}
        gitOpsRepo: cfg.GitOpsRepo
        gitOpsManifestRoot: cfg.GitOpsManifestRoot
        agentImage: cfg.AgentImage
        agentSA: cfg.AgentSA

  5. client.Create(ctx, rjob)
     If AlreadyExists: return nil
     If other error: return error (requeue)

  6. Log creation. Return nil.
```

### 5.3 Event Filtering

Skip-if-no-errors logic is handled in `K8sGPTProvider.ExtractFinding()` returning `nil, nil`
when `len(result.Spec.Error) == 0`. No manager-level predicate is needed — filtering is
provider-specific and belongs in the provider. See [PROVIDER_LLD.md](PROVIDER_LLD.md) §8.

---

## 6. RemediationJobReconciler

### 6.1 Struct

```go
type RemediationJobReconciler struct {
    client.Client
    Scheme     *runtime.Scheme
    Log        *zap.Logger
    JobBuilder domain.JobBuilder
    Cfg        config.Config
}
```

### 6.2 Reconcile Loop

```
Reconcile(ctx, req):
  1. Fetch RemediationJob.
     If NotFound: return nil.

  2. If rjob.Status.Phase == PhaseSucceeded:
       If rjob.Status.CompletedAt != nil AND now >= CompletedAt + RemediationJobTTL:
         client.Delete(ctx, rjob)  // cascades to owned Job via ownerReferences
         return nil
       // TTL not yet due — requeue at the exact moment it becomes due so the
       // reconciler is guaranteed to fire even if no Owns() events arrive
       // (the owned Job is deleted by Kubernetes after ttlSecondsAfterFinished=86400,
       // which may be before the RemediationJob TTL expires).
       If rjob.Status.CompletedAt != nil:
         deadline := CompletedAt.Add(RemediationJobTTL)
         return ctrl.Result{RequeueAfter: time.Until(deadline)}, nil
       return nil  // CompletedAt not yet set; will be set when Job syncs

     If rjob.Status.Phase == PhaseFailed:
       return nil  // terminal; retained indefinitely for postmortem

  3. List batch/v1 Jobs in cfg.AgentNamespace with label
     remediation.mechanic.io/remediation-job=rjob.Name
     If a Job exists:
       a. newPhase = syncPhaseFromJob(job)
       b. If newPhase != rjob.Status.Phase (or CompletedAt/JobRef not yet set):
            Patch rjob.Status: Phase=newPhase, CompletedAt=now (if terminal),
            Condition ConditionJobComplete=True (if Succeeded),
            Condition ConditionJobFailed=True (if Failed).
       c. Return nil.

  4. Check MAX_CONCURRENT_JOBS:
     List batch/v1 Jobs in cfg.AgentNamespace with label
     app.kubernetes.io/managed-by=mechanic-watcher.
     Count those where:
       job.Status.Active > 0
       OR (job.Status.Succeeded == 0 AND job.Status.CompletionTime == nil)
     (This correctly counts Pending and Running Jobs. It excludes Failed Jobs
     because Failed Jobs have CompletionTime != nil once all retries are
     exhausted, and excludes Succeeded Jobs via the Succeeded > 0 branch.
     Note: a freshly-created Job has Active==0, Succeeded==0, CompletionTime==nil
     — it is counted, preventing overrun before any pod starts.)
     If count >= cfg.MaxConcurrentJobs:
       return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

  5. jobBuilder.Build(rjob) → job
     (job is returned with ownerReference and managed-by label already set
     by the builder — see JOBBUILDER_LLD.md §4 and §5)

  6. client.Create(ctx, job)
     If AlreadyExists:
       Re-fetch the existing Job, run syncPhaseFromJob, patch rjob.Status. Return nil.
     If other error: return error (requeue with exponential backoff)

  7. Patch rjob.Status:
       Phase=PhaseDispatched
       JobRef=job.Name
       DispatchedAt=now
       Condition ConditionJobDispatched=True

  8. Return nil.
```

### 6.3 syncPhaseFromJob

```go
func syncPhaseFromJob(job *batchv1.Job) v1alpha1.RemediationJobPhase {
    if job.Status.Succeeded > 0 {
        return v1alpha1.PhaseSucceeded
    }
    // BackoffLimit is a pointer (*int32); the Kubernetes default is 6 but the
    // API server's defaulting may not be reflected in the Go object (e.g. in
    // unit tests using fakeJobBuilder where no defaulting webhook runs).
    // Always nil-guard before dereferencing.
    var backoffLimit int32 = 6 // Kubernetes default
    if job.Spec.BackoffLimit != nil {
        backoffLimit = *job.Spec.BackoffLimit
    }
    if job.Status.Failed >= backoffLimit+1 {
        return v1alpha1.PhaseFailed
    }
    if job.Status.Active > 0 {
        return v1alpha1.PhaseRunning
    }
    return v1alpha1.PhaseDispatched
}
```

Phase transitions are monotonic: `Pending → Dispatched → Running → Succeeded/Failed`.
A phase never moves backwards.

### 6.4 SetupWithManager

```go
func (r *RemediationJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.RemediationJob{}).
        Owns(&batchv1.Job{}).   // re-trigger when owned Job changes
        Complete(r)
}
```

`Owns()` ensures the reconciler is triggered when any `batch/v1 Job` owned by a
`RemediationJob` has a status update — this is what drives phase sync without polling.

---

## 7. Manager Setup (`cmd/watcher/main.go`)

```go
scheme := runtime.NewScheme()
_ = clientgoscheme.AddToScheme(scheme)
_ = batchv1.AddToScheme(scheme)
_ = v1alpha1.AddToScheme(scheme)   // registers Result, ResultList, RemediationJob, RemediationJobList

mgr := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme:                 scheme,
    LeaderElection:         false,
    Metrics: server.Options{BindAddress: ":8080"},
    HealthProbeBindAddress: ":8081",
})

cfg, err := config.FromEnv()
if err != nil {
    log.Fatal("config error", zap.Error(err))
}

// Register the provider-agnostic RemediationJob reconciler
jb, err := jobbuilder.New(jobbuilder.Config{
    AgentNamespace: cfg.AgentNamespace,
})
if err != nil {
    log.Fatal("jobbuilder init failed", zap.Error(err))
}
if err := (&controller.RemediationJobReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jb,
    Cfg: cfg,
}).SetupWithManager(mgr); err != nil {
    log.Fatal("RemediationJobReconciler setup failed", zap.Error(err))
}

// Register compiled-in source providers
enabledProviders := []domain.SourceProvider{
    &k8sgpt.K8sGPTProvider{},
}
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

mgr.AddHealthzCheck("healthz", healthz.Ping)
mgr.AddReadyzCheck("readyz", healthz.Ping)
mgr.Start(ctrl.SetupSignalHandler())
```

---

## 8. Concurrency

Both reconcilers use controller-runtime's default single-worker loop. The
`RemediationJobReconciler` is the only place that writes `batch/v1 Jobs` and patches
`RemediationJob` status — no mutex needed because controller-runtime serialises reconcile
calls for the same object.

---

## 9. Error Handling

| Error | Handling |
|---|---|
| Result not found | Delete corresponding Pending/Dispatched RemediationJob. Return nil. (in SourceProviderReconciler) |
| RemediationJob not found | Return nil |
| RemediationJob AlreadyExists | Return nil (deduplicated) |
| Job AlreadyExists | Re-fetch, sync status, return nil |
| Job creation fails (other) | Return wrapped error — requeues with exponential backoff |
| MAX_CONCURRENT_JOBS reached | Requeue after 30s — not an error |
| Status patch fails | Return error — requeues; phase will be re-synced |
| fingerprintFor panics | Process crashes immediately — controller-runtime does NOT recover from panics in reconcilers. In practice `json.Marshal` on this struct never fails; the panic is a last-resort guard against future code changes adding unmarshalable fields. |
| Config env var missing | Fatal at startup |
| Provider SetupWithManager fails | Fatal at startup |

---

## 10. Logging

Structured fields on every log line:

```go
log.Info("created RemediationJob",
    zap.String("fingerprint", fp[:12]),
    zap.String("kind", result.Spec.Kind),
    zap.String("parentObject", result.Spec.ParentObject),
    zap.String("remediationJob", rjob.Name),
)

log.Info("dispatched agent job",
    zap.String("remediationJob", rjob.Name),
    zap.String("job", job.Name),
    zap.String("namespace", job.Namespace),
)
```

---

## 11. Testing Strategy

### Unit tests (no cluster required)

| Test | Package | Description |
|---|---|---|
| `TestFingerprintFor_SameParentDifferentPods` | provider/k8sgpt | Same namespace + parent + errors → same fingerprint |
| `TestFingerprintFor_DifferentErrors` | provider/k8sgpt | Different error texts → different fingerprint |
| `TestFingerprintFor_ErrorOrderIndependent` | provider/k8sgpt | Reversed error slice → same fingerprint |
| `TestFingerprintFor_DifferentParents` | provider/k8sgpt | Same errors, different parent → different fingerprint |
| `TestFingerprintFor_DifferentNamespaces` | provider/k8sgpt | Same parent + errors, different namespace → different fingerprint |
| `TestFingerprintFor_EmptyErrors` | provider/k8sgpt | nil vs empty slice → same fingerprint |
| `TestFingerprintFor_Deterministic` | provider/k8sgpt | Same input twice → same output |

### Integration tests (envtest)

| Test | Reconciler | Package | Description |
|---|---|---|---|
| `TestSourceProviderReconciler_CreatesRemediationJob` | SourceProviderReconciler | provider/k8sgpt | New Result → RemediationJob created with sourceType="k8sgpt" |
| `TestSourceProviderReconciler_DuplicateFingerprint_Skips` | SourceProviderReconciler | provider/k8sgpt | Same fingerprint → no second RemediationJob |
| `TestSourceProviderReconciler_FailedPhase_ReDispatches` | SourceProviderReconciler | provider/k8sgpt | Existing Failed RemediationJob → new one created |
| `TestSourceProviderReconciler_NoErrors_Skipped` | SourceProviderReconciler | provider/k8sgpt | Result with no errors → ExtractFinding returns nil, nil → no RemediationJob |
| `TestSourceProviderReconciler_ResultDeleted_CancelsPending` | SourceProviderReconciler | provider/k8sgpt | Result deleted → Pending RemediationJob deleted |
| `TestSourceProviderReconciler_ResultDeleted_CancelsDispatched` | SourceProviderReconciler | provider/k8sgpt | Result deleted → Dispatched RemediationJob deleted |
| `TestRemediationJobReconciler_CreatesJob` | RemediationJobReconciler | controller | Pending RemediationJob → Job created |
| `TestRemediationJobReconciler_SyncsStatus_Running` | RemediationJobReconciler | controller | Job active → phase = Running |
| `TestRemediationJobReconciler_SyncsStatus_Succeeded` | RemediationJobReconciler | controller | Job succeeded → phase = Succeeded |
| `TestRemediationJobReconciler_SyncsStatus_Failed` | RemediationJobReconciler | controller | Job failed → phase = Failed |
| `TestRemediationJobReconciler_MaxConcurrentJobs_Requeues` | RemediationJobReconciler | controller | At limit → requeues, no new Job |
| `TestRemediationJobReconciler_OwnerReference` | RemediationJobReconciler | controller | Created Job has ownerRef → RemediationJob |
| `TestRemediationJobReconciler_Terminal_NoOp` | RemediationJobReconciler | controller | Succeeded/Failed phase → no action (no requeue for Failed; TTL requeue for Succeeded) |
