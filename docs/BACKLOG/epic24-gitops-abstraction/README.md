# Epic 24: GitOps Tooling Abstraction

**Feature Tracker:** FT-I8
**Area:** Integration

## Purpose

Remove the hard coupling between mechanic and Flux. The system was originally built
assuming Flux + GitHub as the only GitOps deployment pattern. This epic makes four
independent changes to support any GitOps tool (Flux, ArgoCD, plain Helm, etc.) and any
git host (GitHub, GitLab, Gitea, Bitbucket, self-hosted).

The changes are additive and fully backward-compatible. Existing deployments using
Flux + GitHub App require zero configuration changes — all new fields default to the
current behaviour.

## Status: Not Started

## Deep-Dive Findings (2026-02-24)

### Full coupling audit

There are four distinct layers of Flux coupling:

**Layer 1 — Init script (STORY_02)**
`internal/jobbuilder/job.go:54` (`initScript` constant):
```bash
git clone "https://x-access-token:${TOKEN}@github.com/${GITOPS_REPO}.git"
```
This hardcodes three assumptions simultaneously:
1. `github.com` as the git host (literal domain)
2. `x-access-token:` as the auth prefix (GitHub App token format)
3. GitHub App token exchange as the only auth method

A PAT or token from GitLab, Gitea, or Bitbucket cannot be used. SSH is not supported.

**Layer 2 — Prompt Step 5 (STORY_03)**
`charts/mechanic/files/prompts/core.txt:80–93` — the entire investigation step is
Flux-specific:
- `flux get all -n ${FINDING_NAMESPACE}` — fails with confusing errors on non-Flux clusters
- `kubectl get helmreleases` — Flux CRD, absent on non-Flux clusters
- `kubectl get kustomizations` — Flux CRD, absent on non-Flux clusters
- `flux logs --kind=HelmRelease` — meaningless without Flux

**Layer 3 — Agent image (STORY_04)**
`docker/Dockerfile.agent:65–72` — `flux` binary is unconditionally installed.
`docker/scripts/smoke-test.sh:47` — CI gate hard-asserts `flux` is present.
No ArgoCD CLI is present, making ArgoCD-specific diagnostics impossible.

**Layer 4 — Config / CRD / Helm chart (STORY_01)**
`internal/config/config.go` and `api/v1alpha1/remediationjob_types.go` and the Helm
chart have no `GITOPS_TOOL` or `GITOPS_GIT_HOST` concept at all. These must be added
as optional fields before the other stories can use them.

### What is NOT a problem

The following field and env-var names are already generic and require no renaming:
- `GitOpsRepo` / `GITOPS_REPO` — carries `owner/repo`, not a GitHub-specific format
- `GitOpsManifestRoot` / `GITOPS_MANIFEST_ROOT` — a path within a repo, tool-agnostic
- `SinkType` — controls PR creation; separate from repo cloning

### Backward compatibility contract

All new fields are optional with defaults that preserve current behaviour:
- `GITOPS_TOOL` defaults to `"flux"` — Flux-specific prompt step runs unchanged
- `GITOPS_GIT_HOST` defaults to `"github.com"` — existing GitHub URLs work unchanged
- When `GITOPS_GIT_TOKEN` is absent, GitHub App token exchange runs as today

## Dependencies

- epic03-agent-image complete (`docker/Dockerfile.agent`, init scripts)
- epic05-prompt complete (`charts/mechanic/files/prompts/core.txt`)
- epic10-helm-chart complete (`charts/mechanic/`)

## Blocks

- epic25 (GitLab sink support, when added) — would need `GITOPS_GIT_HOST` from STORY_02
- FT-I4 (ArgoCD sink support) — would use `GITOPS_TOOL=argocd` from STORY_01/STORY_03

## Stories

| Story | File | Status |
|-------|------|--------|
| Config and CRD — `GITOPS_TOOL` and `GITOPS_GIT_HOST` fields | [STORY_01_config_and_crd.md](STORY_01_config_and_crd.md) | Not Started |
| Init script — PAT / non-GitHub auth via `GITOPS_GIT_TOKEN` | [STORY_02_init_script_auth.md](STORY_02_init_script_auth.md) | Not Started |
| Prompt — `GITOPS_TOOL`-conditional investigation step | [STORY_03_prompt_gitops_tool.md](STORY_03_prompt_gitops_tool.md) | Not Started |
| Agent image — ArgoCD CLI and smoke-test update | [STORY_04_agent_image_argocd.md](STORY_04_agent_image_argocd.md) | Not Started |

## Implementation Order

```
STORY_01 (config + CRD) ──> STORY_02 (init script)
                         ──> STORY_03 (prompt)
                         ──> STORY_04 (image)  ← independent of STORY_01
```

STORY_02 and STORY_03 depend on STORY_01 because they inject the new env vars
that STORY_01 defines. STORY_04 is fully independent — it is a Dockerfile and
smoke-test change only.

## Definition of Done

- [ ] `GITOPS_TOOL` optional config field added; accepted values: `flux`, `argocd`, `helm-only`; default `flux`
- [ ] `GITOPS_GIT_HOST` optional config field added; default `github.com`
- [ ] `gitOpsTool` and `gitOpsGitHost` optional fields added to `RemediationJobSpec`
- [ ] Both CRD YAML schemas updated with the new optional fields
- [ ] Helm chart `values.yaml` updated with `gitops.tool` and `gitops.gitHost`
- [ ] `deployment-watcher.yaml` template injects `GITOPS_TOOL` and `GITOPS_GIT_HOST`
- [ ] `initScript` in `job.go` branches on `GITOPS_GIT_TOKEN`: if set, uses token directly; if absent, runs GitHub App exchange as today
- [ ] `GITOPS_GIT_HOST` replaces the hardcoded `github.com` literal in the clone URL
- [ ] Prompt Step 5 renamed to "Understand the GitOps tool state"
- [ ] Prompt Step 5 contains conditional blocks for `flux`, `argocd`, and `helm-only`
- [ ] `${GITOPS_TOOL}` added to the `envsubst` VARS list in `entrypoint-common.sh`
- [ ] `opencode.txt` preamble lists `flux` and `argocd` as conditionally available tools
- [ ] ArgoCD CLI installed and checksum-verified in `docker/Dockerfile.agent`
- [ ] `check_binary argocd` added to `docker/scripts/smoke-test.sh`
- [ ] All existing tests pass with `-race` (zero Go changes in STORY_02/03/04)
- [ ] Go tests pass for STORY_01 config changes
- [ ] Worklog written
