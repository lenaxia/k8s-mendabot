#!/usr/bin/env bash
# agent-entrypoint.sh — dispatcher: routes to the per-agent-runner entrypoint script
# based on the AGENT_TYPE environment variable set by the watcher job builder.
set -euo pipefail

case "${AGENT_TYPE:?AGENT_TYPE must be set by the job builder}" in
  opencode) exec /usr/local/bin/entrypoint-opencode.sh ;;
  claude)   exec /usr/local/bin/entrypoint-claude.sh ;;
  *)
    echo "ERROR: Unknown AGENT_TYPE: ${AGENT_TYPE}" >&2
    echo "Accepted values: opencode, claude" >&2
    exit 1
    ;;
esac
