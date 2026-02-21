# Domain: Watcher Image — Low-Level Design

**Version:** 1.0
**Date:** 2026-02-20
**Status:** Implementation Ready
**HLD Reference:** [Section 4.1](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

The `mendabot-watcher` Docker image packages the Go controller binary that watches
`Result` CRDs and dispatches agent Jobs. It is a minimal, single-binary image with no
additional tools — the watcher does not run any cluster inspection commands itself.

### 1.2 Design Principles

- **Multi-stage build** — Go compilation in a builder stage; only the compiled binary
  is copied to the final image
- **Debian-slim base** — consistent with the agent image; not Alpine
- **Non-root user** — the watcher runs as `uid=1000` (`watcher` user)
- **No extra tooling** — the final image contains only the watcher binary, CA
  certificates, and the minimum libc required to run a Go binary compiled with CGO
  disabled
- **Read-only root filesystem** — no writes to the container filesystem at runtime;
  compatible with `readOnlyRootFilesystem: true` in the pod spec (already enforced in
  `DEPLOY_LLD.md §8`)
- **Pinned base image tag** — `debian:bookworm-slim` uses a floating tag at development
  time; pin to a digest for production builds
- **CGO disabled** — `CGO_ENABLED=0` produces a fully static binary that does not need
  any dynamic libraries from the base image

---

## 2. Build Arguments

| ARG | Default | Purpose |
|---|---|---|
| `GO_VERSION` | `1.23` | Go toolchain version used in the builder stage |
| `TARGETARCH` | `amd64` | Target CPU architecture (`amd64` or `arm64`) |
| `WATCHER_VERSION` | `dev` | Embedded at build time via `ldflags`; used in `--version` output |

---

## 3. Dockerfile

File: `docker/Dockerfile.watcher`

```dockerfile
# syntax=docker/dockerfile:1

# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.23-bookworm AS builder

ARG TARGETARCH=amd64
ARG WATCHER_VERSION=dev

WORKDIR /src

# Cache dependency downloads separately from source compilation.
# Copy go.mod and go.sum first; only re-download if they change.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
# CGO_ENABLED=0  → fully static binary, no libc dependency in the final image
# -trimpath      → remove local build paths from the binary (reproducibility)
# -ldflags       → strip debug info (-s -w) and embed version string
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.Version=${WATCHER_VERSION}" \
      -o /out/watcher \
      ./cmd/watcher

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

# Install only CA certificates (required for the in-cluster API server TLS connection
# and for calling the Kubernetes API using the ServiceAccount token).
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Non-root user
RUN useradd -u 1000 -m -s /bin/sh watcher

# Copy only the compiled binary from the builder stage
COPY --from=builder /out/watcher /usr/local/bin/watcher

USER watcher

ENTRYPOINT ["/usr/local/bin/watcher"]
```

---

## 4. Why debian:bookworm-slim and not distroless

`distroless/static` would also work for a fully static Go binary and would give a smaller
attack surface. `debian:bookworm-slim` is chosen for consistency with the agent image and
because it simplifies debugging in development (a shell is available via `kubectl exec` if
needed). Operators who require a minimal attack surface in production should pin to the
digest of `gcr.io/distroless/static-debian12` and swap it in place of the runtime base.

---

## 5. cmd/watcher Entry Point

The binary lives at `cmd/watcher/main.go`. Its `main()` function:

1. Parses environment variables (no flags — configuration is entirely via env)
2. Creates a `controller-runtime` manager
3. Registers the `RemediationJobReconciler` and one `SourceProviderReconciler` per
   enabled provider (v1: `K8sGPTProvider`)
4. Starts the manager (blocking; handles SIGTERM gracefully)

The binary exposes two HTTP endpoints on separate ports:

| Port | Endpoint | Purpose |
|---|---|---|
| `8080` | `/metrics` | Prometheus metrics (controller-runtime default) |
| `8081` | `/healthz` | Liveness probe |
| `8081` | `/readyz` | Readiness probe |

These match the `containerPort` and probe config in `DEPLOY_LLD.md §8`.

---

## 6. Image Tagging Strategy

Identical to the agent image:

| Tag | Meaning |
|---|---|
| `latest` | Most recent build from `main` branch |
| `sha-<7-char-commit>` | Immutable reference to a specific build |
| `v<semver>` | Tagged release |

The Deployment in `DEPLOY_LLD.md §8` references `ghcr.io/lenaxia/mendabot-watcher:latest`.
Production deployments should pin to `sha-<commit>` or a semver tag.

---

## 7. Multi-Architecture

The Dockerfile uses `ARG TARGETARCH` so `docker buildx` can produce `linux/amd64` and
`linux/arm64` images in a single build manifest. The GitHub Actions workflow uses
`docker/build-push-action` with `platforms: linux/amd64,linux/arm64`.

Go's cross-compilation is handled entirely by `GOOS=linux GOARCH=${TARGETARCH}` — no
extra toolchain is required in the builder stage.

---

## 8. Build Verification

After each image build, a smoke test step in CI runs:

```bash
# Binary is present and executable; --version flag must be implemented in main()
# (controller-runtime does not add --version automatically — main() must check
# os.Args for "--version", print the Version variable, and os.Exit(0)).
# Note: ENTRYPOINT is /usr/local/bin/watcher, so --version is passed as CMD arg.
docker run --rm ghcr.io/lenaxia/mendabot-watcher:<tag> --version

# Binary exits cleanly without a cluster (expect a non-zero exit from the controller
# failing to connect, but the binary itself must start and print startup logs)
docker run --rm \
  -e GITOPS_REPO=owner/repo \
  -e GITOPS_MANIFEST_ROOT=kubernetes \
  -e AGENT_IMAGE=ghcr.io/lenaxia/mendabot-agent:latest \
  -e AGENT_NAMESPACE=mendabot \
  -e AGENT_SA=mendabot-agent \
  ghcr.io/lenaxia/mendabot-watcher:<tag> \
  2>&1 | grep -q "starting manager"

# Image runs as non-root
docker run --rm --entrypoint id ghcr.io/lenaxia/mendabot-watcher:<tag> \
  | grep -q "uid=1000"
```

---

## 9. Security Context (Runtime)

The Deployment spec in `DEPLOY_LLD.md §8` already enforces the following on the watcher
container:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
```

The Pod-level security context additionally sets:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  seccompProfile:
    type: RuntimeDefault
```

The watcher binary must not write to the container filesystem at runtime. All
controller-runtime state (leader election lease, etc.) is stored in the cluster via the
Kubernetes API, not on disk.

**Temporary directory caveat:** controller-runtime does not write to the filesystem by
default. If any dependency requires a writable temp directory, an `emptyDir` volume must
be added to the Pod spec and mounted at `/tmp`. This is not expected to be needed but is
noted here for implementers.

---

## 10. Version Embedding

`WATCHER_VERSION` is injected at build time via `ldflags`:

```
-X main.Version=${WATCHER_VERSION}
```

`cmd/watcher/main.go` declares:

```go
var Version = "dev"  // overwritten at build time by ldflags
```

The GitHub Actions workflow sets `WATCHER_VERSION` to the short commit SHA:

```yaml
build-args: |
  WATCHER_VERSION=sha-${{ github.sha }}
  TARGETARCH=${{ matrix.arch }}
```

This value is printed on startup:

```
{"level":"info","msg":"mendabot-watcher starting","version":"sha-a3f9c2b"}
```
