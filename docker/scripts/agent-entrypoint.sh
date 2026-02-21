#!/usr/bin/env bash
set -euo pipefail

# Verify all required environment variables are present before doing any work.
# This produces a clear error message rather than silently rendering blank fields.
: "${FINDING_KIND:?FINDING_KIND must be set}"
: "${FINDING_NAME:?FINDING_NAME must be set}"
: "${FINDING_NAMESPACE:?FINDING_NAMESPACE must be set}"
: "${FINDING_PARENT:?FINDING_PARENT must be set}"
: "${FINDING_FINGERPRINT:?FINDING_FINGERPRINT must be set}"
: "${FINDING_ERRORS:?FINDING_ERRORS must be set}"
: "${FINDING_DETAILS:?FINDING_DETAILS must be set}"
: "${GITOPS_REPO:?GITOPS_REPO must be set}"
: "${GITOPS_MANIFEST_ROOT:?GITOPS_MANIFEST_ROOT must be set}"

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
