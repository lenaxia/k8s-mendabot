# docs/BACKLOG/

## Purpose

Implementation backlog organised by epic. Each epic folder contains a README describing
the epic and individual story files for each unit of work.

## Rules

- Read the epic README before starting any story in that epic
- Update story checklist items `[ ]` → `[x]` as you complete tasks
- Mark story status as `In Progress` when you start it, `Complete` when done
- Stories within an epic should generally be worked in the order listed in the epic README
- Do not start a new epic until all blocking epics are complete (see dependency table below)

## Epic Overview

| Epic | Folder | Description | Depends On | Status |
|------|--------|-------------|------------|--------|
| epic00 — Foundation | [epic00-foundation/](epic00-foundation/) | Go module, project structure, config, CI skeleton | — | Complete |
| epic00.1 — Interfaces | [epic00.1-interfaces/](epic00.1-interfaces/) | RemediationJob CRD types, JobBuilder interface, reconciler skeletons, envtest suite, fakes | epic00 | Complete |
| epic01 — Controller | [epic01-controller/](epic01-controller/) | SourceProviderReconciler + RemediationJobReconciler | epic00, epic00.1 | Complete |
| epic02 — Job Builder | [epic02-jobbuilder/](epic02-jobbuilder/) | Agent Job spec construction from RemediationJob | epic00.1, epic01 | Complete |
| epic03 — Agent Image | [epic03-agent-image/](epic03-agent-image/) | Dockerfile, tool install, entrypoint script | epic00 | Complete |
| epic04 — Deploy | [epic04-deploy/](epic04-deploy/) | Kustomize manifests, RBAC, Secrets | epic01, epic02, epic03 | Complete |
| epic05 — Prompt | [epic05-prompt/](epic05-prompt/) | OpenCode prompt design and ConfigMap | epic04 | Complete |
| epic06 — CI/CD | [epic06-ci-cd/](epic06-ci-cd/) | GitHub Actions workflows for both images | epic03, epic00 | Complete |
| epic07 — Technical Debt | [epic07-technical-debt/](epic07-technical-debt/) | Issues and improvements discovered during implementation | — | Ongoing |
| epic08 — Pluggable Agent | [epic08-pluggable-agent/](epic08-pluggable-agent/) | Replace hardcoded opencode invocation with a pluggable AgentProvider abstraction | epic02, epic03, epic05 | Complete |
| epic09 — Native Provider | [epic09-native-provider/](epic09-native-provider/) | Replace k8sgpt dependency with a native cluster watcher; move Fingerprint to domain | epic01 | Complete |
| epic10 — Helm Chart | [epic10-helm-chart/](epic10-helm-chart/) | Package mendabot as a Helm chart with fully custom templates, CRD upgrade hook, and metrics support | epic04, epic05 | In Progress |
| epic11 — Self-Remediation Cascade Prevention | [epic11-self-remediation-cascade/](epic11-self-remediation-cascade/) | Prevent infinite cascades where mendabot analyzes its own failures | epic01, epic02, epic04 | Complete |
| epic12 — Security Review | [epic12-security-review/](epic12-security-review/) | Secret redaction, network policy, audit log, RBAC scoping, prompt injection defence, pentest | epic01, epic02, epic04, epic05, epic09 | Complete |
| epic13 — Multi-Signal Correlation | [epic13-multi-signal-correlation/](epic13-multi-signal-correlation/) | Correlate related findings into a single investigation via a CorrelationWindow | epic01, epic02, epic09, epic11 | Not Started |
| epic14 — Test Infrastructure Correctness | [epic14-test-infrastructure/](epic14-test-infrastructure/) | Fix CRD schema drift and envtest isolation defects; document rules to prevent recurrence | epic13 | Not Started |
| epic15 — Namespace Filtering | [epic15-namespace-filtering/](epic15-namespace-filtering/) | WATCH_NAMESPACES / EXCLUDE_NAMESPACES env vars to suppress system namespace noise | epic09 | Not Started |
| epic16 — Annotation Control | [epic16-annotation-control/](epic16-annotation-control/) | Per-resource mendabot.io/enabled, skip-until, priority annotations | epic09, epic15 | Not Started |
| epic17 — Dead-Letter Queue | [epic17-dead-letter-queue/](epic17-dead-letter-queue/) | RetryCount + MaxRetries + PermanentlyFailed phase; stops infinite retry loops | epic00.1, epic01, epic09 | Not Started |
| epic18 — Manifest Validation | [epic18-manifest-validation/](epic18-manifest-validation/) | Promote kubeconform to a HARD RULE in the agent prompt | epic05, epic03 | Not Started |
| epic19 — Secret Redaction (gap check) | [epic19-secret-redaction/](epic19-secret-redaction/) | Verify epic12 STORY_01 completeness; fill any gaps | epic12 | Not Started |
| epic20 — Dry-Run Mode | [epic20-dry-run-mode/](epic20-dry-run-mode/) | DRY_RUN=true; investigate but do not open PRs; write report to status.message | epic00, epic02, epic01, epic05 | Not Started |
| epic21 — Kubernetes Events | [epic21-kubernetes-events/](epic21-kubernetes-events/) | EventRecorder in both reconcilers; lifecycle visible in kubectl describe rjob | epic01, epic09 | Not Started |
| epic22 — Token Expiry Guard | [epic22-token-expiry-guard/](epic22-token-expiry-guard/) | Fast-fail on expired GitHub App token in agent-entrypoint.sh | epic03 | Not Started |
| epic23 — Structured Audit Log (gap check) | [epic23-structured-audit-log/](epic23-structured-audit-log/) | Verify epic12 STORY_03 completeness; fill gaps from epics 15–22 | epic12, epic15–22 | Not Started |
| epic24 — Severity Tiers | [epic24-severity-tiers/](epic24-severity-tiers/) | Severity field on findings; MIN_SEVERITY filter; FINDING_SEVERITY in agent prompt | epic09, epic00.1 | Not Started |

## Implementation Order

```
epic00-foundation
    ├── epic00.1-interfaces
    │       ├── epic01-controller
    │       │         └── epic02-jobbuilder
    │       │                     └── epic04-deploy ──┐
    │       └── (fakes used by epic01 unit tests)     │
    ├── epic03-agent-image ──────────────────────────┤
    │                                                  └── epic05-prompt
    └── epic06-ci-cd (parallel with epic01+)

epic08-pluggable-agent (depends on epic02, epic03, epic05)
epic09-native-provider (depends on epic01)
epic10-helm-chart (depends on epic04, epic05)
epic11-self-remediation-cascade (depends on epic01, epic02, epic04)
epic12-security-review (depends on epic01, epic02, epic04, epic05, epic09)
epic13-multi-signal-correlation (depends on epic01, epic02, epic09, epic11)
epic14-test-infrastructure (depends on epic13)

v1 epics (can be worked in parallel after epic09 and epic12 are complete):
epic15-namespace-filtering (depends on epic09)
epic16-annotation-control (depends on epic09, epic15)
epic17-dead-letter-queue (depends on epic00.1, epic01, epic09)
epic18-manifest-validation (depends on epic03, epic05)
epic19-secret-redaction (depends on epic12 — gap check only)
epic20-dry-run-mode (depends on epic00, epic01, epic02, epic05)
epic21-kubernetes-events (depends on epic01, epic09)
epic22-token-expiry-guard (depends on epic03)
epic23-structured-audit-log (depends on epic12, epic15-22 — gap check)
epic24-severity-tiers (depends on epic09, epic00.1)
```

## Feature Tracker

[`FEATURE_TRACKER.md`](FEATURE_TRACKER.md) contains the full product-level backlog of
potential improvements beyond the current epic roadmap. Organised by area:

| Area | Focus |
|------|-------|
| **Area A — Accuracy & Precision** | Noise reduction, false positive elimination, root cause quality |
| **Area R — Reliability** | Retry budgets, HA, token safety, circuit breakers |
| **Area U — Usability & Operability** | Metrics, events, dry-run, notifications |
| **Area I — Impact & PR Quality** | PR lifecycle, multi-platform sinks, blast radius |
| **Area S — Security** | Redaction, RBAC scoping, audit log, prompt injection |
| **Area P — New Signal Sources** | Prometheus, cert-manager, Velero, HPA, Datadog |

When a feature tracker item is approved for implementation, create an epic folder here
following the existing naming convention and update the feature's status in the tracker.

---

## Story Status Key

- `Not Started` — work has not begun
- `In Progress` — actively being worked on
- `Complete` — all acceptance criteria met, tests passing
- `Blocked` — cannot proceed; see story for blocker details
