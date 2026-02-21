# Story: Init Container Spec

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want the init container spec to use the configured `AgentImage`,
run the GitHub App token exchange shell script, and clone the GitOps repo into the shared
workspace volume so the main container starts with a ready repo checkout.

---

## Acceptance Criteria

- [ ] Init container named `git-token-clone`
- [ ] Uses `rjob.Spec.AgentImage` (same image as main container — has bash, openssl, curl, jq, git)
- [ ] Shell script (from JOBBUILDER_LLD.md §5) injected as `command: ["/bin/bash", "-c"]` + `args`
- [ ] Mounts `shared-workspace` at `/workspace`
- [ ] Mounts `github-app-secret` at `/secrets/github-app` read-only
- [ ] Env vars: `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PRIVATE_KEY`
  from Secret, plus `GITOPS_REPO` as literal
- [ ] Unit test verifies container name, image, volume mounts, and env vars

---

## Tasks

- [ ] Write tests first (TDD)
- [ ] Implement init container spec in `Build()`

---

## Dependencies

**Depends on:** STORY_03 (env vars)
**Blocks:** STORY_06 (volumes)

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
