# Domain: RemediationJob CRD — Low-Level Design

**Version:** 1.0
**Date:** 2026-02-20
**Status:** Authoritative Specification
**HLD Reference:** [Sections 4, 5, 6, 13](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

`RemediationJob` is the custom resource that represents a single investigation of a
k8sgpt `Result` finding. It is the authoritative state record for the entire lifecycle of
one remediation attempt: from detection through Job dispatch to PR outcome.

Introducing this CRD replaces the in-memory `map[string]processedEntry` that the original
design used for deduplication. The CRD makes state durable, observable, and recoverable
across watcher restarts.

### 1.2 Why a CRD Instead of In-Memory State

| Concern | In-memory map | RemediationJob CRD |
|---|---|---|
| Watcher restart | State lost; re-dispatch races | State survives; reconciler checks existing objects |
| Observability | None — no `kubectl get` | `kubectl get remediationjobs -n mechanic` shows everything |
| Audit trail | None | Full history: when dispatched, what Job, what PR |
| Deduplication | Map evicted on restart | CRD exists until explicitly deleted or TTL expires |
| Upstream contribution | Bespoke, hard to integrate | Natural fit as a first-class k8sgpt-operator resource type |
| Job ownership | Orphaned on restart | `ownerReferences` cascades deletion automatically |

### 1.3 Design Principles

- One `RemediationJob` per unique fingerprint — the fingerprint is the deduplication key
- The watcher creates `RemediationJob` objects; a second controller reconciles them into
  `batch/v1 Jobs`
- `RemediationJob` owns the `batch/v1 Job` via `ownerReferences` — deleting a
  `RemediationJob` deletes its Job
- Status is written back to the `RemediationJob` as the Job progresses
- `RemediationJob` objects are created in the same namespace as the watcher (`mechanic`)

---

## 2. API Types

### 2.1 Group / Version / Kind

```
Group:   remediation.mechanic.io
Version: v1alpha1
Kind:    RemediationJob
```

The group `remediation.mechanic.io` is chosen to sit adjacent to the existing
`core.k8sgpt.ai` group, making upstream contribution natural. If this project is
eventually contributed to `k8sgpt-ai/k8sgpt-operator`, the group is already correct.

### 2.2 RemediationJobSpec

```go
type RemediationJobSpec struct {
    // SourceResultRef identifies the k8sgpt Result that triggered this remediation.
    // +kubebuilder:validation:Required
    SourceResultRef ResultRef `json:"sourceResultRef"`

    // Fingerprint is the SHA256 hash used for deduplication.
    // Computed from namespace + kind + parentObject + sorted(error texts).
    // Immutable after creation.
    Fingerprint string `json:"fingerprint"`

    // SourceType identifies which SourceProvider created this RemediationJob.
    // Set to the value of SourceProvider.ProviderName() (e.g. "k8sgpt", "prometheus").
    // Immutable after creation.
    SourceType string `json:"sourceType"`

    // SinkType identifies which sink the agent should use for output.
    // Defaults to "github". Injected as SINK_TYPE env var into the agent Job.
    // Immutable after creation.
    SinkType string `json:"sinkType"`

    // Finding contains the extracted finding context passed to the agent Job.
    Finding FindingSpec `json:"finding"`

    // GitOpsRepo is the GitHub repository in owner/repo format.
    GitOpsRepo string `json:"gitOpsRepo"`

    // GitOpsManifestRoot is the path within the cloned repo to the manifests root.
    GitOpsManifestRoot string `json:"gitOpsManifestRoot"`

    // AgentImage is the full image reference for the agent container.
    AgentImage string `json:"agentImage"`

    // AgentSA is the ServiceAccount name for the agent Job.
    AgentSA string `json:"agentSA"`
}

type ResultRef struct {
    // Name is the name of the k8sgpt Result object.
    Name string `json:"name"`

    // Namespace is the namespace of the k8sgpt Result object.
    Namespace string `json:"namespace"`
}

type FindingSpec struct {
    // Kind is the Kubernetes resource kind identified by k8sgpt (e.g. "Pod", "Deployment").
    Kind string `json:"kind"`

    // Name is the plain resource name (no namespace prefix).
    Name string `json:"name"`

    // Namespace is the namespace of the affected resource.
    Namespace string `json:"namespace"`

    // ParentObject is the owning resource (e.g. the Deployment owning crashing pods).
    ParentObject string `json:"parentObject"`

    // Errors is the serialised []Failure with Sensitive fields redacted.
    // Stored as a JSON string to avoid schema complexity.
    Errors string `json:"errors"`

    // Details is the k8sgpt LLM explanation of the finding.
    Details string `json:"details"`
}
```

### 2.3 RemediationJobStatus

```go
type RemediationJobStatus struct {
    // Phase is the current lifecycle phase of this RemediationJob.
    // +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed
    Phase RemediationJobPhase `json:"phase,omitempty"`

    // JobRef is the name of the batch/v1 Job created for this remediation.
    // Set once the Job has been created.
    JobRef string `json:"jobRef,omitempty"`

    // PRRef is the GitHub PR URL opened or commented on by the agent.
    // Set by the agent via a status patch before it exits (best-effort).
    PRRef string `json:"prRef,omitempty"`

    // DispatchedAt is the time the batch/v1 Job was created.
    DispatchedAt *metav1.Time `json:"dispatchedAt,omitempty"`

    // CompletedAt is the time the batch/v1 Job reached a terminal state.
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // Message is a human-readable description of the current state,
    // e.g. an error message if Phase is Failed.
    Message string `json:"message,omitempty"`

    // Conditions follows the standard Kubernetes condition pattern.
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RemediationJobPhase represents the lifecycle stage.
type RemediationJobPhase string

const (
    // SourceTypeK8sGPT is the SourceType value set by K8sGPTProvider.
    // Used in RemediationJobSpec.SourceType and as the return value of
    // K8sGPTProvider.ProviderName(). Defined here so all packages share one
    // authoritative constant instead of duplicating the magic string "k8sgpt".
    SourceTypeK8sGPT = "k8sgpt"

    // PhasePending means the RemediationJob has been created and the controller
    // has acknowledged it. The controller sets Phase=Pending immediately on the
    // first reconcile (transitioning from the Go zero value ""). A job may also
    // remain Pending while the MAX_CONCURRENT_JOBS limit is reached.
    PhasePending RemediationJobPhase = "Pending"

    // PhaseDispatched means the batch/v1 Job has been created and is starting.
    PhaseDispatched RemediationJobPhase = "Dispatched"

    // PhaseRunning means the batch/v1 Job's pod is actively running.
    PhaseRunning RemediationJobPhase = "Running"

    // PhaseSucceeded means the batch/v1 Job completed successfully.
    // The agent exited 0 — a PR may or may not have been opened (see PRRef).
    PhaseSucceeded RemediationJobPhase = "Succeeded"

    // PhaseFailed means the batch/v1 Job failed (all retries exhausted or
    // activeDeadlineSeconds exceeded).
    PhaseFailed RemediationJobPhase = "Failed"
)
```

### 2.4 Standard Condition Types

```go
const (
    // ConditionJobDispatched is True once the batch/v1 Job has been created.
    ConditionJobDispatched = "JobDispatched"

    // ConditionJobComplete is True once the batch/v1 Job reached Succeeded state.
    ConditionJobComplete = "JobComplete"

    // ConditionJobFailed is True if the batch/v1 Job failed.
    ConditionJobFailed = "JobFailed"
)
```

### 2.5 Full Object

```go
// RemediationJob represents one investigation and remediation attempt for a
// k8sgpt finding.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rjob
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.finding.kind`
// +kubebuilder:printcolumn:name="Parent",type=string,JSONPath=`.spec.finding.parentObject`
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.status.jobRef`
// +kubebuilder:printcolumn:name="PR",type=string,JSONPath=`.status.prRef`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type RemediationJob struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    // Spec is required — omitempty is intentionally absent.
    Spec   RemediationJobSpec   `json:"spec"`
    Status RemediationJobStatus `json:"status,omitempty"`
}

// RemediationJobList contains a list of RemediationJob.
// +kubebuilder:object:root=true
type RemediationJobList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []RemediationJob `json:"items"`
}
```

---

## 3. Controller Architecture

Introducing the `RemediationJob` CRD splits what was one controller into two reconcilers.
Both run in the same `mechanic-watcher` binary.

### 3.1 SourceProviderReconciler

**Watches:** `results.core.k8sgpt.ai` (all namespaces) — via `K8sGPTProvider.ObjectType()`
**Writes:** `RemediationJob` objects (in `mechanic` namespace)
**Does NOT:** create `batch/v1 Jobs` directly — that is now the job of the RemediationJobReconciler

`SourceProviderReconciler` (in `internal/provider/provider.go`) is itself the
`ctrl.Reconciler` registered with the manager. There is no separate `ResultReconciler`
type. Provider-specific logic (`ExtractFinding`, `Fingerprint`) lives in `K8sGPTProvider`;
all reconcile boilerplate lives in `SourceProviderReconciler`.

```
Reconcile(Result):
  1. Fetch Result.
     If NotFound:
       List RemediationJobs in AgentNamespace.
       For each where rjob.Spec.SourceResultRef.Name == req.Name
       AND rjob.Spec.SourceResultRef.Namespace == req.Namespace
        AND rjob.Status.Phase is Pending, Dispatched, or "" (created but not yet
        acknowledged by RemediationJobReconciler):
          Patch Phase=Cancelled, then delete the RemediationJob.
       (A deleted Result means the problem is resolved — only cancel pending work;
       Running/Succeeded/Failed RemediationJobs are left intact.)
       Return nil.
  2. provider.ExtractFinding(result) → finding
     If nil, nil: return nil (skip — no errors on this Result).
     If nil, err: return err (requeue).
  3. fingerprintFor(result.Namespace, result.Spec) → fp
  4. List RemediationJobs in AgentNamespace with label
     remediation.mechanic.io/fingerprint=<fp[:12]>
     If one exists and its Phase is not Failed → return nil (already handled).
  5. Build RemediationJob spec from Result + watcher config.
  6. client.Create(RemediationJob).
     If AlreadyExists → return nil.
     If other error → return error (requeue).
  7. Return nil.
```

`SourceProviderReconciler` does not need an in-memory map. The CRD is the deduplication
state. It also does not enforce `MAX_CONCURRENT_JOBS` — that is enforced by the
`RemediationJobReconciler`.

### 3.2 RemediationJobReconciler

**Watches:** `RemediationJob` (in `mechanic` namespace)
**Also watches:** `batch/v1 Jobs` owned by `RemediationJob` (via `Owns()`)
**Writes:** `batch/v1 Jobs`; patches `RemediationJob.Status`

```
Reconcile(RemediationJob):
  0. Fetch RemediationJob. If NotFound → return nil.
  0a. If Phase is "" (freshly created, Go zero value):
       Patch Status.Phase = Pending via the status subresource.
       Return Requeue:true (immediate re-enqueue via rate limiter).
       Rationale: the Kubernetes API server strips Status on Create, so every
       new object arrives with Phase="". This step ensures Phase is never blank
       in kubectl output and that all downstream logic can rely on named constants.
   1. If Phase is Succeeded:
        Apply TTL deletion logic (see §9 and CONTROLLER_LLD §6.2 step 2).
        If TTL has expired: delete and return nil.
        If TTL is not yet due: return RequeueAfter(deadline - now).
        If CompletedAt is not set: return nil (will be set when Job syncs).
      If Phase is Failed → return nil (terminal, retained indefinitely for postmortem).
  3. Look up owned Job by label remediation.mechanic.io/remediation-job=<rjob.Name>.
     If Job exists:
       a. Sync phase from Job status → update RemediationJob.Status.
       b. Return nil.
   4. Check MAX_CONCURRENT_JOBS:
      List Jobs with label app.kubernetes.io/managed-by=mechanic-watcher in AgentNamespace.
      Count those where job.Status.Active > 0 OR
                       (job.Status.Succeeded == 0 AND job.Status.CompletionTime == nil).
      (This counts Jobs that are actively running or pending; it excludes Failed jobs
      which have CompletionTime==nil but Status.Succeeded==0 and Status.Active==0.)
      If count >= MaxConcurrentJobs → requeue after 30s. Return.
  5. jobBuilder.Build(rjob) → job (with ownerReference pointing at rjob).
  6. client.Create(job).
     If AlreadyExists → re-fetch, sync status. Return nil.
     If other error → return error (requeue).
   7. Patch RemediationJob.Status:
      Phase=Dispatched, JobRef=job.Name, DispatchedAt=now,
      Condition ConditionJobDispatched=True.
  8. Return nil.
```

### 3.3 Job Status Sync

The `RemediationJobReconciler` is re-triggered whenever the owned `batch/v1 Job` changes
(via `Owns()`). On each trigger it maps Job conditions to `RemediationJob.Status.Phase`:

| Job state | RemediationJob Phase |
|---|---|
| Job created, no pods yet | `Dispatched` |
| Pod running | `Running` |
| `job.Status.Succeeded > 0` | `Succeeded` |
| `job.Status.Failed >= backoffLimit` or deadline exceeded | `Failed` |

---

## 4. Naming Convention

### RemediationJob name

```
mechanic-<first-12-chars-of-fingerprint>
```

This mirrors the original Job naming and is deterministic — the watcher can check for
existence by name without listing.

### batch/v1 Job name

```
mechanic-agent-<first-12-chars-of-fingerprint>
```

Unchanged from the original design.

---

## 5. Owner References

The `batch/v1 Job` is created with an `ownerReference` pointing at the `RemediationJob`:

```go
job.OwnerReferences = []metav1.OwnerReference{
    {
        APIVersion:         "remediation.mechanic.io/v1alpha1",
        Kind:               "RemediationJob",
        Name:               rjob.Name,
        UID:                rjob.UID,
        Controller:         ptr(true),
        BlockOwnerDeletion: ptr(true),
    },
}
```

Deleting a `RemediationJob` cascades to delete its `batch/v1 Job` and the Job's pods.

---

## 6. Status Reporting by the Agent

The agent (running inside the `batch/v1 Job`) can optionally patch the `RemediationJob`
status with the PR URL before exiting. This requires the agent ServiceAccount to have
`patch` on `remediationjobs/status` in the `mechanic` namespace (a Role, not ClusterRole).

The agent does this via `kubectl`:

```bash
kubectl patch remediationjob mechanic-<fingerprint[:12]> \
  -n mechanic \
  --subresource=status \
  --type=merge \
  --patch "{\"status\":{\"prRef\":\"${PR_URL}\"}}"
```

This is best-effort. If the patch fails, the `RemediationJob` status will simply not have
a `prRef` — the PR still exists on GitHub. The agent does not retry or fail because of
a status patch failure.

The agent knows its own `RemediationJob` name from the `FINDING_FINGERPRINT` env var:
`mechanic-${FINDING_FINGERPRINT:0:12}`.

---

## 7. CRD Manifest

The CRD manifest is generated from the kubebuilder markers in §2.5. For the initial
implementation without code generation, write it by hand in
`deploy/kustomize/crd-remediationjob.yaml`.

The CRD schema enforces:
- `spec.fingerprint` is required and immutable (`x-kubernetes-validations: [{rule: "self == oldSelf"}]`)
- `status` is a subresource (updated via `/status` endpoint, not via main object PATCH)
- `spec.finding.errors` is stored as a string (pre-serialised JSON) to avoid complex
  array schema definition

---

## 8. RBAC Changes

### Watcher additions

The watcher ClusterRole and Role must be extended:

```yaml
# clusterrole-watcher.yaml — add:
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch", "update"]
```

### Agent addition (status writeback)

```yaml
# role-agent.yaml (new, namespaced to mechanic):
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mechanic-agent
  namespace: mechanic
rules:
- apiGroups: ["remediation.mechanic.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch"]
```

---

## 9. RemediationJob TTL

`RemediationJob` objects should be cleaned up after a configurable retention period.
Two mechanisms:

1. **Succeeded jobs:** A TTL check is implemented in the `RemediationJobReconciler`: once
   `Phase == Succeeded`, on each reconcile triggered by the `Owns()` watch the controller
   checks `CompletedAt + RemediationJobTTL`. If now is past that deadline it deletes the
   object. This cascades to the owned `batch/v1 Job` (which has its own
   `ttlSecondsAfterFinished: 86400`). See `CONTROLLER_LLD.md §6.2 step 2` for the
   exact reconcile logic.
   TTL is configured via `REMEDIATION_JOB_TTL_SECONDS`, default `604800` (7 days).

   **Re-trigger guarantee:** When the TTL is not yet due, the reconciler returns
   `ctrl.Result{RequeueAfter: time.Until(deadline)}` rather than bare `nil`. This
   ensures the TTL deletion fires even when no `Owns()` events arrive after the owned
   `batch/v1 Job` is deleted by Kubernetes (which happens after `ttlSecondsAfterFinished:
   86400` — potentially 6 days before the `RemediationJob` TTL expires).

2. **Failed jobs:** Retained indefinitely by default for postmortem — operator must
   delete manually or implement their own cleanup. A future story can add a
   `failedTTLSecondsAfterFinished` field.

---

## 10. Testing Strategy

### Unit tests (`internal/provider/k8sgpt/`)

| Test | Description |
|---|---|
| `TestRemediationJob_DeepCopy` | DeepCopyObject produces independent copy |
| `TestRemediationJob_DeepCopyStatus` | Mutating status copy does not affect original |

### Integration tests (`internal/provider/` + `internal/controller/`)

| Test | Package | Description |
|---|---|---|
| `TestSourceProviderReconciler_CreatesRemediationJob` | provider | Valid finding → RemediationJob with correct SourceType and SinkType |
| `TestSourceProviderReconciler_DuplicateFingerprint_Skips` | provider | Non-Failed RemediationJob exists → skip |
| `TestSourceProviderReconciler_FailedPhase_ReDispatches` | provider | Failed RemediationJob → new one created |
| `TestRemediationJobReconciler_CreatesJob` | controller | Pending RemediationJob → batch/v1 Job created |
| `TestRemediationJobReconciler_SyncsStatus_Running` | controller | Job pod running → phase = Running |
| `TestRemediationJobReconciler_SyncsStatus_Succeeded` | controller | Job succeeded → phase = Succeeded |
| `TestRemediationJobReconciler_SyncsStatus_Failed` | controller | Job failed → phase = Failed |
| `TestRemediationJobReconciler_MaxConcurrentJobs` | controller | At limit → requeues, no new Job |
| `TestRemediationJobReconciler_OwnerReference` | controller | Created Job has correct ownerRef |
