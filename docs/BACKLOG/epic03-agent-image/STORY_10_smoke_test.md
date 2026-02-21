# Story: Smoke Test Script

**Epic:** [epic03-agent-image](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **developer**, I want a smoke test script that verifies every tool binary is present
and callable in the built image so CI catches any broken installs immediately.

---

## Acceptance Criteria

- [ ] Script at `docker/scripts/smoke-test.sh`
- [ ] Tests all tools: opencode, kubectl, k8sgpt, helm, flux, talosctl, kustomize, gh,
  git, jq, yq, kubeconform, stern, age, sops, curl, openssl
- [ ] Each tool called with `--version`, `version`, or `-v` as appropriate
- [ ] Verifies entrypoint scripts are present and executable:
  `test -x /usr/local/bin/agent-entrypoint.sh`
  `test -x /usr/local/bin/get-github-app-token.sh`
- [ ] Script exits non-zero on any failure with a clear message identifying which tool failed
- [ ] Script used in `build-agent.yaml` CI workflow as a post-build step

---

## Tasks

- [ ] Write `docker/scripts/smoke-test.sh`
- [ ] Run against locally built image: `docker run --rm <image> /bin/bash -c "$(cat docker/scripts/smoke-test.sh)"`
- [ ] Wire into CI workflow (done in epic06-ci-cd STORY_03)

---

## Dependencies

**Depends on:** STORY_09 (full image built)

---

## Definition of Done

- [ ] Script passes against built image for both `amd64` and `arm64`
- [ ] Script exits non-zero if any binary is missing
