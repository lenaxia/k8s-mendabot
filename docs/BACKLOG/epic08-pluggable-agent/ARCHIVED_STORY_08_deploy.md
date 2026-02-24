# Story 08: Deploy Manifest Updates

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **cluster operator**, I want to configure `AGENT_TYPE` in the watcher deployment
manifest so I can switch agents by editing one env var without touching code.

---

## Acceptance Criteria

- [ ] `deploy/kustomize/deployment-watcher.yaml`: `AGENT_TYPE` env var added to the
  watcher container's `env` list with value `"opencode"` as the default; documented
  with a comment indicating valid values
- [ ] `deploy/kustomize/secret-llm.yaml`: comment updated to note that the secret keys
  (`api-key`, `base-url`, `model`) are stable regardless of agent; the values differ
  per-agent (OpenAI-compatible endpoint for opencode, Anthropic API key for claude)
- [ ] `deploy/kustomize/kustomization.yaml` verified to reference both ConfigMaps
  (coordinated with STORY_07)
- [ ] `kubectl apply -k deploy/kustomize/ --dry-run=client` succeeds with no warnings
- [ ] `docs/DESIGN/lld/DEPLOY_LLD.md`: `AGENT_TYPE` env var added to the watcher
  container env var table

---

## Tasks

- [ ] Add `AGENT_TYPE: "opencode"` to `deployment-watcher.yaml`
- [ ] Update comment in `secret-llm.yaml`
- [ ] Update `DEPLOY_LLD.md` env var table
- [ ] Run `kubectl apply -k deploy/kustomize/ --dry-run=client` to verify

---

## Dependencies

**Depends on:** STORY_05, STORY_07
**Blocks:** nothing (final story in this epic)

---

## Definition of Done

- [ ] `kubectl apply -k deploy/kustomize/ --dry-run=client` passes
- [ ] `AGENT_TYPE` visible in the deployment spec
- [ ] `DEPLOY_LLD.md` updated
