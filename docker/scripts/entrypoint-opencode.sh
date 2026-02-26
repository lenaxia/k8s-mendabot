#!/usr/bin/env bash
# entrypoint-opencode.sh — OpenCode agent runner entrypoint.
# Sources entrypoint-common.sh for shared setup, then launches opencode.
set -euo pipefail

# Validate the opaque provider config blob is present.
: "${AGENT_PROVIDER_CONFIG:?AGENT_PROVIDER_CONFIG must be set (provider-config key in llm-credentials-opencode secret)}"

# Run common setup: kubeconfig, gh auth, prompt rendering.
# shellcheck source=entrypoint-common.sh
source /usr/local/bin/entrypoint-common.sh

# Propagate KUBECONFIG into shell init files so opencode's subshells inherit
# the context. Guard against duplicate lines (idempotent for container restarts).
grep -qxF 'export KUBECONFIG=/home/agent/.kube/config' /home/agent/.bashrc  2>/dev/null || echo "export KUBECONFIG=/home/agent/.kube/config" >> /home/agent/.bashrc
grep -qxF 'export KUBECONFIG=/home/agent/.kube/config' /home/agent/.profile 2>/dev/null || echo "export KUBECONFIG=/home/agent/.kube/config" >> /home/agent/.profile

# Pass the opaque provider config blob to opencode via its environment variable.
# The operator is responsible for the content — mendabot does not interpret it.
export OPENCODE_CONFIG_CONTENT="$AGENT_PROVIDER_CONFIG"

# Run opencode. In dry-run mode, do not use exec so the shell continues to
# emit_dry_run_report after opencode exits. In normal mode, exec replaces the
# shell (no overhead; correct exit code forwarding).
if [ "${DRY_RUN:-false}" = "true" ]; then
    opencode run "$(cat /tmp/rendered-prompt.txt)"
    emit_dry_run_report
else
    exec opencode run "$(cat /tmp/rendered-prompt.txt)"
fi
