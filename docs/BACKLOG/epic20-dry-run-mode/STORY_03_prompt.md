# Story: Prompt — dry-run HARD RULE variant

**Epic:** [epic20-dry-run-mode](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 0.5 hours

---

## User Story

As a **cluster operator** evaluating mendabot in shadow mode, I want the agent prompt to
explicitly prohibit PR creation when `DRY_RUN=true`, and to require the agent to write its
full investigation to a known file path, so I can review what mendabot would have done
without any GitOps changes being made.

---

## Background

The prompt is rendered by `docker/scripts/agent-entrypoint.sh` via `envsubst`. The template
lives in `deploy/kustomize/configmap-prompt.yaml` as the `prompt.txt` data key. The shell
script (line 104–105) substitutes a fixed set of `${VAR}` placeholders:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}\
${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}\
${GITOPS_MANIFEST_ROOT}${IS_SELF_REMEDIATION}${CHAIN_DEPTH}${TARGET_REPO_OVERRIDE}'
envsubst "$VARS" < /prompt/prompt.txt > /tmp/rendered-prompt.txt
```

`DRY_RUN` is **not** currently in the `VARS` list. This story adds `${DRY_RUN}` to the
`VARS` list so the prompt can conditionally branch, and adds a new HARD RULE that fires only
when the substituted value is `"true"`.

### Existing HARD RULES

The `=== HARD RULES ===` section (starting at line 238 of `configmap-prompt.yaml`) currently
has rules numbered 1–8, **with a duplicate rule 8** (a pre-existing bug in the file):

- Rule 1: never commit to main
- Rule 2: never touch Secrets
- Rule 3: exactly one outcome per invocation (comment or open PR)
- Rule 4: low-confidence → open investigation PR with `needs-human-review`
- Rule 5: PR body must include fingerprint
- Rule 6: no additional tool installs
- Rule 7: no external API calls except GitHub
- Rule 8 (first): correlated findings constraint
- Rule 8 (second / duplicate): untrusted-input framing for FINDING_ERRORS/FINDING_DETAILS blocks

The new dry-run rule is numbered **9**. The pre-existing duplicate rule 8 is **not** renumbered
by this story — that is a separate cleanup task. The new rule 9 is inserted after the second
rule 8.

---

## Acceptance Criteria

- [ ] `${DRY_RUN}` is added to the `VARS` list in `docker/scripts/agent-entrypoint.sh`
  (the single line that defines which variables `envsubst` will replace)
- [ ] A new HARD RULE 9 is appended to the `=== HARD RULES ===` section of
  `deploy/kustomize/configmap-prompt.yaml`
- [ ] The rule text (see below) is correct: it fires only when `DRY_RUN == "true"`,
  prohibits all PR/git-push operations, and mandates writing the report to
  `/workspace/investigation-report.txt`
- [ ] The `=== DECISION TREE ===` section gains a dry-run branch (see below)
- [ ] No other parts of the prompt are changed by this story

---

## Implementation

### 1. Add `${DRY_RUN}` to `entrypoint.sh`

In `docker/scripts/agent-entrypoint.sh`, line 104, change the `VARS` assignment from:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}${IS_SELF_REMEDIATION}${CHAIN_DEPTH}${TARGET_REPO_OVERRIDE}'
```

to:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}${IS_SELF_REMEDIATION}${CHAIN_DEPTH}${TARGET_REPO_OVERRIDE}${DRY_RUN}'
```

Add `DRY_RUN` to the optional-variables block below the required checks:

```bash
# DRY_RUN is optional — defaults to "false"
DRY_RUN="${DRY_RUN:-false}"
```

### 2. Add HARD RULE 9 to the prompt

Append after the existing second rule 8 in `configmap-prompt.yaml`, still inside the
`=== HARD RULES ===` section:

```
    9. If DRY_RUN is "true":
       a. DO NOT create any git branches, commits, or push any changes.
       b. DO NOT open, update, or comment on any GitHub pull request.
       c. DO NOT call "gh pr create", "gh pr comment", "git push", or any equivalent.
       d. Complete your full investigation (Steps 1–6 of INVESTIGATION STEPS) normally.
       e. Write your complete investigation report — including root cause, evidence, and
          proposed fix — to the file /workspace/investigation-report.txt.
          Use plain text; no shell redirections that could fail silently.
          Example: printf '%s\n' "<your report>" > /workspace/investigation-report.txt
       f. Exit 0 after writing the report. Rule 3 (exactly one PR outcome) does NOT
          apply when DRY_RUN is "true" — writing the report file IS the outcome.
```

### 3. Update the `=== DECISION TREE ===` section

Prepend a dry-run branch at the top of the decision tree:

```
    DRY_RUN is "true" → complete investigation → write /workspace/investigation-report.txt → exit 0 (Rules 3 and 8 do not apply)
```

---

## Report file format

The agent writes `/workspace/investigation-report.txt` in plain text. The watcher reads this
file from Job logs (see STORY_04). The agent is free to structure the content as prose, but
the following sections are expected (for human readability in `status.message`):

```
## Finding
Kind: <FINDING_KIND>
Resource: <FINDING_NAME> / <FINDING_NAMESPACE>
Parent: <FINDING_PARENT>
Fingerprint: <FINDING_FINGERPRINT>

## Root Cause
<one-paragraph assessment>

## Proposed Fix
<what would have been changed>

## Confidence
<high / medium / low — and why>

## Evidence
<kubectl output excerpts>
```

The watcher truncates the report to 10,000 bytes before storing it in `status.message`
(see STORY_04). The most important information (root cause, proposed fix) should appear early
in the file.

---

## Tasks

- [ ] Add `${DRY_RUN}` to the `VARS` line in `docker/scripts/agent-entrypoint.sh`
- [ ] Add `DRY_RUN="${DRY_RUN:-false}"` to the optional-variables block in `agent-entrypoint.sh`
- [ ] Add HARD RULE 9 to `deploy/kustomize/configmap-prompt.yaml` (after the second rule 8)
- [ ] Add dry-run branch to the `=== DECISION TREE ===` section
- [ ] Manual smoke test: render the prompt with `envsubst` and confirm `${DRY_RUN}` is
  substituted correctly with both `DRY_RUN=true` and `DRY_RUN=false`

---

## Dependencies

**Depends on:** STORY_02 (the `DRY_RUN=true` env var must be injected into the Job before
the agent can read it at runtime)
**No Go compilation dependency** — this story only modifies shell script and YAML.

---

## Definition of Done

- [ ] `docker/scripts/agent-entrypoint.sh` has `${DRY_RUN}` in `VARS` and the
  `DRY_RUN` default assignment
- [ ] HARD RULE 9 is present verbatim in `deploy/kustomize/configmap-prompt.yaml`
- [ ] Decision tree has the dry-run branch
- [ ] `git diff --stat` shows only `agent-entrypoint.sh` and `configmap-prompt.yaml` changed
- [ ] Full test suite passes: `go test -timeout 120s -race ./...` (no Go changes in this story)
