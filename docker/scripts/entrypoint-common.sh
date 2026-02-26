#!/usr/bin/env bash
# entrypoint-common.sh — agent-agnostic setup: env validation, kubeconfig, gh auth,
# prompt concatenation. Sourced by per-agent entrypoint scripts.
set -euo pipefail

# Validate agent-agnostic required environment variables.
: "${FINDING_KIND:?FINDING_KIND must be set}"
: "${FINDING_NAME:?FINDING_NAME must be set}"
: "${FINDING_NAMESPACE:?FINDING_NAMESPACE must be set}"
: "${FINDING_FINGERPRINT:?FINDING_FINGERPRINT must be set}"
: "${FINDING_ERRORS:?FINDING_ERRORS must be set}"
: "${GITOPS_REPO:?GITOPS_REPO must be set}"
: "${GITOPS_MANIFEST_ROOT:?GITOPS_MANIFEST_ROOT must be set}"

# Optional variables — default to safe empty values.
FINDING_DETAILS="${FINDING_DETAILS:-}"
FINDING_PARENT="${FINDING_PARENT:-<none>}"
: "${FINDING_SEVERITY:-}"
# DRY_RUN is optional — defaults to "false"
DRY_RUN="${DRY_RUN:-false}"

# Build a kubeconfig from in-cluster credentials.
#
# Token selection strategy (tried in order):
#   1. /var/run/secrets/mendabot/serviceaccount/token — legacy SA token with no
#      audience claim, created by secret-agent-token.yaml. The Kubernetes API server
#      accepts tokens with no aud claim unconditionally (backwards compatibility
#      requirement). This works on every distribution including Talos where projected
#      tokens may fail audience validation due to issuer misconfiguration.
#   2. /var/run/secrets/kubernetes.io/serviceaccount/token — standard projected SA
#      token. Works on EKS, GKE, AKS, k3s, kind, kubeadm and any Talos cluster
#      that has correct service-account-issuer configuration.
#
# Server address: KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT are injected
# into every pod by Kubernetes on every distribution — no operator configuration
# required and no external secret needed.
LEGACY_TOKEN=/var/run/secrets/mendabot/serviceaccount/token
LEGACY_CA=/var/run/secrets/mendabot/serviceaccount/ca.crt
PROJECTED_TOKEN=/var/run/secrets/kubernetes.io/serviceaccount/token
PROJECTED_CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt

if [ -f "$LEGACY_TOKEN" ] && [ -f "$LEGACY_CA" ]; then
    SA_TOKEN_FILE="$LEGACY_TOKEN"
    SA_CA="$LEGACY_CA"
elif [ -f "$PROJECTED_TOKEN" ] && [ -f "$PROJECTED_CA" ]; then
    SA_TOKEN_FILE="$PROJECTED_TOKEN"
    SA_CA="$PROJECTED_CA"
else
    echo "ERROR: no usable ServiceAccount token+CA pair found." >&2
    echo "  Tried legacy:    $LEGACY_TOKEN + $LEGACY_CA" >&2
    echo "  Tried projected: $PROJECTED_TOKEN + $PROJECTED_CA" >&2
    exit 1
fi

: "${KUBERNETES_SERVICE_HOST:?KUBERNETES_SERVICE_HOST not set — is this pod running inside a Kubernetes cluster?}"
: "${KUBERNETES_SERVICE_PORT:?KUBERNETES_SERVICE_PORT not set — is this pod running inside a Kubernetes cluster?}"
KUBE_SERVER="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

mkdir -p /home/agent/.kube
kubectl config set-cluster in-cluster \
    --server="${KUBE_SERVER}" \
    --certificate-authority="${SA_CA}" \
    --embed-certs=true \
    --kubeconfig=/home/agent/.kube/config
kubectl config set-credentials in-cluster \
    --token="$(cat "${SA_TOKEN_FILE}")" \
    --kubeconfig=/home/agent/.kube/config
kubectl config set-context in-cluster \
    --cluster=in-cluster \
    --user=in-cluster \
    --kubeconfig=/home/agent/.kube/config
kubectl config use-context in-cluster \
    --kubeconfig=/home/agent/.kube/config

export KUBECONFIG=/home/agent/.kube/config

# Pre-flight: check that the GitHub App token has not expired (or is not about
# to expire within the next 60 seconds).  The expiry file is written by the init
# container via get-github-app-token.sh.  If the file is absent (e.g. an older
# init container image that pre-dates STORY_01), emit a warning and continue —
# the existing gh auth status check below still catches a truly bad token.
EXPIRY_FILE=/workspace/github-token-expiry
if [ -f "$EXPIRY_FILE" ]; then
    EXPIRY=$(cat "$EXPIRY_FILE")
    NOW=$(date +%s)
    if [ "$NOW" -ge "$((EXPIRY - 60))" ]; then
        echo "ERROR: GitHub App token is expired or expiring imminently." >&2
        echo "  EXPIRY=${EXPIRY}  NOW=${NOW}  (threshold: EXPIRY-60=$((EXPIRY - 60)))" >&2
        echo "  Re-queue the RemediationJob to obtain a fresh token." >&2
        exit 1
    fi
else
    echo "WARNING: /workspace/github-token-expiry not found — skipping expiry pre-flight check." >&2
fi

# Authenticate gh CLI using the token written by the init container.
# Validate that authentication succeeds — a bad token would otherwise only be
# discovered mid-investigation when gh pr list fails.
gh auth login --with-token < /workspace/github-token
if ! gh auth status > /dev/null 2>&1; then
    echo "ERROR: gh authentication failed — check /workspace/github-token" >&2
    exit 1
fi

# Concatenate prompts into a single rendered file.
# Order: agent preamble first, then core instructions.
# /prompt/agent.txt — agent-runner-specific preamble (tool availability, config notes)
# /prompt/core.txt  — shared investigation instructions (appended after preamble)
CORE_PROMPT=/prompt/core.txt
AGENT_PROMPT=/prompt/agent.txt

if [ ! -f "$CORE_PROMPT" ]; then
    echo "ERROR: core prompt file not found at $CORE_PROMPT" >&2
    exit 1
fi

COMBINED_PROMPT=$(cat "$CORE_PROMPT")
if [ -f "$AGENT_PROMPT" ] && [ -s "$AGENT_PROMPT" ]; then
    COMBINED_PROMPT="$(cat "$AGENT_PROMPT")

${COMBINED_PROMPT}"
fi

# Substitute environment variables into the combined prompt template.
# Restrict envsubst to known variable names to avoid corrupting content in
# FINDING_ERRORS or FINDING_DETAILS that may contain literal $ signs.
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${FINDING_SEVERITY}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}${DRY_RUN}'
printf '%s' "$COMBINED_PROMPT" | envsubst "$VARS" > /tmp/rendered-prompt.txt

# emit_dry_run_report — called by per-agent entrypoints after the agent binary
# returns in dry-run mode. Emits the sentinel and report content to stdout so
# the watcher can extract the report via the Kubernetes pod logs API.
emit_dry_run_report() {
    if [ "${DRY_RUN:-false}" = "true" ]; then
        echo "=== DRY_RUN INVESTIGATION REPORT ==="
        if [ -f /workspace/investigation-report.txt ]; then
            cat /workspace/investigation-report.txt
        else
            echo "(investigation-report.txt not found — agent may have exited without writing the report)"
        fi
    fi
}
