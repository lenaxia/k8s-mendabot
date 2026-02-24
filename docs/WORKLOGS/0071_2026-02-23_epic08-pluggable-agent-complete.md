# Worklog: Epic 08 — Pluggable Agent Runner Swappability

**Date:** 2026-02-23
**Session:** Complete epic08 — pluggable agent runner implementation
**Status:** Complete

---

## Objective

Implement epic08: allow mendabot-watcher to swap which AI agent binary runs inside
Kubernetes Jobs, controlled by a single `AGENT_TYPE` env var. Zero maintenance burden
for new providers — the opaque config blob pattern means mendabot never interprets
LLM provider details.

---

## Design Decisions (locked)

- `AGENT_TYPE` env var on watcher deployment (default: `opencode`, fully backwards compatible)
- **Opaque config blob pattern**: Secret contains ONE key: `provider-config` (full
  agent-runner config JSON) — mendabot never interprets provider details, `model` key
  removed as unused dead weight
- Per-agent secrets: `llm-credentials-opencode`, `llm-credentials-claude`
  — **breaking change** from `llm-credentials`
- Env vars injected into Job: `AGENT_PROVIDER_CONFIG` (from `provider-config` key) only —
  `AGENT_MODEL` removed as it was never consumed by any entrypoint
- Compositional prompts: `agent-prompt-core` + `agent-prompt-<agentType>` — two
  ConfigMaps projected into `/prompt` as `core.txt` and `agent.txt`
- Claude entrypoint is a **validated stub** — exits with clear TODO error message
- `deploy/kustomize/` untouched — Helm chart only
- `github-app-secret` volume removed from Job pod spec — credentials are env-var
  injected (GITHUB_APP_ID etc.), never file-mounted; orphaned volume would cause
  `CreateContainerConfigError` if the secret was absent

---

## Work Completed

### Go: internal/config/config.go

- Added `AgentType` typed string type
- Added `AgentTypeOpenCode = "opencode"` and `AgentTypeClaude = "claude"` constants
- Added `AgentType AgentType` field on `Config` struct
- Parsing from `AGENT_TYPE` env var, defaults to `"opencode"`, validates at startup

### Go: internal/config/config_test.go

- Added table-driven tests: default value, valid values, invalid value

### Go: internal/jobbuilder/job.go

- Secret name: `"llm-credentials-" + agentType`
- Replaced `OPENAI_API_KEY`/`OPENAI_BASE_URL`/`OPENAI_MODEL` with `AGENT_PROVIDER_CONFIG`
  sourced from `provider-config` key in per-agent secret — `AGENT_MODEL` not injected
- Removed `github-app-secret` volume from pod spec (volume declared but never mounted
  after BUG-2 mount removal — orphaned volume removed to prevent `CreateContainerConfigError`)
- Core prompt CM: `"agent-prompt-core"` (shared)
- Agent prompt CM: `"agent-prompt-" + agentType`
- Volume changed from single ConfigMap to **projected volume** merging both CMs:
  - `agent-prompt-core` → `core.txt`
  - `agent-prompt-<agentType>` → `agent.txt`

### Go: internal/jobbuilder/job_test.go

- Updated all references from old env var names to `AGENT_PROVIDER_CONFIG`
  (removed `AGENT_MODEL` row from `TestBuild_SecretKeyRefs` — was stale after AGENT_MODEL removal)
- Updated all references from `"llm-credentials"` to `"llm-credentials-opencode"`
- Updated `TestBuild_SecretName_ByAgentType` to validate projected volume structure
  (two sources, correct CM names, correct key mappings)
- `TestBuild_AGENT_MODEL_NotInjected` asserts `AGENT_MODEL` must NOT be present
- `TestBuild_Volumes_AllPresent` asserts `github-app-secret` volume must NOT exist in pod spec

### Docker: docker/scripts/agent-entrypoint.sh

- Dispatcher: routes on `AGENT_TYPE` to per-agent entrypoint scripts

### Docker: docker/scripts/entrypoint-common.sh

- Validates agent-agnostic env vars
- Builds kubeconfig, authenticates gh
- Concatenates `/prompt/core.txt` + `/prompt/agent.txt` → `/tmp/rendered-prompt.txt`
  with envsubst restricted to known variable names

### Docker: docker/scripts/entrypoint-opencode.sh

- Validates `AGENT_PROVIDER_CONFIG`
- Exports `OPENCODE_CONFIG_CONTENT="$AGENT_PROVIDER_CONFIG"`
- Sources common, execs `opencode run "$(cat /tmp/rendered-prompt.txt)"`

### Docker: docker/scripts/entrypoint-claude.sh (stub)

- Validates `AGENT_PROVIDER_CONFIG`
- Sources common
- Exits with clear error: claude entrypoint not yet implemented

### Docker: docker/Dockerfile.agent

- COPY + chmod for all three entrypoint scripts

### Helm: charts/mendabot/values.yaml

- Added `agentType: opencode` field with full documentation
- Replaced `prompt.name`/`prompt.override` with `prompt.coreOverride`/`prompt.agentOverride`
- Updated secrets comment block to reflect new per-agent naming

### Helm: charts/mendabot/templates/deployment-watcher.yaml

- Added `AGENT_TYPE` env var injection from `.Values.agentType`

### Helm: charts/mendabot/templates/configmap-prompt.yaml

- Replaced single `opencode-prompt` CM with two CMs:
  - `agent-prompt-core` (key: `core.txt`) — from `files/prompts/core.txt`
  - `agent-prompt-<agentType>` (key: `agent.txt`) — from `files/prompts/<agentType>.txt`
- Supports `coreOverride` and `agentOverride` values

### Helm: charts/mendabot/files/prompts/

- `core.txt` — shared SRE investigation instructions (content from old `default.txt`)
- `opencode.txt` — opencode-specific preamble (OPENCODE_CONFIG_CONTENT, tool notes)
- `claude.txt` — stub with TODO note
- `default.txt` — **deleted** (breaking change)

### Helm: charts/mendabot/templates/NOTES.txt

- Updated with new secret names and key schema
- Added breaking change migration guide
- Documents opaque config blob pattern with example

---

## Breaking Changes

1. **Secret rename**: `llm-credentials` → `llm-credentials-opencode`
2. **Secret keys changed**: `api-key`/`base-url`/`model` → `provider-config` only
   (`model` key removed — model is specified inside the provider-config blob)
3. **Helm values changed**: `prompt.name`/`prompt.override` → `prompt.coreOverride`/`prompt.agentOverride`
4. **ConfigMap renamed**: `opencode-prompt` → `agent-prompt-core` + `agent-prompt-opencode`

Migration for existing operators: see NOTES.txt.

---

## Tests

All tests pass:
- `go test ./internal/config/...` — green
- `go test ./internal/jobbuilder/...` — green
- `go test ./...` — green (all packages)

---

## Gaps / Known Issues

- Claude entrypoint is a validated stub. The exact claude CLI invocation and
  authentication flow require research before the stub can be completed.
- `deploy/kustomize/` still references the old `opencode-prompt` ConfigMap name
  and old secret name — not updated per design (Helm only).

---

## Post-Implementation Review (same session)

A skeptical reviewer audit was run against all integration points. Findings and fixes:

### FAIL-1 (test breakage) — stale `AGENT_MODEL` row in `TestBuild_SecretKeyRefs`

`job_test.go` contained a row in `TestBuild_SecretKeyRefs` asserting `AGENT_MODEL` must
exist as a `SecretKeyRef`, directly contradicting `TestBuild_AGENT_MODEL_NotInjected` in
the same file. The stale row was a leftover from before AGENT_MODEL removal. Fixed by
deleting the contradictory row. All 12 packages now green.

### WARN-1 (orphaned volume) — `github-app-secret` volume in pod spec

BUG-2 removed the `github-app-secret` volume *mount* from the init container but left
the volume *declaration* in the pod spec. Kubernetes resolves Secret volumes at pod
scheduling time — if the `github-app` Secret was absent, every Job pod would fail with
`CreateContainerConfigError` even though the secret content is never accessed via the
volume. The volume declaration was removed from `job.go`. Test updated to assert the
volume does NOT exist (previously it asserted it DOES exist).

### INFO-1 (misleading comment) — `values.yaml` secrets comment

Comment still listed `model` as a required key in `llm-credentials-*` secrets. Updated
to show `provider-config` only.

### INFO-2 (stale reference) — `opencode.txt` prompt

The `opencode.txt` agent prompt mentioned `AGENT_MODEL is a human-readable label only` —
the variable no longer exists anywhere. Sentence removed.
