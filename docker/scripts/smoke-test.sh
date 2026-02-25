#!/usr/bin/env bash
set -euo pipefail

IMAGE=${1:?Usage: smoke-test.sh <image-tag>}

pass=0
fail=0

check_exec() {
    local path="$1"
    echo -n "Checking executable: $path ... "
    if docker run --rm --entrypoint /bin/sh "$IMAGE" -c "test -x $path"; then
        echo "OK"
        ((pass++)) || true
    else
        echo "FAIL"
        ((fail++)) || true
    fi
}

check_binary() {
    local bin="$1"
    echo -n "Checking binary in PATH: $bin ... "
    if docker run --rm --entrypoint /bin/sh "$IMAGE" -c "command -v $bin > /dev/null 2>&1"; then
        echo "OK"
        ((pass++)) || true
    else
        echo "FAIL"
        ((fail++)) || true
    fi
}

# Entrypoint scripts
check_exec /usr/local/bin/agent-entrypoint.sh
check_exec /usr/local/bin/entrypoint-opencode.sh
check_exec /usr/local/bin/entrypoint-claude.sh
check_exec /usr/local/bin/entrypoint-common.sh
check_exec /usr/local/bin/get-github-app-token.sh

# Runtime binaries called by entrypoint scripts
check_binary opencode
check_binary kubectl
check_binary gh
check_binary envsubst
check_binary git
check_binary helm
check_binary flux
check_binary kustomize
check_binary kubeconform
check_binary yq
check_binary jq
check_binary redact

echo ""
echo "Smoke test complete: ${pass} passed, ${fail} failed."
if [ "$fail" -gt 0 ]; then
    exit 1
fi
