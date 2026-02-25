# Story 02: Shell Wrapper Scripts

**Epic:** [epic25-tool-output-redaction](README.md)
**Priority:** Critical
**Status:** Complete

---

## User Story

As a **mendabot operator**, I want every tool that the LLM agent can invoke for cluster
investigation to have its stdout and stderr passed through `redact` before the output
is returned to the LLM, so that no credential-bearing output can reach the external LLM
API regardless of what command the LLM constructs.

---

## Background

OpenCode resolves tool binaries via `PATH`. The default `PATH` in `debian:bookworm-slim`
is `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`. All manually-installed
tools (`kubectl`, `helm`, etc.) live in `/usr/local/bin/`. Wrappers installed to the same
directory, with real binaries renamed to `<tool>.real`, intercept every invocation
without any `PATH` modification.

`gh` is the exception: it is installed by apt to `/usr/bin/gh`. Its wrapper lives at
`/usr/local/bin/gh` (which comes earlier in `PATH`). The wrapper calls `/usr/bin/gh`
directly by absolute path — the apt-managed binary is never touched.

Each wrapper:
1. Executes the real binary with `"$@"` (all arguments forwarded verbatim)
2. Captures combined stdout+stderr into a temp file (matches what OpenCode merges)
3. Passes the temp file through `redact`
4. Writes redacted output to stdout
5. Cleans up temp file via `trap`
6. Exits with the real binary's exit code

`set -euo pipefail` is **not** used in wrappers — the real binary may exit non-zero
legitimately (e.g. `kubectl get pod nonexistent` exits 1). The wrapper must preserve
that exit code.

---

## Acceptance Criteria

- [x] `docker/scripts/redact-wrappers/` directory contains one script per wrapped tool
- [x] Scripts: `kubectl`, `helm`, `flux`, `gh`, `sops`, `talosctl`, `yq`, `stern`,
      `kubeconform`, `kustomize`, `age`, `age-keygen`
- [x] Each wrapper: calls real binary, captures combined stdout+stderr, pipes through
      `redact`, preserves exit code, uses `trap` for temp file cleanup
- [x] Wrapper hard-fails with exit 1 + stderr message if `redact` binary is not found
      in `PATH` (see §Hard-fail on missing redact below)
- [x] Wrapper hard-fails with exit 1 + stderr message if `mktemp` fails
- [x] `gh` wrapper calls `/usr/bin/gh` by absolute path (not `.real`)
- [x] All other wrappers call `<binary-name>.real` (resolved via PATH, lands in
      `/usr/local/bin/<tool>.real` after Dockerfile rename)
- [x] All wrapper scripts pass `shellcheck` with no errors

---

## Technical Implementation

### Wrapper template

All wrappers follow this pattern (shown for `kubectl`):

```bash
#!/usr/bin/env bash
# kubectl wrapper — pipes output through redact before returning to caller.
# Does NOT use set -e: the real binary may exit non-zero legitimately.

if ! command -v redact > /dev/null 2>&1; then
    echo "[ERROR] redact binary not found in PATH — aborting to prevent unredacted output" >&2
    exit 1
fi

_tmpfile=$(mktemp) || { echo "[ERROR] mktemp failed — aborting" >&2; exit 1; }
trap 'rm -f "$_tmpfile"' EXIT

kubectl.real "$@" > "$_tmpfile" 2>&1
_rc=$?

redact < "$_tmpfile"
exit $_rc
```

Key design decisions:
- Hard-fail guard at top: if `redact` is missing, the wrapper exits 1 immediately with
  a clear error message. This is intentional: a missing `redact` binary means the security
  property cannot be provided. Silent passthrough is a worse failure mode than hard failure.
  Under `set -euo pipefail` in `entrypoint-common.sh`, this propagates upward and aborts
  the entrypoint with a visible error — the operator sees the problem immediately.
- `mktemp` failure guard: if `/tmp` is full or permissions are wrong, the wrapper exits 1
  immediately rather than creating an empty-string filename that silently discards output.
- `mktemp` creates a per-invocation temp file in `/tmp` (default, world-writable-sticky)
- `trap 'rm -f "$_tmpfile"' EXIT` fires on any exit path including signals
- `> "$_tmpfile" 2>&1` merges stdout+stderr into one stream (matches OpenCode's merged view)
- `redact < "$_tmpfile"` pipes the full combined output through the filter
- `exit $_rc` preserves the real binary's exit code exactly

### `gh` wrapper (different real binary path)

```bash
#!/usr/bin/env bash
# gh wrapper — gh is installed by apt to /usr/bin/gh; call it by absolute path.

if ! command -v redact > /dev/null 2>&1; then
    echo "[ERROR] redact binary not found in PATH — aborting to prevent unredacted output" >&2
    exit 1
fi

_tmpfile=$(mktemp) || { echo "[ERROR] mktemp failed — aborting" >&2; exit 1; }
trap 'rm -f "$_tmpfile"' EXIT

/usr/bin/gh "$@" > "$_tmpfile" 2>&1
_rc=$?

redact < "$_tmpfile"
exit $_rc
```

### File layout

```
docker/scripts/redact-wrappers/
    kubectl
    helm
    flux
    gh
    sops
    talosctl
    yq
    stern
    kubeconform
    kustomize
    age
    age-keygen
```

### Why `age` and `age-keygen` are wrapped

`age` is installed at `/usr/local/bin/age` (built from source via `age-builder`).
`age --decrypt -i <keyfile> <encrypted-file>` writes the decrypted plaintext to stdout —
this can contain raw secret values. `age-keygen` generates new key pairs and prints the
private key to stdout. Both are direct credential-exposure vectors equivalent to `sops`
(which wraps `age` under the hood) and must be wrapped.

---

## Definition of Done

- [x] All 12 wrapper scripts exist in `docker/scripts/redact-wrappers/`
- [x] Each script is executable (`chmod +x` in Dockerfile — see STORY_03)
- [x] Each script contains the hard-fail guard (`command -v redact` check at top)
- [x] Each script contains the `mktemp` failure guard (`|| { ... exit 1; }`)
- [x] `shellcheck docker/scripts/redact-wrappers/*` passes with no errors
- [x] Manual review: `gh` wrapper calls `/usr/bin/gh`, all others call `<tool>.real`
- [x] Manual review: all wrappers use `trap` for cleanup
- [x] Manual review: no `set -e` in any wrapper (exit code preservation)
