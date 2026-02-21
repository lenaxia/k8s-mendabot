# Epic: Agent Image

## Purpose

Build the `mendabot-agent` Docker image — a self-contained investigation environment
containing every CLI tool the OpenCode agent needs to inspect a cluster, read a GitOps
repo, and open a pull request.

## Status: Complete

## Dependencies

- Foundation epic complete (repo structure and CI skeleton exist)

## Blocks

- Deploy epic (image must exist before Deployment manifests reference it)
- CI/CD epic (image build workflow targets this Dockerfile)

## Success Criteria

- [ ] `docker build -f docker/Dockerfile.agent .` succeeds
- [ ] All tools present and callable: opencode, kubectl, k8sgpt, helm, flux, talosctl,
  kustomize, gh, git, jq, yq, kubeconform, stern, age, sops, curl, openssl
- [ ] `get-github-app-token.sh` is present, executable, and handles missing env vars
  with a clear error message
- [ ] Image runs as non-root (uid=1000)
- [ ] Image builds for both `linux/amd64` and `linux/arm64` via `docker buildx`
- [ ] Smoke test script verifies all binaries in CI
- [ ] All tool versions are pinned as `ARG` at the top of the Dockerfile

## Stories

| Story | File | Status |
|-------|------|--------|
| Dockerfile base and system packages | [STORY_01_base_image.md](STORY_01_base_image.md) | Complete |
| Install kubectl, k8sgpt, helm, flux, talosctl | [STORY_02_k8s_tools.md](STORY_02_k8s_tools.md) | Complete |
| Install kustomize, yq, jq, kubeconform | [STORY_03_yaml_tools.md](STORY_03_yaml_tools.md) | Complete |
| Install stern, age, sops | [STORY_04_misc_tools.md](STORY_04_misc_tools.md) | Complete |
| Install gh CLI and git | [STORY_05_github_tools.md](STORY_05_github_tools.md) | Complete |
| Install opencode | [STORY_06_opencode.md](STORY_06_opencode.md) | Complete |
| get-github-app-token.sh script | [STORY_07_token_script.md](STORY_07_token_script.md) | Complete |
| Non-root user and entrypoint | [STORY_08_entrypoint.md](STORY_08_entrypoint.md) | Complete |
| Multi-arch build verification | [STORY_09_multiarch.md](STORY_09_multiarch.md) | Complete |
| Smoke test script | [STORY_10_smoke_test.md](STORY_10_smoke_test.md) | Complete |

## Technical Overview

The Dockerfile is in `docker/Dockerfile.agent`. All tool versions are defined as `ARG`
at the top so they can be bumped in one place and tracked in git history.

The image must be built for both `amd64` and `arm64` — the Talos cluster may run on
either architecture.

See [`docs/DESIGN/lld/AGENT_IMAGE_LLD.md`](../../DESIGN/lld/AGENT_IMAGE_LLD.md) for the
complete tool inventory and Dockerfile spec.

## Definition of Done

- [ ] `docker build` succeeds
- [ ] All binaries callable
- [ ] Smoke test passes in CI
- [ ] Image pushes to ghcr.io successfully
- [ ] Multi-arch manifest present in registry
