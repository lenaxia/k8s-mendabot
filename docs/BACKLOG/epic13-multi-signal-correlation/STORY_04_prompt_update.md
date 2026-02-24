# Story 04: Prompt Template Update for Correlated Context

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** Medium
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **mendabot operator**, I want the agent prompt to instruct the agent to use the
`FINDING_CORRELATED_FINDINGS` env var when investigating a correlated group, so that
the agent produces a single coherent PR covering all related findings rather than
investigating only the primary finding in isolation.

---

## Background

`FINDING_CORRELATED_FINDINGS` is only set when the primary `RemediationJob` is part of a
correlation group (STORY_03). When it is set, the agent must:

1. Acknowledge that this finding is part of a group
2. Investigate the root cause that explains all findings in the group
3. Propose a single fix that resolves the entire group
4. Reference all correlated findings in the PR body

This is a prompt-only change. Zero Go code.

---

## Acceptance Criteria

- [ ] `deploy/kustomize/configmap-prompt.yaml` has a new `=== CORRELATED GROUP ===` section
      added to the `=== FINDING ===` block (after line 28, after the existing finding fields)
      referencing `${FINDING_CORRELATED_FINDINGS}` and `${FINDING_CORRELATION_GROUP_ID}`
- [ ] The prompt instructs the agent that if `FINDING_CORRELATED_FINDINGS` is non-empty, the
      investigation must explain all findings in the group, not just the primary
- [ ] The `=== PR BODY FORMAT ===` section (line 178) gains a `## Correlated Findings` entry
      (only rendered by the agent when `FINDING_CORRELATION_GROUP_ID` is non-empty)
- [ ] A new HARD RULE 8 is added (next sequential number after the existing 7 rules at line 217)
- [ ] Prompt change is reviewed for consistency with the existing structure (steps are numbered
      STEP 1–8; HARD RULES are numbered 1–7; there is no STEP 0 or STEP 10)
- [ ] `kubectl apply -k deploy/kustomize/ --dry-run=client` passes

---

## Technical Implementation

**Prompt structure reference:** The prompt (`deploy/kustomize/configmap-prompt.yaml`) has
the following top-level sections (no STEP 0 exists; steps run STEP 1–8):
- `=== FINDING ===` (line 12) — finding fields + self-remediation context
- `=== ENVIRONMENT ===` (line 30)
- `=== SELF-REMEDIATION GUIDANCE ===` (line 43)
- `=== INVESTIGATION STEPS ===` — STEP 1 through STEP 8 (line 84 through line 175)
- `=== PR BODY FORMAT ===` (line 178)
- `=== HARD RULES ===` — HARD RULES 1–7 (line 216)
- `=== DECISION TREE ===` (line 235)

The `configmap-prompt.yaml` is a plain-text template. It is not a shell script. The
`envsubst` substitution is performed at runtime by `agent-entrypoint.sh` — all `${VAR}`
references are substituted before the prompt is passed to `opencode`. Write the new
section as plain text with `${FINDING_CORRELATED_FINDINGS}` and
`${FINDING_CORRELATION_GROUP_ID}` references; do not write shell conditionals in the prompt.

### Addition to the `=== FINDING ===` section

Add after the `AI analysis` block (after line 28, before `=== ENVIRONMENT ===`):

```
    Correlated group:
    ${FINDING_CORRELATION_GROUP_ID}

    Additional findings in this group (JSON array, empty if not correlated):
    ${FINDING_CORRELATED_FINDINGS}

    If FINDING_CORRELATED_FINDINGS is non-empty, your investigation MUST identify the
    root cause shared by all findings in the group. Your proposed fix MUST address that
    root cause. Your PR MUST cover all findings — do not open separate PRs for findings
    in the same correlated group.
```

### New HARD RULE 8 (next sequential rule; existing rules end at 7)

```
    8. If FINDING_CORRELATED_FINDINGS is non-empty, your fix MUST address the shared
       root cause of all findings in the group. You MUST NOT open multiple PRs for
       findings in the same correlated group. The PR body MUST include a
       ## Correlated Findings section listing all finding kinds/names from the group.
```

### Addition to `=== PR BODY FORMAT ===`

Add before the closing `*Opened automatically by mendabot*` line:

```markdown
    ## Correlated Findings
    <!-- Include this section only when FINDING_CORRELATION_GROUP_ID is non-empty -->
    Group ID: `${FINDING_CORRELATION_GROUP_ID}`
    This PR resolves all findings in the correlated group. Additional findings covered:
    ${FINDING_CORRELATED_FINDINGS}
```

**Note on JSON field names in the prompt:** `FindingSpec` JSON tags are lowercase
(`kind`, `name`, `namespace`, `parentObject`, `errors`, `details`). If the agent
uses `jq` to parse `FINDING_CORRELATED_FINDINGS`, it must use lowercase selectors:
`.kind`, `.name`, `.namespace`. The `errors` field is a JSON-encoded string, not an
array — to extract error text with `jq` use:
`.errors | fromjson | map(.text) | join("; ")`

---

## Tasks

- [ ] Update `deploy/kustomize/configmap-prompt.yaml`: add the correlated group block
      to the `=== FINDING ===` section (after the `AI analysis` block, before `=== ENVIRONMENT ===`)
- [ ] Add HARD RULE 8 to the `=== HARD RULES ===` section (the existing rules end at 7)
- [ ] Add `## Correlated Findings` block to the `=== PR BODY FORMAT ===` section
      (before the closing `*Opened automatically by mendabot*` line)
- [ ] Verify `kubectl apply -k deploy/kustomize/ --dry-run=client` passes (no YAML errors)

---

## Dependencies

**Depends on:** STORY_03 (env var names must match what `JobBuilder` injects)
**Blocks:** STORY_05 (integration tests validate the full path including the prompt)

---

## Definition of Done

- [ ] Prompt template updated with correlated context handling in the `=== FINDING ===` section
- [ ] HARD RULE 8 added (correct sequential number; no rules 8 or 9 existed before)
- [ ] `## Correlated Findings` block added to `=== PR BODY FORMAT ===`
- [ ] `kubectl apply -k deploy/kustomize/ --dry-run=client` passes
- [ ] No existing tests broken (prompt is in a ConfigMap; no Go tests cover its content directly)
