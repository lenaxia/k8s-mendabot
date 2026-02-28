# Epic: Native Cluster Provider

## Purpose

Replace the k8sgpt operator dependency with a native `SourceProvider` implementation
that watches core Kubernetes resources directly. mechanic will no longer require
k8sgpt-operator to be installed in the cluster.

## Status: Complete

## Dependencies

- epic00-foundation complete
- epic00.1-interfaces complete
- epic01-controller complete

## Blocks

- Nothing currently blocked; this is an independent improvement epic

## Background

mechanic currently depends on k8sgpt-operator writing `Result` CRDs, which
`K8sGPTProvider` then watches. After reading the k8sgpt source, the analyser logic is
simple rule-checking against standard Kubernetes API state — status field comparisons,
owner reference traversal, and event lookups. None of it requires an external operator.

The dependency costs more than it gives:
- k8sgpt-operator is a hard deployment prerequisite
- k8sgpt in the agent image adds a binary for a tool the agent largely supersedes
- The `details` field (k8sgpt's LLM explanation) is redundant once the agent runs its
  own investigation
- There is added latency from k8sgpt's polling interval before a finding triggers a Job

## Design

### Interface change: Fingerprint moves to domain

`SourceProvider.Fingerprint(f *Finding)` is currently on the interface, but it is not
provider-specific logic — it is pure domain logic over a `*Finding`. The k8sgpt provider
and any native provider would implement an identical algorithm. Keeping it on the interface
forces duplication and requires a `TestFingerprintEquivalence` test to detect divergence
between two copies of the same function.

The fix: promote `Fingerprint` to a package-level function in `internal/domain`:

```go
// internal/domain/provider.go
func FindingFingerprint(f *Finding) (string, error)
```

`SourceProvider` drops `Fingerprint` from its interface. `SourceProviderReconciler` calls
`domain.FindingFingerprint` directly. Every existing and future provider gets correct
fingerprinting for free. `TestFingerprintEquivalence` and `fingerprintFor` in
`internal/provider/k8sgpt/reconciler.go` are deleted.

### SourceProvider interface after this change

```go
type SourceProvider interface {
    ProviderName() string
    ObjectType() client.Object
    ExtractFinding(obj client.Object) (*Finding, error)
}
```

Three methods. Each has a single, clear responsibility. No algorithm duplication.

### Native provider structure

One provider struct per resource kind, all under `internal/provider/native/`. Each
implements `domain.SourceProvider`. They share a common `getParent` helper for
owner-reference traversal and all return `ProviderName() = "native"` so
`RemediationJob.spec.sourceType` reads consistently as `"native"` regardless of which
resource kind triggered the finding.

```
internal/provider/native/
├── parent.go        // getParent() — ownerReference traversal up to workload root
├── pod.go           // PodProvider — CrashLoopBackOff, ImagePullBackOff, OOMKilled, etc.
├── pod_test.go
├── deployment.go    // DeploymentProvider — replicas != readyReplicas
├── deployment_test.go
├── pvc.go           // PVCProvider — PVC stuck in Pending / ProvisioningFailed
├── pvc_test.go
├── node.go          // NodeProvider — NotReady and non-standard conditions
└── node_test.go
```

Each provider is registered as a separate `SourceProviderReconciler` in `main.go`,
one per `ObjectType()`. The existing registration loop in `main.go` handles this
without change — it already iterates over a `[]domain.SourceProvider` slice.

### Analyzers in scope for v1

The four core analyzers cover the large majority of actionable incidents:

| Provider | ObjectType | Failure conditions |
|---|---|---|
| `PodProvider` | `*v1.Pod` | `CrashLoopBackOff`, `ImagePullBackOff`, `OOMKilled`, `ErrImagePull`, unschedulable pending pods, non-zero exit codes |
| `DeploymentProvider` | `*appsv1.Deployment` | `spec.replicas != status.readyReplicas` (excluding scaling transient); `Available=False` condition |
| `PVCProvider` | `*v1.PersistentVolumeClaim` | `Phase == Pending` with `ProvisioningFailed` event |
| `NodeProvider` | `*v1.Node` | `NodeReady == False/Unknown`, non-standard conditions present |
| `StatefulSetProvider` | `*appsv1.StatefulSet` | `spec.replicas != status.readyReplicas` (excluding scaling transient); `Available=False` condition (Kubernetes 1.26+) |
| `JobProvider` | `*batchv1.Job` | `failed > 0 AND active == 0 AND completionTime == nil` (exhausted backoff limit); excludes CronJob-owned instances |

The k8sgpt `LogAnalyzer` (grep for "error" in pod logs) is deliberately excluded — its
noise-to-signal ratio is too high for automated remediation triggering. The stabilisation
window (STORY_12) is the primary mechanism for filtering transient failures across all
providers.

### getParent traversal

Walks `ownerReferences` up the chain to the workload root. Handles:
`ReplicaSet → Deployment`, `StatefulSet`, `DaemonSet`, `Job → CronJob`.
Returns `"Kind/name"` at the root. If no owner references exist, falls back to the
resource's own `Kind/name`.

`getParent` is a package-level function in `internal/provider/native/parent.go`. It
takes a `context.Context`, a `client.Client`, and a `metav1.ObjectMeta`. It is shared
by all native providers — no duplication.

### k8sgpt removal

Once all native providers pass integration tests and are wired into `main.go`:

1. `K8sGPTProvider` and `internal/provider/k8sgpt/` are deleted
2. `api/v1alpha1/result_types.go` is deleted
3. `k8sgpt` is removed from `docker/Dockerfile.agent` (it is currently called by the
   agent in the investigation prompt as step 5; the agent's own `kubectl` calls supersede it)
4. The investigation prompt is updated to remove the `k8sgpt analyze` step
5. `go.mod` k8sgpt dependency entries are removed via `go mod tidy`

The `sourceType` constant `"k8sgpt"` in `api/v1alpha1/remediationjob_types.go` is
replaced with `"native"`. Any existing `RemediationJob` objects in a live cluster with
`sourceType: k8sgpt` will continue to function — the `RemediationJobReconciler` does
not gate on `sourceType`.

## Success Criteria

- [ ] `domain.FindingFingerprint` is a standalone function in `internal/domain/provider.go`
- [ ] `SourceProvider` interface has three methods only: `ProviderName`, `ObjectType`, `ExtractFinding`
- [ ] `SourceProviderReconciler` calls `domain.FindingFingerprint` — no provider delegation for hashing
- [ ] `fingerprintFor` in `internal/provider/k8sgpt/reconciler.go` is deleted
- [ ] `TestFingerprintEquivalence` is deleted (the condition it tested no longer exists)
- [ ] `getParent` correctly traverses Pod→ReplicaSet→Deployment, Pod→StatefulSet, Pod→DaemonSet, Pod→Job→CronJob
- [ ] `PodProvider` detects all failure conditions listed in the analyzer table (no event fetching)
- [ ] `DeploymentProvider` detects replicas mismatch and `Available=False`; ignores scaling transients
- [ ] `PVCProvider` detects stuck Pending PVCs with ProvisioningFailed events
- [ ] `NodeProvider` detects NotReady nodes and non-standard conditions
- [ ] `StatefulSetProvider` detects replicas mismatch and `Available=False`; ignores scaling transients
- [ ] `JobProvider` detects exhausted Jobs (`failed > 0 AND active == 0 AND completionTime == nil`); excludes CronJob-owned instances
- [ ] All six providers return `ProviderName() == "native"`
- [ ] All six providers are registered in `main.go`
- [ ] `config.Config` has `StabilisationWindow time.Duration`; `STABILISATION_WINDOW_SECONDS` env var; default 120s
- [ ] `SourceProviderReconciler` implements the stabilisation window using an in-memory `firstSeen` map
- [ ] `internal/provider/k8sgpt/` package is deleted
- [ ] `api/v1alpha1/result_types.go` is deleted
- [ ] `api/v1alpha1/remediationjob_types.go` updated: `SourceTypeNative = "native"` replaces
  `SourceTypeK8sGPT`; `NewScheme()` no longer calls `AddResultToScheme`
- [ ] `internal/provider/provider_integration_test.go` contains the 6 reconciler integration
  scenarios previously in `k8sgpt/integration_test.go`
- [ ] k8sgpt removed from `docker/Dockerfile.agent`; prompt updated to remove `k8sgpt analyze` step
- [ ] `go mod tidy` removes k8sgpt dependency entries
- [ ] All unit tests pass with `-race`
- [ ] All envtest integration tests pass
- [ ] `go build ./...` and `go vet ./...` clean

## Stories

| Story | File | Status |
|-------|------|--------|
| Promote Fingerprint to domain function | [STORY_01_fingerprint_domain.md](STORY_01_fingerprint_domain.md) | Complete |
| Slim SourceProvider interface + update SourceProviderReconciler | [STORY_02_source_provider_interface.md](STORY_02_source_provider_interface.md) | Complete |
| getParent owner-reference traversal | [STORY_03_parent_traversal.md](STORY_03_parent_traversal.md) | Complete |
| PodProvider | [STORY_04_pod_provider.md](STORY_04_pod_provider.md) | Complete |
| DeploymentProvider | [STORY_05_deployment_provider.md](STORY_05_deployment_provider.md) | Complete |
| PVCProvider | [STORY_06_pvc_provider.md](STORY_06_pvc_provider.md) | Complete |
| NodeProvider | [STORY_07_node_provider.md](STORY_07_node_provider.md) | Complete |
| StatefulSetProvider | [STORY_10_statefulset_provider.md](STORY_10_statefulset_provider.md) | Complete |
| JobProvider | [STORY_11_job_provider.md](STORY_11_job_provider.md) | Complete |
| Wire native providers into main.go | [STORY_08_main_wiring.md](STORY_08_main_wiring.md) | Complete |
| Stabilisation window | [STORY_12_stabilisation_window.md](STORY_12_stabilisation_window.md) | Complete |
| Remove k8sgpt | [STORY_09_remove_k8sgpt.md](STORY_09_remove_k8sgpt.md) | Complete |

## Implementation Order

| Story | Depends on | Blocks |
|-------|-----------|--------|
| STORY_01 | (epic01 complete) | STORY_02 |
| STORY_02 | STORY_01 | STORY_03, STORY_12 |
| STORY_03 | STORY_02 | STORY_04, STORY_05, STORY_06, STORY_07, STORY_10, STORY_11 |
| STORY_04 | STORY_03 | STORY_08 |
| STORY_05 | STORY_03 | STORY_08 |
| STORY_06 | STORY_03 | STORY_08 |
| STORY_07 | STORY_03 | STORY_08 |
| STORY_10 | STORY_03 | STORY_08 |
| STORY_11 | STORY_03 | STORY_08 |
| STORY_08 | STORY_04, STORY_05, STORY_06, STORY_07, STORY_10, STORY_11 | STORY_09 |
| STORY_12 | STORY_01, STORY_02 | STORY_09 |
| STORY_09 | STORY_08, STORY_12 | — (final story) |

STORY_04 through STORY_07 and STORY_10–11 are independent of each other and can be worked
in parallel or any order once STORY_03 is complete.

STORY_12 depends only on STORY_01 and STORY_02 and is independent of STORY_03–11. It can
be worked in parallel with the provider implementations. Both STORY_08 and STORY_12 must
be complete before STORY_09.

## Definition of Done

- [x] All stories complete
- [x] All tests pass with race detector: `go test -timeout 120s -race ./...`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
- [x] k8sgpt-operator is no longer a deployment prerequisite
- [x] Worklog written