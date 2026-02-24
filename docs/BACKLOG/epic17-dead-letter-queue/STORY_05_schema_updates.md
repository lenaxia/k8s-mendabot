# Story 05: CRD Schema Updates — testdata and Helm Chart

**Epic:** [epic17-dead-letter-queue](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **mendabot operator**, I want the CRD YAML files used by `envtest` integration
tests and by the Helm chart to reflect the new `retryCount`, `maxRetries`, and
`PermanentlyFailed` fields so that the operator installs cleanly and integration tests
validate the correct schema.

---

## Background

There are two CRD YAML files that must be kept in sync with `remediationjob_types.go`:

1. **`testdata/crds/remediationjob_crd.yaml`** (root of the repo, at
   `testdata/crds/remediationjob_crd.yaml`) — loaded by `envtest` via the path
   `../../testdata/crds` configured in `internal/controller/suite_test.go:41`.
   This file has 98 lines today.

2. **`charts/mendabot/crds/remediationjob.yaml`** — the Helm chart CRD (105 lines
   today). This file is installed by `helm install` and is the production CRD.

Both files carry the same OpenAPI v3 schema for the `RemediationJob` CRD. There is also
a third file at `deploy/kustomize/crd-remediationjob.yaml` but that file references the
chart CRD or is a separate concern; check its contents before touching it.

The `kustomize` file is not checked here because it is typically a symlink or inclusion
of the chart CRD; see the Tasks section.

---

## Acceptance Criteria

- [x] `testdata/crds/remediationjob_crd.yaml`:
  - `status.phase` enum includes `PermanentlyFailed`
  - `status.retryCount` field added (`type: integer`, `format: int32`, `minimum: 0`)
  - `spec.maxRetries` field added (`type: integer`, `format: int32`, `minimum: 1`, `default: 3`)
- [x] `charts/mendabot/crds/remediationjob.yaml` has the same additions
- [x] `envtest` integration suite loads the updated CRD without error
- [x] `go test -timeout 60s -race ./internal/controller/...` passes (envtest)
- [x] `go test -timeout 60s -race ./internal/provider/...` passes (envtest)

---

## Technical Implementation

### `testdata/crds/remediationjob_crd.yaml`

#### Change 1 — Extend `status.phase` enum (line 87)

```yaml
# Before:
              phase:
                type: string
                enum: [Pending, Dispatched, Running, Succeeded, Failed, Cancelled]

# After:
              phase:
                type: string
                enum: [Pending, Dispatched, Running, Succeeded, Failed, Cancelled, PermanentlyFailed]
```

#### Change 2 — Add `retryCount` to `status.properties` (after `completedAt` at line 92)

```yaml
              retryCount:
                type: integer
                format: int32
                minimum: 0
                description: "Number of times the owned batch/v1 Job has entered the Failed state"
```

#### Change 3 — Add `maxRetries` to `spec.properties` (after `agentSA` at line 81)

```yaml
              maxRetries:
                type: integer
                format: int32
                minimum: 1
                default: 3
                description: "Maximum number of job failures before the RemediationJob is permanently tombstoned"
```

#### Complete updated `testdata/crds/remediationjob_crd.yaml` spec and status sections

```yaml
            properties:
              fingerprint:
                type: string
              sourceType:
                type: string
                description: "Which source provider created this object, e.g. k8sgpt"
              sinkType:
                type: string
                description: "Which sink the agent should use, e.g. github"
              sourceResultRef:
                type: object
                required: [name, namespace]
                properties:
                  name: {type: string}
                  namespace: {type: string}
              finding:
                type: object
                required: [kind, name, namespace, parentObject]
                properties:
                  kind: {type: string}
                  name: {type: string}
                  namespace: {type: string}
                  parentObject: {type: string}
                  errors: {type: string}
                  details: {type: string}
              gitOpsRepo: {type: string}
              gitOpsManifestRoot: {type: string}
              agentImage: {type: string}
              agentSA: {type: string}
              maxRetries:
                type: integer
                format: int32
                minimum: 1
                default: 3
                description: "Maximum number of job failures before the RemediationJob is permanently tombstoned"
          status:
            type: object
            properties:
              phase:
                type: string
                enum: [Pending, Dispatched, Running, Succeeded, Failed, Cancelled, PermanentlyFailed]
              jobRef: {type: string}
              prRef: {type: string}
              message: {type: string}
              dispatchedAt: {type: string, format: date-time}
              completedAt: {type: string, format: date-time}
              retryCount:
                type: integer
                format: int32
                minimum: 0
                description: "Number of times the owned batch/v1 Job has entered the Failed state"
              conditions:
                type: array
                items:
                  type: object
                  x-kubernetes-preserve-unknown-fields: true
```

### `charts/mendabot/crds/remediationjob.yaml`

Apply identical changes. The chart CRD currently has two extra fields compared to the
testdata CRD (`isSelfRemediation`, `chainDepth` in `spec`, and `correlationGroupID` and
`Suppressed` phase in `status`). Those fields must be preserved; only the new fields are
added.

#### Change 1 — Extend `status.phase` enum (line 93)

```yaml
# Before:
              phase:
                type: string
                enum: [Pending, Dispatched, Running, Succeeded, Failed, Cancelled, Suppressed]

# After:
              phase:
                type: string
                enum: [Pending, Dispatched, Running, Succeeded, Failed, Cancelled, Suppressed, PermanentlyFailed]
```

#### Change 2 — Add `retryCount` to `status.properties` (after `correlationGroupID` at line 99)

```yaml
              retryCount:
                type: integer
                format: int32
                minimum: 0
                description: "Number of times the owned batch/v1 Job has entered the Failed state"
```

#### Change 3 — Add `maxRetries` to `spec.properties` (after `chainDepth` at line 87)

```yaml
              maxRetries:
                type: integer
                format: int32
                minimum: 1
                default: 3
                description: "Maximum number of job failures before the RemediationJob is permanently tombstoned"
```

### `deploy/kustomize/crd-remediationjob.yaml`

Read this file before touching it:

```
deploy/kustomize/crd-remediationjob.yaml
```

If it is a standalone copy of the CRD (not a reference to the chart file), apply the
same changes as `testdata/crds/remediationjob_crd.yaml`. If it references or includes
the chart file via kustomize, no changes are needed.

---

## Validation

After making changes, run the envtest suite to confirm the CRD loads cleanly:

```bash
# Requires KUBEBUILDER_ASSETS to be set; skip if not available.
go test -timeout 60s -race ./internal/controller/... -v -run TestSuite_StartsAndStops
go test -timeout 60s -race ./internal/provider/... -v -run TestSuite_StartsAndStops
```

To validate the Helm chart CRD is well-formed YAML:

```bash
helm lint charts/mendabot
```

---

## Tasks

- [x] Read `deploy/kustomize/crd-remediationjob.yaml` — determine if it is a copy or reference
- [x] Update `testdata/crds/remediationjob_crd.yaml`:
  - Add `maxRetries` to spec properties
  - Add `retryCount` to status properties
  - Add `PermanentlyFailed` to phase enum
- [x] Update `charts/mendabot/crds/remediationjob.yaml`:
  - Add `maxRetries` to spec properties
  - Add `retryCount` to status properties
  - Add `PermanentlyFailed` to phase enum
- [x] Update `deploy/kustomize/crd-remediationjob.yaml` if it is a standalone copy
- [x] Run: `go test -timeout 60s -race ./internal/controller/...` — envtest must start
- [x] Run: `go test -timeout 60s -race ./internal/provider/...` — envtest must start
- [x] Run: `helm lint charts/mendabot` — no errors

---

## Dependencies

**Depends on:** STORY_01 (defines what fields exist), STORY_03 (the phase constant
`PermanentlyFailed` must exist before adding it to the enum is meaningful)
**Blocks:** Nothing (schema updates are additive and backward-compatible)

---

## Definition of Done

- [x] `testdata/crds/remediationjob_crd.yaml` phase enum includes `PermanentlyFailed`
- [x] `testdata/crds/remediationjob_crd.yaml` has `spec.maxRetries` field
- [x] `testdata/crds/remediationjob_crd.yaml` has `status.retryCount` field
- [x] `charts/mendabot/crds/remediationjob.yaml` has the same three additions
- [x] `helm lint charts/mendabot` clean
- [x] envtest suite loads the CRD and all integration tests pass
- [x] `go test -timeout 60s -race ./...` green
