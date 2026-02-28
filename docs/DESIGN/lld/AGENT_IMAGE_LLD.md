# Domain: Agent Image — Low-Level Design

**Version:** 1.1
**Date:** 2026-02-19
**Status:** Implementation Ready
**HLD Reference:** [Sections 4.3, 10](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

The `mechanic-agent` Docker image is a self-contained investigation environment. It contains
every CLI tool the OpenCode agent needs to inspect a Kubernetes cluster, read and modify a
Flux/Helm GitOps repository, and open a pull request — all from inside a Kubernetes Job.

### 1.2 Design Principles

- **Debian-slim base** — stable, predictable, rich `apt` ecosystem; not Alpine
- **Pinned tool versions** — every binary is fetched at a specific version, not `latest`,
  to keep builds reproducible. Versions are defined as `ARG` at the top of the Dockerfile
- **SHA256 checksum verification** — every binary download is verified before use to
  guard against CDN compromise or MITM attacks. The agent image has cluster-wide read
  access and GitHub credentials; a compromised binary is a full system compromise
- **Base image tag** — `debian:bookworm-slim` uses a floating tag; pin to a digest
  (`FROM debian:bookworm-slim@sha256:<digest>`) when reproducibility is critical
- **No secrets baked in** — all credentials come from the environment at runtime
- **Small image** — install only what is needed; clean up apt caches in the same layer
- **Non-root user** — the agent runs as `uid=1000` (`agent` user)

---

## 2. Tool Inventory

| Tool | Purpose | Install method |
|---|---|---|
| `opencode` | AI agent driver — runs the investigation prompt | Official release binary (pinned version) |
| `kubectl` | Cluster inspection (`describe`, `get`, `logs`, `events`) | Official release binary |
| `k8sgpt` | Deeper cluster analysis (`analyze`, `explain`) | Official release binary |
| `helm` | Read chart metadata, render templates locally | Official release binary |
| `flux` | Flux CLI — reconcile status, trace, diff | Official release binary |
| `talosctl` | Talos node inspection (`logs`, `dmesg`, `service`) — requires talosconfig mounted separately | Official release binary |
| `kustomize` | Render and diff Kustomize overlays in the repo | Official release binary |
| `gh` | GitHub CLI — PR create, list, comment | GitHub apt repository |
| `git` | Clone, branch, commit, push | apt |
| `jq` | JSON processing in shell scripts | apt |
| `yq` | YAML processing — read and patch YAML files | Official release binary |
| `kubeconform` | Validate Kubernetes manifests against schemas | Official release binary |
| `stern` | Multi-pod log tailing across replicas | Official release binary |
| `age` | Decrypt age-encrypted files — requires key material mounted separately | Official release binary |
| `sops` | Decrypt SOPS-encrypted secrets — requires key material mounted separately | Official release binary |
| `curl` | HTTP requests (used by get-github-app-token.sh) | apt |
| `openssl` | JWT signing in get-github-app-token.sh | apt |
| `bash` | Shell for scripts | apt |
| `ca-certificates` | TLS trust for HTTPS calls | apt |
| `envsubst` | Variable substitution in prompt template | apt (gettext-base) |

---

## 3. Dockerfile

File: `docker/Dockerfile.agent`

**Checksum verification:** Every binary download must be verified against a SHA256 checksum
before use. The pattern for each binary is:
1. Download the binary
2. Download the corresponding checksum file (e.g. `SHA256SUMS`, `checksums.txt`, or `<binary>.sha256`)
3. Run `sha256sum --check` or `echo "<expected_hash>  <file>" | sha256sum --check`
4. Only then `chmod +x` and proceed

The Dockerfile below shows the pattern for `kubectl` in full. All other binaries follow the
same pattern — the checksum URL format varies per project but the verification step is
non-optional. Implementers must look up the checksum URL for each tool version.

```dockerfile
FROM debian:bookworm-slim

# Tool versions — bump these to upgrade
ARG KUBECTL_VERSION=1.32.3
ARG K8SGPT_VERSION=0.4.28
ARG HELM_VERSION=3.17.2
ARG FLUX_VERSION=2.5.1
ARG TALOSCTL_VERSION=1.9.4
ARG KUSTOMIZE_VERSION=5.6.0
ARG YQ_VERSION=4.45.1
ARG KUBECONFORM_VERSION=0.6.7
ARG STERN_VERSION=1.31.0
ARG AGE_VERSION=1.2.1
ARG SOPS_VERSION=3.9.4
ARG OPENCODE_VERSION=0.1.0
ARG TARGETARCH=amd64

ENV DEBIAN_FRONTEND=noninteractive

# Base system packages (includes gettext-base for envsubst)
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    gettext-base \
    git \
    gnupg \
    jq \
    openssl \
    unzip \
    && rm -rf /var/lib/apt/lists/*

# gh CLI (GitHub's official apt repo — GPG-signed, no manual checksum needed)
# Note: use `install -m 0644 /dev/stdin` instead of `dd` for portability
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
      | install -m 0644 /dev/stdin /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
      > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# kubectl — with SHA256 verification
RUN curl -fsSL "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
      -o /usr/local/bin/kubectl \
    && curl -fsSL "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" \
      -o /tmp/kubectl.sha256 \
    && echo "$(cat /tmp/kubectl.sha256)  /usr/local/bin/kubectl" | sha256sum --check \
    && rm /tmp/kubectl.sha256 \
    && chmod +x /usr/local/bin/kubectl

# k8sgpt — with SHA256 verification (checksums.txt from GitHub releases)
RUN curl -fsSL "https://github.com/k8sgpt-ai/k8sgpt/releases/download/v${K8SGPT_VERSION}/k8sgpt_linux_${TARGETARCH}" \
      -o /usr/local/bin/k8sgpt \
    && curl -fsSL "https://github.com/k8sgpt-ai/k8sgpt/releases/download/v${K8SGPT_VERSION}/checksums.txt" \
      | grep "k8sgpt_linux_${TARGETARCH}$" | sha256sum --check \
    && chmod +x /usr/local/bin/k8sgpt

# helm — tarball; verify checksum from get.helm.sh
RUN curl -fsSL "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
      -o /tmp/helm.tar.gz \
    && curl -fsSL "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz.sha256sum" \
      | sha256sum --check \
    && tar -xz -C /usr/local/bin --strip-components=1 -f /tmp/helm.tar.gz "linux-${TARGETARCH}/helm" \
    && rm /tmp/helm.tar.gz

# flux — with checksums.txt
RUN curl -fsSL "https://github.com/fluxcd/flux2/releases/download/v${FLUX_VERSION}/flux_${FLUX_VERSION}_linux_${TARGETARCH}.tar.gz" \
      -o /tmp/flux.tar.gz \
    && curl -fsSL "https://github.com/fluxcd/flux2/releases/download/v${FLUX_VERSION}/flux_${FLUX_VERSION}_checksums.txt" \
      | grep "flux_${FLUX_VERSION}_linux_${TARGETARCH}.tar.gz$" | sha256sum --check \
    && tar -xz -C /usr/local/bin -f /tmp/flux.tar.gz flux \
    && rm /tmp/flux.tar.gz

# talosctl — with SHA256 sidecar file
RUN curl -fsSL "https://github.com/siderolabs/talos/releases/download/v${TALOSCTL_VERSION}/talosctl-linux-${TARGETARCH}" \
      -o /usr/local/bin/talosctl \
    && curl -fsSL "https://github.com/siderolabs/talos/releases/download/v${TALOSCTL_VERSION}/talosctl-linux-${TARGETARCH}.sha256sum" \
      | sha256sum --check \
    && chmod +x /usr/local/bin/talosctl

# kustomize — with checksums from GitHub releases
RUN curl -fsSL "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/v${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz" \
      -o /tmp/kustomize.tar.gz \
    && curl -fsSL "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/v${KUSTOMIZE_VERSION}/checksums.txt" \
      | grep "kustomize_v${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz$" | sha256sum --check \
    && tar -xz -C /usr/local/bin -f /tmp/kustomize.tar.gz kustomize \
    && rm /tmp/kustomize.tar.gz

# yq — with SHA256 sidecar
RUN curl -fsSL "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_linux_${TARGETARCH}" \
      -o /usr/local/bin/yq \
    && curl -fsSL "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/checksums" \
      | grep "yq_linux_${TARGETARCH} " | awk '{print $1 "  /usr/local/bin/yq"}' | sha256sum --check \
    && chmod +x /usr/local/bin/yq

# kubeconform — with checksums
RUN curl -fsSL "https://github.com/yannh/kubeconform/releases/download/v${KUBECONFORM_VERSION}/kubeconform-linux-${TARGETARCH}.tar.gz" \
      -o /tmp/kubeconform.tar.gz \
    && curl -fsSL "https://github.com/yannh/kubeconform/releases/download/v${KUBECONFORM_VERSION}/checksums.txt" \
      | grep "kubeconform-linux-${TARGETARCH}.tar.gz$" | sha256sum --check \
    && tar -xz -C /usr/local/bin -f /tmp/kubeconform.tar.gz kubeconform \
    && rm /tmp/kubeconform.tar.gz

# stern — with checksums
RUN curl -fsSL "https://github.com/stern/stern/releases/download/v${STERN_VERSION}/stern_${STERN_VERSION}_linux_${TARGETARCH}.tar.gz" \
      -o /tmp/stern.tar.gz \
    && curl -fsSL "https://github.com/stern/stern/releases/download/v${STERN_VERSION}/checksums.txt" \
      | grep "stern_${STERN_VERSION}_linux_${TARGETARCH}.tar.gz$" | sha256sum --check \
    && tar -xz -C /usr/local/bin -f /tmp/stern.tar.gz stern \
    && rm /tmp/stern.tar.gz

# age — with checksums
RUN curl -fsSL "https://github.com/FiloSottile/age/releases/download/v${AGE_VERSION}/age-v${AGE_VERSION}-linux-${TARGETARCH}.tar.gz" \
      -o /tmp/age.tar.gz \
    && curl -fsSL "https://github.com/FiloSottile/age/releases/download/v${AGE_VERSION}/checksums.txt" \
      | grep "age-v${AGE_VERSION}-linux-${TARGETARCH}.tar.gz$" | sha256sum --check \
    && tar -xz -C /usr/local/bin --strip-components=1 -f /tmp/age.tar.gz "age/age" "age/age-keygen" \
    && rm /tmp/age.tar.gz

# sops — with SHA256 sidecar
RUN curl -fsSL "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.linux.${TARGETARCH}" \
      -o /usr/local/bin/sops \
    && curl -fsSL "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.checksums.txt" \
      | grep "sops-v${SOPS_VERSION}.linux.${TARGETARCH}$" | sha256sum --check \
    && chmod +x /usr/local/bin/sops

# opencode — pinned release binary with checksum verification
# Verify the exact GitHub org/repo, binary naming, and --file flag before first build.
RUN curl -fsSL "https://github.com/opencode-ai/opencode/releases/download/v${OPENCODE_VERSION}/opencode_linux_${TARGETARCH}" \
      -o /usr/local/bin/opencode \
    && curl -fsSL "https://github.com/opencode-ai/opencode/releases/download/v${OPENCODE_VERSION}/checksums.txt" \
      | grep "opencode_linux_${TARGETARCH}$" | sha256sum --check \
    && chmod +x /usr/local/bin/opencode

# Non-root user
RUN useradd -u 1000 -m -s /bin/bash agent

# Git identity for commits made by the agent
# Use a GitHub noreply address format: <app-id>+<slug>@users.noreply.github.com
ENV GIT_AUTHOR_NAME="mechanic-agent"
ENV GIT_AUTHOR_EMAIL="mechanic-agent@users.noreply.github.com"
ENV GIT_COMMITTER_NAME="mechanic-agent"
ENV GIT_COMMITTER_EMAIL="mechanic-agent@users.noreply.github.com"

# GitHub App token helper (writes token to stdout)
COPY scripts/get-github-app-token.sh /usr/local/bin/get-github-app-token.sh
RUN chmod +x /usr/local/bin/get-github-app-token.sh

# Agent entrypoint: runs envsubst on the prompt then calls opencode run
COPY scripts/agent-entrypoint.sh /usr/local/bin/agent-entrypoint.sh
RUN chmod +x /usr/local/bin/agent-entrypoint.sh

USER agent
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/agent-entrypoint.sh"]
```

**Note on opencode install path:** The Dockerfile fetches the pinned release binary
directly from GitHub releases rather than using the `curl | bash` install script. This
ensures reproducible builds — every build of the same `OPENCODE_VERSION` produces the
same binary. The install script URL and binary path must be verified against the actual
release when the version is first set. Update `OPENCODE_VERSION` to upgrade.

**Note on talosctl:** `talosctl` is included for Talos node inspection. It requires a
`talosconfig` file with node endpoints and client certificates. For the agent to use it,
a `talosconfig` Secret must be mounted at runtime and `TALOSCONFIG` env var set. This is
not part of the default Job spec — operators running on Talos must add the mount manually.

**Note on sops/age:** These tools are installed but require key material (an age private
key or SOPS-compatible key) to decrypt. No key is injected by default. The agent prompt
instructs the agent to skip encrypted files. To enable decryption, mount an age key
Secret and set `SOPS_AGE_KEY_FILE` or `SOPS_AGE_KEY`.

---

## 4. get-github-app-token.sh

File: `docker/scripts/get-github-app-token.sh`

Reads `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, and `GITHUB_APP_PRIVATE_KEY` from the
environment. Outputs the installation token to **stdout**. Callers capture it with
`TOKEN=$(get-github-app-token.sh)`.

Used by the init container inline script (which writes it to `/workspace/github-token`)
and can be re-used by the main container or entrypoint script if needed.

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_APP_ID:?GITHUB_APP_ID must be set}"
: "${GITHUB_APP_INSTALLATION_ID:?GITHUB_APP_INSTALLATION_ID must be set}"
: "${GITHUB_APP_PRIVATE_KEY:?GITHUB_APP_PRIVATE_KEY must be set}"

NOW=$(date +%s)
IAT=$((NOW - 60))
EXP=$((NOW + 540))

b64url() { base64 -w0 | tr '+/' '-_' | tr -d '='; }

HEADER=$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)
# iss must be a JSON number (integer), not a string — GitHub rejects string iss values.
PAYLOAD=$(printf '{"iat":%d,"exp":%d,"iss":%d}' "$IAT" "$EXP" "$GITHUB_APP_ID" | b64url)
UNSIGNED="${HEADER}.${PAYLOAD}"

# Write the private key to a temp file to avoid process substitution (<(…)) which
# requires /dev/fd and may not be available in hardened container environments.
KEY_FILE=$(mktemp)
printf '%s' "$GITHUB_APP_PRIVATE_KEY" > "$KEY_FILE"

SIGNATURE=$(printf '%s' "$UNSIGNED" \
  | openssl dgst -sha256 -sign "$KEY_FILE" \
  | b64url)

rm -f "$KEY_FILE"

JWT="${UNSIGNED}.${SIGNATURE}"

curl -sf \
  -X POST \
  -H "Authorization: Bearer ${JWT}" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/app/installations/${GITHUB_APP_INSTALLATION_ID}/access_tokens" \
  | jq -r '.token'
```

---

## 5. Agent Entrypoint at Runtime

File: `docker/scripts/agent-entrypoint.sh`

The image `ENTRYPOINT` is `/usr/local/bin/agent-entrypoint.sh`. The Job's main container
does not override the entrypoint or inject `args` — the script handles everything.

```bash
#!/usr/bin/env bash
set -euo pipefail

# Authenticate gh CLI using the token written by the init container
gh auth login --with-token < /workspace/github-token

# Substitute environment variables into the prompt template.
# envsubst only replaces ${VAR} patterns it knows about. To avoid corrupting
# content in FINDING_ERRORS or FINDING_DETAILS that may contain literal $ signs
# (e.g. from Helm templates or shell variables in log output), we restrict
# envsubst to only the known variable names.
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
envsubst "$VARS" < /prompt/prompt.txt > /tmp/rendered-prompt.txt

# Run opencode with the rendered prompt passed via a temp file to avoid
# shell word-splitting on the prompt content.
exec opencode run --file /tmp/rendered-prompt.txt
```

**Key decisions:**
- `envsubst "$VARS"` restricts substitution to only the named variables. Any `$word`
  patterns in `FINDING_ERRORS` or `FINDING_DETAILS` that are not in the list are left
  unchanged, preventing silent corruption of LLM input.
- `opencode run --file <path>` passes the prompt via a file rather than an inline shell
  argument, avoiding shell word-splitting and quoting issues with arbitrary content.
- `gh auth login` is done here (not in the prompt steps) so the agent can assume `gh`
  is already authenticated when Step 1 runs.

**OpenCode LLM configuration:** OpenCode reads its LLM provider configuration from
environment variables. The following are injected into the Job by the watcher and
consumed by OpenCode at startup:

| Env var | OpenCode config key | Notes |
|---|---|---|
| `OPENAI_API_KEY` | Provider API key | Required |
| `OPENAI_BASE_URL` | Provider base URL | Optional — overrides default OpenAI endpoint |
| `OPENAI_MODEL` | Model name | Optional — uses OpenCode default if unset |

**Verification required before first build:** Run `opencode run --help` against the
version specified by `OPENCODE_VERSION` to confirm the `--file` flag exists. The entire
entrypoint design depends on `opencode run --file <path>` being a valid invocation. If
this flag does not exist in the installed version, the entrypoint script must be
redesigned before implementation proceeds.

---

## 6. Image Tagging Strategy

| Tag | Meaning |
|---|---|
| `latest` | Most recent build from `main` branch |
| `sha-<7-char-commit>` | Immutable reference to a specific build |
| `v<semver>` | Tagged release |

The Deployment manifest references `sha-<commit>` tags in production (managed by Flux image
automation or manual update). `latest` is only used in development.

---

## 7. Multi-Architecture

The Dockerfile uses `ARG TARGETARCH` so `docker buildx` can produce `linux/amd64` and
`linux/arm64` images in a single build manifest. The GitHub Actions workflow uses
`docker/build-push-action` with `platforms: linux/amd64,linux/arm64`.

---

## 8. Build Verification

After each image build, a smoke test step in CI runs:

```bash
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> opencode --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> kubectl version --client
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> k8sgpt version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> helm version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> flux version --client
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> talosctl version --client
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> kustomize version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> yq --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> gh --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> jq --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> sops --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> age --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> stern --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> kubeconform -v
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> envsubst --version
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> test -x /usr/local/bin/agent-entrypoint.sh
docker run --rm ghcr.io/lenaxia/mechanic-agent:<tag> test -x /usr/local/bin/get-github-app-token.sh
```

All binaries must be present and exit 0 (or with `--version` output) for the build to pass.
