#!/usr/bin/env bash
set -euo pipefail

IMAGE=${1:?Usage: smoke-test.sh <image-tag>}

check() {
    echo "Checking: $*"
    docker run --rm --entrypoint /bin/sh "$IMAGE" -c "$*"
}

check opencode --version
check kubectl version --client
check k8sgpt version
check helm version
check flux version --client
check talosctl version --client
check kustomize version
check yq --version
check gh --version
check jq --version
check sops --version
check age --version
check stern --version
check kubeconform -v
check envsubst --version
check git --version
check curl --version
check openssl version

echo "Checking: agent-entrypoint.sh executable"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "test -x /usr/local/bin/agent-entrypoint.sh"
echo "Checking: get-github-app-token.sh executable"
docker run --rm --entrypoint /bin/sh "$IMAGE" -c "test -x /usr/local/bin/get-github-app-token.sh"
