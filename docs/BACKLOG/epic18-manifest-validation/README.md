# Epic 18: Mandatory Pre-PR Manifest Validation

**Feature Tracker:** FT-A9
**Area:** Accuracy & Precision

## Purpose

Promote manifest validation from a recommended step to a HARD RULE in the agent prompt.
The agent must run `kubeconform` (and `kustomize build | kubeconform` for Kustomize
overlays, `helm template | kubeconform` for Helm values changes) on ALL modified files
before any `git commit`. A schema-invalid manifest must never reach the GitOps repo.

This is a **prompt-only change** — zero Go code. Its value is high because schema-invalid
YAML that passes `git push` and then fails in Flux is the most visible and damaging agent
failure mode.

## Status: Complete

## Deep-Dive Findings (2026-02-25)

### STORY_01 — Agent Image: DONE (no change needed)

`kubeconform` **is already present** in `docker/Dockerfile.agent`:

- `ARG KUBECONFORM_VERSION=0.7.0` at line 39 (in the runtime stage, after `age-builder`
  and `redact-builder` multi-stage blocks)
- Download + SHA256 verification block at lines 128–135
- Install destination: `/usr/local/bin/kubeconform` (as a redact wrapper; real binary
  at `/usr/local/bin/kubeconform.real`)
- Arch-portable via `${TARGETARCH}` build-arg
- Already listed in `charts/mechanic/files/prompts/core.txt` tool inventory at line 33:
  `jq, yq, kubeconform, stern, sops, age.`
- Already checked in `docker/scripts/smoke-test.sh` at line 49: `check_binary kubeconform`

**STORY_01 is closed as done by inspection.** No Dockerfile change required.

### STORY_02 — Prompt: Complete

**File to edit:** `charts/mechanic/files/prompts/core.txt`

Note: `deploy/kustomize/configmap-prompt.yaml` was deleted in epic08 (2026-02-24). The
prompt text now lives in `charts/mechanic/files/prompts/core.txt`. The Helm template at
`charts/mechanic/templates/configmap-prompt.yaml` renders this file into a ConfigMap via
`.Files.Get` — it contains no prompt text itself.

**Current state of HARD RULES in `core.txt` (lines 183–208):**

Rules 1–8, no duplicates. Rule 8 is the untrusted-input delimiter rule. The
correlated-findings rule that existed in the old kustomize configmap was dropped during
the epic08 migration and is not present in the current file.

**Changes required in `charts/mechanic/files/prompts/core.txt`:**

1. Renumber current rule `8` (untrusted input, lines 205–207) to `9` — this makes room
   for the new rule and keeps numbering monotonically increasing.
2. Append new HARD RULE `10` — mandatory kubeconform covering three cases:
   - Case A: plain YAML files
   - Case B: `kustomize build <overlay> | kubeconform`
   - Case C: `helm template <release> <chart> -f <values> | kubeconform`
3. Define fallback when kubeconform exits non-zero: empty-commit placeholder PR with
   `## Validation Errors` section, labels `validation-failed` + `needs-human-review`.
4. Update STEP 7 (lines 121–126) heading to `(MANDATORY — see HARD RULE 10)`, add the
   Helm (Case C) variant, and replace advisory language with mandatory language.

**Flags:** `-strict -ignore-missing-schemas` (strict catches typos in field names;
ignore-missing-schemas prevents false positives on CRDs like `HelmRelease`,
`Kustomization`, `Certificate`).

**Validation after change:**
```bash
helm template mechanic charts/mechanic | grep -A5 "agent-prompt-core"
```

## Dependencies

- epic03-agent-image complete — confirmed: `docker/Dockerfile.agent` has kubeconform v0.7.0
  at line 39, install block at lines 128–135
- epic08-pluggable-agent complete — prompt text moved to `charts/mechanic/files/prompts/core.txt`

## Blocks

- Nothing

## Stories

| Story | File | Status |
|-------|------|--------|
| Agent image — ensure kubeconform is installed | [STORY_01_agent_image_kubeconform.md](STORY_01_agent_image_kubeconform.md) | **Done** |
| Prompt — promote validation to HARD RULE and add Kustomize/Helm variants | [STORY_02_prompt_hard_rule.md](STORY_02_prompt_hard_rule.md) | **Done** |

## Implementation Order

```
STORY_01 (agent image) ─ DONE ──> STORY_02 (prompt) ─ DONE
```

## Definition of Done

- [x] `docker/Dockerfile.agent` includes `kubeconform` installation (v0.7.0, lines 128–135)
- [x] Current rule `8` (untrusted input) renumbered to `9` in `charts/mechanic/files/prompts/core.txt`
- [x] New HARD RULE `10` added covering Case A (plain YAML), Case B (kustomize), Case C (helm template)
- [x] HARD RULE `10` specifies the fallback: no commit, empty-commit PR, `## Validation Errors` section, labels `validation-failed` + `needs-human-review`
- [x] STEP 7 updated to mark validation mandatory, add Helm case, and cross-reference HARD RULE 10
- [x] `helm template mechanic charts/mechanic | grep -A5 "agent-prompt-core"` renders without error
- [x] Worklog written
