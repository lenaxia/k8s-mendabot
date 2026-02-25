# Security Report: 2026-02-24

**Report Date:** 2026-02-24
**Reviewer:** automated (mendabot orchestrator)
**Review Type:** Partial (scope: regression review post-epics 11, 13–16, 21, 24)
**Cluster Available:** no
**CNI (NetworkPolicy Support):** no
**Previous Report:** [2026-02-23_security_report](../2026-02-23_security_report/README.md)
**Git Commit Reviewed:** 2a7d3e1ba0f3647e4a78dc3de5bbb2697e1c5cbf
**Status:** Closed

---

## Finding Summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| 2026-02-24-001 | MEDIUM | Prompt injection envelope missing from Helm chart core.txt | Remediated |
| 2026-02-24-002 | MEDIUM | Watcher ClusterRole grants unnecessary cluster-wide `secrets` permission | Remediated |
| 2026-02-24-003 | LOW | Pod unschedulable message not truncated before RedactSecrets | Open |
| 2026-02-24-004 | LOW | Priority annotation stabilisation bypass emits no audit log | Open |
| 2026-02-24-005 | LOW | Integer overflow in int32 casts in config.go and main.go | Open |
| 2026-02-24-006 | LOW | FINDING_CORRELATED_FINDINGS bypasses redaction/injection detection (latent) | Deferred |
| 2026-02-24-007 | INFO | Gosec G101 false positive on `githubAppSecretName` constant | Open |

**Counts:**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 0 | 0 | 0 | 0 |
| MEDIUM | 0 | 2 | 0 | 0 |
| LOW | 3 | 0 | 0 | 1 |
| INFO | 1 | 0 | 0 | 0 |
| **Total** | **4** | **2** | **0** | **1** |

---

## Scope

This review was a targeted regression check, not a full process run. It covered:
- Re-verification of all 6 original test cases from the 2026-02-23 report
- Full audit of new features introduced by epics 11, 13–16, 21, 24
- Static analysis (gosec, go vet) against all 30 Go source files
- Full test suite (`go test -timeout 30s -race ./...`)

**Phases completed:**

- [x] Phase 1: Static Code Analysis → [phase01_static.md](phase01_static.md)
- [x] Phase 2: Architecture and Design Review → [phase02_architecture.md](phase02_architecture.md)
- [x] Phase 3: Redaction and Injection Testing → [phase03_redaction.md](phase03_redaction.md)
- [x] Phase 4: RBAC Enforcement Testing → [phase04_rbac.md](phase04_rbac.md)
- [ ] Phase 5: Network Egress Testing → [phase05_network.md](phase05_network.md)
- [x] Phase 6: GitHub App Private Key Isolation → [phase06_privkey.md](phase06_privkey.md)
- [x] Phase 7: Audit Log Verification → [phase07_audit.md](phase07_audit.md)
- [ ] Phase 8: Supply Chain Integrity → [phase08_supply_chain.md](phase08_supply_chain.md)
- [ ] Phase 9: Operational Security → [phase09_operational.md](phase09_operational.md)
- [x] Phase 10: Regression Check → [phase10_regression.md](phase10_regression.md)

**Phases skipped:**

| Phase | Reason |
|-------|--------|
| Phase 5 | No live cluster with NetworkPolicy-aware CNI available |
| Phase 8 | Supply chain (go.sum, Dockerfile pins) unchanged since 2026-02-23 review |
| Phase 9 | Operational security (secret rotation, monitoring) unchanged since 2026-02-23 review |

---

## Executive Summary

This review re-examined the epic 12 attack surface after seven feature epics were merged
to `main` following the initial 2026-02-23 security review. All six original test case
controls remain effective. Two MEDIUM findings were identified and remediated in this
session: the untrusted-data envelope (STORY_05 control) was missing from the Helm chart
prompt, and the watcher ClusterRole granted cluster-wide `secrets` access that is
correctly scoped by the existing namespace-scoped `role-watcher` (which covers the
readiness checkers' access to `github-app` and `llm-credentials-*` Secrets in the
`mendabot` namespace). Four low-severity items remain open for a follow-up session.
No HIGH or CRITICAL findings were identified.

---

## Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | Notes |
|---------|-------------|------------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope, default) | yes | STORY_04 opt-in namespace scope unchanged |
| AR-02 | Redaction false negatives | yes | Best-effort heuristic; unchanged |
| AR-03 | NetworkPolicy requires CNI | yes | Manifest correct; operator CNI responsibility |
| AR-04 | Prompt injection not fully preventable | yes | Envelope restored (2026-02-24-001 remediated) |
| AR-05 | GitHub token in shared emptyDir | yes | Init/main container isolation unchanged |
| AR-06 | HARD RULEs are prompt-only controls | yes | HARD RULE 8 added (2026-02-24-001 remediated) |

**New accepted risks this review:**

| ID | Description | Severity | Rationale | Sign-off |
|----|-------------|----------|-----------|---------|
| — | None | — | — | — |

---

## Recommendations for Next Review

- Run a full process review (all 10 phases) when Epic 13 (multi-signal correlation) is
  activated — `FINDING_CORRELATED_FINDINGS` introduces a new data path (finding 2026-02-24-006)
- Address the four open LOW/INFO items (003, 004, 005, 007) in a dedicated hardening session
- Add `govulncheck ./...` output to the next review's `raw/` directory

---

## Sign-off

Checklist completed: no (partial review — phases 5, 8, 9 skipped with documented reasons)

**Reviewer:** automated (mendabot orchestrator)
**Date:** 2026-02-24

*Produced following `docs/SECURITY/PROCESS.md` v1.0*
