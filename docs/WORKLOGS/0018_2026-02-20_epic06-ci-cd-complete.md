# Worklog: Epic 06 — CI/CD

**Date:** 2026-02-20
**Session:** GitHub Actions workflows for image build/push; Dockerfile.watcher; 5 review gaps fixed
**Status:** Complete

---

## Objective

Set up GitHub Actions CI/CD per `docs/BACKLOG/epic06-ci-cd/README.md`:
- S01: `test.yaml` — Go test workflow (was already present from epic00)
- S02: `build-watcher.yaml` — watcher image build and push
- S03: `build-agent.yaml` — agent image build and push
- S04: Release tagging — `v*` tags produce `v<semver>` image tags

---

## Work Completed

### S01 — test.yaml (already complete from epic00)

Verified `.github/workflows/test.yaml` satisfies all criteria:
- Triggers on every push and on PRs targeting main
- Uses `go-version-file: go.mod` (no hardcoded Go version)
- Installs `setup-envtest`, sets `KUBEBUILDER_ASSETS`
- Runs `go build ./...`, `go vet ./...`, `go test -timeout 120s -race ./...`
- S01 marked complete with no changes needed.

### S02+S04 — build-watcher.yaml + docker/Dockerfile.watcher

New files created:

**`docker/Dockerfile.watcher`**:
- Multi-stage: `golang:1.23-bookworm AS builder` + `debian:bookworm-slim` runtime
- `CGO_ENABLED=0` static binary; `-trimpath`; `ldflags` embed `WATCHER_VERSION`
- Only `ca-certificates` installed in runtime stage
- Non-root user uid=1000 (`watcher`)
- `ENTRYPOINT ["/usr/local/bin/watcher"]`

**`.github/workflows/build-watcher.yaml`**:
- Triggers: `push: branches: main` AND `push: tags: v*`
- Uses `docker/setup-qemu-action@v3`, `docker/setup-buildx-action@v3`
- Logs in to `ghcr.io` via `GITHUB_TOKEN`
- `docker/metadata-action@v5` produces `sha-<7char>`, `latest` (on main), `v<semver>` (on tags)
- `docker/build-push-action@v5`: multi-arch `linux/amd64,linux/arm64`; GHA layer cache
- `WATCHER_VERSION=sha-<7chars>` embedded via `build-args` (using a prior step to compute the short SHA)
- Smoke test: `docker run --rm ghcr.io/.../mendabot-watcher:sha-<7chars> --version`

### S03+S04 — build-agent.yaml + Dockerfile.agent update

**`.github/workflows/build-agent.yaml`**:
- Same structure as build-watcher; image `ghcr.io/lenaxia/mendabot-agent`
- Smoke test: `docker run --rm --entrypoint /usr/local/bin/smoke-test.sh ...`

**`docker/Dockerfile.agent`** (update):
- Added `COPY scripts/smoke-test.sh /usr/local/bin/smoke-test.sh` + `chmod +x`
- Required so CI can invoke it as `--entrypoint /usr/local/bin/smoke-test.sh`

---

## Bugs Found and Fixed During Code Review

| # | Severity | File | Bug | Fix |
|---|----------|------|-----|-----|
| 1 | Critical | `Dockerfile.watcher` | `/out` directory never created; `go build -o /out/watcher` fails with "no such file or directory" | Added `mkdir -p /out &&` before the `go build` command |
| 2 | Critical | Both workflows | `docker/build-push-action@v6` does not exist; latest stable is v5 | Changed to `@v5` in both workflows |
| 3 | Critical | Both workflows | `type=semver` metadata tag only fires on Git tag refs, but workflows only triggered on `push: branches: main` — semver tags could never be produced | Added `push: tags: ['v*']` trigger to both workflows |
| 4 | High | `build-watcher.yaml` | `build-args` field does not support shell `$()` substitution; `WATCHER_VERSION=sha-$(echo ... \| cut -c1-7)` would be passed as a literal string | Added a `Compute short SHA` step that writes `short` to `GITHUB_OUTPUT`; both `build-args` and smoke test tag now reference `${{ steps.sha.outputs.short }}` |
| 5 | High | `build-agent.yaml` | Same SHA expansion issue in smoke test tag | Same fix via shared `Compute short SHA` step |

---

## Key Design Notes

- **Why `v*` trigger alongside `main` branch trigger**: The two triggers are independent. On a normal push to main, the `sha-` and `latest` tags are produced. When a `v1.0.0` tag is pushed (a separate git push), the semver tag fires and produces `v1.0.0` (plus sha- and latest again). This is the standard pattern for ghcr.io image versioning.

- **Why `build-push-action@v5` not v6**: v6 is not released. Pinning to a major version that doesn't exist fails the workflow silently (GitHub resolves the tag; if it doesn't exist, the step errors). Always verify action versions before pinning.

- **`smoke-test.sh` in the agent image**: The file is now at `/usr/local/bin/smoke-test.sh` and executable. The CI invokes it via `--entrypoint` override. It is NOT the container's default entrypoint (`agent-entrypoint.sh` remains the ENTRYPOINT).

- **GHA cache scope**: `cache-from/to: type=gha` with no explicit `scope` key uses the workflow file name as the scope automatically. The two image workflows will not share or collide on each other's cache.

---

## Tests Run

```
go build ./... + go test -timeout 30s -race ./... → all 9 packages pass (no Go changes)
```

---

## Next Steps

epic06 is complete. All functional epics (00–06) are now done. Next: **epic07 — Technical Debt** per `docs/BACKLOG/epic07-technical-debt/README.md`. Read that file before starting.

---

## Files Created/Modified

| File | Change |
|------|--------|
| `docker/Dockerfile.watcher` | Created — multi-stage watcher image |
| `.github/workflows/build-watcher.yaml` | Created — build and push watcher |
| `.github/workflows/build-agent.yaml` | Created — build and push agent |
| `docker/Dockerfile.agent` | Updated — add COPY/chmod for smoke-test.sh |
