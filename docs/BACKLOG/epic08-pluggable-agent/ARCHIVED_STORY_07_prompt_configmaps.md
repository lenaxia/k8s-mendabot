# Story 07: Rename and Add Prompt ConfigMaps

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 45 minutes

---

## User Story

As a **cluster operator**, I want separate prompt ConfigMaps for each agent so I can
tune the opencode prompt without breaking the claude prompt and vice versa.

---

## Acceptance Criteria

- [ ] `deploy/kustomize/configmap-prompt.yaml` renamed to
  `deploy/kustomize/configmap-prompt-opencode.yaml`
- [ ] ConfigMap name inside the file changed from `opencode-prompt` to
  `agent-prompt-opencode`
- [ ] Prompt body content unchanged (same 200-line prompt)
- [ ] New `deploy/kustomize/configmap-prompt-claude.yaml` created:
  - ConfigMap name: `agent-prompt-claude`
  - Key: `prompt.txt`
  - Prompt body adapted for Claude Code:
    - Same FINDING section and variable substitutions
    - Same INVESTIGATION STEPS structure and content
    - Same HARD RULES
    - Same PR body format
    - Adjusted ENVIRONMENT section: describes `claude` CLI tools instead of opencode
    - Adjusted agent invocation language (e.g. "you are running as Claude Code" rather
      than references to opencode)
    - Branch naming convention updated: `fix/mechanic-${FINDING_FINGERPRINT}` (drop the
      `k8sgpt-` prefix — this is a good opportunity to make it agent-agnostic)
- [ ] `deploy/kustomize/kustomization.yaml` updated to reference both new ConfigMap
  filenames and remove the old filename
- [ ] Unit tests for the opencode entrypoint's `envsubst` rendering (in
  `docker/scripts/`) continue to pass with the renamed ConfigMap mount path unchanged
  (`/prompt/prompt.txt` — the mount path is independent of the ConfigMap name)

---

## Tasks

- [ ] Rename `configmap-prompt.yaml` → `configmap-prompt-opencode.yaml`
- [ ] Update ConfigMap name inside the renamed file
- [ ] Create `configmap-prompt-claude.yaml` with adapted prompt
- [ ] Update `kustomization.yaml`
- [ ] Update branch naming in the opencode prompt from `fix/k8sgpt-${FINDING_FINGERPRINT}`
  to `fix/mechanic-${FINDING_FINGERPRINT}` for consistency
- [ ] Verify `envsubst` test in epic05 still passes

---

## Dependencies

**Depends on:** STORY_03 (for the new ConfigMap name used by the opencode builder)
**Can run in parallel with:** STORY_04, STORY_06

---

## Notes

The prompt mount path inside the Job container (`/prompt/prompt.txt`) does not change —
it is defined in the `JobBuilder` as a volume mount. Only the ConfigMap name (the
Kubernetes object name) and the `jobbuilder` reference to it change.

The branch naming change from `fix/k8sgpt-${FINDING_FINGERPRINT}` to
`fix/mechanic-${FINDING_FINGERPRINT}` is a breaking change for any existing open PRs.
Note this in the worklog.

---

## Definition of Done

- [ ] `kubectl apply -k deploy/kustomize/ --dry-run=client` succeeds
- [ ] No file named `deploy/kustomize/configmap-prompt.yaml` remains
- [ ] Both ConfigMaps present in the kustomization
- [ ] Prompt rendering test passes
