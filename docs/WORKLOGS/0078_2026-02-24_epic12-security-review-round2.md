# Worklog: Epic 12 Security Review — Round 2

**Date:** 2026-02-24
**Session:** Deep security re-review of epic 12 attack surface after post-epic12 feature merges
**Status:** Complete

---

## Objective

Re-audit the full epic 12 security attack surface against the current codebase state.
Since the original review (2026-02-23), epics 11, 13–16, 21, and 24 were merged to main.
These introduced new features (severity tiers, annotation control, namespace filtering,
Kubernetes Events, Helm chart) that could have introduced security regressions or new
attack surfaces.

---

## Work Completed

### 1. Re-verification of all original test cases (TC-01 through TC-06)

- TC-01 (credential redaction): PASS — all 8 `RedactSecrets` call sites verified in
  current `internal/provider/native/` code
- TC-02 (prompt injection): PASS with regression note — injection detection is in place
  but the Helm chart prompt is missing the untrusted-data envelope (see NEW-02 below)
- TC-03 (egress restriction): SKIPPED — same reason as original; no CNI available
- TC-04 (GitHub App key isolation): PASS — `job.go` init/main container separation
  verified unchanged
- TC-05 (RBAC scope enforcement): PASS — config parsing and RBAC manifests unchanged
- TC-06 (audit log completeness): PASS — 6 new audit events added since original review

### 2. New attack surface analysis

Identified 7 new findings:

| ID | Description | Severity |
|----|-------------|----------|
| NEW-01 | Pod unschedulable message not truncated before RedactSecrets | LOW |
| NEW-02 | Prompt injection envelope absent from Helm chart core.txt (regression) | MEDIUM |
| NEW-03 | FINDING_CORRELATED_FINDINGS carries unredacted data (latent, Epic 13 deferred) | LOW |
| NEW-04 | Priority annotation stabilisation bypass emits no audit log | LOW |
| NEW-05 | Watcher ClusterRole grants unnecessary `secrets` permission | MEDIUM |
| NEW-06 | Integer overflow possible in int32 casts in config.go and main.go | LOW |
| NEW-07 | Gosec G101 false positive on `githubAppSecretName` constant | N/A (FP) |

### 3. Static analysis (gosec)

Ran `gosec ./...` across all 30 files (3,674 lines). Confirmed 4 gosec issues:
- G115 + G109 on `config.go:222` and `main.go:105` — int32 overflow (NEW-06)
- G101 on `sink/github.go:16` — false positive (NEW-07)

`go vet ./...` — clean.
`go test -timeout 30s -race ./...` — all 12 packages pass.

### 4. Written report

Created `docs/BACKLOG/epic12-security-review/pentest-report-2.md` with full
findings, remediation recommendations, updated residual risk table.

---

## Key Decisions

**No HIGH or CRITICAL findings.** The two MEDIUM findings (NEW-02 and NEW-05) should be
remediated in a dedicated follow-up. Neither is a blocker on the current feature work in
`feature/epic12-security-remediation` and `feature/epic16-annotation-control`.

**NEW-02 is a direct regression** of a STORY_05 control. The Helm chart prompt was
written independently and the untrusted-data envelope was not carried over. This warrants
a backlog story in epic12 or a new epic.

**NEW-05** — the `secrets` permission on the watcher ClusterRole has no corresponding
code path. Removing it is a one-line change with zero functional impact.

**NEW-03 is latent.** Epic 13 is deferred on `feature/epic11-13-deferred`. The
correlated findings code path exists in `jobbuilder/job.go` but is currently called with
`nil`. A pre-emptive fix in the jobbuilder is low-effort and eliminates the risk entirely.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./...   # all 12 packages PASS
go vet ./...                        # clean
gosec ./...                         # 4 issues: 2 real (NEW-06), 1 FP (NEW-07), 1 duplicate
```

---

## Next Steps

1. Create backlog story to remediate NEW-02 (restore untrusted-data envelope in
   `charts/mendabot/files/prompts/core.txt` and add HARD RULE 8 equivalent)
2. Create backlog story (or inline fix) to remediate NEW-05 (remove `secrets` from
   watcher ClusterRole in `charts/mendabot/templates/clusterrole-watcher.yaml`)
3. Fix NEW-01 (one-liner in `internal/provider/native/pod.go:104`)
4. Fix NEW-04 (add audit log for priority annotation bypass in `provider.go`)
5. Fix NEW-06 (add overflow guards in `config.go` and `main.go` int32 casts)
6. Fix NEW-07 (add `//nolint:gosec` on `githubAppSecretName`)
7. Add pre-emptive redaction for `correlatedFindings` in `jobbuilder/job.go` (NEW-03)

---

## Files Modified

- `docs/BACKLOG/epic12-security-review/pentest-report-2.md` — created (new report)
- `docs/WORKLOGS/0078_2026-02-24_epic12-security-review-round2.md` — created (this file)
