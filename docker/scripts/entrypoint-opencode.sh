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

# Determine dry-run mode using the same three-layer logic as the wrappers.
# Checking only $DRY_RUN here (Layer 3) would create an inconsistency: a
# subprocess that runs "unset DRY_RUN" before the agent exits could cause
# the entrypoint to exec (replacing the shell) rather than fork, silently
# skipping emit_dry_run_report and losing the report permanently.
#
# Layer 1: sentinel file (tamper-proof read-only mount)
_entrypoint_dry_run="false"
if [ -f /mendabot-cfg/dry-run ] && [ "$(cat /mendabot-cfg/dry-run 2>/dev/null)" = "true" ]; then
    _entrypoint_dry_run="true"
fi
# Layer 2: /proc/1/environ (immutable container-init env)
if [ "$_entrypoint_dry_run" = "false" ] && [ -r /proc/1/environ ]; then
    if tr '\0' '\n' < /proc/1/environ 2>/dev/null | grep -q '^DRY_RUN=true$'; then
        _entrypoint_dry_run="true"
    fi
fi
# Layer 3: current shell env var (fallback / local testing)
if [ "$_entrypoint_dry_run" = "false" ] && [ "${DRY_RUN:-false}" = "true" ]; then
    _entrypoint_dry_run="true"
fi

# Run opencode. In dry-run mode, do not use exec so the shell continues to
# emit_dry_run_report after opencode exits. In normal mode, exec replaces the
# shell (no overhead; correct exit code forwarding).
if [ "$_entrypoint_dry_run" = "true" ]; then
    opencode run "$(cat /tmp/rendered-prompt.txt)" || true
    emit_dry_run_report
else
    exec opencode run "$(cat /tmp/rendered-prompt.txt)"
fi
