# Security Report: 2026-02-23

**Report Date:** 2026-02-23
**Reviewer:** OpenCode (automated review)
**Review Type:** Full (all 10 phases; cluster-dependent tests deferred — see Scope)
**Cluster Available:** no
**CNI (NetworkPolicy Support):** N/A
**Previous Report:** None — first review
**Git Commit Reviewed:** fb639b6c67d451bbae01cb644d214210d399f05a
**Status:** Closed

---

## Finding Summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| 2026-02-23-001 | MEDIUM | Go standard library vulnerabilities (govulncheck) | Open |
| 2026-02-23-002 | INFO | Unhandled error in Prometheus metrics registration | Accepted |
| 2026-02-23-003 | MEDIUM | `FINDING_DETAILS` has no injection detection or prompt envelope | Open |
| 2026-02-23-004 | LOW | LLM config JSON built with `printf` — operator values not sanitised | Accepted |
| 2026-02-23-005 | MEDIUM | Watcher ClusterRole grants ConfigMap write cluster-wide | Open |
| 2026-02-23-006 | MEDIUM | Missing SHA256 checksum for yq, age, and opencode in Dockerfile.agent | Open |
| 2026-02-23-007 | LOW | Base images not pinned to digest | Open |
| 2026-02-23-008 | LOW | GitHub Actions not pinned to commit SHA | Open |
| 2026-02-23-009 | LOW | Trivy CI scan only fails on CRITICAL severity | Open |
| 2026-02-23-010 | MEDIUM | JWT Bearer token not redacted by `RedactSecrets` | Open |
| 2026-02-23-011 | LOW | JSON-encoded credentials not redacted (`"password":"value"`) | Open |
| 2026-02-23-012 | LOW | Redis URL with empty username not redacted | Open |
| 2026-02-23-013 | INFO | Injection detection gap: "stop following the rules" variant | Open |

**Counts:**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 0 | 0 | 0 | 0 |
| MEDIUM | 4 | 0 | 0 | 0 |
| LOW | 5 | 0 | 2 | 0 |
| INFO | 2 | 0 | 1 | 0 |
| **Total** | **11** | **0** | **2** | **0** |

---

## Scope

**Phases completed:**

- [x] Phase 1: Static Code Analysis → [phase01_static.md](phase01_static.md)
- [x] Phase 2: Architecture and Design Review → [phase02_architecture.md](phase02_architecture.md)
- [x] Phase 3: Redaction and Injection Testing → [phase03_redaction.md](phase03_redaction.md)
- [x] Phase 4: RBAC Enforcement Testing → [phase04_rbac.md](phase04_rbac.md)
- [x] Phase 5: Network Egress Testing → [phase05_network.md](phase05_network.md)
- [x] Phase 6: GitHub App Private Key Isolation → [phase06_privkey.md](phase06_privkey.md)
- [x] Phase 7: Audit Log Verification → [phase07_audit.md](phase07_audit.md)
- [x] Phase 8: Supply Chain Integrity → [phase08_supply_chain.md](phase08_supply_chain.md)
- [x] Phase 9: Operational Security → [phase09_operational.md](phase09_operational.md)
- [x] Phase 10: Regression Check → [phase10_regression.md](phase10_regression.md)

**Phases skipped:**

| Phase | Reason |
|-------|--------|
| Phase 3 — E2E tests (3.3) | No cluster available |
| Phase 4 — live RBAC tests (4.1, 4.2, 4.4 partial) | No cluster available |
| Phase 5 — all live egress tests | No cluster available; no CNI |
| Phase 6 — live container env checks (6.2) | No cluster available |
| Phase 7 — live audit log collection (7.1, 7.2 fire-and-observe) | No cluster available |
| Phase 8 — Trivy image scans (8.3) | No Docker in review environment |
| Phase 1 — staticcheck (1.1) | Binary not installed |
| Phase 1 — go list -u outdated check (1.2) | Command timed out |

---

## Executive Summary

This is the first security review of mendabot following the `docs/SECURITY/PROCESS.md` v1.0 process. The review covered all ten phases by code review; cluster-dependent live tests were deferred due to the absence of a running cluster in the review environment. No CRITICAL or HIGH findings were identified.

The overall security posture is good. Core controls — secret redaction in all providers, prompt injection detection, init-container key isolation, read-only agent RBAC, prompt envelope, and HARD RULE 8 — are all confirmed to be in place and correctly implemented. Audit logging is comprehensive and does not leak credential values.

The most significant open findings are four MEDIUM-severity items: (1) three Go standard library CVEs requiring a toolchain upgrade; (2) a missing injection-detection and prompt-envelope check on the `FINDING_DETAILS` field, which represents a secondary injection path; (3) cluster-wide ConfigMap write permission in the watcher ClusterRole, which exceeds the least-privilege principle; and (4) missing SHA256 checksum verification for three binaries in the agent Dockerfile (`yq`, `age`, `opencode`). None of these represent an immediately exploitable HIGH or CRITICAL vulnerability given the current deployment context, but all should be addressed in the next development cycle.

Three new redaction gaps were identified in `domain.RedactSecrets` (JWT Bearer headers, JSON-encoded credentials, and Redis empty-username URLs) that partially narrow the accepted residual risk AR-02 (redaction false negatives).

---

## Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | Notes |
|---------|-------------|------------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | **yes** | Namespace scope mode available as opt-in mitigation |
| AR-02 | Redaction false negatives | **yes** | New gaps found (2026-02-23-010–012); gaps documented and should be fixed |
| AR-03 | NetworkPolicy requires CNI | **yes** | Manifests exist; CNI is operator responsibility |
| AR-04 | Prompt injection not fully preventable | **yes** | Envelope + HARD RULE 8 + read-only RBAC mitigate; 2026-02-23-003 is a gap to close |
| AR-05 | GitHub token in shared emptyDir | **yes** | Init container isolation confirmed |
| AR-06 | HARD RULEs are prompt-only controls | **yes** | Read-only RBAC provides hard backstop |

**New accepted risks this review:**

| ID | Description | Severity | Rationale | Sign-off |
|----|-------------|----------|-----------|---------|
| — | None | — | — | — |

---

## Recommendations for Next Review

1. **Cluster required** — The next review must include a running cluster with a NetworkPolicy-aware CNI (Cilium or Calico) to complete Phases 3.3, 4, 5, 6.2, and 7 live tests.
2. **Fix MEDIUM findings before next review** — Findings 2026-02-23-001 (Go CVEs), 2026-02-23-003 (FINDING_DETAILS injection path), 2026-02-23-005 (watcher ConfigMap write), and 2026-02-23-006 (binary checksums) should be remediated.
3. **Trivy scan results** — Review CI Trivy scan output for the current release to assess any HIGH CVEs in base images (deferred this cycle due to no Docker environment).
4. **Install staticcheck** — Add `staticcheck` to the review environment or CI to close the gap from 1.1.
5. **AR-02 surface narrowed** — After fixing 2026-02-23-010–012, re-run the redaction gap analysis to update the accepted residual risk scope.

---

## Sign-off

Checklist completed: yes (cluster-dependent items marked SKIP with reason)

**Reviewer:** OpenCode (automated review)
**Date:** 2026-02-23

*Produced following `docs/SECURITY/PROCESS.md` v1.0*
