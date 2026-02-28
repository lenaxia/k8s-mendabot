# Worklog 0057 — Epic 13 STORY_04: Prompt Template Update for Correlated Context

**Date:** 2026-02-25
**Branch:** feature/epic13-multi-signal-correlation
**Story:** STORY_04_prompt_update.md

## Summary

Updated the agent prompt template to instruct the agent to use `FINDING_CORRELATED_FINDINGS`
when investigating a correlated group. Both the Helm chart prompt and the kustomize configmap
are now fully in sync.

## Changes

### `charts/mechanic/files/prompts/default.txt`

- Added `Severity: ${FINDING_SEVERITY}` field to the `=== FINDING ===` section
- Added `<<<MECHANIC:UNTRUSTED_INPUT:FINDING_ERRORS:BEGIN/END>>>` security delimiters around `${FINDING_ERRORS}`
- Added `<<<MECHANIC:UNTRUSTED_INPUT:AI_ANALYSIS:BEGIN/END>>>` security delimiters around `${FINDING_DETAILS}`
- `=== CORRELATED GROUP ===` section already present (added in previous session); confirmed correct
- `## Correlated Findings` block in `=== PR BODY FORMAT ===` already present; confirmed correct
- Expanded STEP 7 to detail all three kubeconform cases (plain YAML, Kustomize overlay, Helm values)
- Added `FINDING_SEVERITY` context header before hard rules
- Added Rule 9 (untrusted input delimiters)
- Added Rule 10 (mandatory kubeconform before any `git commit`, with three-case procedure and fallback)
- Rule 11 (correlated findings) already present; confirmed correct

### `deploy/kustomize/configmap-prompt.yaml`

- Added `=== SELF-REMEDIATION GUIDANCE ===` section (was absent; present in default.txt)
- Added `=== SELF-REMEDIATION DECISION TREE ===` section (was absent; present in default.txt)
- Both files now have identical section structure

## Verification

- `helm template mechanic charts/mechanic --set gitops.repo=... --set gitops.manifestRoot=... ...` renders without error
- Section structure verified to be identical between both prompt files
- `go build ./...` — clean (no Go changes in this story)
- `go test -timeout 30s -race ./...` — all packages pass

## Gaps from Code Review

1. Self-remediation guidance and decision tree were absent from kustomize file — fixed.
2. default.txt was missing FINDING_SEVERITY field, security delimiters for FINDING_ERRORS and AI_ANALYSIS,
   Rule 9, and Rule 10 — fixed.
