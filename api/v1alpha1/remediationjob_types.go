package v1alpha1

//go:generate make -C ../.. generate

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	remediationGroupVersion = schema.GroupVersion{
		Group:   "remediation.mechanic.io",
		Version: "v1alpha1",
	}
	// AddRemediationToScheme registers RemediationJob and RemediationJobList under
	// remediation.mechanic.io/v1alpha1.
	AddRemediationToScheme = addRemediationTypes
)

func addRemediationTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(remediationGroupVersion,
		&RemediationJob{},
		&RemediationJobList{},
	)
	metav1.AddToGroupVersion(s, remediationGroupVersion)
	return nil
}

// NewScheme creates a fresh scheme with all v1alpha1 types registered.
func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := AddRemediationToScheme(s); err != nil {
		panic(fmt.Sprintf("failed to register scheme: %v", err))
	}
	return s
}

// Source and sink type constants.
const (
	// SourceTypeNative is the SourceType value set by native Kubernetes providers.
	// Defined here so all packages share one authoritative constant.
	SourceTypeNative = "native"
)

// RemediationJobPhase represents the lifecycle stage of a RemediationJob.
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
	PhaseSucceeded RemediationJobPhase = "Succeeded"

	// PhaseFailed means the batch/v1 Job failed (all retries exhausted or deadline exceeded).
	PhaseFailed RemediationJobPhase = "Failed"

	// PhaseCancelled means the RemediationJob was deleted before it could complete
	// because its source Result was deleted while the job was Pending or Running.
	PhaseCancelled RemediationJobPhase = "Cancelled"

	// PhasePermanentlyFailed means RetryCount has reached MaxRetries.
	// The RemediationJob will never be re-dispatched. The SourceProviderReconciler
	// treats this phase as a terminal tombstone and does not delete-and-recreate.
	PhasePermanentlyFailed RemediationJobPhase = "PermanentlyFailed"

	// PhaseSuppressed means the RemediationJob was grouped with a correlated finding
	// and will not be dispatched independently. A separate primary job covers the group.
	PhaseSuppressed RemediationJobPhase = "Suppressed"
)

// Standard condition type constants.
const (
	// ConditionJobDispatched is True once the batch/v1 Job has been created.
	ConditionJobDispatched = "JobDispatched"

	// ConditionJobComplete is True once the batch/v1 Job reached Succeeded state.
	ConditionJobComplete = "JobComplete"

	// ConditionJobFailed is True if the batch/v1 Job failed.
	ConditionJobFailed = "JobFailed"

	// ConditionPermanentlyFailed is True when RetryCount >= MaxRetries and the
	// RemediationJob has entered the PermanentlyFailed phase.
	ConditionPermanentlyFailed = "PermanentlyFailed"

	// ConditionCorrelationSuppressed is True when this job was suppressed because a
	// correlated primary job handles the investigation for this finding group.
	ConditionCorrelationSuppressed = "CorrelationSuppressed"
)

// RemediationJobSpec defines the desired state of a RemediationJob.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.fingerprint) || self.fingerprint == oldSelf.fingerprint",message="spec.fingerprint is immutable"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.sourceType) || self.sourceType == oldSelf.sourceType",message="spec.sourceType is immutable"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.sinkType) || self.sinkType == oldSelf.sinkType",message="spec.sinkType is immutable"
type RemediationJobSpec struct {
	// SourceResultRef identifies the source object that triggered this remediation.
	// +kubebuilder:validation:Required
	SourceResultRef ResultRef `json:"sourceResultRef"`

	// Fingerprint is the SHA256 hash used for deduplication.
	// Computed from namespace + kind + parentObject + sorted(error texts).
	// Immutable after creation.
	Fingerprint string `json:"fingerprint"`

	// SourceType identifies which SourceProvider created this RemediationJob.
	// Set to the value of SourceProvider.ProviderName() (e.g. "native", "prometheus").
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

	// MaxRetries is the maximum number of times the owned batch/v1 Job may fail
	// before this RemediationJob is permanently tombstoned.
	// Populated by SourceProviderReconciler from config.Config.MaxInvestigationRetries.
	// Zero means "use the operator default" (resolved at creation time — the field
	// will always be > 0 after creation).
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// Severity is the impact tier of the finding that triggered this job.
	// Values: critical, high, medium, low.
	// +optional
	Severity string `json:"severity,omitempty"`
}

// ResultRef is a back-reference to the source object that triggered a RemediationJob.
type ResultRef struct {
	// Name is the name of the source object.
	Name string `json:"name"`

	// Namespace is the namespace of the source object.
	Namespace string `json:"namespace"`
}

// FindingSpec holds the extracted finding context injected as env vars into the agent Job.
type FindingSpec struct {
	// Kind is the Kubernetes resource kind (e.g. "Pod", "Deployment").
	Kind string `json:"kind"`

	// Name is the plain resource name (no namespace prefix).
	Name string `json:"name"`

	// Namespace is the namespace of the affected resource.
	Namespace string `json:"namespace"`

	// ParentObject is the owning resource (e.g. the Deployment owning crashing pods).
	ParentObject string `json:"parentObject"`

	// Errors is the serialised []Failure with Sensitive fields redacted.
	// Stored as a JSON string.
	Errors string `json:"errors"`

	// Details is a human-readable explanation of the finding.
	Details string `json:"details"`

	// ChainDepth is the self-remediation cascade depth. Zero for normal findings.
	// Not part of the deduplication fingerprint.
	// +optional
	ChainDepth int32 `json:"chainDepth,omitempty"`
}

// SinkRef identifies the GitHub PR or issue opened by the agent.
// Set by the agent via a status patch after gh pr create succeeds.
// Used by the watcher to auto-close the sink when the finding resolves.
type SinkRef struct {
	// Type is "pr" or "issue".
	Type string `json:"type"`
	// URL is the full HTML URL (e.g. https://github.com/org/repo/pull/42).
	// Used in log messages and closure comments.
	URL string `json:"url"`
	// Number is the PR or issue number. Required for GitHub REST API calls.
	Number int `json:"number"`
	// Repo is "owner/repo" format (e.g. "lenaxia/talos-ops-prod").
	// Required for GitHub REST API calls.
	Repo string `json:"repo"`
}

// RemediationJobStatus defines the observed state of a RemediationJob.
type RemediationJobStatus struct {
	// Phase is the current lifecycle phase of this RemediationJob.
	// +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed;Cancelled;PermanentlyFailed;Suppressed
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

	// RetryCount is the number of times the owned batch/v1 Job has entered the
	// Failed state. Incremented by RemediationJobReconciler each time the job
	// transitions to PhaseFailed. Read by SourceProviderReconciler to decide
	// whether to re-dispatch or tombstone.
	RetryCount int32 `json:"retryCount,omitempty"`

	// Conditions follows the standard Kubernetes condition pattern.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CorrelationGroupID is set when this job is part of a correlated group.
	// Empty when not correlated.
	//
	// Design note: STORY_00 also specified RelatedFindings, CorrelationRole, and
	// CorrelationRule as spec/status fields. The implementation intentionally stores
	// these as labels (mechanic.io/correlation-group-id, mechanic.io/correlation-role)
	// and passes correlated findings as a runtime slice to dispatch() rather than
	// persisting them. The labels are searchable via kubectl and the recovery path
	// (controller.go) reconstructs AllFindings from suppressed peers on restart.
	// CorrelationGroupID here is the only status field needed for recovery.
	CorrelationGroupID string `json:"correlationGroupID,omitempty"`

	// SinkRef identifies the GitHub PR or issue opened by the agent.
	// Empty until the agent writes it after opening the sink.
	// +optional
	SinkRef SinkRef `json:"sinkRef,omitempty"`
}

// RemediationJob represents one investigation and remediation attempt for a
// finding.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName={rjob,rjobs,remjob,remjobs}
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.finding.kind`
// +kubebuilder:printcolumn:name="Parent",type=string,JSONPath=`.spec.finding.parentObject`
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.status.jobRef`
// +kubebuilder:printcolumn:name="PR",type=string,JSONPath=`.status.prRef`
// +kubebuilder:printcolumn:name="GroupID",type=string,JSONPath=`.status.correlationGroupID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type RemediationJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is required — omitempty is intentionally absent.
	Spec   RemediationJobSpec   `json:"spec"`
	Status RemediationJobStatus `json:"status,omitempty"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *RemediationJob) DeepCopyInto(out *RemediationJob) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	// Spec contains only value types; a shallow copy is sufficient.
	out.Spec = in.Spec
	out.Status.Phase = in.Status.Phase
	out.Status.JobRef = in.Status.JobRef
	out.Status.PRRef = in.Status.PRRef
	out.Status.Message = in.Status.Message
	out.Status.RetryCount = in.Status.RetryCount
	if in.Status.DispatchedAt != nil {
		t := *in.Status.DispatchedAt
		out.Status.DispatchedAt = &t
	}
	if in.Status.CompletedAt != nil {
		t := *in.Status.CompletedAt
		out.Status.CompletedAt = &t
	}
	if in.Status.Conditions != nil {
		conditions := make([]metav1.Condition, len(in.Status.Conditions))
		copy(conditions, in.Status.Conditions)
		out.Status.Conditions = conditions
	}
	out.Status.CorrelationGroupID = in.Status.CorrelationGroupID
	// SinkRef contains only value types (string, int); shallow copy is correct.
	out.Status.SinkRef = in.Status.SinkRef
}

// DeepCopyObject implements runtime.Object.
func (in *RemediationJob) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RemediationJob)
	in.DeepCopyInto(out)
	return out
}

// RemediationJobList contains a list of RemediationJob.
// +kubebuilder:object:root=true
type RemediationJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemediationJob `json:"items"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *RemediationJobList) DeepCopyInto(out *RemediationJobList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		items := make([]RemediationJob, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&items[i])
		}
		out.Items = items
	}
}

// DeepCopyObject implements runtime.Object.
func (in *RemediationJobList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RemediationJobList)
	in.DeepCopyInto(out)
	return out
}

// Ensure RemediationJob and RemediationJobList implement runtime.Object at compile time.
var _ runtime.Object = (*RemediationJob)(nil)
var _ runtime.Object = (*RemediationJobList)(nil)
