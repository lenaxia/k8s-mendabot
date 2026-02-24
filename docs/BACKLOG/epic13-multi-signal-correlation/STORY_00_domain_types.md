# Story 00: Correlation Domain Types and Rule Interface

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mendabot developer**, I want a well-defined `CorrelationRule` interface and
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

- [ ] `internal/domain/correlation.go` exists with:
  - `CorrelationRule` interface with method `Evaluate(ctx context.Context, candidate *v1alpha1.RemediationJob, peers []*v1alpha1.RemediationJob, c client.Client) (CorrelationResult, error)`
  - `CorrelationResult` struct: `Matched bool`, `GroupID string`, `PrimaryUID types.UID`, `Reason string`
  - `NewCorrelationGroupID() string` — generates a stable 12-char hex ID from a UUID
  - `CorrelationGroupIDLabel = "mendabot.io/correlation-group-id"` constant
  - `CorrelationGroupRoleLabel = "mendabot.io/correlation-role"` constant (values: `"primary"`, `"correlated"`)
- [ ] `api/v1alpha1/remediationjob_types.go` gains:
  - `PhaseSuppressed RemediationJobPhase = "Suppressed"` constant
  - `CorrelationGroupID string` field in `RemediationJobStatus`
  - The `+kubebuilder:validation:Enum` marker on `RemediationJobStatus.Phase` (line 162)
    must be updated to include `Suppressed`:
    `// +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed;Cancelled;Suppressed`
    Without this, the API server will reject any status patch that sets `Phase=Suppressed`.
  - `deploy/kustomize/crd-remediationjob.yaml` must be updated to reflect the new enum
    value in the `status.phase` validation schema (the `enum:` list under
    `spec.versions[0].schema.openAPIV3Schema.properties.status.properties.phase`)
- [ ] `internal/domain/correlation_test.go` tests `NewCorrelationGroupID()` for uniqueness
      and correct length (12 hex chars)
- [ ] `go test -timeout 30s -race ./internal/domain/...` passes

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

    "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

const (
    CorrelationGroupIDLabel   = "mendabot.io/correlation-group-id"
    CorrelationGroupRoleLabel = "mendabot.io/correlation-role"
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
func NewCorrelationGroupID() string {
    b := make([]byte, 6)
    if _, err := rand.Read(b); err != nil {
        panic("correlation: failed to read random bytes: " + err.Error())
    }
    return hex.EncodeToString(b)
}
```

### `api/v1alpha1/remediationjob_types.go` additions

```go
const (
    // existing phases ...
    PhaseSuppressed RemediationJobPhase = "Suppressed"
)

// In RemediationJobStatus:
// CorrelationGroupID is set when this job is part of a correlated group.
// Empty when not correlated.
CorrelationGroupID string `json:"correlationGroupID,omitempty"`
```

---

- [ ] Write `internal/domain/correlation_test.go` (TDD — must fail first)
- [ ] Write `internal/domain/correlation.go` (interface + types + ID generator)
- [ ] Add `PhaseSuppressed` and `CorrelationGroupID` to `api/v1alpha1/remediationjob_types.go`:
  - Add `PhaseSuppressed RemediationJobPhase = "Suppressed"` to the phase constants block (after `PhaseCancelled`, line 68)
  - Add `CorrelationGroupID string \`json:"correlationGroupID,omitempty"\`` to `RemediationJobStatus` (after the `Conditions` field, line 186)
  - Update the `+kubebuilder:validation:Enum` marker on `RemediationJobStatus.Phase` (line 162) to add `Suppressed`:
    `// +kubebuilder:validation:Enum=Pending;Dispatched;Running;Succeeded;Failed;Cancelled;Suppressed`
- [ ] Add `out.Status.CorrelationGroupID = in.Status.CorrelationGroupID` to the hand-written
      `DeepCopyInto` method in `remediationjob_types.go` (after line 220, where `out.Status.Message`
      is copied). There is no `zz_generated_deepcopy.go` — deep copy is manual in this file.
      Every new status field must be explicitly added or it will be silently dropped by
      `DeepCopyObject()`, corrupting status patches.
- [ ] Update `deploy/kustomize/crd-remediationjob.yaml`: add `"Suppressed"` to the `enum:` list
      under `spec.versions[0].schema.openAPIV3Schema.properties.status.properties.phase`
- [ ] Run `go test -timeout 30s -race ./internal/domain/... ./api/...` — must pass

---

## Dependencies

**Depends on:** epic09-native-provider (existing domain types in `internal/domain/`)
**Blocks:** STORY_01, STORY_02, STORY_03

---

## Definition of Done

- [ ] `CorrelationRule` interface and all supporting types exist and compile
- [ ] `PhaseSuppressed` is a valid `RemediationJobPhase` constant and accepted by the API server
      (kubebuilder enum marker updated + CRD YAML updated)
- [ ] `DeepCopyInto` explicitly copies `CorrelationGroupID`
- [ ] All tests pass
