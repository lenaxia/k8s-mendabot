# Story 01: CRD Types — RetryCount, MaxRetries, PermanentlyFailed

**Epic:** [epic17-dead-letter-queue](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mechanic operator**, I want `RemediationJobStatus` to track how many times a
remediation attempt has been retried and `RemediationJobSpec` to declare a per-job
retry cap, so that the reconciler and source provider can cooperate to stop re-dispatching
jobs that have exhausted all retries.

---

## Background

All subsequent stories in this epic depend on these type definitions. The current
`remediationjob_types.go` has no retry-related fields. The phase constant
`PhaseFailed` is reused today for both "this attempt failed" and "permanently done
with failures" — two distinct states that must be distinguished so the
`SourceProviderReconciler` knows when to stop re-dispatching.

Exact locations in the file where additions are made:

- `RemediationJobPhase` constants block: `api/v1alpha1/remediationjob_types.go:49–69`
- `RemediationJobSpec` struct: `api/v1alpha1/remediationjob_types.go:84–118`
- `RemediationJobStatus` struct: `api/v1alpha1/remediationjob_types.go:152–179`
- `DeepCopyInto` method: `api/v1alpha1/remediationjob_types.go:203–226`
- `+kubebuilder:validation:Enum` marker: `api/v1alpha1/remediationjob_types.go:154`

---

## Acceptance Criteria

- [x] `PhasePermanentlyFailed RemediationJobPhase = "PermanentlyFailed"` constant added
- [x] `+kubebuilder:validation:Enum` marker on `Status.Phase` extended with `PermanentlyFailed`
- [x] `RemediationJobSpec.MaxRetries int32` field added with kubebuilder marker `// +kubebuilder:default=3`
- [x] `RemediationJobStatus.RetryCount int32` field added
- [x] `DeepCopyInto` copies both new fields correctly
- [x] `ConditionPermanentlyFailed` string constant added alongside existing condition constants
- [x] All tests pass: `go test -timeout 30s -race ./api/...`

---

## Technical Implementation

### `api/v1alpha1/remediationjob_types.go`

#### 1. New phase constant (after `PhaseCancelled` at line 68)

```go
// PhasePermanentlyFailed means RetryCount has reached MaxRetries.
// The RemediationJob will never be re-dispatched. The SourceProviderReconciler
// treats this phase as a terminal tombstone and does not delete-and-recreate.
PhasePermanentlyFailed RemediationJobPhase = "PermanentlyFailed"
```

#### 2. New condition constant (after `ConditionJobFailed` at line 80)

```go
// ConditionPermanentlyFailed is True when RetryCount >= MaxRetries and the
// RemediationJob has entered the PermanentlyFailed phase.
ConditionPermanentlyFailed = "PermanentlyFailed"
```

#### 3. `RemediationJobSpec` — new `MaxRetries` field (after `AgentSA` at line 117)

```go
// MaxRetries is the maximum number of times the owned batch/v1 Job may fail
// before this RemediationJob is permanently tombstoned.
// Populated by SourceProviderReconciler from config.Config.MaxInvestigationRetries.
// Zero means "use the operator default" (resolved at creation time — the field
// will always be > 0 after creation).
// +kubebuilder:default=3
// +kubebuilder:validation:Minimum=1
MaxRetries int32 `json:"maxRetries,omitempty"`
```

#### 4. `RemediationJobStatus` — new `RetryCount` field (after `Message` at line 173)

```go
// RetryCount is the number of times the owned batch/v1 Job has entered the
// Failed state. Incremented by RemediationJobReconciler each time the job
// transitions to PhaseFailed. Read by SourceProviderReconciler to decide
// whether to re-dispatch or tombstone.
RetryCount int32 `json:"retryCount,omitempty"`
```

#### 5. Updated `+kubebuilder:validation:Enum` marker (line 154)

```go
// Before:
// +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed;Cancelled

// After:
// +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed;Cancelled;PermanentlyFailed
```

#### 6. `DeepCopyInto` — add `RetryCount` copy (after `Status.Message` copy at line 212)

The existing `DeepCopyInto` copies individual status fields by assignment. Add:

```go
out.Status.RetryCount = in.Status.RetryCount
```

`MaxRetries` in Spec is a value type (`int32`) already covered by `out.Spec = in.Spec`
at line 208 — no change needed there.

### Complete diff summary

```go
// api/v1alpha1/remediationjob_types.go

// Phase constants block — add after PhaseCancelled:
PhasePermanentlyFailed RemediationJobPhase = "PermanentlyFailed"

// Condition constants block — add after ConditionJobFailed:
ConditionPermanentlyFailed = "PermanentlyFailed"

// RemediationJobSpec — add after AgentSA field:
// +kubebuilder:default=3
// +kubebuilder:validation:Minimum=1
MaxRetries int32 `json:"maxRetries,omitempty"`

// RemediationJobStatus — update enum marker:
// +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed;Cancelled;PermanentlyFailed

// RemediationJobStatus — add after Message field:
RetryCount int32 `json:"retryCount,omitempty"`

// DeepCopyInto — add after Status.Message copy:
out.Status.RetryCount = in.Status.RetryCount
```

---

## Test Cases

File: `api/v1alpha1/remediationjob_types_test.go` (new file — the package currently
has no test file; the tests compile under `package v1alpha1_test`).

```go
package v1alpha1_test

import (
    "testing"

    v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

// TestPhasePermanentlyFailed_ConstantValue pins the string value so a rename
// does not silently break existing persisted objects in etcd.
func TestPhasePermanentlyFailed_ConstantValue(t *testing.T) {
    if string(v1alpha1.PhasePermanentlyFailed) != "PermanentlyFailed" {
        t.Errorf("PhasePermanentlyFailed = %q, want %q",
            v1alpha1.PhasePermanentlyFailed, "PermanentlyFailed")
    }
}

// TestConditionPermanentlyFailed_ConstantValue pins the condition type string.
func TestConditionPermanentlyFailed_ConstantValue(t *testing.T) {
    if v1alpha1.ConditionPermanentlyFailed != "PermanentlyFailed" {
        t.Errorf("ConditionPermanentlyFailed = %q, want %q",
            v1alpha1.ConditionPermanentlyFailed, "PermanentlyFailed")
    }
}

// TestDeepCopyInto_CopiesRetryCount verifies RetryCount is preserved by DeepCopyInto.
func TestDeepCopyInto_CopiesRetryCount(t *testing.T) {
    tests := []struct {
        name       string
        retryCount int32
    }{
        {"zero", 0},
        {"one", 1},
        {"at-max", 3},
        {"over-max", 99},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            src := &v1alpha1.RemediationJob{}
            src.Status.RetryCount = tt.retryCount
            dst := &v1alpha1.RemediationJob{}
            src.DeepCopyInto(dst)
            if dst.Status.RetryCount != tt.retryCount {
                t.Errorf("DeepCopyInto: RetryCount = %d, want %d",
                    dst.Status.RetryCount, tt.retryCount)
            }
        })
    }
}

// TestDeepCopyInto_CopiesMaxRetries verifies MaxRetries is preserved (via Spec copy).
func TestDeepCopyInto_CopiesMaxRetries(t *testing.T) {
    tests := []struct {
        name       string
        maxRetries int32
    }{
        {"default", 3},
        {"one", 1},
        {"ten", 10},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            src := &v1alpha1.RemediationJob{}
            src.Spec.MaxRetries = tt.maxRetries
            dst := &v1alpha1.RemediationJob{}
            src.DeepCopyInto(dst)
            if dst.Spec.MaxRetries != tt.maxRetries {
                t.Errorf("DeepCopyInto: MaxRetries = %d, want %d",
                    dst.Spec.MaxRetries, tt.maxRetries)
            }
        })
    }
}

// TestRemediationJobSpec_MaxRetriesField verifies the field is accessible and zero-valued
// on a fresh struct (default applied by kubebuilder marker at creation time, not in Go).
func TestRemediationJobSpec_MaxRetriesField(t *testing.T) {
    spec := v1alpha1.RemediationJobSpec{}
    if spec.MaxRetries != 0 {
        t.Errorf("zero-value MaxRetries = %d, want 0 (kubebuilder default applies at API server admission, not in Go)", spec.MaxRetries)
    }
}

// TestRemediationJobStatus_RetryCountField verifies RetryCount is zero-valued on fresh struct.
func TestRemediationJobStatus_RetryCountField(t *testing.T) {
    status := v1alpha1.RemediationJobStatus{}
    if status.RetryCount != 0 {
        t.Errorf("zero-value RetryCount = %d, want 0", status.RetryCount)
    }
}
```

---

## Tasks

- [x] Write tests in `api/v1alpha1/remediationjob_types_test.go` (TDD — run first, must fail to compile)
- [x] Add `PhasePermanentlyFailed` constant after `PhaseCancelled`
- [x] Add `ConditionPermanentlyFailed` constant after `ConditionJobFailed`
- [x] Add `MaxRetries int32` field to `RemediationJobSpec` with kubebuilder markers
- [x] Update `+kubebuilder:validation:Enum` marker to include `PermanentlyFailed`
- [x] Add `RetryCount int32` field to `RemediationJobStatus`
- [x] Add `out.Status.RetryCount = in.Status.RetryCount` to `DeepCopyInto`
- [x] Run: `go test -timeout 30s -race ./api/...` — must pass
- [x] Run: `go build ./...` — must compile clean

---

## Dependencies

**Depends on:** Nothing (this is the foundation story)
**Blocks:** STORY_02, STORY_03, STORY_04, STORY_05

---

## Definition of Done

- [x] `PhasePermanentlyFailed` constant exists with value `"PermanentlyFailed"`
- [x] `ConditionPermanentlyFailed` constant exists with value `"PermanentlyFailed"`
- [x] `RemediationJobSpec.MaxRetries int32` field present with `json:"maxRetries,omitempty"`
- [x] `RemediationJobStatus.RetryCount int32` field present with `json:"retryCount,omitempty"`
- [x] `+kubebuilder:validation:Enum` on `Status.Phase` includes `PermanentlyFailed`
- [x] `DeepCopyInto` copies `RetryCount`
- [x] `go test -timeout 30s -race ./api/...` green
- [x] `go vet ./api/...` clean
