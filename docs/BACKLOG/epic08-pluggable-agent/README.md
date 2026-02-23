# Epic 08: Pluggable Agent

## Purpose

Replace the hardcoded opencode invocation with a pluggable `AgentProvider` abstraction,
so operators can substitute a different AI agent (e.g. Claude Code, Kiro, aider, or a
custom implementation) without forking the project.

## Status: Not Started

## Dependencies

- epic00 — Foundation complete
- epic00.1 — Interfaces complete
- epic02 — Job Builder complete
- epic03 — Agent Image complete
- epic05 — Prompt complete

## Blocks

- Nothing currently blocked; this is an independent improvement epic

---

## Problem Statement

Every coupling point to opencode is currently hardcoded:

| Location | Coupling |
|---|---|
| `docker/Dockerfile.agent` | `OPENCODE_VERSION`, download URL, `ENTRYPOINT` |
| `docker/scripts/agent-entrypoint.sh` | `OPENCODE_CONFIG_CONTENT` JSON schema, `opencode run` |
| `internal/jobbuilder/job.go` | `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_MODEL` env var names; ConfigMap name `opencode-prompt` |
| `deploy/kustomize/configmap-prompt.yaml` | ConfigMap named `opencode-prompt`; prompt body is opencode-specific |

Swapping to a different agent today requires modifying code in all four locations plus
rebuilding the image. There is no configuration surface that says "use agent X".

---

## Architecture

### Design principle

Follow the existing `SourceProvider` pattern. Just as `SourceProvider` isolates
"what is the finding source" from the reconciler, an `AgentProvider` isolates
"how to build the agent Job" from the `RemediationJobReconciler`.

The `JobBuilder` interface (`internal/domain/interfaces.go`) already provides the
right abstraction boundary. The work here is:
1. Making the **selection** of which `JobBuilder` implementation to use configurable
2. Extracting the opencode-specific logic into its own implementation
3. Building a parallel implementation for at least one other agent to validate the abstraction

### What changes and what stays the same

**Does not change:**
- `RemediationJob` CRD reconciliation logic
- `SourceProvider` abstraction and all provider implementations
- Deduplication fingerprinting
- GitHub App token exchange (init container pattern stays)
- Kustomize deploy manifests structure
- `RemediationJob` CRD schema — one new field added (`spec.agentType`), backwards-compatible

**Changes:**
- `internal/config/config.go` — add `AgentType` field
- `api/v1alpha1/remediationjob_types.go` — add `AgentType` to `RemediationJobSpec`
- `internal/domain/interfaces.go` — `JobBuilder` interface gains a companion `AgentType() string` method
- `internal/jobbuilder/` — current builder renamed to `opencode` sub-package; a new `claude` sub-package added
- `internal/jobbuilder/registry.go` — new: maps agent type string to `JobBuilder`
- `docker/Dockerfile.agent` — renamed `Dockerfile.agent.opencode`; a new `Dockerfile.agent.claude` added
- `docker/scripts/` — `agent-entrypoint.sh` becomes `agent-entrypoint-opencode.sh`; a new `agent-entrypoint-claude.sh` added
- `deploy/kustomize/configmap-prompt.yaml` — renamed `configmap-prompt-opencode.yaml`; a new `configmap-prompt-claude.yaml` added
- `deploy/kustomize/deployment-watcher.yaml` — `AGENT_TYPE` env var added

### Agent type identifier

The agent type is a lowercase string constant stored in:
- `config.AgentType` (env var `AGENT_TYPE`, default: `"opencode"`)
- `RemediationJobSpec.AgentType`
- `JobBuilder.AgentType()` return value (used for self-identification and registration)

Valid values at launch: `"opencode"`, `"claude"`.

### JobBuilder registry

A new `internal/jobbuilder/registry.go` provides:

```go
type Registry struct {
    builders map[string]domain.JobBuilder
}

func NewRegistry(builders ...domain.JobBuilder) (*Registry, error)
func (r *Registry) Get(agentType string) (domain.JobBuilder, error)
```

`NewRegistry` validates that no two builders share the same `AgentType()`. `Get` returns
a typed error for unknown agent types. The `RemediationJobReconciler` holds a `*Registry`
instead of a single `domain.JobBuilder`, and calls `registry.Get(rjob.Spec.AgentType)`
before calling `Build`.

### Per-agent image and entrypoint

Each agent has its own Docker image, entrypoint script, and prompt ConfigMap:

| Agent | Image tag suffix | Entrypoint script | Prompt ConfigMap |
|---|---|---|---|
| opencode | `-opencode` | `agent-entrypoint-opencode.sh` | `agent-prompt-opencode` |
| claude | `-claude` | `agent-entrypoint-claude.sh` | `agent-prompt-claude` |

The `AGENT_IMAGE` env var already flows through to `RemediationJob.Spec.AgentImage` and
then to the Job spec. Operators set this to the appropriate versioned image for their
chosen agent. The `JobBuilder` implementation for that agent selects the correct ConfigMap
name, env var names, and entrypoint config.

### LLM credentials abstraction

Different agents use different env var names and auth patterns:

| Agent | Credential env vars |
|---|---|
| opencode | `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_MODEL` |
| claude | `ANTHROPIC_API_KEY`, `CLAUDE_MODEL` |

Each `JobBuilder` implementation is responsible for mapping the credentials secret keys
to the env var names its agent expects. The Kubernetes secret schema stays stable
(keys: `api-key`, `base-url`, `model`) — the mapping is encapsulated in the builder.

---

## Success Criteria

- [ ] `AgentType` field present in `Config` and `RemediationJobSpec`; defaults to `"opencode"`
- [ ] `JobBuilder` interface has `AgentType() string` method; compile-time assertions present
- [ ] `Registry` maps agent type strings to `JobBuilder` implementations; unknown type returns typed error
- [ ] opencode builder extracted to `internal/jobbuilder/opencode/`; behaviour identical to current
- [ ] claude builder implemented in `internal/jobbuilder/claude/`; produces a valid Job spec
- [ ] `RemediationJobReconciler` uses `Registry.Get(rjob.Spec.AgentType)` — no hardcoded builder
- [ ] `Dockerfile.agent.opencode` and `Dockerfile.agent.claude` both build and pass smoke tests
- [ ] Entrypoint scripts renamed and agent-specific; no opencode references in the claude script
- [ ] Prompt ConfigMaps renamed with agent suffix; `kustomization.yaml` updated
- [ ] `AGENT_TYPE` env var wired into watcher deployment manifest
- [ ] All existing tests pass without modification
- [ ] New unit tests for `Registry` (happy path, unknown type, duplicate registration)
- [ ] New unit tests for `opencode` builder (identical coverage to current `jobbuilder` tests)
- [ ] New unit tests for `claude` builder (same test table as opencode)
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] `go build ./...` passes

---

## Stories

| Story | File | Status |
|-------|------|--------|
| AgentType in config and CRD types | [STORY_01_agent_type_field.md](STORY_01_agent_type_field.md) | Not Started |
| AgentType() on JobBuilder interface + registry | [STORY_02_registry.md](STORY_02_registry.md) | Not Started |
| Extract opencode builder to sub-package | [STORY_03_opencode_builder.md](STORY_03_opencode_builder.md) | Not Started |
| Implement claude builder | [STORY_04_claude_builder.md](STORY_04_claude_builder.md) | Not Started |
| Wire registry into RemediationJobReconciler | [STORY_05_reconciler_wiring.md](STORY_05_reconciler_wiring.md) | Not Started |
| Rename agent image and entrypoint artifacts | [STORY_06_image_artifacts.md](STORY_06_image_artifacts.md) | Not Started |
| Rename and add prompt ConfigMaps | [STORY_07_prompt_configmaps.md](STORY_07_prompt_configmaps.md) | Not Started |
| Deploy manifest updates | [STORY_08_deploy.md](STORY_08_deploy.md) | Not Started |

---

## Implementation Order

```
STORY_01 (config + CRD field)
    └── STORY_02 (interface + registry)
            ├── STORY_03 (opencode builder extracted)
            │       └── STORY_05 (reconciler wiring)
            └── STORY_04 (claude builder)
                    └── STORY_05 (reconciler wiring)

STORY_06 (image artifacts) — parallel with STORY_03/04
STORY_07 (prompt ConfigMaps) — parallel with STORY_03/04
STORY_08 (deploy) — after STORY_05 + STORY_07
```

---

## Definition of Done

- [ ] All stories complete
- [ ] All tests pass with race detector
- [ ] `go vet` clean
- [ ] Both Docker images build
- [ ] Worklog written
