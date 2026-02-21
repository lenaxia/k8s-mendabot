# Epic: CI/CD

## Purpose

Set up GitHub Actions workflows to build, test, and push both Docker images to ghcr.io
on every push to main and on tagged releases.

## Status: Not Started

## Dependencies

- Foundation epic complete (go.mod exists, test workflow skeleton exists)
- Agent Image epic complete (Dockerfile.agent exists and builds)

## Blocks

Nothing — CI/CD is the last epic.

## Success Criteria

- [ ] `test.yaml` runs `go test -timeout 30s -race ./...` on every push and PR
- [ ] `build-watcher.yaml` builds and pushes the watcher image to ghcr.io on push to main
- [ ] `build-agent.yaml` builds and pushes the agent image to ghcr.io on push to main
- [ ] Both image workflows tag with `sha-<7-char-commit>` and `latest`
- [ ] Both image workflows produce multi-arch manifests (`linux/amd64`, `linux/arm64`)
- [ ] Smoke test runs inside `build-agent.yaml` to verify all binaries are present
- [ ] Release tags (`v*`) also push a `v<semver>` image tag
- [ ] No secrets are hardcoded — all credentials come from GitHub Actions secrets

## Stories

| Story | File | Status |
|-------|------|--------|
| test.yaml — Go test workflow | [STORY_01_test_workflow.md](STORY_01_test_workflow.md) | Not Started |
| build-watcher.yaml — watcher image workflow | [STORY_02_watcher_image.md](STORY_02_watcher_image.md) | Not Started |
| build-agent.yaml — agent image workflow | [STORY_03_agent_image.md](STORY_03_agent_image.md) | Not Started |
| Release tagging for versioned images | [STORY_04_release_tags.md](STORY_04_release_tags.md) | Not Started |

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

- [ ] All three workflows pass on main
- [ ] Both images visible in ghcr.io package registry
- [ ] Multi-arch manifests confirmed with `docker manifest inspect`
- [ ] No workflow contains hardcoded secrets
