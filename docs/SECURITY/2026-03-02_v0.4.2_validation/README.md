# Security Report: YYYY-MM-DD

**Report Date:** YYYY-MM-DD
**Reviewer:**
**Review Type:** Full / Partial (scope: _____________)
**Cluster Available:** yes / no
**CNI (NetworkPolicy Support):** yes / no / N/A
**Previous Report:** [link or "None"]
**Git Commit Reviewed:** <!-- git rev-parse HEAD -->
**Status:** Open / Closed

---

## Finding Summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| | | | |

**Counts:**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 0 | 0 | 0 | 0 |
| MEDIUM | 0 | 0 | 0 | 0 |
| LOW | 0 | 0 | 0 | 0 |
| INFO | 0 | 0 | 0 | 0 |
| **Total** | **0** | **0** | **0** | **0** |

---

## Scope

**Phases completed:**

- [ ] Phase 1: Static Code Analysis → [phase01_static.md](phase01_static.md)
- [ ] Phase 2: Architecture and Design Review → [phase02_architecture.md](phase02_architecture.md)
- [ ] Phase 3: Redaction and Injection Testing → [phase03_redaction.md](phase03_redaction.md)
- [ ] Phase 4: RBAC Enforcement Testing → [phase04_rbac.md](phase04_rbac.md)
- [ ] Phase 5: Network Egress Testing → [phase05_network.md](phase05_network.md)
- [ ] Phase 6: GitHub App Private Key Isolation → [phase06_privkey.md](phase06_privkey.md)
- [ ] Phase 7: Audit Log Verification → [phase07_audit.md](phase07_audit.md)
- [ ] Phase 8: Supply Chain Integrity → [phase08_supply_chain.md](phase08_supply_chain.md)
- [ ] Phase 9: Operational Security → [phase09_operational.md](phase09_operational.md)
- [ ] Phase 10: Regression Check → [phase10_regression.md](phase10_regression.md)

**Phases skipped:**

| Phase | Reason |
|-------|--------|
| | |

---

## Executive Summary

<!-- Write last. 3–5 sentences covering: what was reviewed, what was found,
     overall security posture, and the most important open items or changes. -->

---

## Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | Notes |
|---------|-------------|------------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | yes / no | |
| AR-02 | Redaction false negatives | yes / no | |
| AR-03 | NetworkPolicy requires CNI | yes / no | |
| AR-04 | Prompt injection not fully preventable | yes / no | |
| AR-05 | GitHub token in shared emptyDir | yes / no | |
| AR-06 | HARD RULEs are prompt-only controls | yes / no | |
| AR-07 | `curl` bypasses dry-run wrappers | yes / no | |
| AR-08 | Watcher ClusterRole cluster-wide secrets read | yes / no | |

**New accepted risks this review:**

| ID | Description | Severity | Rationale | Sign-off |
|----|-------------|----------|-----------|---------|
| | | | | |

---

## Recommendations for Next Review

<!-- New attack surfaces, areas that need more time, tooling improvements. -->

---

## Sign-off

Checklist completed: yes / no (if no, explain)

**Reviewer:**
**Date:**

*Produced following `docs/SECURITY/PROCESS.md` v1.0*
