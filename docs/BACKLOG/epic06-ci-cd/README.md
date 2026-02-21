# Epic: CI/CD

## Purpose

Set up GitHub Actions workflows to build, test, and push both Docker images to ghcr.io
on every push to main and on tagged releases.

## Status: Complete

## Dependencies

- Foundation epic complete (go.mod exists, test workflow skeleton exists)
- Agent Image epic complete (Dockerfile.agent exists and builds)

## Blocks

Nothing — CI/CD is the last epic.

## Success Criteria

- [x] `test.yaml` runs `go test -timeout 30s -race ./...` on every push and PR
- [x] `build-watcher.yaml` builds and pushes the watcher image to ghcr.io on push to main
- [x] `build-agent.yaml` builds and pushes the agent image to ghcr.io on push to main
- [x] Both image workflows tag with `sha-<7-char-commit>` and `latest`
- [x] Both image workflows produce multi-arch manifests (`linux/amd64`, `linux/arm64`)
- [x] Smoke test runs inside `build-agent.yaml` to verify all binaries are present
- [x] Release tags (`v*`) also push a `v<semver>` image tag
- [x] No secrets are hardcoded — all credentials come from GitHub Actions secrets

## Stories

| Story | File | Status |
|-------|------|--------|
| test.yaml — Go test workflow | [STORY_01_test_workflow.md](STORY_01_test_workflow.md) | Complete |
| build-watcher.yaml — watcher image workflow | [STORY_02_watcher_image.md](STORY_02_watcher_image.md) | Complete |
| build-agent.yaml — agent image workflow | [STORY_03_agent_image.md](STORY_03_agent_image.md) | Complete |
| Release tagging for versioned images | [STORY_04_release_tags.md](STORY_04_release_tags.md) | Complete |

## Technical Overview

All workflows use:
- `docker/setup-buildx-action` for multi-arch builds
- `docker/login-action` with `GITHUB_TOKEN` for ghcr.io authentication
- `docker/build-push-action` with `platforms: linux/amd64,linux/arm64`
- `docker/metadata-action` for tag generation

The watcher image is a simple Go binary build — `CGO_ENABLED=0`, static binary,
`debian:bookworm-slim` runtime base (consistent with the agent image; see WATCHER_IMAGE_LLD.md §3 and §4).

The agent image is heavier and takes longer to build. Layer caching via
`cache-from: type=gha` and `cache-to: type=gha,mode=max` is important.

## Definition of Done

- [x] All three workflows pass on main
- [x] Both images visible in ghcr.io package registry
- [x] Multi-arch manifests confirmed with `docker manifest inspect`
- [x] No workflow contains hardcoded secrets
