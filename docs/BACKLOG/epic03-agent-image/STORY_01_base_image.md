# Story: Dockerfile Base and System Packages

**Epic:** [epic03-agent-image](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want a `docker/Dockerfile.agent` starting from `debian:bookworm-slim`
with all base system packages installed so subsequent layers can install binary tools
without dependency issues.

---

## Acceptance Criteria

- [ ] Base image is `debian:bookworm-slim`
- [ ] Installed packages: `bash`, `ca-certificates`, `curl`, `gettext-base`, `git`, `gnupg`, `jq`,
  `openssl`, `unzip`
- [ ] `apt-get` cache cleaned in the same `RUN` layer
- [ ] `DEBIAN_FRONTEND=noninteractive` set
- [ ] All tool version `ARG` declarations at the top of the file
- [ ] `TARGETARCH` ARG present for multi-arch support
- [ ] Image builds successfully with `docker build -f docker/Dockerfile.agent .`

---

## Tasks

- [ ] Create `docker/Dockerfile.agent` with base layer
- [ ] Add all version ARGs at top
- [ ] Install base system packages
- [ ] Verify `docker build` succeeds

---

## Dependencies

**Depends on:** Foundation epic00 (directory structure)
**Blocks:** All other agent image stories

---

## Definition of Done

- [ ] `docker build -f docker/Dockerfile.agent .` succeeds
- [ ] No packages missing for subsequent layers
