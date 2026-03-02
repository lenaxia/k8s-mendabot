# Story 05: Feedback Prompt Mode

## Status: Not Started

## Objective

Add a `feedback-mode.txt` prompt to the ConfigMap so the agent knows how to handle
feedback-iteration runs. This prompt is used when `FEEDBACK_MODE=true` is set.

## Acceptance Criteria

- [ ] `deploy/kustomize/configmap-prompt.yaml` has a new key `feedback-mode.txt` with
      a prompt that instructs the agent to:
  1. Read `FEEDBACK_COMMENT` and `FEEDBACK_COMMENT_AUTHOR` from env
  2. Review the existing PR diff on the current branch to understand what was proposed
  3. Address the reviewer's concern specifically
  4. Push an amended commit to the same branch (no new PR)
  5. Reply to the reviewer comment (`gh pr comment <number> -b "..."`) summarising changes
- [ ] `charts/mechanic/files/prompts/feedback-mode.txt` has the same content (Helm chart)
- [ ] The `agent-entrypoint.sh` selects either `main-mode.txt` (default) or
      `feedback-mode.txt` based on `$FEEDBACK_MODE`:
      ```bash
      if [ "${FEEDBACK_MODE:-false}" = "true" ]; then
        PROMPT_FILE="/prompts/feedback-mode.txt"
      else
        PROMPT_FILE="/prompts/main-mode.txt"
      fi
      ```
- [ ] No Go code changes required for this story

## New Files

| File | Purpose |
|------|---------|
| `charts/mechanic/files/prompts/feedback-mode.txt` | Feedback-iteration prompt |

## Modified Files

| File | Change |
|------|--------|
| `deploy/kustomize/configmap-prompt.yaml` | Add `feedback-mode.txt` key |
| `docker/scripts/agent-entrypoint.sh` | Select prompt file based on `FEEDBACK_MODE` |

## Notes

- Check `docker/scripts/agent-entrypoint.sh` for the current prompt file selection logic
  and follow the same pattern.
- Check `deploy/kustomize/configmap-prompt.yaml` for the existing key structure.
- The feedback prompt should be terse but specific: the agent only needs to address
  the comment, not re-investigate the full cluster state.
- `FEEDBACK_ITERATION` should be logged at the start of the prompt so the agent knows
  which iteration it is on (useful for the commit message).
