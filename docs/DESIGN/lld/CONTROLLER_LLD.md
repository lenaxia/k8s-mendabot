# Domain: Controller — Low-Level Design

**Version:** 3.0
**Date:** 2026-02-20
**Status:** Implementation Ready
**HLD Reference:** [Sections 4.1, 5, 6, 7, 12](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

The controller layer contains two distinct concerns:

1. **SourceProviders** (`internal/provider/`) — translate external signals into
   `RemediationJob` objects. v1 has one: `K8sGPTSourceProvider` which wraps the
   `ResultReconciler`.
2. **RemediationJobReconciler** (`internal/controller/`) — provider-agnostic reconciler
   that watches all `RemediationJob` objects and drives the Job lifecycle.

### 1.2 Design Principles

- **CRD as state** — no in-memory map; all deduplication state lives in `RemediationJob` objects
- **Single responsibility** — ResultReconciler only creates RemediationJobs; RemediationJobReconciler only dispatches Jobs and tracks status
- **Safe under restart** — watcher restart loses no state; everything is reconstructed from the API server
- **Owner references** — batch/v1 Jobs are owned by RemediationJobs; deletion cascades
- **Fail loud** — errors are returned so controller-runtime requeues; never swallowed

---

## 2. Package Structure

```
api/
└── v1alpha1/
    ├── result_types.go            # vendored k8sgpt-operator CRD types
    └── remediationjob_types.go    # our own CRD types (includes sourceType field)

internal/
├── provider/
│   ├── interface.go               # SourceProvider interface
│   └── k8sgpt/
│       ├── provider.go            # K8sGPTSourceProvider — SetupWithManager
│       ├── reconciler.go          # ResultReconciler (watches Result CRDs)
│       └── reconciler_test.go
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
`RemediationJob`. The `ResultReconciler` computes it; the `RemediationJobReconciler` reads
it from `spec.fingerprint`.

```go
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

    b, err := json.Marshal(payload)
    if err != nil {
        panic(fmt.Sprintf("fingerprintFor: json.Marshal failed: %v", err))
    }
    return fmt.Sprintf("%x", sha256.Sum256(b))
}
```

---

## 5. K8sGPTSourceProvider

### 5.0 Provider Struct (`internal/provider/k8sgpt/provider.go`)

```go
type K8sGPTSourceProvider struct {
    Cfg config.Config
    Log *zap.Logger
}

func NewProvider(cfg config.Config, log *zap.Logger) *K8sGPTSourceProvider {
    return &K8sGPTSourceProvider{Cfg: cfg, Log: log}
}

func (p *K8sGPTSourceProvider) SetupWithManager(mgr ctrl.Manager) error {
    return (&ResultReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Log:    p.Log,
        Cfg:    p.Cfg,
    }).SetupWithManager(mgr)
}
```

`K8sGPTSourceProvider` satisfies `provider.SourceProvider`. It is the only exported
symbol from this package that `main.go` needs.

### 5.1 ResultReconciler Struct (`internal/provider/k8sgpt/reconciler.go`)

```go
type ResultReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    Log    *zap.Logger
    Cfg    config.Config
}
```

No mutex. No in-memory map. All state is in the cluster.

### 5.2 Reconcile Loop

```
Reconcile(ctx, req):
  1. Fetch Result.
     If NotFound:
       - Find RemediationJob with annotation opencode.io/result-name=req.Name
         in req.Namespace. If found and phase is Pending or Dispatched, delete it.
         (A deleted Result means the problem is resolved — cancel pending work.)
       - Return nil.

  2. fingerprintFor(result.Namespace, result.Spec) → fp

  3. List RemediationJobs in cfg.AgentNamespace with label
     remediation.mendabot.io/fingerprint=fp[:12]
     For each match:
       if rjob.Spec.Fingerprint == fp AND rjob.Status.Phase != PhaseFailed:
         return nil  // already handled, not failed

   4. Build RemediationJob from result + fp:
      name: "mendabot-" + fp[:12]
      namespace: cfg.AgentNamespace
      labels:
        remediation.mendabot.io/fingerprint: fp[:12]
      annotations:
        opencode.io/fingerprint-full: fp
        opencode.io/result-name: result.Name
        opencode.io/result-namespace: result.Namespace
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

A predicate filters out Results with no errors before they enter the reconcile queue:

```go
WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
    result, ok := obj.(*v1alpha1.Result)
    if !ok {
        return true
    }
    return len(result.Spec.Error) > 0
}))
```

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

  2. If rjob.Status.Phase == PhaseSucceeded or PhaseFailed:
     return nil  // terminal

  3. Look up owned Job:
     list Jobs in cfg.AgentNamespace with label
     remediation.mendabot.io/remediation-job=rjob.Name
     If exactly one exists:
       a. syncPhaseFromJob(rjob, job) → patch status if changed
       b. return nil

  4. Check MAX_CONCURRENT_JOBS:
     list Jobs in cfg.AgentNamespace with label
     app.kubernetes.io/managed-by=mendabot-watcher
     count where job.Status.CompletionTime == nil
     if count >= cfg.MaxConcurrentJobs:
       return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

  5. jobBuilder.Build(rjob) → job
     (job has ownerReference pointing at rjob)

  6. client.Create(ctx, job)
     If AlreadyExists: re-fetch job, syncPhaseFromJob, return nil
     If other error: return error (requeue)

  7. Patch rjob.Status:
     Phase=PhaseDispatched
     JobRef=job.Name
     DispatchedAt=now
     Condition ConditionJobDispatched=True

  8. Log dispatch. Return nil.
```

### 6.3 syncPhaseFromJob

```go
func syncPhaseFromJob(rjob *v1alpha1.RemediationJob, job *batchv1.Job) RemediationJobPhase {
    if job.Status.Succeeded > 0 {
        return PhaseSucceeded
    }
    if job.Status.Failed >= *job.Spec.BackoffLimit+1 {
        return PhaseFailed
    }
    if job.Status.Active > 0 {
        return PhaseRunning
    }
    return PhaseDispatched
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
(&controller.RemediationJobReconciler{
    Client:     mgr.GetClient(),
    Scheme:     mgr.GetScheme(),
    Log:        logger,
    JobBuilder: jobbuilder.New(domain.JobBuilderConfig{
        GitOpsRepo:         cfg.GitOpsRepo,
        GitOpsManifestRoot: cfg.GitOpsManifestRoot,
        AgentImage:         cfg.AgentImage,
        AgentNamespace:     cfg.AgentNamespace,
        AgentSA:            cfg.AgentSA,
    }),
    Cfg: cfg,
}).SetupWithManager(mgr)

// Register compiled-in source providers
providers := []provider.SourceProvider{
    k8sgpt.NewProvider(cfg, logger),
}
for _, p := range providers {
    if err := p.SetupWithManager(mgr); err != nil {
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
| Result not found | Delete corresponding Pending/Dispatched RemediationJob. Return nil. (in provider/k8sgpt) |
| RemediationJob not found | Return nil |
| RemediationJob AlreadyExists | Return nil (deduplicated) |
| Job AlreadyExists | Re-fetch, sync status, return nil |
| Job creation fails (other) | Return wrapped error — requeues with exponential backoff |
| MAX_CONCURRENT_JOBS reached | Requeue after 30s — not an error |
| Status patch fails | Return error — requeues; phase will be re-synced |
| fingerprintFor panics | Caught by controller-runtime; logged as fatal |
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
| `TestResultReconciler_CreatesRemediationJob` | Result | provider/k8sgpt | New Result → RemediationJob created with sourceType="k8sgpt" |
| `TestResultReconciler_DuplicateFingerprint_Skips` | Result | provider/k8sgpt | Same fingerprint → no second RemediationJob |
| `TestResultReconciler_FailedPhase_ReDispatches` | Result | provider/k8sgpt | Existing Failed RemediationJob → new one created |
| `TestResultReconciler_NoErrors_Skipped` | Result | provider/k8sgpt | Result with no errors → no RemediationJob |
| `TestResultReconciler_ResultDeleted_CancelsPending` | Result | provider/k8sgpt | Result deleted → Pending RemediationJob deleted |
| `TestRemediationJobReconciler_CreatesJob` | RemediationJob | controller | Pending RemediationJob → Job created |
| `TestRemediationJobReconciler_SyncsStatus_Running` | RemediationJob | controller | Job active → phase = Running |
| `TestRemediationJobReconciler_SyncsStatus_Succeeded` | RemediationJob | controller | Job succeeded → phase = Succeeded |
| `TestRemediationJobReconciler_SyncsStatus_Failed` | RemediationJob | controller | Job failed → phase = Failed |
| `TestRemediationJobReconciler_MaxConcurrentJobs_Requeues` | RemediationJob | controller | At limit → requeues, no new Job |
| `TestRemediationJobReconciler_OwnerReference` | RemediationJob | controller | Created Job has ownerRef → RemediationJob |
| `TestRemediationJobReconciler_Terminal_NoOp` | RemediationJob | controller | Succeeded/Failed phase → no action |
