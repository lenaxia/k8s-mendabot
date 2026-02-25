# Story 04: Agent Image — ArgoCD CLI and Smoke-Test Update

**Epic:** [epic24-gitops-abstraction](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator running ArgoCD**, I want the `argocd` CLI to be available inside
the agent Job container so that the agent can run ArgoCD-specific diagnostics
(`argocd app get`, `argocd app diff`, etc.) during investigation.

---

## Background

`docker/Dockerfile.agent` currently installs `flux` (lines 65–72) but not `argocd`.
When `GITOPS_TOOL=argocd`, the agent's Step 5 prompt instructs it to run
`kubectl get applications` and other ArgoCD diagnostics. The ArgoCD CLI (`argocd`) is
available as a standalone binary from GitHub releases and follows the same install
pattern as the other binaries in the image.

`docker/scripts/smoke-test.sh` asserts that specific binaries are present. After this
story, `argocd` must be added to the assertion list.

This story is **independent of STORY_01–03** — it requires no Go changes and no changes
to config or CRD types.

---

## Design

### ArgoCD CLI install block in `docker/Dockerfile.agent`

ArgoCD releases provide individual binaries (not tarballs) for `linux/amd64` and
`linux/arm64`, with a `cli_checksums.txt` file per release. The install pattern mirrors
how `talosctl` and `yq` are installed.

Latest stable release at time of writing: **v3.3.2**

Binary SHA256 digests (from `cli_checksums.txt`):
- `argocd-linux-amd64`: `7820a7fa23dc0f57b5550c739b4fa5a69f3521765ce20a3559d83b27f3180488`
- `argocd-linux-arm64`: `1cf3637199ff09eddc674655e1358dd82ba42288d09a331150cfbacb808937d4`

Add to the `ARG` block at the top of the Dockerfile (after `FLUX_VERSION`):

```dockerfile
ARG ARGOCD_VERSION=3.3.2
ARG ARGOCD_SHA256_AMD64=7820a7fa23dc0f57b5550c739b4fa5a69f3521765ce20a3559d83b27f3180488
ARG ARGOCD_SHA256_ARM64=1cf3637199ff09eddc674655e1358dd82ba42288d09a331150cfbacb808937d4
```

Add an install block immediately after the `flux` block (line 72), following the
`yq`/`age` pattern of embedding SHA256 per arch directly (upstream provides a
`cli_checksums.txt` but its format uses full paths and may change between releases; the
pinned-value pattern is safer and consistent with other binaries in this Dockerfile):

```dockerfile
# argocd CLI — embed SHA256 per arch; single binary release (no tarball)
RUN EXPECTED=$([ "$TARGETARCH" = "amd64" ] && echo "${ARGOCD_SHA256_AMD64}" || echo "${ARGOCD_SHA256_ARM64}") \
    && curl -fsSL --retry 3 --retry-delay 5 \
        "https://github.com/argoproj/argo-cd/releases/download/v${ARGOCD_VERSION}/argocd-linux-${TARGETARCH}" \
        -o /usr/local/bin/argocd \
    && echo "${EXPECTED}  /usr/local/bin/argocd" | sha256sum --check \
    && chmod +x /usr/local/bin/argocd
```

### Smoke test update (`docker/scripts/smoke-test.sh`)

Add `check_binary argocd` immediately after `check_binary flux` (line 47):

```bash
check_binary flux
check_binary argocd
```

### Image size note

The ArgoCD CLI binary for linux/amd64 is approximately 215 MB uncompressed. This is
large. The decision to include it (per user preference expressed during epic design)
is that the image ships all supported GitOps tool CLIs and the agent picks the right
one based on `GITOPS_TOOL`. Image size optimisation (build variants, slim images) is
deferred to a later epic.

---

## Files to modify

| File | Change |
|------|--------|
| `docker/Dockerfile.agent` | Add `ARGOCD_VERSION`, `ARGOCD_SHA256_AMD64`, `ARGOCD_SHA256_ARM64` ARGs; add `argocd` install `RUN` block |
| `docker/scripts/smoke-test.sh` | Add `check_binary argocd` |

No Go changes. No Helm chart changes. No config changes.

---

## Acceptance Criteria

- [ ] `docker/Dockerfile.agent` has `ARG ARGOCD_VERSION`, `ARG ARGOCD_SHA256_AMD64`, `ARG ARGOCD_SHA256_ARM64` in the ARG block
- [ ] The `argocd` install block uses the per-arch SHA256 embed pattern (same as `yq` and `age`)
- [ ] SHA256 verification passes before the binary is made executable
- [ ] `chmod +x /usr/local/bin/argocd` is applied
- [ ] The binary is installed to `/usr/local/bin/argocd`
- [ ] `docker/scripts/smoke-test.sh` includes `check_binary argocd`
- [ ] `docker build -f docker/Dockerfile.agent .` completes without error (when network available)
- [ ] `docker run --rm <image> argocd version --client` exits 0

---

## Tasks

- [ ] Add three ARG lines for ArgoCD version and checksums to `Dockerfile.agent`
- [ ] Add the `argocd` install RUN block after the `flux` block
- [ ] Add `check_binary argocd` to `smoke-test.sh`
- [ ] Verify the binary URL format by checking the GitHub release assets:
      `https://github.com/argoproj/argo-cd/releases/tag/v3.3.2`
- [ ] When updating the version in future: fetch new SHA256 values from
      `https://github.com/argoproj/argo-cd/releases/download/v<VER>/cli_checksums.txt`
      and update all three ARG values

---

## Version pinning note

The three ARGs must be updated together when bumping the ArgoCD version:
1. `ARGOCD_VERSION` — the release tag (without `v` prefix)
2. `ARGOCD_SHA256_AMD64` — from `cli_checksums.txt` for `argocd-linux-amd64`
3. `ARGOCD_SHA256_ARM64` — from `cli_checksums.txt` for `argocd-linux-arm64`

The `cli_checksums.txt` URL is:
`https://github.com/argoproj/argo-cd/releases/download/v<VER>/cli_checksums.txt`

---

## Dependencies

**Depends on:** Nothing (fully independent of STORY_01–03)
**Blocks:** Nothing

---

## Definition of Done

- [ ] All acceptance criteria satisfied
- [ ] `go test -timeout 30s -race ./...` still passes (no Go changes in this story)
- [ ] `go build ./...` still builds (no Go changes)
