# Story 00: Correlation Domain Types and Rule Interface

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mechanic developer**, I want a well-defined `CorrelationRule` interface and
supporting domain types, so that all built-in rules and any future rules share a
consistent contract and the correlator can apply them generically.

---

## Background

Correlation logic needs to operate on `RemediationJob` objects, not raw `Finding` values,
because `RemediationJob` objects are the durable state in the cluster and their labels and
metadata are what the correlator will read and write. The domain types established here
are the foundation for every subsequent story in this epic.

---

## Acceptance Criteria

- [x] `internal/domain/correlation.go` exists with:
  - `CorrelationRule` interface with method `Evaluate(ctx context.Context, candidate *v1alpha1.RemediationJob, peers []*v1alpha1.RemediationJob, c client.Client) (CorrelationResult, error)`
  - `CorrelationResult` struct: `Matched bool`, `GroupID string`, `PrimaryUID types.UID`, `Reason string`
  - `NewCorrelationGroupID() string` — generates a stable 12-char hex ID from 6 random bytes
  - `CorrelationGroupIDLabel = "mechanic.io/correlation-group-id"` constant
  - `CorrelationGroupRoleLabel = "mechanic.io/correlation-role"` constant (values: `"primary"`, `"correlated"`)
  - `CorrelationRolePrimary = "primary"` and `CorrelationRoleCorrelated = "correlated"` constants
- [x] `api/v1alpha1/remediationjob_types.go` gains:
  - `PhaseSuppressed RemediationJobPhase = "Suppressed"` constant
  - `CorrelationGroupID string` field in `RemediationJobStatus`
  - `+kubebuilder:validation:Enum` marker updated to include `Suppressed`
  - `ConditionCorrelationSuppressed = "CorrelationSuppressed"` constant
  - `testdata/crds/remediationjob_crd.yaml` updated with `Suppressed` enum and `correlationGroupID` field
- [x] `DeepCopyInto` explicitly copies `CorrelationGroupID`
- [x] `internal/domain/correlation_test.go` tests `NewCorrelationGroupID()` for uniqueness
      and correct length (12 hex chars)
- [x] `go test -timeout 30s -race ./internal/domain/... ./api/...` passes

---

## Technical Implementation

### `internal/domain/correlation.go`

```go
package domain

import (
    "context"
    "crypto/rand"
    "encoding/hex"

    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

const (
    CorrelationGroupIDLabel   = "mechanic.io/correlation-group-id"
    CorrelationGroupRoleLabel = "mechanic.io/correlation-role"
    CorrelationRolePrimary    = "primary"
    CorrelationRoleCorrelated = "correlated"
)

// CorrelationResult is returned by a CorrelationRule evaluation.
type CorrelationResult struct {
    Matched    bool
    GroupID    string
    PrimaryUID types.UID
    Reason     string
}

// CorrelationRule evaluates whether candidate and one or more peers should be
// grouped into a single investigation.
type CorrelationRule interface {
    // Name returns a stable identifier for the rule (used in log lines).
    Name() string
    // Evaluate returns a CorrelationResult. If Matched is false, the rule did
    // not find a correlation; the correlator tries the next rule.
    Evaluate(ctx context.Context, candidate *v1alpha1.RemediationJob, peers []*v1alpha1.RemediationJob, c client.Client) (CorrelationResult, error)
}

// NewCorrelationGroupID returns a 12-character lowercase hex string suitable
// for use as a correlation group identifier.
// Uses panic rather than (string, error) because crypto/rand.Read reads from
// /dev/urandom and is documented to never fail on Linux/macOS/Windows in
// practice. A failure here is an unrecoverable OS-level entropy source failure,
// not a domain error, and cannot be meaningfully handled by the caller.
func NewCorrelationGroupID() string {
    b := make([]byte, 6)
    if _, err := rand.Read(b); err != nil {
        // crypto/rand.Read only fails on catastrophic OS entropy failure.
        // Panic is intentional — this is not a recoverable domain error.
        panic("correlation: failed to read random bytes: " + err.Error())
    }
    return hex.EncodeToString(b)
}
```

### `api/v1alpha1/remediationjob_types.go` additions

Add `PhaseSuppressed` after `PhasePermanentlyFailed` (line 75):

```go
// PhaseSuppressed means the RemediationJob was correlated with another finding
// and will not be dispatched independently. The primary job in the group handles
// the investigation. Terminal — never re-dispatched.
PhaseSuppressed RemediationJobPhase = "Suppressed"
```

Add `ConditionCorrelationSuppressed` after `ConditionPermanentlyFailed` (line 91):

```go
// ConditionCorrelationSuppressed is True when this job was suppressed because
// a correlated primary job covers the investigation.
ConditionCorrelationSuppressed = "CorrelationSuppressed"
```

Add `CorrelationGroupID` after `Conditions` (line 217) in `RemediationJobStatus`:

```go
// CorrelationGroupID is set when this job is part of a correlated group.
// Empty when not correlated.
// +optional
CorrelationGroupID string `json:"correlationGroupID,omitempty"`
```

Update enum marker at line 187:

```go
// +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed;Cancelled;PermanentlyFailed;Suppressed
```

Add to `DeepCopyInto` (after `out.Status.RetryCount = in.Status.RetryCount` at line 252):

```go
out.Status.CorrelationGroupID = in.Status.CorrelationGroupID
```

---

## Tasks

- [x] Write `internal/domain/correlation_test.go` (TDD — must fail first)
- [x] Write `internal/domain/correlation.go` (interface + types + ID generator)
- [x] Add `PhaseSuppressed` constant after `PhasePermanentlyFailed`
- [x] Add `ConditionCorrelationSuppressed` constant after `ConditionPermanentlyFailed`
- [x] Update the `+kubebuilder:validation:Enum` marker to include `Suppressed`
- [x] Add `CorrelationGroupID string` field to `RemediationJobStatus`
- [x] Add `out.Status.CorrelationGroupID = in.Status.CorrelationGroupID` to `DeepCopyInto`
- [x] Update `testdata/crds/remediationjob_crd.yaml`
- [x] Run `go test -timeout 30s -race ./internal/domain/... ./api/...` — must pass

---

## Dependencies

**Depends on:** epic09-native-provider (existing domain types in `internal/domain/`)
**Blocks:** STORY_01, STORY_02, STORY_03

---

## Definition of Done

- [x] `CorrelationRule` interface and all supporting types exist and compile
- [x] `PhaseSuppressed` is a valid `RemediationJobPhase` constant accepted by the API server
      (enum marker updated + CRD YAML updated)
- [x] `ConditionCorrelationSuppressed` constant exists
- [x] `DeepCopyInto` explicitly copies `CorrelationGroupID`
- [x] `testdata/crds/remediationjob_crd.yaml` reflects the new enum value and field
- [x] All tests pass
