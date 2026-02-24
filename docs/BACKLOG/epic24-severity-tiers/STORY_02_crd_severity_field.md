# Story 02: CRD — Add Severity Field to RemediationJobSpec

**Epic:** [epic24-severity-tiers](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **cluster operator**, I want `kubectl get rjob` to show the severity of each finding
so that I can triage open investigations without reading the full status.

---

## Background

`RemediationJobSpec` stores the snapshot of a finding at creation time. Adding `Severity`
here means the severity is durable — it survives watcher restarts and is readable via the
Kubernetes API without consulting logs.

`testdata/crds/remediationjob_crd.yaml` is the manually maintained CRD schema loaded by
envtest. It must be kept in sync (see Testing Requirements, Rule 1 in README-LLM.md).

---

## Design

### api/v1alpha1/remediationjob_types.go

Add `Severity string` to `RemediationJobSpec` **at the top level** — not inside the
`Finding FindingSpec` sub-struct. The finding data (Kind, Name, Namespace, ParentObject,
Errors, Details) all live inside `FindingSpec`; severity is a dispatch-level concern on
the outer spec.

```go
type RemediationJobSpec struct {
    // ... existing fields ...

    // Severity is the impact tier of the finding that triggered this job.
    // Values: critical, high, medium, low.
    // +optional
    Severity string `json:"severity,omitempty"`
}
```

### testdata/crds/remediationjob_crd.yaml

**Actual path:** `testdata/crds/remediationjob_crd.yaml` (repository root — not under
`internal/controller/`). This file is the manually maintained CRD schema loaded by
envtest.

Under `spec.versions[0].schema.openAPIV3Schema.properties.spec.properties`, add:

```yaml
severity:
  type: string
```

Do **not** add an `enum` constraint here. The `omitempty` tag means an absent field sends
no value (which passes any CRD validation), but an explicit `enum` would cause the
Kubernetes API server to reject any object with an explicitly empty string value — which
can occur in tests. The `enum` constraint is not needed for correctness and adds friction.
Validation of the severity value at write time is handled by `ParseSeverity` in the
reconciler before the object is created.

### charts/mendabot/crds/remediationjob.yaml

The Helm chart packages its own CRD at `charts/mendabot/crds/remediationjob.yaml`. This
file must receive the **same change** as `testdata/crds/remediationjob_crd.yaml`. Locate
the `spec.properties` block and add:

```yaml
severity:
  type: string
```

---

## Acceptance Criteria

- [ ] `RemediationJobSpec` has a `Severity string` field with `json:"severity,omitempty"`
- [ ] `testdata/crds/remediationjob_crd.yaml` (repo root) has `severity: {type: string}` under spec properties — no enum constraint
- [ ] `charts/mendabot/crds/remediationjob.yaml` has the same `severity: {type: string}` addition
- [ ] `DeepCopyInto` requires no change (all fields in `RemediationJobSpec` are value types; shallow struct copy is sufficient)
- [ ] Existing unit tests and envtest integration tests all pass without modification

---

## Tasks

- [ ] Add `Severity string` to `RemediationJobSpec` in `api/v1alpha1/remediationjob_types.go`
- [ ] Add `severity: {type: string}` to `testdata/crds/remediationjob_crd.yaml` under `spec.properties`
- [ ] Add `severity: {type: string}` to `charts/mendabot/crds/remediationjob.yaml` under `spec.properties`
- [ ] Run `go build ./...` — must be clean
- [ ] Run `go test -race -timeout 30s ./...` — all tests must pass

---

## Dependencies

**Depends on:** STORY_01 (severity type defined, though this story uses `string`)
**Blocks:** STORY_04 (reconciler reads severity from Finding to write to spec)

---

## Definition of Done

- [ ] `RemediationJobSpec.Severity` field present in Go types
- [ ] CRD schema updated in `testdata/crds/remediationjob_crd.yaml`
- [ ] Full test suite passes with `-race`
