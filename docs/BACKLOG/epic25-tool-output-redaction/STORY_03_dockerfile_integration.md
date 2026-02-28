# Story 03: Dockerfile.agent Integration

**Epic:** [epic25-tool-output-redaction](README.md)
**Priority:** Critical
**Status:** Complete
**Depends on:** STORY_01 (`cmd/redact/main.go` must exist before `go build ./cmd/redact`
can succeed), STORY_02 (wrapper scripts must exist before `COPY docker/scripts/redact-wrappers/...`)

---

## User Story

As a **mechanic operator**, I want the agent Docker image to be built with the `redact`
binary and all wrapper scripts in place, so that every container started from the image
has output redaction active without any runtime configuration.

---

## Acceptance Criteria

- [ ] `Dockerfile.agent` has a `redact-builder` stage that compiles `cmd/redact`
- [ ] `redact` binary is copied to `/usr/local/bin/redact` in the runtime image
- [ ] All 13 wrapper scripts are copied to `/usr/local/bin/<tool>` (replacing the
      original binary at that path)
- [ ] All original `/usr/local/bin/<tool>` binaries are renamed to `<tool>.real`
      (except `gh` which is not at `/usr/local/bin/` and needs no rename)
- [ ] A `RUN test -x /usr/bin/gh` assertion appears after the `gh` apt install to
      fail the build fast if `gh` ever moves to a different path
- [ ] The `ENV PATH` in the Dockerfile is **not changed** — `/usr/local/bin` is already
      at position 2 in the default path and wrappers are placed there

---

## Technical Implementation

### New build stage: `redact-builder`

Add immediately **after** the `age-builder` stage and **before** the
`# ── Runtime image ──` comment line (i.e., between the last line of `age-builder`
and the `FROM debian:bookworm-slim` runtime stage):

```dockerfile
# ── redact build stage — compiles cmd/redact filter binary ──────────────────
FROM golang:1.25.7-bookworm@sha256:0b5f101af6e4f905da4e1c5885a76b1e7a9fbc97b1a41d971dbcab7be16e70a1 AS redact-builder
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p /out \
    && CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
       go build -trimpath -o /out/redact ./cmd/redact
```

Uses the **exact same Go image digest** as the existing `age-builder` stage
(`sha256:0b5f101af6e4f905da4e1c5885a76b1e7a9fbc97b1a41d971dbcab7be16e70a1`) — no new
layer pull. COPY follows the `go.mod`/`go.sum` first (layer cache), then full source
pattern.

### gh install assertion

Add a `RUN test -x /usr/bin/gh` immediately after the existing `gh` apt install block
(after `rm -rf /var/lib/apt/lists/*` on the gh install layer). This catches any future
change to the gh package install location at build time:

```dockerfile
# Verify gh is at the expected absolute path used by the gh wrapper script
RUN test -x /usr/bin/gh || (echo "[ERROR] gh not found at /usr/bin/gh" >&2 && exit 1)
```

#### Exact insertion point

The rename+copy block is inserted between the opencode install block and the
`# Non-root user` block.

This ensures:
- All tool binaries are already present (the `COPY --from=age-builder` instructions
  and all apt/curl installs are complete)
- `mv` and `chmod` run as root (before the `USER agent` instruction)

Concretely, insert **after**:
```dockerfile
    && tar -xz -C /usr/local/bin -f /tmp/opencode.tar.gz opencode \
    && rm /tmp/opencode.tar.gz
```

And **before**:
```dockerfile
# Non-root user
RUN useradd -u 1000 -m -s /bin/bash agent
```

#### Rename + copy block

```dockerfile
# ── Redaction wrappers ────────────────────────────────────────────────────────
# Rename real binaries so wrappers can shadow them at the original PATH entry.
# gh: installed by apt to /usr/bin/gh — not renamed here; wrapper at /usr/local/bin/gh
#   calls /usr/bin/gh directly (verified by test -x /usr/bin/gh assertion above).
# age/age-keygen: compiled from source via age-builder COPY — rename happens here,
#   after all COPY --from=age-builder instructions have run.
RUN mv /usr/local/bin/kubectl       /usr/local/bin/kubectl.real       \
    && mv /usr/local/bin/helm        /usr/local/bin/helm.real          \
    && mv /usr/local/bin/flux        /usr/local/bin/flux.real          \
    && mv /usr/local/bin/sops        /usr/local/bin/sops.real          \
    && mv /usr/local/bin/talosctl    /usr/local/bin/talosctl.real      \
    && mv /usr/local/bin/yq          /usr/local/bin/yq.real            \
    && mv /usr/local/bin/stern       /usr/local/bin/stern.real         \
    && mv /usr/local/bin/kubeconform /usr/local/bin/kubeconform.real   \
    && mv /usr/local/bin/kustomize   /usr/local/bin/kustomize.real     \
    && mv /usr/local/bin/age         /usr/local/bin/age.real           \
    && mv /usr/local/bin/age-keygen  /usr/local/bin/age-keygen.real

COPY --chmod=755 --from=redact-builder /out/redact          /usr/local/bin/redact
COPY --chmod=755 docker/scripts/redact-wrappers/kubectl     /usr/local/bin/kubectl
COPY --chmod=755 docker/scripts/redact-wrappers/helm        /usr/local/bin/helm
COPY --chmod=755 docker/scripts/redact-wrappers/flux        /usr/local/bin/flux
COPY --chmod=755 docker/scripts/redact-wrappers/gh          /usr/local/bin/gh
COPY --chmod=755 docker/scripts/redact-wrappers/sops        /usr/local/bin/sops
COPY --chmod=755 docker/scripts/redact-wrappers/talosctl    /usr/local/bin/talosctl
COPY --chmod=755 docker/scripts/redact-wrappers/yq          /usr/local/bin/yq
COPY --chmod=755 docker/scripts/redact-wrappers/stern       /usr/local/bin/stern
COPY --chmod=755 docker/scripts/redact-wrappers/kubeconform /usr/local/bin/kubeconform
COPY --chmod=755 docker/scripts/redact-wrappers/kustomize   /usr/local/bin/kustomize
COPY --chmod=755 docker/scripts/redact-wrappers/age         /usr/local/bin/age
COPY --chmod=755 docker/scripts/redact-wrappers/age-keygen  /usr/local/bin/age-keygen
```

`--chmod=755` on each `COPY` sets the executable bit atomically — no separate
`chmod +x` RUN layer is needed. This also means wrapper scripts do not need to be
committed with `+x` in the repository.

**Why `age`/`age-keygen` renames happen here and not in the `age-builder` COPY:** The
`COPY --from=age-builder` instructions copy the binaries directly to `/usr/local/bin/age`
and `/usr/local/bin/age-keygen`. The rename `RUN` layer runs after all
`COPY --from=age-builder` instructions and renames the live files in the runtime layer.

---

## Dependency chain note

`entrypoint-common.sh` calls `kubectl config set-cluster ...` under `set -euo pipefail`.
After the wrappers are installed, every `kubectl` call routes through the wrapper, which
contains a hard-fail guard at the top:

```bash
if ! command -v redact > /dev/null 2>&1; then
    echo "[ERROR] redact binary not found in PATH — aborting to prevent unredacted output" >&2
    exit 1
fi
```

If `redact` is absent from PATH entirely (e.g. the `COPY --from=redact-builder` layer
was skipped in a broken build), every wrapper exits 1. Under `set -euo pipefail` in
`entrypoint-common.sh`, the first `kubectl config set-cluster` call aborts the entire
entrypoint with a clear error message. The container fails fast with a visible error —
the operator sees the problem immediately rather than getting silent empty output.

`wrapper-test.sh` (STORY_04) verifies `redact` is present and executable in a post-build
CI step. Note: the CI workflow (`build-agent.yaml`) pushes the image before running the
wrapper test — a broken image would already be in GHCR before the test fails. The
hard-fail guard is therefore the primary runtime protection; the CI test is secondary
verification.

## Definition of Done

- [x] `redact-builder` stage added with same Go image digest as existing `age-builder`
      (`sha256:0b5f101af6e4f905da4e1c5885a76b1e7a9fbc97b1a41d971dbcab7be16e70a1`)
- [x] `redact-builder` stage inserted **after** `age-builder` and **before** the
      `# ── Runtime image ──` comment / `FROM debian:bookworm-slim` line
- [x] `RUN test -x /usr/bin/gh` assertion added after `gh` apt install block
- [x] Rename+copy block inserted **after** `&& rm /tmp/opencode.tar.gz` and
      **before** `# Non-root user` / `RUN useradd ...`
- [x] All 11 `.real` renames in one `RUN` layer: kubectl, helm, flux, sops,
      talosctl, yq, stern, kubeconform, kustomize, age, age-keygen
- [x] `COPY --chmod=755 --from=redact-builder` copies `redact` to `/usr/local/bin/redact`
- [x] All 12 wrapper scripts copied with `--chmod=755` to `/usr/local/bin/`
      (11 above + gh; gh wrapper calls `/usr/bin/gh` by absolute path)
- [x] No separate `RUN chmod +x` layer needed (`--chmod=755` on COPY handles it)
- [x] `docker build -f docker/Dockerfile.agent .` succeeds (no build errors)
- [x] `docker run --rm <image> kubectl version --client` outputs redacted/clean text
      (not the raw binary path error — confirms wrapper is called, not original binary)
