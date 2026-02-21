# Story: Core Domain Types

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1.5 hours

---

## User Story

As a **developer**, I want all shared domain types defined in one place so that the
controller and job builder packages share a single, authoritative definition with no
duplication.

---

## Acceptance Criteria

- [ ] `api/v1alpha1/remediationjob_types.go` defines all types from REMEDIATIONJOB_LLD.md §2:
  - `RemediationJobSpec`, `ResultRef`, `FindingSpec`
  - `SourceType` constant(s): `SourceTypeK8sGPT = "k8sgpt"`
  - `RemediationJobStatus`, `RemediationJobPhase` constants
  - Condition type constants (`ConditionJobDispatched`, `ConditionJobComplete`, `ConditionJobFailed`)
  - `RemediationJob` (with kubebuilder markers)
  - `RemediationJobList`
  - `DeepCopyObject()` and `DeepCopyInto()` for both `RemediationJob` and `RemediationJobList`
  - One `AddToScheme` function for the `remediation.mendabot.io/v1alpha1` group:
    registers `RemediationJob` and `RemediationJobList`.
    This is called in `cmd/watcher/main.go` alongside the k8sgpt scheme registration.

- [ ] The `core.k8sgpt.ai/v1alpha1` scheme registration (Result + ResultList) is done
  by a separate `AddToScheme` function in `api/v1alpha1/result_types.go`
  (see epic00-foundation/STORY_04). Do NOT add a second Result scheme registration
  inside `remediationjob_types.go` — that would create duplicate registrations.

- [ ] `RemediationJobSpec.SourceType` is a required string field, immutable after creation;
  `K8sGPTProvider` always sets it to `"k8sgpt"`

- [ ] `internal/domain/provider.go` defines `SourceProvider`, `Finding`, and `SourceRef`
  as specified in PROVIDER_LLD.md §3. This file is owned by this story.

- [ ] Unit tests in `api/v1alpha1/remediationjob_types_test.go` verify:
  - `DeepCopyObject()` produces an independent copy (mutating copy does not affect original)
  - `DeepCopyInto()` performs true deep copy of slice fields (`Conditions`, `Spec.Finding`)
  - Phase constants have the expected string values
  - `SourceType` constants have the expected string values (`"k8sgpt"`)
  - Zero value `RemediationJobStatus` has empty Phase (not an invalid phase)

- [ ] No other package duplicates these types

- [ ] **Note:** There is no `domain.JobBuilderConfig` type. The `jobbuilder.Builder` reads
  all finding context (`AgentImage`, `AgentSA`, `GitOpsRepo`, `GitOpsManifestRoot`) directly
  from `rjob.Spec`. The only `jobbuilder.Config` field is `AgentNamespace` (see JOBBUILDER_LLD §3).
  Do not create a `JobBuilderConfig` in `internal/domain/`.

---

## Why `FindingSpec` Stores Errors as a String

`FindingSpec.Errors` is a pre-serialised JSON string rather than `[]Failure`. This avoids
needing to define the full `Failure` schema inside the `RemediationJob` CRD and ensures
that what the agent receives via env var is exactly what is stored in the CRD — no
additional serialisation step at Job creation time.

Redaction of `Sensitive` fields happens in `SourceProviderReconciler.Reconcile()` before the
`RemediationJob` is created.

---

## Tasks

- [ ] Create `api/v1alpha1/remediationjob_types_test.go` (TDD — tests first)
- [ ] Create `api/v1alpha1/remediationjob_types.go` with all types and deep copy
- [ ] Create `internal/domain/provider.go` with `SourceProvider`, `Finding`, `SourceRef`
- [ ] Create `internal/domain/provider_test.go`
- [ ] Verify `go build ./...` still clean

---

## Dependencies

**Depends on:** epic00-foundation/STORY_01 (module setup)
**Depends on:** epic00-foundation/STORY_04 (result_types.go — AddToScheme extends it)
**Blocks:** STORY_02 (Builder interface uses `*v1alpha1.RemediationJob`)
**Blocks:** STORY_03 (Reconciler skeleton uses both types)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] No duplicate type definitions elsewhere in the codebase
