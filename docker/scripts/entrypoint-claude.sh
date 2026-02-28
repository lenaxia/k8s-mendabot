#!/usr/bin/env bash
# entrypoint-claude.sh — Claude Code agent runner entrypoint stub.
# Sources entrypoint-common.sh for shared setup.
#
# TODO: This is a validated stub. The exact claude CLI invocation has not been
# verified. Before enabling this entrypoint, verify:
#   1. The correct CLI command to run claude with a prompt file
#   2. How AGENT_PROVIDER_CONFIG maps to claude's settings mechanism
#   3. The expected exit codes for success/failure
#
# See docs/BACKLOG/epic08-pluggable-agent/README.md for context.
set -euo pipefail

# Validate the opaque provider config blob is present.
: "${AGENT_PROVIDER_CONFIG:?AGENT_PROVIDER_CONFIG must be set (provider-config key in llm-credentials-claude secret)}"

# Run common setup: kubeconfig, gh auth, prompt rendering.
# shellcheck source=entrypoint-common.sh
source /usr/local/bin/entrypoint-common.sh

# TODO: Map AGENT_PROVIDER_CONFIG to claude's settings mechanism.
# Until this is implemented, this entrypoint exits with a clear error.
# Determine dry-run mode using the same three-layer logic as the wrappers.
# Layer 1: sentinel file
_entrypoint_dry_run="false"
if [ -f /mechanic-cfg/dry-run ] && [ "$(cat /mechanic-cfg/dry-run 2>/dev/null)" = "true" ]; then
    _entrypoint_dry_run="true"
fi
# Layer 2: /proc/1/environ
if [ "$_entrypoint_dry_run" = "false" ] && [ -r /proc/1/environ ]; then
    if tr '\0' '\n' < /proc/1/environ 2>/dev/null | grep -q '^DRY_RUN=true$'; then
        _entrypoint_dry_run="true"
    fi
fi
# Layer 3: current shell env var
if [ "$_entrypoint_dry_run" = "false" ] && [ "${DRY_RUN:-false}" = "true" ]; then
    _entrypoint_dry_run="true"
fi

if [ "$_entrypoint_dry_run" = "true" ]; then
    # claude run "$(cat /tmp/rendered-prompt.txt)"   # TODO: verify invocation
    echo "ERROR: Claude Code entrypoint is not yet implemented." >&2
    exit 1
else
    echo "ERROR: Claude Code entrypoint is not yet implemented." >&2
    exit 1
    # exec claude run "$(cat /tmp/rendered-prompt.txt)"   # TODO: verify invocation
fi
