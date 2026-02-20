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

- [ ] `internal/domain/types.go` defines `JobBuilderConfig`:
  ```go
  type JobBuilderConfig struct {
      GitOpsRepo         string
      GitOpsManifestRoot string
      AgentImage         string
      AgentNamespace     string
      AgentSA            string
  }
  ```
  The job builder must not depend on the full `config.Config` struct — it receives only
  the fields it needs via this type.

- [ ] `api/v1alpha1/remediationjob_types.go` defines all types from REMEDIATIONJOB_LLD.md §2:
  - `RemediationJobSpec`, `ResultRef`, `FindingSpec`
  - `SourceType` constant(s): `SourceTypeK8sGPT = "k8sgpt"`
  - `RemediationJobStatus`, `RemediationJobPhase` constants
  - Condition type constants (`ConditionJobDispatched`, `ConditionJobComplete`, `ConditionJobFailed`)
  - `RemediationJob` (with kubebuilder markers)
  - `RemediationJobList`
  - `DeepCopyObject()` and `DeepCopyInto()` for both `RemediationJob` and `RemediationJobList`
  - `AddToScheme` that registers both `Result`/`ResultList` (from STORY_04 of epic00) and
    `RemediationJob`/`RemediationJobList` under scheme group `remediation.mendabot.io/v1alpha1`

- [ ] `RemediationJobSpec.SourceType` is a required string field, immutable after creation;
  `K8sGPTSourceProvider` always sets it to `"k8sgpt"`

- [ ] Unit tests in `api/v1alpha1/remediationjob_types_test.go` verify:
  - `DeepCopyObject()` produces an independent copy (mutating copy does not affect original)
  - `DeepCopyInto()` performs true deep copy of slice fields (`Conditions`, `Spec.Finding`)
  - Phase constants have the expected string values
  - `SourceType` constants have the expected string values (`"k8sgpt"`)
  - Zero value `RemediationJobStatus` has empty Phase (not an invalid phase)

- [ ] No other package duplicates these types

---

## Why `FindingSpec` Stores Errors as a String

`FindingSpec.Errors` is a pre-serialised JSON string rather than `[]Failure`. This avoids
needing to define the full `Failure` schema inside the `RemediationJob` CRD and ensures
that what the agent receives via env var is exactly what is stored in the CRD — no
additional serialisation step at Job creation time.

Redaction of `Sensitive` fields happens in the `ResultReconciler` before the
`RemediationJob` is created.

---

## Tasks

- [ ] Create `api/v1alpha1/remediationjob_types_test.go` (TDD — tests first)
- [ ] Create `api/v1alpha1/remediationjob_types.go` with all types and deep copy
- [ ] Create `internal/domain/types.go` with `JobBuilderConfig`
- [ ] Create `internal/domain/types_test.go`
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
