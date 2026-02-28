# Epic 08: Pluggable Agent Runner

## Purpose

The watcher previously hardcoded the OpenCode agent binary and its LLM credential shape
(three named Secret keys: `api-key`, `base-url`, `model`). Adding any other agent runner
(Claude Code, Kiro, etc.) required code changes to the job builder and the entrypoint
script, and exposed the operator to a never-ending maintenance burden as each new LLM
provider demanded its own credential shape.

This epic introduces two changes:

1. **`AGENT_TYPE` env var** — the watcher is told which agent runner to use at
   deployment time. Defaults to `opencode`, so existing deployments are unaffected.

2. **Opaque provider-config blob** — the Secret that reaches the agent Job contains a
   single pre-rendered, runner-specific config blob (`provider-config` key). Mechanic
   never interprets the blob contents — it passes them through unchanged. Adding a new
   LLM provider (Bedrock, Azure, Moonshot, DeepSeek, etc.) requires zero changes to
   the watcher; the operator updates their Secret.

The entrypoint is restructured into a dispatcher + per-agent scripts. Claude Code ships
as a validated stub (correct wiring, explicit TODO on the exact invocation) because the
CLI flags have not been verified.

## Status: Complete

## Dependencies

- epic02-jobbuilder complete (`internal/jobbuilder/job.go`)
- epic03-agent-image complete (`docker/scripts/`, `docker/Dockerfile.agent`)
- epic05-prompt complete (`charts/mechanic/files/prompts/`, `configmap-prompt.yaml`)
- epic10-helm-chart complete (Helm is the only delivery surface)

## Blocks

Nothing in the current roadmap depends on this epic.

## Breaking Changes

- Secret `llm-credentials` is renamed to `llm-credentials-<agentType>` (e.g.
  `llm-credentials-opencode`). Operators must recreate the secret with the new name
  and the new key structure (`provider-config` — one key only).
- Prompt ConfigMap `opencode-prompt` is replaced by `agent-prompt-core` and
  `agent-prompt-opencode`. The old name is gone.
- `prompt.name` / `prompt.override` Helm values are replaced by `prompt.coreOverride`
  and `prompt.agentOverride`.

Both breaking changes are documented in `NOTES.txt` with exact migration commands.

## Success Criteria

- [x] `AgentType` typed constant exists in `internal/config/config.go`; `FromEnv()`
      reads `AGENT_TYPE`, defaults to `"opencode"`, rejects unknown values at startup
- [x] `internal/jobbuilder/job.go` derives secret name from `AgentType`
      (`llm-credentials-<agentType>`), injects `AGENT_PROVIDER_CONFIG` env var
      sourced from the `provider-config` key, and names the prompt ConfigMap
      `agent-prompt-<agentType>`
- [x] `docker/scripts/agent-entrypoint.sh` is a pure dispatcher; all agent-specific
      logic is in per-agent scripts
- [x] `docker/scripts/entrypoint-common.sh` handles kubeconfig, gh auth, prompt
      concatenation, envsubst
- [x] `docker/scripts/entrypoint-opencode.sh` validates `AGENT_PROVIDER_CONFIG`,
      exports `OPENCODE_CONFIG_CONTENT`, propagates `KUBECONFIG` to subshells, execs
      `opencode run`
- [x] `docker/scripts/entrypoint-claude.sh` validates `AGENT_PROVIDER_CONFIG`, stubs
      the invocation with an explicit error and TODO comment
- [x] Helm chart renders two ConfigMaps: `agent-prompt-core` and
      `agent-prompt-<agentType>`; prompt file is concatenated at runtime by the
      entrypoint
- [x] `values.agentType` propagates to `AGENT_TYPE` on the watcher Deployment
- [x] `go test -timeout 30s -race ./...` passes
- [x] `helm lint charts/mechanic` passes
- [x] NOTES.txt documents the migration from `llm-credentials` to
      `llm-credentials-opencode`

## Stories (implemented)

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| AgentType config field | [STORY_01_agent_type_config.md](STORY_01_agent_type_config.md) | Complete | High | 1h |
| Agent-aware job builder | [STORY_02_agent_aware_jobbuilder.md](STORY_02_agent_aware_jobbuilder.md) | Complete | High | 2h |
| Entrypoint split | [STORY_03_entrypoint_split.md](STORY_03_entrypoint_split.md) | Complete | High | 2h |
| Helm chart update | [STORY_04_helm_chart_update.md](STORY_04_helm_chart_update.md) | Complete | High | 2h |

## Archived Stories (registrar architecture — not pursued)

The following stories describe a more complex "registrar" architecture with per-agent
Go sub-packages, a `JobBuilder` interface with `AgentType()`, a registry struct, and
per-agent Dockerfiles. This design was evaluated and rejected in favour of the simpler
parameterized jobbuilder above. The stories are archived here for reference.

The registrar pattern would add ~300 lines of Go to achieve the same runtime behaviour.
It pays off only when agent types need fundamentally different Job shapes. If that
requirement emerges, these stories can be revived at that time.

| Story | File | Status |
|-------|------|--------|
| AgentType field on CRD spec | [ARCHIVED_STORY_01_agent_type_field.md](ARCHIVED_STORY_01_agent_type_field.md) | Archived |
| JobBuilder registry | [ARCHIVED_STORY_02_registry.md](ARCHIVED_STORY_02_registry.md) | Archived |
| OpenCode builder sub-package | [ARCHIVED_STORY_03_opencode_builder.md](ARCHIVED_STORY_03_opencode_builder.md) | Archived |
| Claude builder sub-package | [ARCHIVED_STORY_04_claude_builder.md](ARCHIVED_STORY_04_claude_builder.md) | Archived |
| Reconciler wiring for registry | [ARCHIVED_STORY_05_reconciler_wiring.md](ARCHIVED_STORY_05_reconciler_wiring.md) | Archived |
| Per-agent Dockerfiles | [ARCHIVED_STORY_06_image_artifacts.md](ARCHIVED_STORY_06_image_artifacts.md) | Archived |
| Prompt ConfigMaps (registrar version) | [ARCHIVED_STORY_07_prompt_configmaps.md](ARCHIVED_STORY_07_prompt_configmaps.md) | Archived |
| Deploy (registrar version) | [ARCHIVED_STORY_08_deploy.md](ARCHIVED_STORY_08_deploy.md) | Archived |

## New Configuration Variables

```bash
# Agent runner to use. Default: opencode. Accepted: opencode, claude.
AGENT_TYPE=opencode
```

## Secret Structure (per agent type)

### llm-credentials-opencode

```
provider-config  — full OPENCODE_CONFIG_CONTENT JSON blob (model, providers, etc.)
```

### llm-credentials-claude (stub — not yet validated)

```
provider-config  — full claude settings JSON blob (structure TBD when claude entrypoint is implemented)
```

## Definition of Done

- [x] All unit tests pass: `go test -timeout 30s -race ./...`
- [x] `go build ./...` succeeds
- [x] `go vet ./...` clean
- [x] `helm lint charts/mechanic` passes
- [x] Worklog entry created in `docs/WORKLOGS/`
- [x] `docs/BACKLOG/README.md` epic table updated
