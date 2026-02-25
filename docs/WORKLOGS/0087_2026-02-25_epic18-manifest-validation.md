# Worklog: Epic 18 — Mandatory Manifest Validation

**Date:** 2026-02-25
**Session:** Implement epic18 STORY_02 — promote kubeconform validation to HARD RULE
**Status:** Complete

---

## Objective

Promote manifest validation from an advisory STEP 7 to a non-negotiable HARD RULE in
the agent prompt (`charts/mendabot/files/prompts/core.txt`). Zero Go code changes.

---

## Work Completed

### 1. Epic 18 doc rewrite (pre-implementation)

Validation of the epic docs revealed they were written against `deploy/kustomize/configmap-prompt.yaml`,
which was deleted in epic08 (2026-02-24). Both the README and STORY_02 were rewritten to
target the correct file (`charts/mendabot/files/prompts/core.txt`) and to reflect the
actual current state of the HARD RULES (no duplicate rule 8, rules 1–8 clean).

### 2. Prompt changes (`charts/mendabot/files/prompts/core.txt`)

Three edits:

1. **Rule 8 → rule 9** — renumbered the untrusted-input delimiter rule to make room.
2. **New HARD RULE 10** — mandatory kubeconform before any `git commit`, covering:
   - Case A: plain YAML (`kubeconform -strict -ignore-missing-schemas <file>`)
   - Case B: Kustomize overlay (`kustomize build <overlay> | kubeconform ...`)
   - Case C: Helm values (`helm template <release> <chart> -f <values> | kubeconform ...`)
   - Fallback when non-zero: no commit, empty-commit placeholder PR, `## Validation Errors`
     section in PR body, labels `validation-failed` + `needs-human-review`
3. **STEP 7 updated** — heading now reads `(MANDATORY — see HARD RULE 10)`; added Helm
   (Case C) variant; replaced advisory language with mandatory language; added fallback
   cross-reference.

---

## Key Decisions

- **Flags `-strict -ignore-missing-schemas`** retained from the previous advisory step.
  `-strict` catches field-name typos; `-ignore-missing-schemas` prevents false positives
  on CRDs (`HelmRelease`, `Kustomization`, `Certificate`) that have no bundled schema.
- **Empty-commit placeholder PR** chosen as the fallback because the agent must still
  satisfy HARD RULE 3b (exactly one PR per invocation). An empty-commit PR with the
  `validation-failed` label surfaces the failure to a human reviewer without silently
  dropping the finding.

---

## Blockers

None.

---

## Tests Run

No Go tests — prompt-only change. Validation:

```bash
helm template mendabot charts/mendabot \
  --set gitops.repo=https://github.com/example/repo \
  --set gitops.manifestRoot=clusters/prod \
  | grep -E "STEP 7|HARD RULE|^\s+[0-9]+\."
```

Output confirmed:
- STEP 7 heading includes `(MANDATORY — see HARD RULE 10)`
- HARD RULES section contains rules 1–7, 9, 10 (no rule 8 — correct)
- Rule 10 text renders with all three cases and fallback procedure intact
- `helm template` exits 0

---

## Next Steps

1. Epic 18 is complete. Update STORY_02 status and epic README status.
2. Next recommended epic: **epic22** (GitHub App token expiry guard) — shell-only,
   two stories, high reliability value.

---

## Files Modified

| File | Change |
|------|--------|
| `charts/mendabot/files/prompts/core.txt` | Rule 8→9, append HARD RULE 10, update STEP 7 |
| `docs/BACKLOG/epic18-manifest-validation/README.md` | Rewritten: corrected file paths, line numbers, validation command, DoD |
| `docs/BACKLOG/epic18-manifest-validation/STORY_02_prompt_hard_rule.md` | Rewritten: corrected target file, BEFORE/AFTER blocks, implementation steps |
