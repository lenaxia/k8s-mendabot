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
| Observability | None — no `kubectl get` | `kubectl get remediationjobs -n mendabot` shows everything |
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
- `RemediationJob` objects are created in the same namespace as the watcher (`mendabot`)

---

## 2. API Types

### 2.1 Group / Version / Kind

```
Group:   remediation.mendabot.io
Version: v1alpha1
Kind:    RemediationJob
```

The group `remediation.mendabot.io` is chosen to sit adjacent to the existing
`core.k8sgpt.ai` group, making upstream contribution natural. If this project is
eventually contributed to `k8sgpt-ai/k8sgpt-operator`, the group is already correct.

### 2.2 RemediationJobSpec

```go
type RemediationJobSpec struct {
    // SourceResultRef identifies the k8sgpt Result that triggered this remediation.
    SourceResultRef ResultRef `json:"sourceResultRef"`

    // Fingerprint is the SHA256 hash used for deduplication.
    // Computed from namespace + kind + parentObject + sorted(error texts).
    Fingerprint string `json:"fingerprint"`

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
    // PhasePending means the RemediationJob has been created but no batch/v1 Job
    // exists yet (e.g. MAX_CONCURRENT_JOBS limit is currently reached).
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

    Spec   RemediationJobSpec   `json:"spec,omitempty"`
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
Both run in the same `mendabot-watcher` binary.

### 3.1 ResultReconciler

**Watches:** `results.core.k8sgpt.ai` (all namespaces)
**Writes:** `RemediationJob` objects (in `mendabot` namespace)
**Does NOT:** create `batch/v1 Jobs` directly — that is now the job of the RemediationJobReconciler

```
Reconcile(Result):
  1. Fetch Result. If NotFound → delete corresponding RemediationJob (if any). Return nil.
  2. Compute fingerprint.
  3. List RemediationJobs in AgentNamespace with label
     remediation.mendabot.io/fingerprint=<fingerprint>
     If one exists and its Phase is not Failed → return nil (already handled).
  4. Build RemediationJob spec from Result + watcher config.
  5. client.Create(RemediationJob).
     If AlreadyExists → return nil.
     If other error → return error (requeue).
  6. Return nil.
```

The `ResultReconciler` no longer needs an in-memory map. The CRD is the deduplication
state. It also no longer enforces `MAX_CONCURRENT_JOBS` — that is enforced by the
`RemediationJobReconciler`.

### 3.2 RemediationJobReconciler

**Watches:** `RemediationJob` (in `mendabot` namespace)
**Also watches:** `batch/v1 Jobs` owned by `RemediationJob` (via `Owns()`)
**Writes:** `batch/v1 Jobs`; patches `RemediationJob.Status`

```
Reconcile(RemediationJob):
  1. Fetch RemediationJob. If NotFound → return nil.
  2. If Phase is Succeeded or Failed → return nil (terminal, nothing to do).
  3. Look up owned Job by label remediation.mendabot.io/remediation-job=<rjob.Name>.
     If Job exists:
       a. Sync phase from Job status → update RemediationJob.Status.
       b. Return nil.
  4. Check MAX_CONCURRENT_JOBS:
     List Jobs with label app.kubernetes.io/managed-by=mendabot-watcher in AgentNamespace.
     Count those where CompletionTime == nil.
     If count >= MaxConcurrentJobs → requeue after 30s. Return.
  5. jobBuilder.Build(rjob) → job (with ownerReference pointing at rjob).
  6. client.Create(job).
     If AlreadyExists → re-fetch, sync status. Return nil.
     If other error → return error (requeue).
  7. Patch RemediationJob.Status:
     Phase=Dispatched, JobRef=job.Name, DispatchedAt=now.
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
mendabot-<first-12-chars-of-fingerprint>
```

This mirrors the original Job naming and is deterministic — the watcher can check for
existence by name without listing.

### batch/v1 Job name

```
mendabot-agent-<first-12-chars-of-fingerprint>
```

Unchanged from the original design.

---

## 5. Owner References

The `batch/v1 Job` is created with an `ownerReference` pointing at the `RemediationJob`:

```go
job.OwnerReferences = []metav1.OwnerReference{
    {
        APIVersion:         "remediation.mendabot.io/v1alpha1",
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
`patch` on `remediationjobs/status` in the `mendabot` namespace (a Role, not ClusterRole).

The agent does this via `kubectl`:

```bash
kubectl patch remediationjob mendabot-<fingerprint[:12]> \
  -n mendabot \
  --subresource=status \
  --type=merge \
  --patch "{\"status\":{\"prRef\":\"${PR_URL}\"}}"
```

This is best-effort. If the patch fails, the `RemediationJob` status will simply not have
a `prRef` — the PR still exists on GitHub. The agent does not retry or fail because of
a status patch failure.

The agent knows its own `RemediationJob` name from the `FINDING_FINGERPRINT` env var:
`mendabot-${FINDING_FINGERPRINT:0:12}`.

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
- apiGroups: ["remediation.mendabot.io"]
  resources: ["remediationjobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["remediation.mendabot.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch", "update"]
```

### Agent addition (status writeback)

```yaml
# role-agent.yaml (new, namespaced to mendabot):
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mendabot-agent
  namespace: mendabot
rules:
- apiGroups: ["remediation.mendabot.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch"]
```

---

## 9. RemediationJob TTL

`RemediationJob` objects should be cleaned up after a configurable retention period.
Two mechanisms:

1. **Succeeded jobs:** A `ttlSecondsAfterFinished`-equivalent is implemented in the
   `RemediationJobReconciler`: once `Phase == Succeeded`, requeue after
   `RemediationJobTTL` (default 7 days) and delete the object. This cascades to the
   owned `batch/v1 Job` (which has its own `ttlSecondsAfterFinished: 86400`).

2. **Failed jobs:** Retained indefinitely by default for postmortem — operator must
   delete manually or implement their own cleanup. A future story can add a
   `failedTTLSecondsAfterFinished` field.

---

## 10. Testing Strategy

### Unit tests (`api/v1alpha1/`)

| Test | Description |
|---|---|
| `TestRemediationJob_DeepCopy` | DeepCopyObject produces independent copy |
| `TestRemediationJob_DeepCopyStatus` | Mutating status copy does not affect original |

### Unit tests (`internal/controller/`)

| Test | Description |
|---|---|
| `TestResultReconciler_CreatesRemediationJob` | New Result → RemediationJob created |
| `TestResultReconciler_DuplicateFingerprint_Skips` | Same fingerprint → no second RemediationJob |
| `TestResultReconciler_FailedPhase_ReDispatches` | Existing RemediationJob in Failed phase → new one created |
| `TestRemediationJobReconciler_CreatesJob` | Pending RemediationJob → batch/v1 Job created |
| `TestRemediationJobReconciler_SyncsStatus_Running` | Job pod running → phase = Running |
| `TestRemediationJobReconciler_SyncsStatus_Succeeded` | Job succeeded → phase = Succeeded |
| `TestRemediationJobReconciler_SyncsStatus_Failed` | Job failed → phase = Failed |
| `TestRemediationJobReconciler_MaxConcurrentJobs` | At limit → requeues, no new Job |
| `TestRemediationJobReconciler_OwnerReference` | Created Job has correct ownerRef |
