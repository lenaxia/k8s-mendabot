# Story 04: Wrapper Integration Tests

**Epic:** [epic25-tool-output-redaction](README.md)
**Priority:** High
**Status:** Complete

---

## User Story

As a **mendabot operator**, I want the CI pipeline to verify after every agent image
build that all wrappers are present, that output containing known secret patterns is
redacted, and that the original tool exit codes are preserved, so that any regression
in the wrapper layer is caught in CI.

---

## Acceptance Criteria

- [x] `docker/scripts/wrapper-test.sh <image>` exists and is executable
- [x] For each of the 12 wrapped tools, the script verifies:
  - The wrapper script is present and executable at `/usr/local/bin/<tool>`
  - The real binary is present at `/usr/local/bin/<tool>.real` (or `/usr/bin/gh`
    for the apt-installed tool)
  - The `redact` binary is present and executable at `/usr/local/bin/redact`
- [x] The script runs a **functional redaction check** using `redact` directly:
  - Pipes a string containing `password=hunter2` through `redact`
  - Verifies the output contains `[REDACTED]` and does not contain `hunter2`
- [x] The script verifies that `redact` preserves clean text unchanged (no false positives
      on `CrashLoopBackOff`)
- [x] The script performs a **functional exit code passthrough test** using a stub binary
      for all tools where the real binary can be intercepted via PATH (see §Exit code test)
- [x] `docker/scripts/smoke-test.sh` adds a single presence check for `/usr/local/bin/redact`
      only — all wrapper-specific checks live in `wrapper-test.sh`, not in `smoke-test.sh`
- [x] `build-agent.yaml` runs `wrapper-test.sh` after the existing smoke test step as a
      **separate CI step** — `smoke-test.sh` does NOT call `wrapper-test.sh`
- [x] `wrapper-test.sh` passes `shellcheck` with no errors

---

## Design Notes

The wrapper test does **not** attempt to call `kubectl`, `helm`, etc. with real secrets —
those binaries require a live cluster or valid credentials. Instead it:

1. Verifies structural presence (wrappers exist, `.real` binaries exist, `redact` exists)
2. Tests the `redact` binary itself end-to-end using `docker run ... sh -c "echo ... | redact"`
3. Verifies that a wrapper script's shebang and structure are correct by inspecting the
   file content (grep for `trap`, `_rc=$?`, `redact <`, `command -v redact`)

**Exit code test limitation:** `gh` wrapper calls its real binary by absolute path
(`/usr/bin/gh`). No PATH manipulation can intercept it. Its exit code passthrough is
verified structurally (grep for `_rc=$?` and `exit $_rc`) rather than functionally.

**arm64 limitation:** CI tests run on amd64 runners and pull the amd64 image variant.
The arm64 variant is built and pushed but not functionally tested. This is a known
limitation — adding `--platform linux/arm64` test steps would require arm64 runners.

**Post-push test timing:** The `build-agent.yaml` workflow pushes the image before
running `wrapper-test.sh`. A broken image would already be in GHCR before this test
fails. The hard-fail guard in every wrapper (`command -v redact || exit 1`) is the
primary runtime protection; this CI test is secondary verification.

---

## Technical Implementation

### `docker/scripts/wrapper-test.sh`

```bash
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

# ── Wrapper structure checks (structural, not functional) ─────────────────────
for tool in kubectl helm flux gh sops talosctl yq stern kubeconform kustomize age age-keygen; do
    printf 'Checking wrapper structure: %s ... ' "$tool"
    if docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
        "grep -q 'trap' /usr/local/bin/${tool} && \
         grep -q '_rc=\$?' /usr/local/bin/${tool} && \
         grep -q 'redact < ' /usr/local/bin/${tool} && \
         grep -q 'command -v redact' /usr/local/bin/${tool}"; then
        echo "OK"; ((pass++)) || true
    else
        echo "FAIL"; ((fail++)) || true
    fi
done

# ── Functional exit code passthrough tests ────────────────────────────────────
# Only for tools that use <tool>.real (PATH-interceptable).
# gh uses absolute path (/usr/bin/gh) and cannot be intercepted this way —
# its exit code passthrough is verified structurally above.
for tool in kubectl helm flux sops talosctl yq stern kubeconform kustomize age age-keygen; do
    check_exit_code "$tool" 42
done

echo ""
echo "Wrapper test complete: ${pass} passed, ${fail} failed."
if [ "$fail" -gt 0 ]; then
    exit 1
fi
```

### Scope of `smoke-test.sh` vs `wrapper-test.sh`

These are separate concerns with separate responsibilities:

| Check | `smoke-test.sh` | `wrapper-test.sh` |
|-------|----------------|-------------------|
| All expected binaries in PATH | Yes (existing checks) | No |
| `redact` binary present | **Add one check** | Yes |
| Wrappers present + executable | No | Yes |
| `.real` binaries present | No | Yes |
| Wrapper structure (trap, redact) | No | Yes |
| Functional redaction | No | Yes |
| Exit code passthrough | No | Yes |

`smoke-test.sh` gets **one new line**: `check_binary redact`. Everything else stays in
`wrapper-test.sh`. `smoke-test.sh` does NOT call `wrapper-test.sh` — they are invoked
as separate CI steps in `build-agent.yaml`.

### `build-agent.yaml` update

Add after the existing smoke test step:

```yaml
- name: Wrapper test
  run: |
    chmod +x docker/scripts/wrapper-test.sh
    docker/scripts/wrapper-test.sh ghcr.io/lenaxia/mendabot-agent:sha-${{ steps.sha.outputs.short }}
```

---

## Definition of Done

- [x] `docker/scripts/wrapper-test.sh` exists and is executable
- [x] `wrapper-test.sh` passes `shellcheck` with no errors
- [x] Script passes when run against a correctly built image
- [x] Script fails (exit 1) when a wrapper is missing or `redact` is absent
- [x] Script fails when `redact` passes `hunter2` through unredacted
- [x] Script fails when a PATH-interceptable wrapper does not propagate exit code
- [x] `check_redact_filters` uses `-e REDACT_INPUT` (no shell injection risk)
- [x] `check_exec` uses single-quoted path inside `-c` string (no word-splitting)
- [x] Structure check greps for `'redact < '` (verifies actual pipe, not just the guard comment)
- [x] `gh` excluded from functional exit code loop (calls `/usr/bin/gh` by absolute path; documented in script)
- [x] All 11 PATH-interceptable tools covered in functional exit code loop
- [x] `smoke-test.sh` has exactly one new line: `check_binary redact`
- [x] `smoke-test.sh` does NOT call `wrapper-test.sh`
- [x] `build-agent.yaml` calls `wrapper-test.sh` as a separate step after smoke test
- [x] CI run succeeds end-to-end with new step
