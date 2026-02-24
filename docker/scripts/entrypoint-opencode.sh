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

# Run opencode with the rendered prompt. The prompt is passed as a single
# quoted string argument — word-splitting is not a concern because the shell
# expands "$(cat ...)" as one argument to `opencode run`.
exec opencode run "$(cat /tmp/rendered-prompt.txt)"
