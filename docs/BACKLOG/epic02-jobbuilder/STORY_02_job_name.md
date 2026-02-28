# Story: Job Name Generation

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want the Job name to be deterministically derived from the first 12
characters of the fingerprint so that the same finding always produces the same Job name,
enabling safe `IsAlreadyExists` detection in the controller.

---

## Acceptance Criteria

- [ ] Job name format: `mechanic-agent-<first-12-chars-of-fingerprint>`
- [ ] Same fingerprint always produces the same name
- [ ] Different fingerprints produce different names
- [ ] Name is valid as a Kubernetes resource name (lowercase alphanumeric and hyphens only)

---

## Tasks

- [ ] Write tests (TDD)
- [ ] Implement name generation inside `Build()`

---

## Dependencies

**Depends on:** STORY_01 (builder struct)
**Blocks:** STORY_03 (env vars) — name must be set before testing full Job output

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
