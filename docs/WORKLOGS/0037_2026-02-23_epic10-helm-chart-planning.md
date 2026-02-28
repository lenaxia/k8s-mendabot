# Worklog: Epic 10 Helm Chart — Planning and Backlog Creation

**Date:** 2026-02-23
**Session:** Design decisions, backlog creation, skeptical validation of all stories
**Status:** Complete

---

## Objective

Design and document epic10-helm-chart: package mechanic as a Helm chart using fully
custom templates (no library dependencies), with a CRD upgrade hook, metrics support,
and accurate references to the actual Go codebase.

---

## Work Completed

### 1. Design decisions

| Decision | Rationale |
|---|---|
| Fully custom templates — dropped bjw-s common library | Chart is simple; library adds more friction than value for a single-Deployment operator chart |
| Secret names match existing hardcoded names: `github-app`, `llm-credentials` | `job.go` has these names hardcoded; changing them is a separate code change out of scope |
| `crds/` + pre-upgrade hook Job | Helm skips CRD upgrades from `crds/` on `helm upgrade`; hook compensates |
| `selfRemediation.enabled` gates env var emission (not a Go config boolean) | No `SELF_REMEDIATION_ENABLED` env var exists in Go — self-remediation is detected by `JobProvider` reading `app.kubernetes.io/managed-by` label |
| Image tags lockstepped to `Chart.appVersion` | Watcher and agent always released together |

### 2. Backlog created

Created `docs/BACKLOG/epic10-helm-chart/` with 14 files:
- `README.md` — epic overview, architecture, values schema, success criteria, story table, DoD
- 12 story files (`STORY_01` through `STORY_12`)

### 3. Supporting file updates

- `docs/BACKLOG/README.md` — epic10 row added; epic09 status corrected to Complete
- `README-LLM.md` — `feature/epic10-helm-chart` added to Active Branches table

### 4. Skeptical validation against actual code

Read the following source files before finalising the stories:
- `internal/jobbuilder/job.go` — Secret names, env var names, ConfigMap name
- `api/v1alpha1/remediationjob_types.go` — CRD spec fields
- `internal/config/config.go` — all env var names and defaults
- `internal/provider/provider.go` — RemediationJob creation path
- `docker/scripts/agent-entrypoint.sh` — which env vars are validated mandatory
- `deploy/kustomize/secret-*-placeholder.yaml` — actual Secret names
- All 4 RBAC source files — exact rule sets

Gaps found and fixed:

| Gap | Fix |
|-----|-----|
| Secret default names were `mechanic-github-app` / `mechanic-llm` — wrong | Corrected to `github-app` / `llm-credentials` (matches `job.go` and placeholder files) |
| LLM Secret key `kube-api-server` was missing entirely | Added to STORY_02 values schema, STORY_10 NOTES.txt example, STORY_12 Quick Start |
| `SELF_REMEDIATION_ENABLED` env var claimed to exist in STORY_06 — does not | Corrected: no such env var; self-remediation is controlled by `JobProvider` label detection; `selfRemediation.enabled` only gates whether the three config env vars are emitted |
| epic09 status in backlog README was still "Not Started" | Fixed to "Complete" |

---

## Key Decisions

- **No bjw-s library**: decided in this session after evaluating the complexity/benefit
  ratio for a single-Deployment chart. Fully custom templates only.
- **`kube-api-server` is a required Secret key**: the entrypoint validates
  `KUBE_API_SERVER` as mandatory. New operators must populate this key or agent Jobs
  will fail immediately at startup.

---

## Blockers

None. All gaps were corrected before closing the session.

---

## Tests Run

None — planning session, no code written.

---

## Next Steps

Implementation is ready to begin. Start with `feature/epic10-helm-chart` branch:

1. STORY_01: Create `charts/mechanic/Chart.yaml`, stub `values.yaml`, `templates/`,
   and copy `deploy/kustomize/crd-remediationjob.yaml` to `charts/mechanic/crds/`
2. STORY_02: Write full `values.yaml` and `templates/_helpers.tpl`
3. Continue in implementation order: STORY_03 → STORY_10 (mostly parallel)
4. STORY_11: CI workflow
5. STORY_12: README update

---

## Files Modified

- `docs/BACKLOG/epic10-helm-chart/README.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_01_chart_scaffold.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_02_values_schema.md` — created; fixed secret names + kube-api-server key
- `docs/BACKLOG/epic10-helm-chart/STORY_03_namespace.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_04_serviceaccounts.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_05_rbac.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_06_deployment.md` — created; fixed SELF_REMEDIATION_ENABLED error
- `docs/BACKLOG/epic10-helm-chart/STORY_07_prompt_configmap.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_08_crd_install_upgrade.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_09_metrics.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_10_notes_secrets.md` — created; fixed secret names + kube-api-server
- `docs/BACKLOG/epic10-helm-chart/STORY_11_ci_chart_test.md` — created
- `docs/BACKLOG/epic10-helm-chart/STORY_12_readme_helm.md` — created; fixed secret names
- `docs/BACKLOG/README.md` — epic10 row added; epic09 status corrected
- `README-LLM.md` — Active Branches table updated
