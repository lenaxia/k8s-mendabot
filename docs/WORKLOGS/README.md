# docs/WORKLOGS/

## Purpose

Session-by-session record of all work done on this project. This is the institutional
memory. When starting a new session, read the last 2–3 entries here before doing anything
else.

## Rules

- Write a worklog entry after every meaningful session — see README-LLM.md for the
  mandatory format and discipline rules
- File naming: `NNNN_YYYY-MM-DD_short-description.md`
- Update the index table below every time a new entry is added
- Never retroactively rewrite an entry — if something was wrong, note the correction
  in the next entry
- Next entry number: check the highest `NNNN` in the table and add 1

## Index

| # | Date | Description | Status |
|---|------|-------------|--------|
| [0001](0001_2026-02-19_initial-design-and-docs.md) | 2026-02-19 | Initial design, HLD, LLDs, backlog | Complete |
| [0002](0002_2026-02-19_design-review-fixes.md) | 2026-02-19 | Skeptical design review + remediation of all 37 findings | Complete |
| [0003](0003_2026-02-20_second-design-review-fixes.md) | 2026-02-20 | Second design review + remediation of all 44 findings | Complete |
| [0004](0004_2026-02-20_watcher-image-lld.md) | 2026-02-20 | Watcher image LLD + STATUS.md update | Complete |
| [0005](0005_2026-02-20_third-design-review-fixes.md) | 2026-02-20 | Third design review — gap analysis and fixes across all docs | Complete |
| [0006](0006_2026-02-20_epic00-foundation-complete.md) | 2026-02-20 | Epic 00 foundation complete — CRD types, config, logging, main.go wiring | Complete |
| [0007](0007_2026-02-20_epic00.1-stories-01-02.md) | 2026-02-20 | Epic 00.1 S01-S02: RemediationJob types, domain interfaces | Complete |
| [0008](0008_2026-02-20_epic00.1-story04-envtest-suite.md) | 2026-02-20 | Epic 00.1 S04: envtest suite setup for integration tests | Complete |
| [0009](0009_2026-02-20_epic00.1-story05-fakes.md) | 2026-02-20 | Epic 00.1 S05: fakeJobBuilder, defaultFakeJob, suite skip-flag pattern | Complete |
| [0010](0010_2026-02-20_epic00.1-story03-reconciler-skeletons.md) | 2026-02-20 | Epic 00.1 S03: reconciler/provider/jobbuilder skeletons + main.go wiring | Complete |
| [0011](0011_2026-02-20_epic01-controller-core-logic.md) | 2026-02-20 | Epic 01 S02–S05: fingerprintFor, K8sGPTProvider, SourceProviderReconciler, RemediationJobReconciler | Complete |
| [0012](0012_2026-02-20_story07-integration-tests.md) | 2026-02-20 | Epic 01 S07: 13 envtest integration tests + 3 bug fixes | Complete |
| [0013](0013_2026-02-20_epic02-jobbuilder-complete.md) | 2026-02-20 | Epic 02: Build() pure function, 28 tests, 9-gap review cycle | Complete |
| [0014](0014_2026-02-20_epic03-agent-image-complete.md) | 2026-02-20 | Epic 03: Dockerfile.agent, entrypoint, token script, smoke test; 14 gaps fixed | Complete |
| [0015](0015_2026-02-20_robustness-audit.md) | 2026-02-20 | Robustness audit: Fingerprint error return, PhaseCancelled, 21 findings fixed | Complete |
| [0016](0016_2026-02-20_epic04-deploy-manifests.md) | 2026-02-20 | Epic 04: Kustomize manifests — namespace, CRD, RBAC, secrets, deployment | Complete |
| [0017](0017_2026-02-20_epic05-prompt-complete.md) | 2026-02-20 | Epic 05: Prompt configmap + entrypoint hardening; 5 gaps fixed including critical opencode run bug | Complete |
| [0018](0018_2026-02-20_epic06-ci-cd-complete.md) | 2026-02-20 | Epic 06: CI/CD — Dockerfile.watcher, build-watcher, build-agent; 5 gaps fixed | Complete |
| [0019](0019_2026-02-22_epic09-design.md) | 2026-02-22 | Epic 09: native provider design, backlog stories 01–09 created | Complete |
| [0020](0020_2026-02-22_epic09-backlog-review.md) | 2026-02-22 | Epic 09: stories 10–12 added; 9 integration gaps found and fixed | Complete |
| [0021](0021_2026-02-22_epic09-finding-fixes.md) | 2026-02-22 | Epic 09: all 24 skeptical reviewer findings applied across 12 story files | Complete |
| [0022](0022_2026-02-22_epic09-story02-slim-interface.md) | 2026-02-22 | Epic 09 STORY_02: slim SourceProvider interface, reconciler calls domain.FindingFingerprint | Complete |
| [0023](0023_2026-02-22_epic09-story03-parent-traversal.md) | 2026-02-22 | Epic 09 STORY_03: getParent owner-reference traversal, 9 tests, fake client | Complete |
| [0024](0024_2026-02-22_epic09-story04-pod-provider.md) | 2026-02-22 | Epic 09 STORY_04: PodProvider with failure detection, 16 tests | Complete |
| [0025](0025_2026-02-22_epic09-story05-deployment-provider.md) | 2026-02-22 | Epic 09 STORY_05: DeploymentProvider with replicas/available detection, 13 tests | Complete |
| [0026](0026_2026-02-22_epic09-story06-pvc-provider.md) | 2026-02-22 | Epic 09 STORY_06: PVCProvider with ProvisioningFailed event detection, 11 tests | Complete |
| [0027](0027_2026-02-22_epic09-story07-node-provider.md) | 2026-02-22 | Epic 09 STORY_07: NodeProvider with condition-based detection, 16 tests | Complete |
| [0028](0028_2026-02-22_epic09-story10-statefulset-provider.md) | 2026-02-22 | Epic 09 STORY_10: StatefulSetProvider with generation-based scaling detection, 13 tests | Complete |
| [0029](0029_2026-02-22_epic09-story11-job-provider.md) | 2026-02-22 | Epic 09 STORY_11: JobProvider with backoff-exhaustion detection, 17 tests | Complete |
| [0030](0030_2026-02-22_epic09-story12-stabilisation-window.md) | 2026-02-22 | Epic 09 STORY_12: stabilisation window in config and SourceProviderReconciler | Complete |
| [0031](0031_2026-02-22_epic09-story08-main-wiring.md) | 2026-02-22 | Epic 09 STORY_08: wire all six native providers into main.go | Complete |
| [0032](0032_2026-02-22_epic09-story09-remove-k8sgpt.md) | 2026-02-22 | Epic 09 STORY_09: remove k8sgpt provider, result_types, migrate 6 integration tests | Complete |
| [0033](0033_2026-02-22_epic09-native-provider-complete.md) | 2026-02-22 | Epic 09 complete: all 12 stories, 6 native providers, k8sgpt removed, code review 0 gaps | Complete |
| [0034](0034_2026-02-23_epic09-merge-and-release.md) | 2026-02-23 | Epic 09 merge: skeptical review, 5 gaps fixed, merge-readiness blockers resolved, v0.3.0 tagged | Complete |
| [0035](0035_2026-02-23_cascade-prevention-complete.md) | 2026-02-23 | Comprehensive cascade prevention system: circuit breaker, chain depth tracking, cascade checker, metrics, integration | Complete |
| [0036](0036_2026-02-23_helm-chart-design.md) | 2026-02-23 | README overhaul + Helm chart architecture design | Complete |
| [0037](0037_2026-02-23_epic10-helm-chart-planning.md) | 2026-02-23 | Epic 10 Helm chart: design decisions, backlog created, 4 gaps found and fixed | Complete |
| [0038](0038_2026-02-23_epic10-helm-chart-implementation.md) | 2026-02-23 | Epic 10 Helm chart: all 13 stories implemented, helm lint passes, CI workflow added | Complete |
| [0039](0039_2026-02-23_epic11-bug-fixes.md) | 2026-02-23 | Epic 11 bug fixes: 7 bugs fixed, 3 tests added, race detector clean, story DoD updated | Complete |
| [0040](0040_2026-02-23_epic11-story06-complete.md) | 2026-02-23 | Epic 11 STORY_06: EventRecorder (3 events, 4 tests), 10-gap review cycle, Grafana dashboard, alert rules | Complete |
| [0041](0041_2026-02-23_helm-crd-upgrader-image-fix.md) | 2026-02-23 | Helm CRD upgrader image fix: nonexistent tag v1.28.16 corrected to v1.28.15, image made configurable | Complete |
| [0042](0042_2026-02-23_epic13-story03-jobbuilder-multi-finding.md) | 2026-02-23 | Epic 13 STORY_03: Build() two-arg signature, correlated findings env var injection, all call sites fixed | Complete |
| [0043](0043_2026-02-24_epic13-story02-correlation-window.md) | 2026-02-24 | Epic 13 STORY_02: Correlator struct, correlation window hold, config fields, controller wiring | Complete |
| [0044](0044_2026-02-23_epic13-story05-integration-tests.md) | 2026-02-23 | Epic 13 STORY_05: 6 envtest integration tests for correlation rules and escape hatch | Complete |
| [0045](0045_2026-02-23_epic13-complete.md) | 2026-02-23 | Epic 13 complete: all 6 stories committed, story/epic status markers updated | Complete |
| [0046](0046_2026-02-23_epic13-test-fix.md) | 2026-02-23 | Epic 13 test fix: 7 failing controller tests fixed via deep code review | Complete |
| [0047](0047_2026-02-23_epic14-story01-test-isolation.md) | 2026-02-23 | Epic 14 S01: test isolation — newIntegrationJob namespace fix, pre-test stale guards, cleanupJobsInNS | Complete |
| [0048](0048_2026-02-23_epic14-count3-finalizer-fix.md) | 2026-02-23 | Epic 14 -count=3: deleteJob helper strips finalizers, corrNS unique namespace counters | Complete |
| [0049](0049_2026-02-23_epic13-14-review-gaps.md) | 2026-02-23 | Epic 13+14 skeptical review: 9 gaps found and fixed (DC-5, XC-2 major; 7 minor) | Complete |
| [0050](0050_2026-02-23_epic13-design-review-fixes.md) | 2026-02-23 | Epic 13 design review: off-by-one bug, nondeterminism, prefix over-match, scope docs | Complete |
| [0051](0051_2026-02-23_epic11-13-branch-extraction-test-fixes.md) | 2026-02-23 | Epic 11+13 branch extraction: test isolation fixes, readiness gate, epic14 test infra | Complete |
| [0052](0052_2026-02-23_remove-crd-hook-job.md) | 2026-02-23 | Helm: remove CRD hook job in favour of native crds/ directory | Complete |
| [0053](0053_2026-02-23_epic12-story00-security-infra.md) | 2026-02-23 | Epic 12 STORY_00: Makefile, kind-config, Trivy CI steps, gosec baseline placeholder | Complete |
| [0054](0054_2026-02-23_story01-secret-redaction.md) | 2026-02-23 | STORY_01: RedactSecrets applied at all 6 native provider error text sites, 8 new tests | Complete |
| [0055](0055_2026-02-23_story05-prompt-injection-defence.md) | 2026-02-23 | STORY_05: injection detection, truncate helper, config field, prompt envelope, HARD RULE 8 | Complete |
| [0056](0056_2026-02-23_story03-audit-log.md) | 2026-02-23 | STORY_03: structured audit log lines at all 10 decision points in both reconcilers | Complete |
| [0057](0057_2026-02-23_story02-network-policy.md) | 2026-02-23 | STORY_02: NetworkPolicy egress restriction for agent Jobs, security overlay | Complete |
| [0058](0058_2026-02-23_story04-agent-rbac-scoping.md) | 2026-02-23 | STORY_04: AgentRBACScope config, NS SA selection, 4 RBAC manifests, 5 TDD tests | Complete |
| [0059](0059_2026-02-23_epic12-story06-pentest-complete.md) | 2026-02-23 | STORY_06: penetration test plan executed; 5 PASS, 1 SKIP; no HIGH/CRITICAL findings | Complete |
| [0060](0060_2026-02-23_go-toolchain-cve-remediation.md) | 2026-02-23 | Finding 2026-02-23-001: upgrade go 1.23.0 → 1.23.12 to fix GO-2026-4341/4340/4337 | Complete |
| [0061](0061_2026-02-23_epic12-security-remediation-complete.md) | 2026-02-23 | Epic 12 orchestrator: all 11 open security findings remediated, report updated | Complete |
| [0062](0062_2026-02-23_epic17-story01-crd-types.md) | 2026-02-23 | Epic 17 STORY_01: PhasePermanentlyFailed, ConditionPermanentlyFailed, MaxRetries, RetryCount | Complete |
| [0063](0063_2026-02-23_epic17-story02-config-retries.md) | 2026-02-23 | Epic 17 STORY_02: MaxInvestigationRetries int32 field + MAX_INVESTIGATION_RETRIES env var parsing | Complete |
| [0064](0064_2026-02-23_epic17-story03-retry-count.md) | 2026-02-23 | Epic 17 STORY_03: RetryCount increment, PermanentlyFailed cap, terminal switch, audit log | Complete |
| [0065](0065_2026-02-23_epic17-story05-crd-schema-updates.md) | 2026-02-23 | Epic 17 STORY_05: CRD YAML schema updates — maxRetries, retryCount, PermanentlyFailed | Complete |
| [0066](0066_2026-02-24_epic17-story04-provider-gate.md) | 2026-02-24 | Epic 17 STORY_04: switch-based dedup loop, PermanentlyFailed tombstone gate, MaxRetries population | Complete |
| [0067](0067_2026-02-23_epic17-dead-letter-queue-complete.md) | 2026-02-23 | Epic 17 complete: all 5 stories, 12 packages green, dead-letter queue fully implemented | Complete |
| [0068](0068_2026-02-23_cross-epic-validation-gap-fixes.md) | 2026-02-23 | Cross-epic skeptical validation: 11 gaps found and fixed (4 Major, 7 Minor) | Complete |
| [0069](0069_2026-02-23_second-pass-validation-gap-fixes.md) | 2026-02-23 | Second-pass validation: 4 gaps found and fixed (1 Critical, 1 Major, 2 Minor) | Complete |
| [0070](0070_2026-02-24_third-pass-validation-gap-fixes.md) | 2026-02-24 | Third-pass validation: 5 gaps found and fixed (2 Major, 3 Minor) | Complete |
| [0071](0071_2026-02-23_epic08-pluggable-agent-complete.md) | 2026-02-23 | Epic 08: pluggable agent runner — AGENT_TYPE, opaque config blob, projected prompt volumes | Complete |
