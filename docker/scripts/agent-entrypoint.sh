#!/usr/bin/env bash
set -euo pipefail

# Verify all required environment variables are present before doing any work.
# This produces a clear error message rather than silently rendering blank fields.
: "${FINDING_KIND:?FINDING_KIND must be set}"
: "${FINDING_NAME:?FINDING_NAME must be set}"
: "${FINDING_NAMESPACE:?FINDING_NAMESPACE must be set}"
: "${FINDING_FINGERPRINT:?FINDING_FINGERPRINT must be set}"
: "${FINDING_ERRORS:?FINDING_ERRORS must be set}"
: "${GITOPS_REPO:?GITOPS_REPO must be set}"
: "${GITOPS_MANIFEST_ROOT:?GITOPS_MANIFEST_ROOT must be set}"
: "${OPENAI_API_KEY:?OPENAI_API_KEY must be set}"
: "${OPENAI_BASE_URL:?OPENAI_BASE_URL must be set}"
: "${OPENAI_MODEL:?OPENAI_MODEL must be set}"
: "${KUBE_API_SERVER:?KUBE_API_SERVER must be set}"
# FINDING_DETAILS is optional — native providers may not include additional context
FINDING_DETAILS="${FINDING_DETAILS:-}"
# FINDING_PARENT is optional — not all native provider findings have a parent object
FINDING_PARENT="${FINDING_PARENT:-<none>}"

# Build the opencode config from injected LLM credentials and export it so
# opencode picks it up without needing a config file on disk.
# Uses a custom OpenAI-compatible provider pointing at the configured base URL.
export OPENCODE_CONFIG_CONTENT
OPENCODE_CONFIG_CONTENT=$(printf '{
  "$schema": "https://opencode.ai/config.json",
  "autoupdate": false,
  "permission": {
    "*": "allow",
    "external_directory": {
      "/tmp/*": "allow",
      "/tmp/**": "allow"
    }
  },
  "provider": {
    "custom": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Custom",
      "options": {
        "baseURL": "%s",
        "apiKey": "%s"
      },
      "models": {
        "%s": {}
      }
    }
  },
  "model": "custom/%s"
}' "$OPENAI_BASE_URL" "$OPENAI_API_KEY" "$OPENAI_MODEL" "$OPENAI_MODEL")

# Build a kubeconfig pointing at the correct API server address.
# Use the long-lived legacy SA token (no audience claim) mounted from the
# mendabot-agent-token secret, which is accepted by the API server regardless
# of the projected token issuer mismatch on worker nodes in this Talos cluster.
LEGACY_TOKEN=/var/run/secrets/mendabot/serviceaccount/token
SA_CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
KUBE_SERVER="${KUBE_API_SERVER:-}"
SA_TOKEN_FILE="${LEGACY_TOKEN}"
if [ ! -f "$LEGACY_TOKEN" ]; then
    SA_TOKEN_FILE=/var/run/secrets/kubernetes.io/serviceaccount/token
    KUBE_SERVER="${KUBE_API_SERVER:-$(cat "$SA_TOKEN_FILE" | cut -d. -f2 | base64 -d 2>/dev/null | jq -r '.iss // empty')}"
fi
if [ -n "$KUBE_SERVER" ]; then
    mkdir -p /home/agent/.kube
    kubectl config set-cluster in-cluster \
        --server="$KUBE_SERVER" \
        --certificate-authority="$SA_CA" \
        --embed-certs=true \
        --kubeconfig=/home/agent/.kube/config
    kubectl config set-credentials in-cluster \
        --token="$(cat $SA_TOKEN_FILE)" \
        --kubeconfig=/home/agent/.kube/config
    kubectl config set-context in-cluster \
        --cluster=in-cluster \
        --user=in-cluster \
        --kubeconfig=/home/agent/.kube/config
    kubectl config use-context in-cluster \
        --kubeconfig=/home/agent/.kube/config
    export KUBECONFIG=/home/agent/.kube/config
    # Also write to shell profile so opencode's bash subshells inherit it
    echo "export KUBECONFIG=/home/agent/.kube/config" >> /home/agent/.bashrc
    echo "export KUBECONFIG=/home/agent/.kube/config" >> /home/agent/.profile
fi

# Authenticate gh CLI using the token written by the init container.
# Validate that authentication succeeds — a bad token would otherwise only be
# discovered mid-investigation when gh pr list fails.
gh auth login --with-token < /workspace/github-token
if ! gh auth status > /dev/null 2>&1; then
    echo "ERROR: gh authentication failed — check /workspace/github-token" >&2
    exit 1
fi

# Substitute environment variables into the prompt template.
# envsubst only replaces ${VAR} patterns it knows about. To avoid corrupting
# content in FINDING_ERRORS or FINDING_DETAILS that may contain literal $ signs
# (e.g. from Helm templates or shell variables in log output), we restrict
# envsubst to only the known variable names.
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
envsubst "$VARS" < /prompt/prompt.txt > /tmp/rendered-prompt.txt

# Run opencode with the rendered prompt. The prompt is passed as a single
# quoted string argument — word-splitting is not a concern because the shell
# expands "$(cat ...)" as one argument to `opencode run`.
exec opencode run "$(cat /tmp/rendered-prompt.txt)"
