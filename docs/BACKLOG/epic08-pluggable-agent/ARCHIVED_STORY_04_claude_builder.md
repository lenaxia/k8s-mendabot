# Story 04: Implement Claude Builder

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 60 minutes

---

## User Story

As a **cluster operator**, I want to set `AGENT_TYPE=claude` and have the watcher
create Jobs that run Claude Code with the correct credentials and prompt, so I can
use Anthropic's model instead of opencode without any code changes.

---

## Acceptance Criteria

- [ ] New package `internal/jobbuilder/claude/` containing:
  - `builder.go` — `Builder` struct; `AgentType() string` returns `"claude"`;
    `Build(*v1alpha1.RemediationJob)` produces a valid `batch/v1 Job` spec
  - `builder_test.go` — same test table structure as the opencode builder tests
  - Compile-time assertion `var _ domain.JobBuilder = (*Builder)(nil)`
- [ ] The claude builder Job spec differs from the opencode builder in exactly these ways:
  - Prompt ConfigMap name: `"agent-prompt-claude"` (see STORY_07)
  - LLM credential env var mapping (from the same secret keys `api-key`, `base-url`, `model`):
    - `api-key` → `ANTHROPIC_API_KEY`
    - `base-url` → `ANTHROPIC_BASE_URL` (optional; claude CLI respects this for proxies)
    - `model` → `CLAUDE_MODEL`
  - The `base-url` env var is mounted even if empty — the claude entrypoint script handles
    the absent-means-default case; the builder does not add conditional logic
- [ ] All Job lifecycle settings (backoff, deadline, TTL, security context) are identical
  to the opencode builder
- [ ] All FINDING_* and GITOPS_* env vars are identical to the opencode builder
- [ ] All existing tests pass

---

## Tasks

- [ ] Write tests first (TDD) — copy the opencode test table, adjust expected env var names
  and ConfigMap name
- [ ] Implement `internal/jobbuilder/claude/builder.go`
- [ ] Register the claude builder in `cmd/watcher/main.go` alongside the opencode builder

---

## Dependencies

**Depends on:** STORY_02, STORY_03 (for pattern reference, not a hard code dependency)
**Blocks:** STORY_05

---

## Notes

The claude builder is not a copy-paste of the opencode builder. Extract any shared Job
construction helpers (security context, lifecycle settings, FINDING env vars, volume
definitions) into `internal/jobbuilder/shared/` if duplication becomes significant.
Use judgement — if the two builders share more than ~40 lines of identical code, extract
it. If less, duplication is acceptable to keep each builder self-contained and readable.

---

## Definition of Done

- [ ] Tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
- [ ] No opencode references in `internal/jobbuilder/claude/`
