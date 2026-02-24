# Story 06: Rename Agent Image and Entrypoint Artifacts

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 60 minutes

---

## User Story

As a **developer**, I want each agent to have its own Dockerfile and entrypoint script
so the opencode and claude images are independently buildable and their toolsets do not
contaminate each other.

---

## Acceptance Criteria

### Dockerfile changes

- [ ] `docker/Dockerfile.agent` renamed to `docker/Dockerfile.agent.opencode`; content
  unchanged except the `ENTRYPOINT` now references `agent-entrypoint-opencode.sh`
- [ ] New `docker/Dockerfile.agent.claude` created:
  - Same base image (`debian:bookworm-slim`) and apt packages
  - Same binary tools: kubectl, k8sgpt, helm, flux, gh, kustomize, kubeconform, stern,
    age, sops, yq — **identical versions** to the opencode Dockerfile
  - Replaces opencode binary with the Claude Code CLI (`claude`) downloaded from
    `https://github.com/anthropics/claude-code/releases/...`; new build arg `CLAUDE_VERSION`
  - `ENTRYPOINT ["/usr/local/bin/agent-entrypoint-claude.sh"]`
  - Same user, workdir, and git identity configuration as the opencode image
  - `smoke-test.sh` updated to verify `claude` instead of `opencode`

### Entrypoint script changes

- [ ] `docker/scripts/agent-entrypoint.sh` renamed to
  `docker/scripts/agent-entrypoint-opencode.sh`; content otherwise identical except
  references to the script's own name in error messages
- [ ] New `docker/scripts/agent-entrypoint-claude.sh` created:
  - Same required env var guard (same FINDING_*, GITOPS_*, credential vars except
    credentials are `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `CLAUDE_MODEL`)
  - Same kubeconfig construction logic (copy verbatim — it is cluster-specific, not
    agent-specific)
  - Same gh CLI authentication logic (copy verbatim)
  - Same prompt rendering with `envsubst` (copy verbatim)
  - Agent invocation: `exec claude --print "$(cat /tmp/rendered-prompt.txt)"` — the
    `--print` flag runs non-interactively and exits with the agent's exit code

### CI/CD changes

- [ ] `.github/workflows/build-agent.yaml` updated to build both images; the job matrix
  gains a second entry for the claude image with its own `Dockerfile.agent.claude` and
  image tag suffix `-claude`; the existing entry is updated to tag suffix `-opencode`

---

## Tasks

- [ ] Rename `Dockerfile.agent` → `Dockerfile.agent.opencode`
- [ ] Update `ENTRYPOINT` reference in `Dockerfile.agent.opencode`
- [ ] Create `Dockerfile.agent.claude`
- [ ] Rename `agent-entrypoint.sh` → `agent-entrypoint-opencode.sh`
- [ ] Create `agent-entrypoint-claude.sh`
- [ ] Update `smoke-test.sh` for the claude image
- [ ] Update `.github/workflows/build-agent.yaml` matrix

---

## Dependencies

**Depends on:** STORY_01 (for `CLAUDE_MODEL` env var name)
**Can run in parallel with:** STORY_03, STORY_04

---

## Notes

- The `get-github-app-token.sh` script is shared and does not need renaming or copying.
- Do not add `talosctl` to the claude image if it is not present in the opencode image
  at the time this story is implemented. Keep parity between the two images' tool sets.
- The claude entrypoint does not need `OPENCODE_CONFIG_CONTENT` or the opencode JSON
  config builder. Claude Code reads `ANTHROPIC_API_KEY` directly from the environment.
- If the Claude Code CLI release URL or binary name is not yet known at implementation
  time, use a placeholder `TODO` comment in the Dockerfile and raise it as a blocker.
  Do not guess the URL.

---

## Definition of Done

- [ ] Both Dockerfiles build locally with `docker build`
- [ ] Both images pass their respective smoke tests
- [ ] CI workflow updated and valid YAML
- [ ] No file named `docker/Dockerfile.agent` or `docker/scripts/agent-entrypoint.sh`
  remains
