# Story 00: Fix CRD Schema Drift

**Epic:** [epic14-test-infrastructure](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **mendabot developer**, I want `testdata/crds/remediationjob_crd.yaml` to
accurately reflect all fields in `RemediationJobStatus` and `RemediationJobSpec`, so
that integration tests using envtest observe the same field values that the code writes,
and new fields are not silently stripped by the API server's schema validator.

---

## Background

envtest starts a real Kubernetes API server backed by etcd. That API server enforces
the CRD schema declared in `testdata/crds/remediationjob_crd.yaml`. When a status patch
includes a field not present in the schema, the API server strips it silently — no error
is returned, the field simply does not appear in the stored object.

The status subresource has no `x-kubernetes-preserve-unknown-fields: true`, so there is
no fallback. The fake client used in unit tests does not enforce schema, which is why
unit tests for the same behaviour pass.

Two fields are currently missing from the testdata CRD spec:

| Field | Type | Location in Go types | Missing from |
|-------|------|---------------------|--------------|
| `isSelfRemediation` | boolean | `RemediationJobSpec` | `spec.properties` |
| `chainDepth` | integer | `RemediationJobSpec` | `spec.properties` |

These are not currently causing test failures because no integration tests exercise
the self-remediation path. However, they represent the same category of drift: a field
exists in the Go types but not in the testdata CRD, meaning future integration tests
for that feature will fail silently for a non-obvious reason.

Note: `correlationGroupID` in `RemediationJobStatus` is already present in the CRD
at line 91 (`correlationGroupID: {type: string}`). The integration test for that field
(`TestCorrelationIntegration_TC02b_SecondaryIsSuppressed`) has not been written yet —
it is part of epic13 (`multi-signal-correlation`, Not Started). When that test is
written as part of epic13, the CRD schema will already be correct.

The canonical source of truth for the CRD schema is the Go struct tags in
`api/v1alpha1/remediationjob_types.go`. The testdata YAML must match it exactly.

---

## Acceptance Criteria

- [ ] `testdata/crds/remediationjob_crd.yaml` `spec.properties` includes:
  - `isSelfRemediation: {type: boolean}`
  - `chainDepth: {type: integer}`
- [ ] `go test -count=1 -timeout 90s -race ./internal/controller/...` passes

---

## Technical Implementation

### File to change

**`testdata/crds/remediationjob_crd.yaml`**

The spec `properties` block currently ends at line 81 with `agentSA: {type: string}`.
Add the two missing fields after it:

```yaml
              isSelfRemediation: {type: boolean}
              chainDepth: {type: integer}
```

The full current `spec.properties` block (lines 53–81) ends as:

```yaml
              gitOpsRepo: {type: string}
              gitOpsManifestRoot: {type: string}
              agentImage: {type: string}
              agentSA: {type: string}
```

After the fix it must include:

```yaml
              gitOpsRepo: {type: string}
              gitOpsManifestRoot: {type: string}
              agentImage: {type: string}
              agentSA: {type: string}
              isSelfRemediation: {type: boolean}
              chainDepth: {type: integer}
```

The `status` block (lines 82–96) already contains all required fields including
`correlationGroupID: {type: string}` at line 91. **No changes needed in `status`.**

### Verification

```bash
go test -count=1 -timeout 90s -race ./internal/controller/...
```

---

## Implementation Steps

- [ ] Read `testdata/crds/remediationjob_crd.yaml` in full (confirm current state)
- [ ] Read `api/v1alpha1/remediationjob_types.go` (confirm all fields that need schema coverage)
- [ ] Add `isSelfRemediation: {type: boolean}` and `chainDepth: {type: integer}` to `spec.properties` after `agentSA`
- [ ] Run `go test -count=1 -timeout 90s -race ./internal/controller/...` — must pass

---

## Dependencies

**Depends on:** None (standalone YAML edit)
**Blocks:** STORY_02 (documentation)

---

## Definition of Done

- [ ] `testdata/crds/remediationjob_crd.yaml` updated with both missing spec fields
- [ ] Full controller test suite passes
