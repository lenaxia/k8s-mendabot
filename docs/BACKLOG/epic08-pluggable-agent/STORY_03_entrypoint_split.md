# Story 03: Entrypoint Split

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mechanic operator**, I want the agent container entrypoint to dispatch to an
agent-specific script based on `AGENT_TYPE`, so that the OpenCode-specific logic is
isolated and adding a new agent runner requires only a new script — not modifications
to the existing entrypoint.

---

## Background

`agent-entrypoint.sh` currently contains OpenCode-specific logic inline (building
`OPENCODE_CONFIG_CONTENT`, calling `opencode run`). This story extracts the
agent-agnostic work (kubeconfig, gh auth, prompt rendering) into a shared common
script and the OpenCode-specific work into its own script.

---

## Acceptance Criteria

- [ ] `docker/scripts/agent-entrypoint.sh` is a pure dispatcher:
  ```bash
  case "${AGENT_TYPE:-opencode}" in
    opencode) exec /usr/local/bin/entrypoint-opencode.sh ;;
    claude)   exec /usr/local/bin/entrypoint-claude.sh ;;
    *)        echo "ERROR: Unknown AGENT_TYPE: ${AGENT_TYPE}" >&2; exit 1 ;;
  esac
  ```
- [ ] `docker/scripts/entrypoint-common.sh` contains:
  - env var validation for all agent-agnostic vars
    (`FINDING_*`, `GITOPS_*`, `KUBE_API_SERVER`, `AGENT_MODEL`)
  - kubeconfig setup
  - gh auth login + validation
  - prompt concatenation (`core.txt` + agent supplement) + envsubst
    → `/tmp/rendered-prompt.txt`
  - exported as a sourced library (no `exec` at the end)
- [ ] `docker/scripts/entrypoint-opencode.sh`:
  - sources `entrypoint-common.sh`
  - validates `AGENT_PROVIDER_CONFIG` is set
  - `export OPENCODE_CONFIG_CONTENT="$AGENT_PROVIDER_CONFIG"`
  - `exec opencode run "$(cat /tmp/rendered-prompt.txt)"`
- [ ] `docker/scripts/entrypoint-claude.sh`:
  - sources `entrypoint-common.sh`
  - validates `AGENT_PROVIDER_CONFIG` is set
  - prints a clear error with `TODO` comment explaining stub status
  - exits 1
- [ ] Prompt concatenation in `entrypoint-common.sh` reads `/prompt/core.txt` then
      `/prompt/agent.txt` (the two files mounted from the two ConfigMaps by the job
      builder). If `/prompt/agent.txt` does not exist or is empty, only `core.txt` is
      used.
- [ ] `docker/Dockerfile.agent` copies all four scripts and marks them executable

---

## Technical Implementation

### entrypoint-common.sh (sourced by per-agent scripts)

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${FINDING_KIND:?FINDING_KIND must be set}"
: "${FINDING_NAME:?FINDING_NAME must be set}"
: "${FINDING_NAMESPACE:?FINDING_NAMESPACE must be set}"
: "${FINDING_FINGERPRINT:?FINDING_FINGERPRINT must be set}"
: "${FINDING_ERRORS:?FINDING_ERRORS must be set}"
: "${GITOPS_REPO:?GITOPS_REPO must be set}"
: "${GITOPS_MANIFEST_ROOT:?GITOPS_MANIFEST_ROOT must be set}"
: "${AGENT_MODEL:?AGENT_MODEL must be set}"
: "${KUBE_API_SERVER:?KUBE_API_SERVER must be set}"

FINDING_DETAILS="${FINDING_DETAILS:-}"
FINDING_PARENT="${FINDING_PARENT:-<none>}"
IS_SELF_REMEDIATION="${IS_SELF_REMEDIATION:-false}"
CHAIN_DEPTH="${CHAIN_DEPTH:-0}"
TARGET_REPO_OVERRIDE="${TARGET_REPO_OVERRIDE:-}"

# kubeconfig setup ...
# gh auth ...
# prompt concat + envsubst -> /tmp/rendered-prompt.txt
```

### Dockerfile.agent additions

```dockerfile
COPY docker/scripts/entrypoint-common.sh /usr/local/bin/entrypoint-common.sh
COPY docker/scripts/entrypoint-opencode.sh /usr/local/bin/entrypoint-opencode.sh
COPY docker/scripts/entrypoint-claude.sh /usr/local/bin/entrypoint-claude.sh
RUN chmod +x /usr/local/bin/entrypoint-common.sh \
             /usr/local/bin/entrypoint-opencode.sh \
             /usr/local/bin/entrypoint-claude.sh
```

---

## Dependencies

Depends on STORY_02 — the job builder must mount two ConfigMap keys (`core.txt` and
`agent.txt`) at `/prompt/` for the common script to concatenate them.

Note: The job builder mounts a **single ConfigMap** named `agent-prompt-<agentType>` at
`/prompt/`. The Helm chart (STORY_04) renders that ConfigMap with two keys: `core.txt`
and `agent.txt`. The entrypoint reads both files from the same mount point.

## Definition of Done

- [ ] Four scripts exist, all executable in the image
- [ ] Dispatcher correctly routes to per-agent scripts
- [ ] OpenCode entrypoint produces identical behaviour to the old `agent-entrypoint.sh`
- [ ] Claude stub exits 1 with a clear error message
