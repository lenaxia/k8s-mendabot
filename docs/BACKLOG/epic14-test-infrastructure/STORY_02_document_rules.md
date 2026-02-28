# Story 02: Document Test Infrastructure Rules

**Epic:** [epic14-test-infrastructure](README.md)
**Priority:** Medium
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **mechanic developer** (or agent session), I want the test infrastructure rules
documented in `README-LLM.md`, so that future sessions adding new CRD fields or new
envtest tests do not re-introduce the same class of defects that this epic was created
to fix.

---

## Background

The two bugs fixed in STORY_00 and STORY_01 recurred specifically because there was no
written rule preventing them:

1. No rule saying "update `testdata/crds/` when adding fields to CRD types"
2. No rule saying "pre-delete deterministically named objects at the start of envtest tests"

Both rules need to live in `README-LLM.md` under **Testing Requirements** so they are
visible to every agent session that reads the file before starting work.

---

## Acceptance Criteria

- [ ] `README-LLM.md` **Testing Requirements** section contains a subsection titled
      `### envtest integration tests` with:
  - The CRD testdata maintenance rule (update `testdata/crds/` when adding CRD fields)
  - The pre-test cleanup rule (delete deterministically named objects before creating them)
- [ ] The documentation is specific enough that a developer seeing the codebase for the
      first time understands what to do without reading this epic

---

## Technical Implementation

### File to change

**`README-LLM.md`**

Add a new subsection at the end of the **Testing Requirements** section (currently ending
at line 948). Insert before the closing `---` of that section:

```markdown
### envtest integration tests

Integration tests in `internal/controller/` share a single envtest process. Two rules
apply to all tests in this package:

**Rule 1 — CRD testdata maintenance.** `testdata/crds/remediationjob_crd.yaml` is a
manually maintained copy of the CRD schema loaded by envtest. The Kubernetes API server
enforces this schema and silently strips unknown fields during status and object patches.
The fake client used in unit tests does NOT enforce schema, which means a missing field
will pass unit tests but fail integration tests.

When adding a field to `RemediationJobStatus` or `RemediationJobSpec` in
`api/v1alpha1/remediationjob_types.go`, you MUST also add the corresponding entry to
`testdata/crds/remediationjob_crd.yaml`:

- New `status` fields go under `spec.versions[0].schema.openAPIV3Schema.properties.status.properties`
- New `spec` fields go under `spec.versions[0].schema.openAPIV3Schema.properties.spec.properties`

Use the correct OpenAPI type: `{type: string}`, `{type: boolean}`, `{type: integer}`,
`{type: string, format: date-time}`.

Example: when `isSelfRemediation bool` was added to `RemediationJobSpec`, the
corresponding entry added to the CRD was:

```yaml
              isSelfRemediation: {type: boolean}
```

**Rule 2 — Pre-test cleanup for deterministic object names.** When a test creates a
Kubernetes object with a name derived from a fixed constant (e.g. a `batch/v1` Job
named `mechanic-agent-<fingerprint[:12]>`), add a pre-test delete at the start of the
test body, before creating any objects:

```go
// Pre-test cleanup: delete any stale object from a previous run.
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mechanic-agent-" + fp[:12],
    Namespace: integrationCtrlNamespace,
}})
```

Ignore the error (`_ =`): a not-found result is the normal case and must not fail the
test. Do not rely solely on `t.Cleanup` for this — `t.Cleanup` runs *after* the test
and cannot protect the next run if cleanup failed or the process was interrupted.
```

---

## Implementation Steps

- [ ] Read `README-LLM.md` lines 895–950 to find the exact insertion point in the
      **Testing Requirements** section
- [ ] Insert the `### envtest integration tests` subsection before the final `---` of
      that section
- [ ] Verify the markdown renders correctly (no broken headers, proper code blocks)

---

## Dependencies

**Depends on:** STORY_00 and STORY_01 complete (the rules document what those stories
established — documentation should reflect the final implemented state)

---

## Definition of Done

- [ ] `README-LLM.md` updated with the `### envtest integration tests` subsection
- [ ] Both rules clearly documented with code examples
- [ ] No existing content in `README-LLM.md` modified or removed
