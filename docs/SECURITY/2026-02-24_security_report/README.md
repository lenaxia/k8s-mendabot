# Security Report: 2026-02-24 (Full Pentest — v0.3.9, live cluster)

**Report Date:** 2026-02-24
**Review Type:** Full pentest — all 10 phases, live cluster
**Cluster Available:** yes (6-node Talos cluster, default namespace)
**CNI (NetworkPolicy Support):** yes (Cilium)
**Previous Report:** [2026-02-23](../2026-02-23_security_report/README.md)
**Git Commit Reviewed:** 7488cff (v0.3.9 pre-remediation)
**Remediation Commit:** cd7d53b
**Status:** Closed — all code-addressable findings remediated; 2 operator actions pending

---

## Finding Summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| 2026-02-24-P-004 | HIGH | Agent ClusterRole wildcard grants nodes/proxy access | Remediated |
| 2026-02-24-P-005 | HIGH | Watcher ClusterRole secrets — live cluster not upgraded | Upgrade-pending |
| 2026-02-24-P-008 | MEDIUM | DetectInjection not called in RemediationJobController dispatch | Remediated |
| 2026-02-24-P-009 | MEDIUM | NetworkPolicy not deployed despite Cilium available | Operator-action |
| 2026-02-24-P-003 | MEDIUM | golang.org/x/net v0.30.0 — 4 CVEs | Remediated |
| 2026-02-24-P-006 | LOW | PEM private key header not covered by redaction | Remediated |
| 2026-02-24-P-007 | LOW | X-API-Key HTTP header not covered by redaction | Remediated |
| 2026-02-24-P-002 | LOW | Local dev Go toolchain 1.25.5 — 3 CVEs (images unaffected) | Open (dev env) |
| 2026-02-24-P-001 | INFO | Unused `ptr` generic helper in job_test.go | Remediated |
| 2026-02-24-003 | LOW | Pod unschedulable message not truncated before RedactSecrets | Remediated |
| 2026-02-24-004 | LOW | Priority annotation bypass emits no audit log | Remediated |
| 2026-02-24-005 | LOW | int32 overflow in config.go / main.go casts | Remediated |
| 2026-02-24-006 | LOW | FINDING_CORRELATED_FINDINGS bypasses redaction (latent) | Deferred |
| 2026-02-24-007 | INFO | Gosec G101 false positive on githubAppSecretName | Remediated |

**Counts (post-remediation):**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 1 | 1 | 0 | 0 |
| MEDIUM | 1 | 2 | 0 | 0 |
| LOW | 2 | 5 | 0 | 1 |
| INFO | 0 | 2 | 0 | 0 |
| **Total** | **3** | **10** | **0** | **1** |

*Open = operator action required (P-005 helm upgrade, P-009 NetworkPolicy, P-002 local toolchain). No code changes outstanding.*

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
| Phase 5 (egress live test) | No pod-level network testing tools available in cluster; Cilium presence confirmed, NetworkPolicy absence confirmed |

---

## Executive Summary

A full 10-phase pentest was conducted against mechanic v0.3.9 deployed in the live cluster (default namespace, 6-node Talos/Cilium). The overall security posture is **good**: no CRITICAL findings, two HIGH findings both of which are either remediated or require a single helm upgrade. The most significant new finding was the agent ClusterRole using `resources: ["*"]` wildcard, which exposed `nodes/proxy` (kubelet HTTP proxy) to the agent ServiceAccount — this was remediated by replacing the wildcard with an explicit resource list. A live injection test confirmed that the LLM prompt envelope (HARD RULE 8) successfully refused a direct kubectl exfiltration command injected via a crafted RemediationJob, though the `DetectInjection` gate was absent from the controller dispatch path — this gap was also remediated. Two redaction gaps (PEM private key headers, X-API-Key HTTP header format) were identified and fixed. The two remaining open items are operator actions: a `helm upgrade` to apply the already-fixed watcher ClusterRole to the live cluster, and deployment of a Cilium NetworkPolicy to restrict agent egress.

---

## Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | Notes |
|---------|-------------|------------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | yes | Required for investigation mandate; scoped to get/list/watch only |
| AR-02 | Redaction false negatives | yes | PEM + X-API-Key gaps now closed; novel formats remain a known limitation |
| AR-03 | NetworkPolicy requires CNI | yes | Cilium present; NetworkPolicy not yet applied — P-009 is open operator action |
| AR-04 | Prompt injection not fully preventable | yes | DetectInjection now in controller path; LLM envelope is second layer |
| AR-05 | GitHub token in shared emptyDir | yes | Init/main container separation unchanged; token TTL 1h |
| AR-06 | HARD RULEs are prompt-only controls | yes | No change; acknowledged as design constraint |

**New accepted risks this review:** None.

---

## Recommendations for Next Review

1. Verify P-005 (helm upgrade) and P-009 (NetworkPolicy) are applied before next pentest — both are operator actions with no code dependency.
2. Activate Epic 13 only after remediating 2026-02-24-006 (correlated findings redaction in `jobbuilder/job.go`).
3. Consider adding `govulncheck` to CI to catch vulnerable indirect dependencies earlier (P-003 was only caught in the manual review).
4. Expand injection detection test coverage to include novel phrasing discovered in this review (`new directive`, persona-shift attempts).

---

## Sign-off

Checklist completed: yes — [checklist.md](checklist.md)

**Date:** 2026-02-24

*Produced following `docs/SECURITY/PROCESS.md` v1.0*
