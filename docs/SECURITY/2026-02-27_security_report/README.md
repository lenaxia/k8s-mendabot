# Security Report: 2026-02-27

**Report Date:** 2026-02-27
**Reviewer:** OpenCode (AI-assisted)
**Review Type:** Partial (scope: RBAC re-classification + broad code review; no live cluster)
**Cluster Available:** no
**CNI (NetworkPolicy Support):** N/A
**Previous Report:** [2026-02-24_pentest_report](../2026-02-24_pentest_report/README.md)
**Git Commit Reviewed:** e757ae4336ae0397c3a9255cca87afc0ff4c817f
**Status:** Open

---

## Finding Summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| 2026-02-27-001 | MEDIUM | Watcher ClusterRole `secrets` — root cause identified; namespace Role now sufficient pending live verification | Accepted (AR-08) |
| 2026-02-27-002 | HIGH | GitHub App private key exposed as plain-text env var in watcher pod when `prAutoClose=true` | Open |
| 2026-02-27-003 | MEDIUM | `git` dry-run wrapper only blocks `push`/`commit`/annotated `tag` — destructive subcommands pass through | Open |
| 2026-02-27-004 | MEDIUM | `gh api` not blocked in dry-run mode — raw REST/GraphQL write calls bypass all dry-run enforcement | Open |
| 2026-02-27-005 | HIGH | `agentImage` field has no CRD validation — any creator of a RemediationJob can substitute an arbitrary image | Open |
| 2026-02-27-006 | HIGH | `agentSA` field has no CRD validation — any creator of a RemediationJob can substitute an arbitrary ServiceAccount | Open |
| 2026-02-27-007 | HIGH | 4 CI workflows use `anomalyco/opencode/github@latest` (unpinned) — supply chain risk with `contents: write` | Open |
| 2026-02-27-008 | HIGH | `renovate-analysis.yml` gives an LLM autonomous merge authority — no human gate | Open |
| 2026-02-27-009 | MEDIUM | `emit_dry_run_report` writes agent output and `git diff` to ConfigMap without redaction | Open |
| 2026-02-27-010 | MEDIUM | Circuit breaker state is in-memory only — resets to zero on watcher pod restart | Open |
| 2026-02-27-011 | MEDIUM | Agent Job containers have no resource limits — unbounded CPU/memory consumption | Open |
| 2026-02-27-012 | LOW | `sinkRef.url` has no format validation — unvalidated URL written to status, events, and logs | Open |
| 2026-02-27-013 | HIGH | `ai-comment.yml` — any authenticated GitHub user can trigger a privileged LLM run via `/ai` comment | Open |
| 2026-02-27-014 | LOW | `finding.Details` is never passed through `RedactSecrets` — latent when Details field is populated | Deferred |

**Counts:**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 5 | 0 | 0 | 0 |
| MEDIUM | 5 | 0 | 1 | 0 |
| LOW | 2 | 0 | 0 | 0 |
| INFO | 0 | 0 | 0 | 1 |
| **Total** | **12** | **0** | **1** | **1** |

---

## Scope

**Phases completed:**

- [x] Phase 1: Static Code Analysis (partial — no automated scan run; manual code review)
- [x] Phase 2: Architecture and Design Review → [phase02_architecture.md](phase02_architecture.md)
- [x] Phase 3: Redaction and Injection Testing (partial — code review only, no live tests)
- [x] Phase 4: RBAC Enforcement Testing (partial — code and chart review only)
- [ ] Phase 5: Network Egress Testing
- [ ] Phase 6: GitHub App Private Key Isolation
- [ ] Phase 7: Audit Log Verification
- [x] Phase 8: Supply Chain Integrity (CI/CD workflow review)
- [x] Phase 9: Operational Security (resource limits, circuit breaker, dry-run)
- [ ] Phase 10: Regression Check

**Phases skipped:**

| Phase | Reason |
|-------|--------|
| 5, 6, 7, 10 | No live cluster available |

---

## Executive Summary

This review started as a targeted re-classification of P-005 (watcher ClusterRole secrets)
and expanded into a broad code review of all Go source, RBAC templates, CI/CD workflows,
and agent scripts. Thirteen new findings were identified, covering a range of severities.

The five HIGH findings are the most actionable: two CI/CD workflows (`renovate-analysis.yml`
and `ai-comment.yml`) give an LLM or external users direct write access to the main branch
with no human gate; the `agentImage` and `agentSA` CRD fields have no validation allowing
in-cluster adversaries to substitute arbitrary images or ServiceAccounts; and the GitHub App
private key is exposed as a plain-text environment variable in the watcher pod when
`prAutoClose=true`.

The four open MEDIUM findings affect dry-run enforcement integrity and operational safety:
the `git` wrapper's blocklist is incomplete (only covers `push`/`commit`), `gh api` bypasses
dry-run entirely, the `emit_dry_run_report` function writes unredacted agent output to the
Kubernetes API, and the circuit breaker resets on pod restart.

For finding 2026-02-27-001 (watcher ClusterRole secrets), analysis of the git history
identified the precise root cause of the previous failed removal: the namespace Role had
only `secrets: get` at the time (not `list, watch`). The namespace Role now has all three
verbs and the cache is scoped via `cache.ByObject`. The ClusterRole entry is likely
removable with a live cluster test — it is retained and classified as AR-08 pending that
verification.

---

## Accepted Residual Risk Re-confirmation

| Risk ID | Description | Acceptance Still Valid? | Notes |
|---------|-------------|------------------------|-------|
| AR-01 | Agent reads all Secrets (cluster scope) | yes | No change |
| AR-02 | Redaction false negatives | yes | No change |
| AR-03 | NetworkPolicy requires CNI | yes | No change |
| AR-04 | Prompt injection not fully preventable | yes | No change |
| AR-05 | GitHub token in shared emptyDir | yes | No change |
| AR-06 | HARD RULEs are prompt-only controls | yes | No change |
| AR-07 | `curl` bypasses dry-run wrappers | yes | No change |
| AR-08 | Watcher ClusterRole cluster-wide secrets read | yes | Pending live cluster verification of removal |

**New accepted risks this review:**

| ID | Description | Severity | Rationale | Sign-off |
|----|-------------|----------|-----------|---------|
| AR-08 | Watcher ClusterRole cluster-wide `secrets: get, list, watch` | MEDIUM | Pending live verification — see finding 2026-02-27-001 | 2026-02-27 |

---

## Recommendations for Next Review

- Apply and verify the watcher ClusterRole `secrets` removal against a live cluster (finding 2026-02-27-001)
- Fix `agentImage` and `agentSA` CRD validation before any multi-tenant or shared-cluster deployment (findings 2026-02-27-005, -006)
- Pin `anomalyco/opencode/github@latest` to a commit SHA in all 4 workflows (finding 2026-02-27-007)
- Remove or gate the `renovate-analysis.yml` auto-merge capability (finding 2026-02-27-008)
- Gate `ai-comment.yml` on `author_association` (finding 2026-02-27-013)
- Run Phase 6 (GitHub App key isolation) in next live cluster session — AV-04 has not been re-verified since epic26 added the watcher-side key injection
- Confirm P-009 (NetworkPolicy) status — still open/operator-action

---

## Sign-off

Checklist completed: no (partial review — live cluster phases not run)

**Reviewer:** OpenCode (AI-assisted)
**Date:** 2026-02-27

*Produced following `docs/SECURITY/PROCESS.md` v1.0*
