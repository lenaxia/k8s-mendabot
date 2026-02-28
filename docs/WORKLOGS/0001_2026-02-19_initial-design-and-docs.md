# Worklog: Initial Design and Documentation

**Date:** 2026-02-19
**Session:** Full design phase — HLD, LLDs, backlog epics and stories, repo scaffolding
**Status:** Complete

---

## Objective

Establish the complete design documentation for the `k8s-mechanic` project before
writing any implementation code. Produce a repo structure, architectural decisions, and a
prioritised backlog of user stories ready for TDD-driven implementation.

---

## Work Completed

### 1. Repository scaffolding

- `git init` with `main` as default branch
- `go.mod` initialised with module path `github.com/lenaxia/k8s-mechanic`
- `go.sum` generated (empty at this point — no imports yet)
- `README.md` written (user-facing project overview)
- `README-LLM.md` written (LLM implementation guide with hard rules, worklog format, dev
  workflow, common commands, branch management, testing requirements)

### 2. Design documents

- `docs/DESIGN/README.md` — index of design docs
- `docs/DESIGN/HLD.md` — authoritative high-level design covering: problem statement, goals,
  system overview, component design, data flow, deduplication strategy, RBAC design, GitHub
  App authentication, agent investigation strategy, security constraints, failure modes,
  configuration reference, deployment model, upstream contribution path, v1 scope, success
  criteria
- `docs/DESIGN/lld/CONTROLLER_LLD.md` — Result controller and reconcile loop design
- `docs/DESIGN/lld/JOBBUILDER_LLD.md` — Job spec construction and environment variable design
- `docs/DESIGN/lld/AGENT_IMAGE_LLD.md` — Docker image contents, tool versions, entrypoint
- `docs/DESIGN/lld/PROMPT_LLD.md` — OpenCode prompt design, PR deduplication check, reasoning
  format
- `docs/DESIGN/lld/DEPLOY_LLD.md` — Kustomize manifests, RBAC, ConfigMaps, Secrets layout

### 3. Backlog

- `docs/BACKLOG/README.md` — backlog structure and reading guide
- **epic00-foundation** (`STORY_01` – `STORY_04`): module setup, config struct, logging,
  CRD types
- **epic01-controller** (`STORY_01` – `STORY_07`): scheme registration, fingerprinting,
  dedup map, reconcile loop, event predicates, manager entrypoint, integration tests
- **epic02-jobbuilder** (`STORY_01` – `STORY_07`): builder struct, job name generation,
  environment variables, init container, main container, volumes, metadata labels
- **epic03-agent-image** (`STORY_01` – `STORY_10`): base image, k8s tools (kubectl, k8sgpt,
  helm, flux, talosctl), YAML tools (yq, kustomize, kubeconform), misc tools (jq, curl, wget,
  git, bat, ripgrep, difftastic), GitHub tools (gh, stern), opencode install, GitHub App token
  script, entrypoint script, multi-arch build, smoke test
- **epic04-deploy** README only (stories deferred — deploy stories to be written before
  epic04 begins)
- **epic05-prompt** README only
- **epic06-ci-cd** README only
- **epic07-technical-debt** README only
- `docs/WORKLOGS/README.md` — worklog index with rules and naming conventions

---

## Key Decisions

| Decision | Rationale |
|---|---|
| Standalone repo (not a fork of k8sgpt-operator) | Easier to iterate; may contribute upstream once stable |
| In-memory deduplication, not ConfigMap or Redis | Simplest correct solution; dedup-on-restart is safe because the agent checks for existing PRs before opening new ones |
| Owner-reference-aware fingerprint hashing | One pod crash from a bad Deployment should not spawn multiple investigations |
| Fingerprint = sha256(kind + parentObject + sorted(errors)) | Deterministic, stable under ordering changes, changes when the error set changes materially |
| GitHub App auth (not PAT) | Short-lived tokens, no long-lived secrets; installation token exchanged in init container |
| debian:bookworm-slim base image (not alpine) | Rich apt ecosystem; more stable binary compatibility for large tool set |
| Images pushed to ghcr.io | Free, integrated with GitHub Actions, no external registry dependency |
| Kustomize for deployment manifests | Matches talos-ops-prod GitOps pattern; Flux-compatible |
| Agent prompt must check for existing PRs first | Prevents duplicate PRs on watcher restart; agent comments on existing PR instead |
| TDD mandatory for all implementation | Hard rule in README-LLM.md; tests written before functional code |
| Docs-first approach | All design complete and reviewed before any implementation begins |

---

## Blockers

None.

---

## Tests Run

No tests run — this session was documentation-only. No implementation code exists yet.

---

## Next Steps

1. Write `docs/STATUS.md` — project status snapshot
2. Do a cross-reference review pass across HLD and all 5 LLDs to verify consistency
3. Begin **epic00-foundation**, starting with `STORY_01_module_setup.md`:
   - Add controller-runtime, zap, and k8sgpt-operator API dependencies to `go.mod`
   - Run `go mod tidy`
4. Follow TDD for every story: write the test first, confirm it fails, then implement

**First implementation task (next session):**
Add `controller-runtime v0.19.3`, `go.uber.org/zap`, and the k8sgpt-operator CRD types
to `go.mod`, then implement the `Config` struct in `internal/config/config.go` per
`docs/BACKLOG/epic00-foundation/STORY_02_config.md` — tests first.

---

## Files Modified

| File | Action |
|---|---|
| `README.md` | Created |
| `README-LLM.md` | Created |
| `go.mod` | Created |
| `go.sum` | Created |
| `docs/README.md` | Created |
| `docs/DESIGN/README.md` | Created |
| `docs/DESIGN/HLD.md` | Created |
| `docs/DESIGN/lld/CONTROLLER_LLD.md` | Created |
| `docs/DESIGN/lld/JOBBUILDER_LLD.md` | Created |
| `docs/DESIGN/lld/AGENT_IMAGE_LLD.md` | Created |
| `docs/DESIGN/lld/PROMPT_LLD.md` | Created |
| `docs/DESIGN/lld/DEPLOY_LLD.md` | Created |
| `docs/BACKLOG/README.md` | Created |
| `docs/BACKLOG/epic00-foundation/README.md` | Created |
| `docs/BACKLOG/epic00-foundation/STORY_01_module_setup.md` | Created |
| `docs/BACKLOG/epic00-foundation/STORY_02_config.md` | Created |
| `docs/BACKLOG/epic00-foundation/STORY_03_logging.md` | Created |
| `docs/BACKLOG/epic00-foundation/STORY_04_crd_types.md` | Created |
| `docs/BACKLOG/epic01-controller/README.md` | Created |
| `docs/BACKLOG/epic01-controller/STORY_01_scheme.md` | Created |
| `docs/BACKLOG/epic01-controller/STORY_02_fingerprint.md` | Created |
| `docs/BACKLOG/epic01-controller/STORY_03_dedup_map.md` | Created |
| `docs/BACKLOG/epic01-controller/STORY_04_reconcile.md` | Created |
| `docs/BACKLOG/epic01-controller/STORY_05_predicate.md` | Created |
| `docs/BACKLOG/epic01-controller/STORY_06_manager.md` | Created |
| `docs/BACKLOG/epic01-controller/STORY_07_integration_tests.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/README.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/STORY_01_builder_struct.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/STORY_02_job_name.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/STORY_03_env_vars.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/STORY_04_init_container.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/STORY_05_main_container.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/STORY_06_volumes.md` | Created |
| `docs/BACKLOG/epic02-jobbuilder/STORY_07_metadata.md` | Created |
| `docs/BACKLOG/epic03-agent-image/README.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_01_base_image.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_02_k8s_tools.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_03_yaml_tools.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_04_misc_tools.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_05_github_tools.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_06_opencode.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_07_token_script.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_08_entrypoint.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_09_multiarch.md` | Created |
| `docs/BACKLOG/epic03-agent-image/STORY_10_smoke_test.md` | Created |
| `docs/BACKLOG/epic04-deploy/README.md` | Created |
| `docs/BACKLOG/epic05-prompt/README.md` | Created |
| `docs/BACKLOG/epic06-ci-cd/README.md` | Created |
| `docs/BACKLOG/epic07-technical-debt/README.md` | Created |
| `docs/WORKLOGS/README.md` | Created |
