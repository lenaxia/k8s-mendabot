# Epic 25: Tool Call Output Redaction Wrappers

**Feature Tracker:** FT-S6
**Area:** Security
**Branch:** `feature/epic25-tool-output-redaction`

## Status: Not Started

## Problem Statement

When the OpenCode LLM agent executes a tool call (e.g. `kubectl get secret xyz -o yaml`),
the raw output is returned directly to the LLM context and sent to the external LLM API.
`domain.RedactSecrets` only runs at source — when the native provider builds
`Finding.Errors` from Kubernetes status fields. It has zero visibility into tool call output.

Data flow that exposes secrets:

```
LLM calls: kubectl get secret xyz -o yaml
    ↓  opencode spawns: bash -c "kubectl get secret xyz -o yaml"
    ↓  OS resolves 'kubectl' via PATH
    ↓  raw YAML (base64-encoded secret values) returned as tool result
    ↓  fed into LLM context
    ↓  sent to external LLM API           ← secrets leave the cluster
```

Prompt-level HARD RULEs are not a sufficient technical control — an LLM can ignore
instructions at any time (novel phrasing, model drift, adversarial input). Technical
enforcement at the output layer is required.

### What OpenCode v1.2.10 does (verified from source)

- All tool execution goes through `child_process.spawn(command, { shell: "<shell>" })` —
  equivalent to `bash -c <command>`. No embedded interpreter.
- The OS shell and all binaries it invokes are resolved through normal `PATH`.
- Output is **fully buffered** before being returned to the LLM (tool awaits process exit).
- stdout and stderr are **merged** into one string in arrival order.
- The only post-processing is truncation (2000 lines / 50 KiB). No content filtering.
- PATH-based wrappers will intercept every tool call without exception.

## Solution

A thin Go filter binary (`cmd/redact`) reads stdin and writes redacted stdout, importing
`internal/domain.RedactSecrets` directly — same compiled regexes, zero pattern drift.

Shell wrappers for each target tool: the wrapper calls the real binary, buffers full
output, pipes through `redact`, preserves the original exit code. Both stdout and stderr
from the real binary are captured (matching what OpenCode merges into a single stream).

Wrappers are installed to `/usr/local/bin/` (where all manually-installed tools already
live). Real binaries are renamed to `<tool>.real` in the same directory. The default
`PATH` in `debian:bookworm-slim` is `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`
— wrappers at `/usr/local/bin/` are found before any fallback paths.

## Dependencies

- epic12-security-review complete (`internal/domain/redact.go` — `RedactSecrets` function)
- epic03-agent-image complete (`docker/Dockerfile.agent` — binary locations established)

## Tool Scope

Tools wrapped (output can contain raw Kubernetes Secret data or credentials):

| Tool | Real binary path | Wrapper path | Risk |
|------|-----------------|--------------|------|
| `kubectl` | `/usr/local/bin/kubectl.real` | `/usr/local/bin/kubectl` | Secrets, ConfigMaps, pod logs |
| `helm` | `/usr/local/bin/helm.real` | `/usr/local/bin/helm` | Helm-managed secrets |
| `flux` | `/usr/local/bin/flux.real` | `/usr/local/bin/flux` | Git credentials, SOPS keys |
| `gh` | `/usr/bin/gh` (apt, not renamed) | `/usr/local/bin/gh` | GitHub API tokens in responses |
| `sops` | `/usr/local/bin/sops.real` | `/usr/local/bin/sops` | Decrypted secret values |
| `talosctl` | `/usr/local/bin/talosctl.real` | `/usr/local/bin/talosctl` | Node credentials, machine configs |
| `yq` | `/usr/local/bin/yq.real` | `/usr/local/bin/yq` | YAML secret values |
| `stern` | `/usr/local/bin/stern.real` | `/usr/local/bin/stern` | Log content with credentials |
| `kubeconform` | `/usr/local/bin/kubeconform.real` | `/usr/local/bin/kubeconform` | Manifest content |
| `kustomize` | `/usr/local/bin/kustomize.real` | `/usr/local/bin/kustomize` | Rendered manifests |
| `age` | `/usr/local/bin/age.real` | `/usr/local/bin/age` | Decrypted plaintext from `--decrypt` |
| `age-keygen` | `/usr/local/bin/age-keygen.real` | `/usr/local/bin/age-keygen` | Private key printed to stdout |
Tools explicitly NOT wrapped (would break init container or entrypoint scripts):

| Tool | Reason |
|------|--------|
| `curl` | Used by `get-github-app-token.sh` in init container — GitHub API response contains `ghs_...` token. Wrapping breaks token extraction. The LLM may also call curl directly; this is a known residual risk documented below. |
| `jq` | Pipes `curl` output through `.token` extraction in init container — wrapping output would redact the token before `TOKEN=$(...)` captures it. |
| `openssl` | Used by `get-github-app-token.sh` (same init container as `curl`/`jq`) — `openssl dgst -sha256 -sign` writes a raw binary DER signature to stdout. The `redact` base64 pattern would match runs of printable bytes in the binary output and corrupt the signature, breaking JWT generation and preventing the init container from obtaining a GitHub App token. The agent container would never start. Residual LLM risk: the LLM could call `openssl rsa`/`openssl pkey` to extract private key material; this is a known residual risk documented below. |
| `cat` | Used in `entrypoint-common.sh` to read SA token file and prompt files; in `entrypoint-opencode.sh` to pass rendered prompt to opencode. Wrapping corrupts control plane reads. |
| `env`/`printenv` | Marginal value; `FINDING_ERRORS` is already redacted at source in all six providers. High risk of shell breakage. |
| `git` | `git log`, `git diff`, and `git show` can surface credentials in commit history or diffs, but git is a core workflow tool used extensively in legitimate remediation PRs. Wrapping git output would break diff-based PR workflows. Accepted residual risk. |

## Known limitations

- **Short Kubernetes Secret values (< 30 raw bytes):** The base64 pattern requires ≥40
  characters (covering ≥30 raw bytes). A `kubectl get secret` field whose value encodes
  fewer than 30 bytes (e.g. a short password like `hunter2`) will not be caught by the
  base64 pattern. It is only protected if its YAML key name matches one of the named
  patterns (`password`, `token`, `secret`, `api-key`). Fields with arbitrary names (e.g.
  `my-custom-key: aHVudGVyMg==`) are NOT redacted. This is an intentional threshold
  trade-off to avoid false positives on base64-encoded non-secret data.

- **curl/jq/openssl bypass:** The LLM can construct a `curl` command that hits the
  Kubernetes API directly using the SA token and receive unredacted JSON. Similarly,
  `openssl rsa`/`openssl pkey` can extract private key material to stdout. None of
  these can be wrapped without breaking `get-github-app-token.sh`. These are known
  residual risks.

- **arm64 wrapper test:** CI runs wrapper tests only on the amd64 image variant. The arm64
  variant is built and pushed but not functionally tested in CI.

- **Post-push test timing:** The CI workflow (`build-agent.yaml`) pushes the image before
  running `wrapper-test.sh`. A broken image (missing `redact`) would already be in GHCR
  before the test fails. The hard-fail guard in every wrapper is the primary runtime
  protection.

## Success Criteria

- [ ] `cmd/redact/main.go` exists and imports `internal/domain.RedactSecrets`
- [ ] `cmd/redact/main_test.go` covers: redaction applied, exit 0, empty input, large input, multi-line input
- [ ] `go test -timeout 30s -race ./cmd/redact/...` passes
- [ ] Wrapper scripts exist for all 12 tools in `docker/scripts/redact-wrappers/`
- [ ] Each wrapper: calls real binary, preserves exit code, uses `trap` for cleanup, hard-fails if `redact` is absent
- [ ] `Dockerfile.agent` has `redact-builder` stage compiling `cmd/redact`
- [ ] `Dockerfile.agent` renames real binaries to `.real` and copies wrappers
- [ ] `docker/scripts/wrapper-test.sh` verifies all 12 wrappers filter known-secret output
- [ ] `docker/scripts/wrapper-test.sh` verifies exit code passthrough for each wrapper
- [ ] `docker/scripts/smoke-test.sh` updated to verify wrapper presence and `redact` binary
- [ ] `build-agent.yaml` runs wrapper-test after build
- [ ] Security finding documented in pentest report and phase03 addendum

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| `cmd/redact` filter binary | [STORY_01_redact_binary.md](STORY_01_redact_binary.md) | Not Started | Critical | 2h |
| Shell wrapper scripts | [STORY_02_wrapper_scripts.md](STORY_02_wrapper_scripts.md) | Not Started | Critical | 3h |
| Dockerfile.agent integration | [STORY_03_dockerfile_integration.md](STORY_03_dockerfile_integration.md) | Not Started | Critical | 2h |
| Wrapper integration tests | [STORY_04_integration_tests.md](STORY_04_integration_tests.md) | Not Started | High | 2h |
| Security documentation | [STORY_05_security_docs.md](STORY_05_security_docs.md) | Not Started | Medium | 1h |
