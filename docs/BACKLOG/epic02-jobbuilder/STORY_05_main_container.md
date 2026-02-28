# Story: Main Container Spec

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want the main container spec to use the configured agent image,
mount all required volumes, and include the full environment variable set so OpenCode has
everything it needs to run.

---

## Acceptance Criteria

- [ ] Main container named `mechanic-agent`
- [ ] Uses `rjob.Spec.AgentImage`
- [ ] Mounts `shared-workspace` at `/workspace`
- [ ] Mounts `prompt-configmap` at `/prompt` read-only
- [ ] **Does NOT mount `github-app-secret`** — the private key must only be accessible to
  the init container. The main container reads the short-lived token from
  `/workspace/github-token` (written by the init container via the shared `emptyDir`).
  See JOBBUILDER_LLD.md §4 security note.
- [ ] All env vars from STORY_03 present
- [ ] No `command` override — entrypoint is set in the image itself
- [ ] Unit test verifies name, image, all mounts (shared-workspace + prompt-configmap only),
  and env var count

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Implement main container spec in `Build()`

---

## Dependencies

**Depends on:** STORY_03 (env vars), STORY_04 (init container — volume list must be consistent)
**Blocks:** STORY_06 (volumes)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
