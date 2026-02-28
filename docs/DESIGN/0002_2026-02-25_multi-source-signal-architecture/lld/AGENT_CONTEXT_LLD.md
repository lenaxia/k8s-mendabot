# Agent Context Enrichment — Low-Level Design

**Version:** 1.0
**Date:** 2026-02-25
**Status:** Proposed
**HLD Reference:** [§12](../HLD.md)

---

## 1. Overview

The agent Job receives finding context via environment variables. In v1 all findings come
from native K8s providers, so the variable set is homogeneous. In v2 findings may come from
external alert sources that carry richer context (alert name, raw label set, previous PR URL).

This LLD specifies:
- New environment variables injected by the `jobbuilder`
- How `FINDING_ERRORS` is constructed for alert-sourced findings
- Changes to `agent-entrypoint.sh`
- Changes to the prompt template (`default.txt`) for the two new behaviours:
  1. Use alert labels for richer investigation when `FINDING_SOURCE_TYPE != "native"`
  2. Update an existing PR rather than opening a new one when `FINDING_PREVIOUS_PR_URL` is set

---

## 2. New Environment Variables

### 2.1 Full Variable Set (v2)

| Variable | v1 | v2 | Source |
|---|---|---|---|
| `FINDING_KIND` | ✓ | ✓ | `RemediationJob.Spec.Finding.Kind` |
| `FINDING_NAME` | ✓ | ✓ | `RemediationJob.Spec.Finding.Name` |
| `FINDING_NAMESPACE` | ✓ | ✓ | `RemediationJob.Spec.Finding.Namespace` |
| `FINDING_PARENT` | ✓ | ✓ | `RemediationJob.Spec.Finding.ParentObject` |
| `FINDING_ERRORS` | ✓ | ✓ | `RemediationJob.Spec.Finding.Errors` (JSON) |
| `FINDING_DETAILS` | ✓ | ✓ | `RemediationJob.Spec.Finding.Details` |
| `FINDING_FINGERPRINT` | ✓ | ✓ | `RemediationJob.Spec.Fingerprint` |
| `FINDING_CORRELATED_FINDINGS` | ✓ | ✓ | JSON array; set for correlated primaries |
| `FINDING_CORRELATION_GROUP_ID` | ✓ | ✓ | Set if correlation group label present |
| `IS_SELF_REMEDIATION` | ✓ | ✓ | Bool string |
| `CHAIN_DEPTH` | ✓ | ✓ | Int string |
| `FINDING_SOURCE_TYPE` | — | ✓ NEW | `RemediationJob.Spec.SourceType` |
| `FINDING_ALERT_NAME` | — | ✓ NEW | `RemediationJob.Spec.Finding.AlertName` (empty for native) |
| `FINDING_ALERT_LABELS` | — | ✓ NEW | JSON map; `RemediationJob.Spec.Finding.AlertLabels` (empty for native) |
| `FINDING_PREVIOUS_PR_URL` | — | ✓ NEW | `RemediationJob.Spec.Finding.PreviousPRURL` (empty if not set) |

### 2.2 jobbuilder Changes

In `internal/jobbuilder/job.go`, add the new variables to the `envVars` slice:

```go
// existing variables...

// New v2 variables (always injected; empty string if not applicable)
{Name: "FINDING_SOURCE_TYPE",     Value: rjob.Spec.SourceType},
{Name: "FINDING_ALERT_NAME",      Value: rjob.Spec.Finding.AlertName},
{Name: "FINDING_ALERT_LABELS",    Value: alertLabelsJSON(rjob.Spec.Finding.AlertLabels)},
{Name: "FINDING_PREVIOUS_PR_URL", Value: rjob.Spec.Finding.PreviousPRURL},
```

Helper:
```go
func alertLabelsJSON(labels map[string]string) string {
    if len(labels) == 0 {
        return ""
    }
    b, _ := json.Marshal(labels)
    return string(b)
}
```

Injecting empty strings for unused variables ensures prompt template `${VAR:-default}` bash
substitution works correctly without conditional logic in `agent-entrypoint.sh`.

---

## 3. FINDING_ERRORS Construction for Alert Sources

For native findings, `FINDING_ERRORS` is the raw error JSON from the provider
(e.g. `[{"text":"container nginx: waiting: CrashLoopBackOff"}]`).

For alert-sourced findings, the adapter constructs `FINDING_ERRORS` as a human-readable
summary of the alert name and key identifying labels:

```
[{"text":"<alertname>: <k1>=<v1> <k2>=<v2> ..."}]
```

Key labels included in the summary (in this order, if present):
1. `namespace`
2. `deployment` / `pod` / `node` / `service` / `pvc` (whichever resolved the resource)
3. `severity`
4. `reason` (if present)

Example:
```
[{"text":"KubeDeploymentReplicasMismatch: namespace=default deployment=test-broken-image severity=warning"}]
```

The full raw label set is in `FINDING_ALERT_LABELS`. `FINDING_ERRORS` is intentionally
concise to avoid cluttering the primary error field with verbose label dumps.

---

## 4. agent-entrypoint.sh Changes

### 4.1 New Variable Export

```bash
# Existing exports (unchanged)
export FINDING_KIND FINDING_NAME FINDING_NAMESPACE FINDING_PARENT
export FINDING_ERRORS FINDING_DETAILS FINDING_FINGERPRINT
# ...

# New v2 exports
export FINDING_SOURCE_TYPE="${FINDING_SOURCE_TYPE:-native}"
export FINDING_ALERT_NAME="${FINDING_ALERT_NAME:-}"
export FINDING_ALERT_LABELS="${FINDING_ALERT_LABELS:-}"
export FINDING_PREVIOUS_PR_URL="${FINDING_PREVIOUS_PR_URL:-}"
```

### 4.2 Prompt Variable Substitution

The `envsubst` call that renders the prompt template already substitutes all exported
variables. No changes to the substitution call are needed — the new variables are
automatically available in the rendered prompt.

### 4.3 Validation

The existing validation block checks that required variables are non-empty. The new
variables are all optional (they may be empty for native findings). No new required-variable
checks are added.

---

## 5. Prompt Template Changes

The prompt template (`charts/mechanic/files/prompts/default.txt`) is updated in two places.

### 5.1 Alert Context Section (new)

Added after the finding context section. Because `envsubst` does not support `${VAR:+text}`
conditional expansion, the conditional block is pre-rendered in `agent-entrypoint.sh` into
a variable `ALERT_CONTEXT_BLOCK`, which `envsubst` then substitutes unconditionally:

```bash
# agent-entrypoint.sh (new section)
if [[ "${FINDING_SOURCE_TYPE}" != "native" && -n "${FINDING_SOURCE_TYPE}" ]]; then
    export ALERT_CONTEXT_BLOCK="## Alert Source Context

This finding was generated by: **${FINDING_SOURCE_TYPE}**
Alert name: **${FINDING_ALERT_NAME}**
Labels: \`${FINDING_ALERT_LABELS}\`

Use these labels to query the monitoring system for metric history and trend data."
else
    export ALERT_CONTEXT_BLOCK=""
fi
```

The prompt template uses `${ALERT_CONTEXT_BLOCK}` which `envsubst` replaces with the
fully-rendered block or empty string.

### 5.2 PR Handling Section (updated)

The existing "Check for existing PRs" step in the prompt is extended to handle
`FINDING_PREVIOUS_PR_URL`:

**Existing (v1):**
```
1. Check for an existing PR on branch fix/mechanic-${FINDING_FINGERPRINT}.
   If found, add a comment with updated findings and exit. Do not open a duplicate PR.
```

**Updated (v2):**
```
1. PR handling — follow exactly ONE of these paths, in order:

   a. If FINDING_PREVIOUS_PR_URL is set ("${FINDING_PREVIOUS_PR_URL}"):
      A previous investigation ran for this resource and opened or updated this PR.
      - Fetch and read the existing PR: gh pr view <URL>
      - Understand what was already investigated and proposed
      - Update the PR with the new higher-quality signal from ${FINDING_SOURCE_TYPE}:
        * If the previous fix is still correct: add a comment confirming with new context
        * If the previous fix was incomplete or incorrect: amend the PR description and
          push an updated commit to the same branch
      - Do NOT open a new PR. Do NOT open a new branch.
      - Exit after updating.

   b. If an open PR exists on branch fix/mechanic-${FINDING_FINGERPRINT}:
      (Check: gh pr list --repo ${GITOPS_REPO} --state open --head fix/mechanic-${FINDING_FINGERPRINT})
      - Add a comment to the existing PR with updated findings
      - Do NOT open a duplicate PR
      - Exit after commenting.

   c. Otherwise:
      - Proceed with investigation (steps 2 onwards)
      - Open a new PR on branch fix/mechanic-${FINDING_FINGERPRINT}
```

This three-path structure ensures:
- Path (a): higher-priority alert updates the existing PR from the lower-priority investigation
- Path (b): idempotent re-runs of the same finding add context rather than duplicating
- Path (c): normal first-time investigation

Similarly, render `PREVIOUS_PR_BLOCK` in the entrypoint:

```bash
if [[ -n "${FINDING_PREVIOUS_PR_URL}" ]]; then
    export PREVIOUS_PR_BLOCK="A previous investigation for this resource produced: ${FINDING_PREVIOUS_PR_URL}
Read this PR before proceeding. Update it rather than opening a new one."
else
    export PREVIOUS_PR_BLOCK=""
fi
```

---

## 6. RemediationJob Spec Changes (jobbuilder view)

The `jobbuilder` reads these fields from `RemediationJobSpec.Finding`:

```go
// api/v1alpha1/remediationjob_types.go (additions to FindingSpec)
type FindingSpec struct {
    // ... existing fields unchanged ...

    // AlertName is the Prometheus/PagerDuty/OpsGenie alert name.
    // Empty for native-sourced findings.
    // +optional
    AlertName string `json:"alertName,omitempty"`

    // AlertLabels is the complete set of labels from the originating alert.
    // Empty for native-sourced findings.
    // +optional
    AlertLabels map[string]string `json:"alertLabels,omitempty"`

    // PreviousPRURL is the GitHub PR URL from a prior RemediationJob for this resource.
    // Set when this RJ was created from a pending-alert annotation after a Succeeded RJ.
    // +optional
    PreviousPRURL string `json:"previousPRURL,omitempty"`
}
```

---

## 7. Testing Strategy

### Unit tests — jobbuilder

| Test | Description |
|---|---|
| `TestJobBuilder_InjectsSourceType` | `FINDING_SOURCE_TYPE` present in Job env vars |
| `TestJobBuilder_InjectsAlertName` | `FINDING_ALERT_NAME` present when set |
| `TestJobBuilder_InjectsAlertLabels` | `FINDING_ALERT_LABELS` is valid JSON when labels are present |
| `TestJobBuilder_InjectsEmptyAlertLabels` | `FINDING_ALERT_LABELS` is empty string (not "null") for native findings |
| `TestJobBuilder_InjectsPreviousPRURL` | `FINDING_PREVIOUS_PR_URL` present when set |
| `TestJobBuilder_InjectsEmptyPreviousPRURL` | `FINDING_PREVIOUS_PR_URL` is empty string when not set |

### Prompt integration tests (manual / LLM eval)

These are not automated unit tests — they are evaluation criteria for prompt review:

| Scenario | Expected agent behaviour |
|---|---|
| `FINDING_SOURCE_TYPE=alertmanager`, `FINDING_ALERT_LABELS` set | Agent references alert labels in investigation; mentions Prometheus metric history |
| `FINDING_SOURCE_TYPE=native`, `FINDING_ALERT_LABELS=""` | Alert context block absent from rendered prompt; agent uses standard investigation flow |
| `FINDING_PREVIOUS_PR_URL` set, PR exists on GitHub | Agent fetches existing PR, updates it, does NOT open a new PR |
| `FINDING_PREVIOUS_PR_URL` set, PR was merged | Agent creates a new PR (the existing one is closed/merged, not open) |
| `FINDING_PREVIOUS_PR_URL=""` | Agent checks for existing branch PR, creates new if not found |
