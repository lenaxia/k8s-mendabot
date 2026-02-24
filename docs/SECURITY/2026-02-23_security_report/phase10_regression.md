# Phase 10: Regression Check

**Date run:** 2026-02-23
**Reviewer:** OpenCode (automated review)
**Previous report:** None — this is the first security review

---

## 10.1 Previous Findings Verification

**N/A — first review.** No previous findings to verify.

---

## 10.2 Accepted Residual Risk Re-confirmation

All six accepted residual risks from `docs/SECURITY/THREAT_MODEL.md` reviewed. This is the first review, so all acceptances are being confirmed for the first time.

| Risk ID | Description | Acceptance Valid? | New Control Available? | Notes |
|---------|-------------|------------------|----------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | **yes** | no | ClusterRole still grants `get/list/watch` on `*`. Namespace scope mode (`AGENT_RBAC_SCOPE=namespace`) exists as a mitigating control for operators who can enumerate watch namespaces. Acceptance rationale still valid. |
| AR-02 | Redaction false negatives | **yes** | partial | Phase 3 identified three new redaction gaps (JWT Bearer, JSON password, Redis empty-user URL). These are findings 2026-02-23-010–012. They do not invalidate the acceptance but narrow the scope — the gaps are documented and should be closed. |
| AR-03 | NetworkPolicy requires CNI | **yes** | no | NetworkPolicy manifest exists; CNI is an operator responsibility. Acceptance rationale still valid. |
| AR-04 | Prompt injection not fully preventable | **yes** | no | The data envelope and HARD RULE 8 reduce risk. Phase 3 confirmed several injection patterns are not detected (findings 2026-02-23-013 and the upstream context-loss risk from 2026-02-23-003). Acceptance rationale still valid; mitigating controls are in place. |
| AR-05 | GitHub token in shared emptyDir | **yes** | no | Token is a short-lived installation token, not the private key. Init container isolation confirmed in Phase 6. Acceptance rationale still valid. |
| AR-06 | HARD RULEs are prompt-only controls | **yes** | no | HARD RULE 8 and the data envelope are the primary controls. The read-only RBAC posture provides a hard backstop. Acceptance rationale still valid. |

---

## 10.3 Architecture Changes Since Last Review

**N/A — first review.** All architecture has been reviewed in Phases 1–9 of this report.

---

## Phase 10 Summary

**Total findings:** 0 new
**Findings added to findings.md:** none
**Previous findings verified:** N/A — first review
**Accepted risks re-confirmed:** All 6 — AR-01 through AR-06 still valid
**Note:** AR-02 (redaction false negatives) is partially addressed by this review. Findings 2026-02-23-010, 2026-02-23-011, and 2026-02-23-012 should be fixed before the next review cycle to reduce the accepted residual risk surface.
