# Story: Environment Variable Injection

**Epic:** [epic02-jobbuilder](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want all FINDING_* and GITOPS_REPO environment variables injected
into the main container spec so the OpenCode agent has full finding context available at
runtime.

---

## Acceptance Criteria

- [ ] All variables from JOBBUILDER_LLD.md §4 are present in the main container env
- [ ] `FINDING_ERRORS` is read directly from `rjob.Spec.Finding.Errors` — verbatim,
  no additional serialisation. Redaction was done by `SourceProviderReconciler` before the
  `RemediationJob` was created.
- [ ] Secret-sourced vars (`OPENAI_*`) use `valueFrom.secretKeyRef`
- [ ] **`GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PRIVATE_KEY` are
  NOT injected into the main container.** These are init-container-only env vars
  (injected from Secret `github-app`). The main container reads only the short-lived
  installation token from `/workspace/github-token`. See JOBBUILDER_LLD.md §4 security note.
- [ ] Config-sourced vars (GITOPS_REPO, GITOPS_MANIFEST_ROOT) use literal `value`
  (sourced from `rjob.Spec.GitOpsRepo` and `rjob.Spec.GitOpsManifestRoot`)
- [ ] Unit tests verify all variables are present and correctly sourced

---

## Tasks

- [ ] Write tests first (TDD) — test helper to find env var by name in container spec
- [ ] Implement env var building in `Build()`

---

## Dependencies

**Depends on:** STORY_02 (job name)
**Blocks:** STORY_04 (init container), STORY_05 (main container)

---

## Definition of Done

- [ ] All env var tests pass with `-race`
- [ ] `go vet` clean
