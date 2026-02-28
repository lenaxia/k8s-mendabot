# Story 06: Prompt — Use FINDING_SEVERITY in Agent Instructions

**Epic:** [epic24-severity-tiers](README.md)
**Priority:** Medium
**Status:** Complete
**Estimated Effort:** 30 minutes

---

## User Story

As a **cluster operator**, I want the agent to propose proportional fixes — thorough and
confident for critical findings, conservative and minimal for low severity findings — so
that high-impact incidents get the most aggressive remediation and low-noise events are not
over-acted upon.

---

## Background

The agent prompt lives in `charts/mechanic/files/prompts/core.txt`. This file is
loaded by the Helm template `charts/mechanic/templates/configmap-prompt.yaml` and
rendered into the `opencode-prompt` ConfigMap. The `deploy/kustomize/` directory does
not exist in this repository — Kustomize was replaced by the Helm chart.

Variable substitution happens in `docker/scripts/entrypoint-common.sh` via
`envsubst "$VARS"`. The `VARS` string on line 105 is the gate for substitution — only
variables listed there are replaced. **STORY_05 must add `${FINDING_SEVERITY}` to that
string before this story can work end-to-end.**

## Design

### charts/mechanic/files/prompts/core.txt

**Finding context block** (currently lines 7–17): add `Severity` after `Fingerprint`:

```
Kind:         ${FINDING_KIND}
Resource:     ${FINDING_NAME}
Namespace:    ${FINDING_NAMESPACE}
Parent:       ${FINDING_PARENT}
Fingerprint:  ${FINDING_FINGERPRINT}
Severity:     ${FINDING_SEVERITY}
```

**Severity calibration instruction**: add after the finding context block (before STEP 1):

```
SEVERITY CALIBRATION:
- critical: Immediate service impact. Investigate all plausible root causes.
  Propose a specific, confident fix. Do not hedge unless evidence is genuinely ambiguous.
- high: Significant degradation likely visible to users. Investigate thoroughly.
  Propose a targeted fix with clear rationale. Note any assumptions.
- medium: Partial degradation or elevated error rate. Propose a conservative,
  minimal-change fix. Prefer smaller changes over broad refactors.
- low: Minor or intermittent issue. Propose a fix only if one is clear with low
  blast radius. If uncertain, document findings in the PR body and recommend human
  review rather than making a change.
- (empty or unrecognised): Treat as medium.
```

---

## Acceptance Criteria

- [ ] `FINDING_SEVERITY=${FINDING_SEVERITY}` line added to the finding context block in `charts/mechanic/files/prompts/core.txt`
- [ ] Severity calibration instruction present covering all four values plus the empty case
- [ ] `charts/mechanic/templates/configmap-prompt.yaml` does not need changes (it loads the file contents via `{{ .Files.Get "files/prompts/core.txt" }}`)
- [ ] `${FINDING_SEVERITY}` is present in the `VARS` string in `docker/scripts/entrypoint-common.sh` (done in STORY_05 — verify before merging)

---

## Tasks

- [ ] Read `charts/mechanic/files/prompts/core.txt` in full to identify the exact insertion points
- [ ] Add `Severity:     ${FINDING_SEVERITY}` to the finding context block
- [ ] Add severity calibration instruction after the finding context block
- [ ] Verify `docker/scripts/entrypoint-common.sh` VARS string contains `${FINDING_SEVERITY}` (added in STORY_05)
- [ ] Run `helm lint charts/mechanic/` — must pass with no errors
- [ ] Write worklog entry for the completed epic

---

## Dependencies

**Depends on:** STORY_05 (`FINDING_SEVERITY` env var injected into agent Job)
**Blocks:** Nothing

---

## Definition of Done

- [ ] `charts/mechanic/files/prompts/core.txt` updated with `Severity: ${FINDING_SEVERITY}` in the finding context block
- [ ] Severity calibration instruction present in the prompt
- [ ] `helm lint charts/mechanic/` passes with no errors
- [ ] Worklog written for the epic
