# Phase 10: Regression Check

**Date run:** 2026-02-24
**Previous reports:** 2026-02-23, 2026-02-24 (two prior reviews)

---

## 10.1 Previous Findings Verification

### From 2026-02-23 report

| Finding ID | Title | Previous Status | Still Remediated? | Evidence |
|-----------|-------|----------------|-------------------|---------|
| 2026-02-23-001 through 2026-02-23-013 | (13 findings) | Remediated | Yes | All addressed in Epic 12; 2026-02-24 report re-confirmed |

All 2026-02-23 findings were marked Remediated in the 2026-02-24 report and re-verified against the current codebase. No regressions.

### From 2026-02-24 report

| Finding ID | Title | Previous Status | Still Remediated / Still Open? | Evidence |
|-----------|-------|----------------|-------------------------------|---------|
| 2026-02-24-001 | Prompt injection envelope missing from Helm chart | Remediated | **Verified Remediated** | `charts/mendabot/files/prompts/core.txt` has `=== BEGIN/END FINDING ERRORS ===` and HARD RULE 8 |
| 2026-02-24-002 | Watcher ClusterRole grants cluster-wide `secrets` | Remediated | **REGRESSION in deployed cluster** | Chart source fixed; live cluster not redeployed. Live `mendabot-watcher` ClusterRole still has `"secrets"`. See finding P-005. |
| 2026-02-24-003 | Pod unschedulable message not truncated | Open | **Still open** | `internal/provider/native/pod.go:104` — no truncate before RedactSecrets |
| 2026-02-24-004 | Priority annotation bypass emits no audit log | Open | **Still open** | `internal/provider/provider.go:260–261` — no log on priority critical path |
| 2026-02-24-005 | Integer overflow int32 casts | Open | **Still open** | `config.go:222`, `main.go:105` — unchanged |
| 2026-02-24-006 | FINDING_CORRELATED_FINDINGS bypasses redaction (latent) | Deferred | **Still deferred / latent** | Controller still calls `Build(rjob, nil)` — path not active |
| 2026-02-24-007 | Gosec G101 false positive on githubAppSecretName | Open | **Still open** | No `//nolint:gosec` comment added |

**Critical note on 2026-02-24-002:** The finding was marked Remediated in the source code but the fix was never applied to the live cluster. This means the cluster is running with the pre-fix RBAC. This is recorded as new finding P-005 in this report.

---

## 10.2 Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | New Control Available? | Notes |
|---------|-------------|------------------------|----------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | Yes | No | nodes/proxy access (P-004) is an additional dimension of this same risk; the wildcard ClusterRole is the root cause of both |
| AR-02 | Redaction false negatives | Yes | No | Two new gaps found (P-006, P-007) but severity remains LOW and the pattern is best-effort by design |
| AR-03 | NetworkPolicy requires CNI | Yes | Yes — Cilium is deployed | Cilium is running but NetworkPolicy was not applied (P-009). This risk is now partially addressable — operator action required |
| AR-04 | Prompt injection not fully preventable | Yes | No new controls | LLM correctly refused pentest payload but this is not a technical control |
| AR-05 | GitHub token in shared emptyDir | Yes | No | Design unchanged; isolation verified in Phase 6 |
| AR-06 | HARD RULEs are prompt-only controls | Yes | No | Prompt envelope verified present |

**AR-03 update:** The acceptance rationale included "Operator responsibility; documented prerequisite." However, this cluster has Cilium deployed and capable. The NetworkPolicy not being applied is now a deployment gap rather than a CNI availability gap. Recommend re-categorising P-009 as MEDIUM finding rather than fully accepted.

---

## 10.3 Architecture Changes Since Last Review

The 2026-02-24 review was a partial review covering epics 11, 13–16, 21, 24. This review covers the same codebase with a live cluster pentest.

No architectural changes were made between the 2026-02-24 report and this pentest.

---

## Phase 10 Summary

**Regression discovered:** 2026-02-24-002 was remediated in source but not deployed (finding P-005).
**All other remediations verified intact.**
**Open findings (003, 004, 005, 006, 007) still open — no regressions, no improvements.**

**Total new findings:** 0 (regressions already captured as P-005)
