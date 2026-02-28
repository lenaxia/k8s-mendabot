# Story 11: CI — Helm Lint Workflow

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Medium
**Status:** Not Started
**Estimated Effort:** 20 minutes

---

## User Story

As a **maintainer**, I want a GitHub Actions workflow that runs `helm lint` on every
PR that touches the chart so regressions are caught before merge.

---

## Acceptance Criteria

- [ ] `.github/workflows/chart-test.yaml` exists
- [ ] Workflow triggers on `push` and `pull_request` when any file under
  `charts/**` changes
- [ ] Workflow uses `ubuntu-latest` runner
- [ ] Steps:
  1. `actions/checkout@v4`
  2. `azure/setup-helm@v4` (or equivalent) to install Helm
  3. `helm lint charts/mechanic/` with `--strict` flag
  4. `helm template charts/mechanic/ --set gitops.repo=org/repo --set gitops.manifestRoot=kubernetes`
     to verify rendering succeeds
- [ ] Workflow name: `Chart Lint`
- [ ] Job name: `lint`
- [ ] Workflow passes on main after this story is complete

---

## Tasks

- [ ] Write `.github/workflows/chart-test.yaml`
- [ ] Verify workflow syntax is valid YAML
- [ ] Push to branch and confirm CI runs and passes

---

## Notes

- `helm lint --strict` promotes warnings to errors. This is intentional — a chart
  with lint warnings is not acceptable.
- The `helm template` step catches render errors (e.g. required value missing,
  template syntax error) that `helm lint` does not always catch.
- The `--set gitops.repo=org/repo --set gitops.manifestRoot=kubernetes` values
  satisfy the two `required` guards in the Deployment template.
- No `chart-testing` (ct) tool is needed at this stage — `helm lint` + `helm template`
  provides sufficient coverage. `ct` can be added later if needed.

---

## Dependencies

**Depends on:** all template stories (STORY_03 through STORY_10) must be complete
  for lint to pass cleanly
**Blocks:** STORY_12

---

## Definition of Done

- [ ] CI workflow file exists at `.github/workflows/chart-test.yaml`
- [ ] Workflow runs on a PR touching `charts/` and passes
- [ ] `helm lint --strict charts/mechanic/` passes locally
