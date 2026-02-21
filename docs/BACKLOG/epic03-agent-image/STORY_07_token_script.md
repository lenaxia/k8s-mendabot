# Story: get-github-app-token.sh Script

**Epic:** [epic03-agent-image](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want `docker/scripts/get-github-app-token.sh` to exchange a GitHub
App private key for a short-lived installation token so the agent can authenticate to
GitHub without storing long-lived credentials.

---

## Acceptance Criteria

- [ ] Script reads `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`,
  `GITHUB_APP_PRIVATE_KEY` from environment
- [ ] Missing env vars cause the script to exit non-zero with a clear error message
- [ ] Script generates a valid JWT signed with RS256 using `openssl`
- [ ] Script exchanges the JWT for an installation token via GitHub API
- [ ] Token is printed to stdout (callers capture it with `$(get-github-app-token.sh)`)
- [ ] Script exits non-zero if the API call fails
- [ ] Script is executable (`chmod +x`) and present at `/usr/local/bin/` in the image

---

## Tasks

- [ ] Write `docker/scripts/get-github-app-token.sh` following **AGENT_IMAGE_LLD.md §4** spec
- [ ] Test locally with a real GitHub App (or mock with a stub API)
- [ ] Add `COPY` and `chmod` to Dockerfile
- [ ] Verify script is callable in built image

---

## Dependencies

**Depends on:** STORY_01 (base image — needs openssl and curl)
**Blocks:** STORY_08 (entrypoint uses this script)

---

## Definition of Done

- [ ] Script handles all error cases with non-zero exit
- [ ] Script callable in built image
- [ ] Script documented with inline comments for each step
