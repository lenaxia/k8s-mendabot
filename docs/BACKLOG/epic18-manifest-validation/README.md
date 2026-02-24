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

## Status: In Progress (STORY_01 Done, STORY_02 Not Started)

## Deep-Dive Findings (2026-02-23)

### STORY_01 — Agent Image: DONE (no change needed)

`kubeconform` **is already present** in `docker/Dockerfile.agent`:

- `ARG KUBECONFORM_VERSION=0.7.0` at line 10
- Download + SHA256 verification block at lines 99–106
- Install destination: `/usr/local/bin/kubeconform`
- Arch-portable via `${TARGETARCH}` build-arg
- Also already listed in the prompt's tool inventory at `configmap-prompt.yaml:54`

**STORY_01 is closed as done by inspection.** No Dockerfile change required.

Optional follow-up (non-blocking): add `check kubeconform --version` to
`docker/scripts/smoke-test.sh` alongside other tool checks.

### STORY_02 — Prompt: Not Started

**Pre-existing bug found:** the `=== HARD RULES ===` section has a **duplicate rule 8**.
The first rule 8 (correlated findings) and the second rule 8 (untrusted input framing)
were added at different times without renumbering.

**Changes required in `deploy/kustomize/configmap-prompt.yaml`:**

1. Rename the second rule `8` (untrusted input) to `9`.
2. Append new HARD RULE `10` — mandatory kubeconform covering three cases:
   - Case A: plain YAML files
   - Case B: `kustomize build <overlay> | kubeconform`
   - Case C: `helm template <release> <chart> -f <values> | kubeconform`
3. Define fallback when kubeconform exits non-zero: empty-commit placeholder PR with
   `## Validation Errors` section, labels `validation-failed` + `needs-human-review`.
4. Update STEP 7 heading to `(MANDATORY — see HARD RULE 10)` and replace advisory text
   with the mandatory three-case version.

**Flags:** `-strict -ignore-missing-schemas` (strict catches typos; ignore-missing-schemas
prevents false positives on CRDs like `HelmRelease`, `Kustomization`, `Certificate`).

**Validation after change:**
```bash
kubectl apply --dry-run=client -f deploy/kustomize/configmap-prompt.yaml
kustomize build deploy/kustomize | grep -A5 "name: opencode-prompt"
```

## Dependencies

- epic05-prompt complete (`deploy/kustomize/configmap-prompt.yaml`)
- epic03-agent-image complete — **confirmed**: `docker/Dockerfile.agent` already has kubeconform v0.7.0

## Blocks

- Nothing

## Stories

| Story | File | Status |
|-------|------|--------|
| Agent image — ensure kubeconform is installed | [STORY_01_agent_image_kubeconform.md](STORY_01_agent_image_kubeconform.md) | **Done** |
| Prompt — promote validation to HARD RULE and add Kustomize/Helm variants | [STORY_02_prompt_hard_rule.md](STORY_02_prompt_hard_rule.md) | Not Started |

## Implementation Order

```
STORY_01 (agent image) ─ DONE ──> STORY_02 (prompt) — ready to implement now
```

## Definition of Done

- [x] `docker/Dockerfile.agent` includes `kubeconform` installation (v0.7.0, lines 10, 99–106)
- [ ] Duplicate rule 8 in `configmap-prompt.yaml` renumbered to 9
- [ ] New HARD RULE 10 added covering Case A (plain YAML), Case B (kustomize), Case C (helm template)
- [ ] HARD RULE 10 specifies the fallback: no commit, empty-commit PR, `## Validation Errors` section, labels `validation-failed` + `needs-human-review`
- [ ] STEP 7 updated to mark validation mandatory and cross-reference HARD RULE 10
- [ ] `kubectl apply --dry-run=client` passes on the modified configmap
- [ ] Worklog written
