# API Reference

## Packages
- [remediation.mechanic.io/v1alpha1](#remediationmechaniciov1alpha1)


## remediation.mechanic.io/v1alpha1

Package v1alpha1 contains API types for the remediation.mechanic.io/v1alpha1 API group.


### Resource Types
- [RemediationJob](#remediationjob)
- [RemediationJobList](#remediationjoblist)



#### FindingSpec



FindingSpec holds the extracted finding context injected as env vars into the agent Job.



_Appears in:_
- [RemediationJobSpec](#remediationjobspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | Kind is the Kubernetes resource kind (e.g. "Pod", "Deployment"). |  |  |
| `name` _string_ | Name is the plain resource name (no namespace prefix). |  |  |
| `namespace` _string_ | Namespace is the namespace of the affected resource. |  |  |
| `parentObject` _string_ | ParentObject is the owning resource (e.g. the Deployment owning crashing pods). |  |  |
| `errors` _string_ | Errors is the serialised []Failure with Sensitive fields redacted.<br />Stored as a JSON string. |  |  |
| `details` _string_ | Details is a human-readable explanation of the finding. |  |  |
| `chainDepth` _integer_ | ChainDepth is the self-remediation cascade depth. Zero for normal findings.<br />Not part of the deduplication fingerprint. |  | Optional: \{\} <br /> |


#### RemediationJob



RemediationJob represents one investigation and remediation attempt for a
finding.



_Appears in:_
- [RemediationJobList](#remediationjoblist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `remediation.mechanic.io/v1alpha1` | | |
| `kind` _string_ | `RemediationJob` | | |
| `spec` _[RemediationJobSpec](#remediationjobspec)_ | Spec is required — omitempty is intentionally absent. |  |  |
| `status` _[RemediationJobStatus](#remediationjobstatus)_ |  |  |  |


#### RemediationJobList



RemediationJobList contains a list of RemediationJob.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `remediation.mechanic.io/v1alpha1` | | |
| `kind` _string_ | `RemediationJobList` | | |
| `items` _[RemediationJob](#remediationjob) array_ |  |  |  |


#### RemediationJobPhase

_Underlying type:_ _string_

RemediationJobPhase represents the lifecycle stage of a RemediationJob.



_Appears in:_
- [RemediationJobStatus](#remediationjobstatus)

| Field | Description |
| --- | --- |
| `Pending` | PhasePending means the RemediationJob has been created but no batch/v1 Job<br />exists yet (e.g. MAX_CONCURRENT_JOBS limit is currently reached).<br /> |
| `Dispatched` | PhaseDispatched means the batch/v1 Job has been created and is starting.<br /> |
| `Running` | PhaseRunning means the batch/v1 Job's pod is actively running.<br /> |
| `Succeeded` | PhaseSucceeded means the batch/v1 Job completed successfully.<br /> |
| `Failed` | PhaseFailed means the batch/v1 Job failed (all retries exhausted or deadline exceeded).<br /> |
| `Cancelled` | PhaseCancelled means the RemediationJob was deleted before it could complete<br />because its source Result was deleted while the job was Pending or Running.<br /> |
| `PermanentlyFailed` | PhasePermanentlyFailed means RetryCount has reached MaxRetries.<br />The RemediationJob will never be re-dispatched. The SourceProviderReconciler<br />treats this phase as a terminal tombstone and does not delete-and-recreate.<br /> |
| `Suppressed` | PhaseSuppressed means the RemediationJob was grouped with a correlated finding<br />and will not be dispatched independently. A separate primary job covers the group.<br /> |


#### RemediationJobSpec



RemediationJobSpec defines the desired state of a RemediationJob.



_Appears in:_
- [RemediationJob](#remediationjob)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceResultRef` _[ResultRef](#resultref)_ | SourceResultRef identifies the source object that triggered this remediation. |  | Required: \{\} <br /> |
| `fingerprint` _string_ | Fingerprint is the SHA256 hash used for deduplication.<br />Computed from namespace + kind + parentObject + sorted(error texts).<br />Immutable after creation. |  |  |
| `sourceType` _string_ | SourceType identifies which SourceProvider created this RemediationJob.<br />Set to the value of SourceProvider.ProviderName() (e.g. "native", "prometheus").<br />Immutable after creation. |  |  |
| `sinkType` _string_ | SinkType identifies which sink the agent should use for output.<br />Defaults to "github". Injected as SINK_TYPE env var into the agent Job.<br />Immutable after creation. |  |  |
| `finding` _[FindingSpec](#findingspec)_ | Finding contains the extracted finding context passed to the agent Job. |  |  |
| `gitOpsRepo` _string_ | GitOpsRepo is the GitHub repository in owner/repo format. |  |  |
| `gitOpsManifestRoot` _string_ | GitOpsManifestRoot is the path within the cloned repo to the manifests root. |  |  |
| `agentImage` _string_ | AgentImage is the full image reference for the agent container. |  |  |
| `agentSA` _string_ | AgentSA is the ServiceAccount name for the agent Job. |  |  |
| `maxRetries` _integer_ | MaxRetries is the maximum number of times the owned batch/v1 Job may fail<br />before this RemediationJob is permanently tombstoned.<br />Populated by SourceProviderReconciler from config.Config.MaxInvestigationRetries.<br />Zero means "use the operator default" (resolved at creation time — the field<br />will always be > 0 after creation). | 3 | Minimum: 1 <br /> |
| `severity` _string_ | Severity is the impact tier of the finding that triggered this job.<br />Values: critical, high, medium, low. |  | Optional: \{\} <br /> |


#### RemediationJobStatus



RemediationJobStatus defines the observed state of a RemediationJob.



_Appears in:_
- [RemediationJob](#remediationjob)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[RemediationJobPhase](#remediationjobphase)_ | Phase is the current lifecycle phase of this RemediationJob. |  | Enum: [Pending Dispatched Running Succeeded Failed Cancelled PermanentlyFailed Suppressed] <br /> |
| `jobRef` _string_ | JobRef is the name of the batch/v1 Job created for this remediation.<br />Set once the Job has been created. |  |  |
| `prRef` _string_ | PRRef is the GitHub PR URL opened or commented on by the agent.<br />Set by the agent via a status patch before it exits (best-effort). |  |  |
| `message` _string_ | Message is a human-readable description of the current state,<br />e.g. an error message if Phase is Failed. |  |  |
| `retryCount` _integer_ | RetryCount is the number of times the owned batch/v1 Job has entered the<br />Failed state. Incremented by RemediationJobReconciler each time the job<br />transitions to PhaseFailed. Read by SourceProviderReconciler to decide<br />whether to re-dispatch or tombstone. |  |  |
| `correlationGroupID` _string_ | CorrelationGroupID is set when this job is part of a correlated group.<br />Empty when not correlated.<br />Design note: STORY_00 also specified RelatedFindings, CorrelationRole, and<br />CorrelationRule as spec/status fields. The implementation intentionally stores<br />these as labels (mechanic.io/correlation-group-id, mechanic.io/correlation-role)<br />and passes correlated findings as a runtime slice to dispatch() rather than<br />persisting them. The labels are searchable via kubectl and the recovery path<br />(controller.go) reconstructs AllFindings from suppressed peers on restart.<br />CorrelationGroupID here is the only status field needed for recovery. |  |  |
| `sinkRef` _[SinkRef](#sinkref)_ | SinkRef identifies the GitHub PR or issue opened by the agent.<br />Empty until the agent writes it after opening the sink. |  | Optional: \{\} <br /> |


#### ResultRef



ResultRef is a back-reference to the source object that triggered a RemediationJob.



_Appears in:_
- [RemediationJobSpec](#remediationjobspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the source object. |  |  |
| `namespace` _string_ | Namespace is the namespace of the source object. |  |  |


#### SinkRef



SinkRef identifies the GitHub PR or issue opened by the agent.
Set by the agent via a status patch after gh pr create succeeds.
Used by the watcher to auto-close the sink when the finding resolves.



_Appears in:_
- [RemediationJobStatus](#remediationjobstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is "pr" or "issue". |  |  |
| `url` _string_ | URL is the full HTML URL (e.g. https://github.com/org/repo/pull/42).<br />Used in log messages and closure comments. |  |  |
| `number` _integer_ | Number is the PR or issue number. Required for GitHub REST API calls. |  |  |
| `repo` _string_ | Repo is "owner/repo" format (e.g. "lenaxia/talos-ops-prod").<br />Required for GitHub REST API calls. |  |  |


