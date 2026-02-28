# Story 00: SinkRef Domain Type + SinkCloser Interface

## Status: Complete

## Goal

Define the `SinkRef` struct in `api/v1alpha1/remediationjob_types.go` and add it to
`RemediationJobStatus`. Define the `SinkCloser` interface in `internal/domain/sink.go`.
These are the shared types that all downstream stories depend on.

## Background

See [README.md](README.md) for the full design. The `SinkRef` struct carries the
information the watcher needs to close a GitHub PR or issue via the REST API: the URL
(for humans), the repo in `owner/repo` format, and the numeric PR/issue number (for
API calls). The `SinkCloser` interface decouples the reconciler from the GitHub
implementation and allows test doubles.

## Acceptance Criteria

- [x] `SinkRef` struct in `api/v1alpha1/remediationjob_types.go` with fields:
      `Type string`, `URL string`, `Number int`, `Repo string`
- [x] `RemediationJobStatus` has `SinkRef SinkRef \`json:"sinkRef,omitempty"\``
- [x] `RemediationJob.DeepCopyInto` copies `SinkRef` correctly
- [x] `SinkCloser` interface in `internal/domain/sink.go`
- [x] `testdata/crds/remediationjob_crd.yaml` updated with `sinkRef` under status
- [x] Unit tests in `internal/domain/sink_test.go` (interface satisfaction + nil cases)
- [x] `go test -timeout 30s -race ./...` passes
- [x] `go build ./...` succeeds

## Implementation Notes

### api/v1alpha1/remediationjob_types.go

Add the `SinkRef` struct before `RemediationJobStatus`:

```go
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
```

Add the field to `RemediationJobStatus`:

```go
// SinkRef identifies the GitHub PR or issue opened by the agent.
// Empty until the agent writes it after opening the sink.
// +optional
SinkRef SinkRef `json:"sinkRef,omitempty"`
```

Update `DeepCopyInto` to copy the `SinkRef` value field:

```go
out.Status.SinkRef = in.Status.SinkRef
```

`SinkRef` contains only value types (string, int) so a shallow copy is correct.

### internal/domain/sink.go

```go
package domain

import (
    "context"

    v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

// SinkCloser closes an open sink (PR or issue) when the underlying finding resolves.
// Implementations must be idempotent: closing an already-closed sink returns nil.
// Close returns nil immediately if rjob.Status.SinkRef.URL is empty.
type SinkCloser interface {
    Close(ctx context.Context, rjob *v1alpha1.RemediationJob, reason string) error
}

// NoopSinkCloser is a SinkCloser that does nothing.
// Used when PR_AUTO_CLOSE=false or in tests that do not need real closure.
type NoopSinkCloser struct{}

func (NoopSinkCloser) Close(_ context.Context, _ *v1alpha1.RemediationJob, _ string) error {
    return nil
}
```

### testdata/crds/remediationjob_crd.yaml

Under `spec.versions[0].schema.openAPIV3Schema.properties.status.properties`, add:

```yaml
sinkRef:
  type: object
  properties:
    type:   {type: string}
    url:    {type: string}
    number: {type: integer}
    repo:   {type: string}
```

### internal/domain/sink_test.go

Test scenarios (table-driven):

- `NoopSinkCloser.Close` always returns nil regardless of rjob contents
- `NoopSinkCloser.Close` returns nil when `SinkRef.URL` is empty
- `NoopSinkCloser.Close` returns nil when `SinkRef` is fully populated
- Interface satisfaction: `var _ SinkCloser = NoopSinkCloser{}`

## Files Touched

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add `SinkRef` struct + field + DeepCopyInto entry |
| `internal/domain/sink.go` | New file: `SinkCloser` interface + `NoopSinkCloser` |
| `internal/domain/sink_test.go` | New file: unit tests |
| `testdata/crds/remediationjob_crd.yaml` | Add `sinkRef` to status schema |

## TDD Sequence

1. Write `sink_test.go` — all tests fail (types don't exist yet)
2. Add `SinkRef` struct and `SinkRef` field to `remediationjob_types.go`
3. Add `SinkCloser` + `NoopSinkCloser` to `sink.go`
4. Update `DeepCopyInto`
5. Update `testdata/crds/remediationjob_crd.yaml`
6. All tests pass
