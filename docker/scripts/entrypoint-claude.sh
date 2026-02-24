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
echo "ERROR: Claude Code entrypoint is not yet implemented." >&2
echo "The AGENT_PROVIDER_CONFIG and rendered prompt are available but" >&2
echo "the exact 'claude' CLI invocation has not been verified." >&2
echo "Set AGENT_TYPE=opencode to use the OpenCode runner instead." >&2
exit 1
