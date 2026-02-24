# Phase 10: Regression Check

**Date run:**
**Reviewer:**
**Previous report:** [link or "None — first review"]

---

## 10.1 Previous Findings Verification

For each finding from the most recent previous report, verify the remediation is
still in place. If there is no previous report, write "N/A — first review."

| Finding ID | Title | Previous Status | Still Remediated? | Evidence | Notes |
|-----------|-------|----------------|-------------------|---------|-------|
| | | | yes / no / N/A | | |

---

## 10.2 Accepted Residual Risk Re-confirmation

For each accepted residual risk in `docs/SECURITY/THREAT_MODEL.md`, confirm:
- The acceptance rationale is still valid
- No new controls could now address it (if yes, note as a finding)

| Risk ID | Description | Acceptance Still Valid? | New Control Available? | Notes |
|---------|-------------|------------------------|----------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | yes / no | yes / no | |
| AR-02 | Redaction false negatives | yes / no | yes / no | |
| AR-03 | NetworkPolicy requires CNI | yes / no | yes / no | |
| AR-04 | Prompt injection not fully preventable | yes / no | yes / no | |
| AR-05 | GitHub token in shared emptyDir | yes / no | yes / no | |
| AR-06 | HARD RULEs are prompt-only controls | yes / no | yes / no | |

---

## 10.3 Architecture Changes Since Last Review

List any significant changes to the codebase since the previous review that were
NOT covered by a separate dedicated review:

| Change | Files Affected | Security Implication | Reviewed in This Report? |
|--------|---------------|---------------------|--------------------------|
| | | | yes / no |

---

## Phase 10 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
