#!/usr/bin/env bash
# wrapper-test.sh — verifies redact binary and wrapper presence in a built image.
# Usage: wrapper-test.sh <image-tag>
set -euo pipefail

IMAGE=${1:?Usage: wrapper-test.sh <image-tag>}

pass=0
fail=0

check_exec() {
    local path="$1"
    printf 'Checking executable: %s ... ' "$path"
    if docker run --rm --entrypoint /bin/sh "$IMAGE" -c "test -x '$path'"; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL"; ((fail++)) || true
    fi
}

# check_redact_filters: passes input via environment variable to avoid shell injection.
# $input is never interpolated into the -c string; it is read from the environment
# inside the container using "$REDACT_INPUT".
check_redact_filters() {
    local input="$1"
    local must_contain="$2"
    local must_not_contain="$3"
    printf 'Checking redact filters input=[%s] must_contain=[%s] must_not=[%s] ... ' \
        "$input" "$must_contain" "$must_not_contain"
    local out
    out=$(docker run --rm \
        -e REDACT_INPUT="$input" \
        --entrypoint /bin/sh "$IMAGE" \
        -c 'printf "%s" "$REDACT_INPUT" | redact')
    if printf '%s' "$out" | grep -qF "$must_contain" \
    && ! printf '%s' "$out" | grep -qF "$must_not_contain"; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL (got: $out)"; ((fail++)) || true
    fi
}

# check_exit_code: installs a stub binary that exits with a known code, runs the
# named wrapper pointing at the stub via PATH, and verifies the wrapper propagates
# the exit code.
# Only valid for tools where the wrapper calls <tool>.real (not absolute-path tools
# like gh and openssl which call /usr/bin/<tool> and cannot be intercepted this way).
check_exit_code() {
    local tool="$1"
    local expected_rc="$2"
    printf 'Checking exit code passthrough: %s (expect %d) ... ' "$tool" "$expected_rc"
    # Create stub at /tmp/stub/<tool>.real; prepend /tmp/stub to PATH so the wrapper's
    # bare `<tool>.real "$@"` call resolves to the stub.
    # Redirect tool output to /dev/null; capture only the echo of $? to avoid
    # false failures if redact emits anything on empty input.
    local rc
    rc=$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
        "mkdir -p /tmp/stub \
         && printf '#!/bin/sh\nexit ${expected_rc}\n' > /tmp/stub/${tool}.real \
         && chmod +x /tmp/stub/${tool}.real \
         && PATH=/tmp/stub:\$PATH ${tool} > /dev/null 2>&1; echo \$?") || true
    if [ "$rc" = "$expected_rc" ]; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL (got exit code: $rc, expected: $expected_rc)"; ((fail++)) || true
    fi
}

# ── redact binary ─────────────────────────────────────────────────────────────
check_exec /usr/local/bin/redact

# ── Functional redaction checks ───────────────────────────────────────────────
check_redact_filters "password=hunter2" "[REDACTED]" "hunter2"
check_redact_filters "token=ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" "[REDACTED]" "ghp_"
check_redact_filters "Authorization: ghs_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" "[REDACTED-GH-TOKEN]" "ghs_"
check_redact_filters "CrashLoopBackOff" "CrashLoopBackOff" "[REDACTED]"

# ── Wrapper presence + real binary presence ───────────────────────────────────
for tool in kubectl helm flux sops talosctl yq stern kubeconform kustomize age age-keygen; do
    check_exec "/usr/local/bin/${tool}"
    check_exec "/usr/local/bin/${tool}.real"
done

# gh: wrapper at /usr/local/bin/gh, real binary at /usr/bin/gh (apt-installed)
check_exec /usr/local/bin/gh
check_exec /usr/bin/gh

# git: wrapper at /usr/local/bin/git, real binary at /usr/bin/git.real (apt-installed, renamed by Dockerfile)
check_exec /usr/local/bin/git
check_exec /usr/bin/git.real

# ── Wrapper structure checks (structural, not functional) ─────────────────────
for tool in kubectl helm flux gh sops talosctl yq stern kubeconform kustomize age age-keygen; do
    printf 'Checking wrapper structure: %s ... ' "$tool"
    if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
        "grep -q 'trap' /usr/local/bin/${tool} && \
         grep -q '_rc=\$?' /usr/local/bin/${tool} && \
         grep -q 'redact < ' /usr/local/bin/${tool} && \
         grep -q 'command -v redact' /usr/local/bin/${tool} && \
         grep -q 'exit 1' /usr/local/bin/${tool} && \
         grep -q '_rr=\$?' /usr/local/bin/${tool}"; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL"; ((fail++)) || true
    fi
done

# gh wrapper: verify it calls /usr/bin/gh specifically (not gh.real)
printf 'Checking gh wrapper calls /usr/bin/gh ... '
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    "grep -qF '/usr/bin/gh' /usr/local/bin/gh && \
     ! grep -qF 'gh.real' /usr/local/bin/gh"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL"; ((fail++)) || true
fi

# git wrapper: verify it contains all three dry-run enforcement layers
printf 'Checking git wrapper has sentinel file layer ... '
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    "grep -qF '/mechanic-cfg/dry-run' /usr/local/bin/git"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL"; ((fail++)) || true
fi

printf 'Checking git wrapper has /proc/1/environ layer ... '
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    "grep -qF '/proc/1/environ' /usr/local/bin/git"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL"; ((fail++)) || true
fi

printf 'Checking git wrapper has env var fallback layer ... '
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    "grep -qF 'DRY_RUN' /usr/local/bin/git"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL"; ((fail++)) || true
fi

# git wrapper: verify sentinel file layer blocks even when DRY_RUN is unset
printf 'Checking git wrapper blocks via sentinel file when DRY_RUN is unset ... '
block_rc=$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    'mkdir -p /mechanic-cfg && echo -n true > /mechanic-cfg/dry-run \
     && unset DRY_RUN \
     && git push 2>&1; echo "exit:$?"') || true
if echo "$block_rc" | grep -q "DRY_RUN.*blocked" && echo "$block_rc" | grep -q "exit:0"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL (output: $block_rc)"; ((fail++)) || true
fi

# gh wrapper: verify all three layers present
printf 'Checking gh wrapper has sentinel file layer ... '
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    "grep -qF '/mechanic-cfg/dry-run' /usr/local/bin/gh"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL"; ((fail++)) || true
fi

printf 'Checking gh wrapper has /proc/1/environ layer ... '
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    "grep -qF '/proc/1/environ' /usr/local/bin/gh"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL"; ((fail++)) || true
fi

# gh wrapper: verify sentinel file layer blocks even when DRY_RUN is unset
printf 'Checking gh wrapper blocks via sentinel file when DRY_RUN is unset ... '
block_gh_rc=$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    'mkdir -p /mechanic-cfg && echo -n true > /mechanic-cfg/dry-run \
     && unset DRY_RUN \
     && /usr/local/bin/gh pr create 2>&1; echo "exit:$?"') || true
if echo "$block_gh_rc" | grep -q "DRY_RUN.*blocked" && echo "$block_gh_rc" | grep -q "exit:0"; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL (output: $block_gh_rc)"; ((fail++)) || true
fi

# ── Functional hard-fail test: wrapper must exit 1 when redact is absent ──────
# Strategy: move /usr/local/bin/redact aside inside the container so that
# `command -v redact` returns false, then invoke the kubectl wrapper and assert
# it exits 1. We restore the binary name immediately after so no other test is
# affected (each docker run is a fresh container, so this is moot in practice,
# but the comment documents intent).
# Note: chmod 000 does NOT work — bash's `command -v` skips non-executable
# files and would fall through to the real redact binary instead.
printf 'Checking wrapper hard-fail when redact absent (kubectl) ... '
hf_rc=$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
    'mv /usr/local/bin/redact /usr/local/bin/redact.bak \
     && kubectl --version > /dev/null 2>&1; echo $?') || true
if [ "$hf_rc" = "1" ]; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL (got exit code: $hf_rc, expected: 1)"; ((fail++)) || true
fi

# ── Functional exit code passthrough tests ────────────────────────────────────
# Only for tools that use <tool>.real (PATH-interceptable).
# gh uses absolute path (/usr/bin/gh) and cannot be intercepted this way —
# its exit code passthrough is verified structurally above.
for tool in kubectl helm flux sops talosctl yq stern kubeconform kustomize age age-keygen; do
    check_exit_code "$tool" 42
done

# ── Tier 1 write-block tests ──────────────────────────────────────────────────
# Strategy: install stub kubectl.real (exits 0) and stub redact (cat passthrough)
# at /tmp/stub/, then invoke the kubectl wrapper with a write subcommand.
# The wrapper must exit 1 with a [KUBECTL] message before reaching kubectl.real.
# For read subcommands the wrapper must exit 0 (stub kubectl.real exits 0).

# check_write_blocked: asserts the kubectl wrapper blocks the given subcommand.
check_write_blocked() {
    local subcmd="$1"
    shift
    local args="$*"
    printf 'Checking kubectl write-block: kubectl %s %s ... ' "$subcmd" "$args"
    local out rc
    out=$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
        "mkdir -p /tmp/stub \
         && printf '#!/bin/sh\nexit 0\n' > /tmp/stub/kubectl.real \
         && chmod +x /tmp/stub/kubectl.real \
         && printf '#!/bin/sh\ncat\n' > /tmp/stub/redact \
         && chmod +x /tmp/stub/redact \
         && PATH=/tmp/stub:\$PATH kubectl ${subcmd} ${args} 2>&1; echo \"exit:\$?\"") || true
    if printf '%s' "$out" | grep -q '\[KUBECTL\]' \
    && printf '%s' "$out" | grep -q 'blocked' \
    && printf '%s' "$out" | grep -q 'exit:1'; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL (output: $out)"; ((fail++)) || true
    fi
}

# check_write_allowed: asserts the kubectl wrapper does NOT block the given subcommand.
# Stub kubectl.real exits 0; if wrapper passes through it also exits 0.
check_write_allowed() {
    local subcmd="$1"
    shift
    local args="$*"
    printf 'Checking kubectl no-block: kubectl %s %s ... ' "$subcmd" "$args"
    local out rc
    out=$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
        "mkdir -p /tmp/stub \
         && printf '#!/bin/sh\nexit 0\n' > /tmp/stub/kubectl.real \
         && chmod +x /tmp/stub/kubectl.real \
         && printf '#!/bin/sh\ncat\n' > /tmp/stub/redact \
         && chmod +x /tmp/stub/redact \
         && PATH=/tmp/stub:\$PATH kubectl ${subcmd} ${args} 2>&1; echo \"exit:\$?\"") || true
    if ! printf '%s' "$out" | grep -q '\[KUBECTL\]' \
    && printf '%s' "$out" | grep -q 'exit:0'; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL (output: $out)"; ((fail++)) || true
    fi
}

# Blocked write subcommands (must exit 1 with [KUBECTL] message)
check_write_blocked apply -f manifest.yaml
check_write_blocked create deployment foo --image=nginx
check_write_blocked delete pod foo
check_write_blocked edit deployment foo
check_write_blocked patch deployment foo -p '{}'
check_write_blocked replace -f manifest.yaml
check_write_blocked scale deployment foo --replicas=3
check_write_blocked set image deployment/foo container=nginx:latest
check_write_blocked label pod foo app=bar
check_write_blocked annotate pod foo note=test
check_write_blocked taint node foo key=val:NoSchedule
check_write_blocked drain node foo
check_write_blocked cordon node foo
check_write_blocked uncordon node foo
check_write_blocked rollout restart deployment/foo
check_write_blocked rollout undo deployment/foo

# Read subcommands (must NOT be blocked)
check_write_allowed get pods
check_write_allowed describe deployment foo
check_write_allowed logs foo
check_write_allowed diff -f manifest.yaml
check_write_allowed rollout status deployment/foo
check_write_allowed rollout history deployment/foo
# ─────────────────────────────────────────────────────────────────────────────

# ── Tier 2 hardened-mode tests ────────────────────────────────────────────────
# Strategy: install stub kubectl.real (exits 0) and stub redact (cat passthrough)
# at /tmp/stub/, set HARDEN_KUBECTL=true via env, then invoke the kubectl wrapper.
# Blocked calls must exit 1 with a [KUBECTL-HARDENED] message.
# Allowed calls must exit 0 without the [KUBECTL-HARDENED] message.

# check_hardened_blocked: asserts the wrapper blocks the given subcommand in hardened mode.
check_hardened_blocked() {
    local subcmd="$1"
    shift
    local args="$*"
    printf 'Checking kubectl hardened-block: kubectl %s %s ... ' "$subcmd" "$args"
    local out
    out=$(docker run --rm \
        -e HARDEN_KUBECTL=true \
        --entrypoint /bin/sh "$IMAGE" -c \
        "mkdir -p /tmp/stub \
         && printf '#!/bin/sh\nexit 0\n' > /tmp/stub/kubectl.real \
         && chmod +x /tmp/stub/kubectl.real \
         && printf '#!/bin/sh\ncat\n' > /tmp/stub/redact \
         && chmod +x /tmp/stub/redact \
         && PATH=/tmp/stub:\$PATH kubectl ${subcmd} ${args} 2>&1; echo \"exit:\$?\"") || true
    if printf '%s' "$out" | grep -q '\[KUBECTL-HARDENED\]' \
    && printf '%s' "$out" | grep -q 'blocked' \
    && printf '%s' "$out" | grep -q 'exit:1'; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL (output: $out)"; ((fail++)) || true
    fi
}

# check_hardened_allowed: asserts the wrapper does NOT block in hardened mode.
check_hardened_allowed() {
    local subcmd="$1"
    shift
    local args="$*"
    printf 'Checking kubectl hardened-allowed: kubectl %s %s ... ' "$subcmd" "$args"
    local out
    out=$(docker run --rm \
        -e HARDEN_KUBECTL=true \
        --entrypoint /bin/sh "$IMAGE" -c \
        "mkdir -p /tmp/stub \
         && printf '#!/bin/sh\nexit 0\n' > /tmp/stub/kubectl.real \
         && chmod +x /tmp/stub/kubectl.real \
         && printf '#!/bin/sh\ncat\n' > /tmp/stub/redact \
         && chmod +x /tmp/stub/redact \
         && PATH=/tmp/stub:\$PATH kubectl ${subcmd} ${args} 2>&1; echo \"exit:\$?\"") || true
    if ! printf '%s' "$out" | grep -q '\[KUBECTL-HARDENED\]' \
    && printf '%s' "$out" | grep -q 'exit:0'; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL (output: $out)"; ((fail++)) || true
    fi
}

# Blocked in hardened mode
check_hardened_blocked get secret
check_hardened_blocked get secrets
check_hardened_blocked get secret/my-secret
check_hardened_blocked get secret my-secret -n foo
check_hardened_blocked describe secret my-secret
check_hardened_blocked describe secrets
check_hardened_blocked get all
check_hardened_blocked get all -n kube-system
check_hardened_blocked exec my-pod -- /bin/sh
check_hardened_blocked port-forward svc/my-svc 8080:80
check_hardened_blocked get pods,secrets
check_hardened_blocked get secrets,pods

# Allowed in hardened mode (must NOT be over-blocked)
check_hardened_allowed get pods
check_hardened_allowed get configmaps
check_hardened_allowed describe deployment foo
check_hardened_allowed get pods,configmaps
check_hardened_allowed logs foo

# Verify hardened mode is OFF by default (no HARDEN_KUBECTL set)
printf 'Checking kubectl hardened mode off by default: get secret ... '
default_out=$(docker run --rm \
    --entrypoint /bin/sh "$IMAGE" -c \
    "mkdir -p /tmp/stub \
     && printf '#!/bin/sh\nexit 0\n' > /tmp/stub/kubectl.real \
     && chmod +x /tmp/stub/kubectl.real \
     && printf '#!/bin/sh\ncat\n' > /tmp/stub/redact \
     && chmod +x /tmp/stub/redact \
     && PATH=/tmp/stub:\$PATH kubectl get secret 2>&1; echo \"exit:\$?\"") || true
if ! printf '%s' "$default_out" | grep -q '\[KUBECTL-HARDENED\]' \
&& printf '%s' "$default_out" | grep -q 'exit:0'; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL (output: $default_out)"; ((fail++)) || true
fi

# Verify sentinel file layer activates hardened mode (even when HARDEN_KUBECTL is unset)
printf 'Checking hardened mode via sentinel file: get secret ... '
sentinel_out=$(docker run --rm \
    --entrypoint /bin/sh "$IMAGE" -c \
    "mkdir -p /tmp/stub /mechanic-cfg \
     && echo -n true > /mechanic-cfg/harden-kubectl \
     && printf '#!/bin/sh\nexit 0\n' > /tmp/stub/kubectl.real \
     && chmod +x /tmp/stub/kubectl.real \
     && printf '#!/bin/sh\ncat\n' > /tmp/stub/redact \
     && chmod +x /tmp/stub/redact \
     && unset HARDEN_KUBECTL \
     && PATH=/tmp/stub:\$PATH kubectl get secret 2>&1; echo \"exit:\$?\"") || true
if printf '%s' "$sentinel_out" | grep -q '\[KUBECTL-HARDENED\]' \
&& printf '%s' "$sentinel_out" | grep -q 'exit:1'; then
    echo "OK"; ((pass++)) || true
else
    echo "FAIL (output: $sentinel_out)"; ((fail++)) || true
fi
# ─────────────────────────────────────────────────────────────────────────────

echo ""
echo "Wrapper test complete: ${pass} passed, ${fail} failed."
if [ "$fail" -gt 0 ]; then
    exit 1
fi
