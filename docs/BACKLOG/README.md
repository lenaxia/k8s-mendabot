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
| epic10 — Helm Chart | [epic10-helm-chart/](epic10-helm-chart/) | Package mendabot as a Helm chart with fully custom templates, CRD upgrade hook, and metrics support | epic04, epic05 | Complete |
| epic11 — Self-Remediation Cascade Prevention | [epic11-self-remediation-cascade/](epic11-self-remediation-cascade/) | Prevent infinite cascades where mendabot analyzes its own failures | epic01, epic02, epic04 | Complete |
| epic12 — Security Review | [epic12-security-review/](epic12-security-review/) | Secret redaction, network policy, audit log, RBAC scoping, prompt injection defence, pentest | epic01, epic02, epic04, epic05, epic09 | Complete |
| epic13 — Multi-Signal Correlation | [epic13-multi-signal-correlation/](epic13-multi-signal-correlation/) | Correlate related findings into a single investigation via a CorrelationWindow | epic01, epic02, epic09, epic11 | Deferred |
| epic14 — Test Infrastructure Correctness | [epic14-test-infrastructure/](epic14-test-infrastructure/) | Fix CRD schema drift and envtest isolation defects; document rules to prevent recurrence | epic09, epic12 | Complete |
| epic15 — Namespace Filtering | [epic15-namespace-filtering/](epic15-namespace-filtering/) | WATCH_NAMESPACES / EXCLUDE_NAMESPACES env vars to suppress system namespace noise | epic09 | Complete |
| epic16 — Annotation Control | [epic16-annotation-control/](epic16-annotation-control/) | Per-resource mendabot.io/enabled, skip-until, priority annotations | epic09, epic15 | Complete |
| epic17 — Dead-Letter Queue | [epic17-dead-letter-queue/](epic17-dead-letter-queue/) | RetryCount + MaxRetries + PermanentlyFailed phase; stops infinite retry loops | epic00.1, epic01, epic09 | Complete |
| epic18 — Manifest Validation | [epic18-manifest-validation/](epic18-manifest-validation/) | Promote kubeconform to a HARD RULE in the agent prompt | epic05, epic03 | Not Started |
| epic19 — Secret Redaction (gap check) | [epic19-secret-redaction/](epic19-secret-redaction/) | Verify epic12 STORY_01 completeness; fill any gaps | epic12 | Complete |
| epic20 — Dry-Run Mode | [epic20-dry-run-mode/](epic20-dry-run-mode/) | DRY_RUN=true; investigate but do not open PRs; write report to status.message | epic00, epic02, epic01, epic05 | Not Started |
| epic21 — Kubernetes Events | [epic21-kubernetes-events/](epic21-kubernetes-events/) | EventRecorder in both reconcilers; lifecycle visible in kubectl describe rjob | epic01, epic09 | Complete |
| epic22 — Token Expiry Guard | [epic22-token-expiry-guard/](epic22-token-expiry-guard/) | Fast-fail on expired GitHub App token in agent-entrypoint.sh | epic03 | Not Started |
| epic23 — Structured Audit Log (gap check) | [epic23-structured-audit-log/](epic23-structured-audit-log/) | Verify epic12 STORY_03 completeness; fill gaps from epics 15–22 | epic12, epic15–22 | Complete |
| epic24 — Severity Tiers | [epic24-severity-tiers/](epic24-severity-tiers/) | Severity field on findings; MIN_SEVERITY filter; FINDING_SEVERITY in agent prompt | epic09, epic00.1 | Complete |
| epic25 — Tool Output Redaction | [epic25-tool-output-redaction/](epic25-tool-output-redaction/) | cmd/redact binary + PATH-shadowing shell wrappers; all tool output redacted before LLM API | epic12, epic03 | Complete |
| epic26 — Auto-Close Resolved Sinks | [epic26-auto-close-resolved/](epic26-auto-close-resolved/) | Close open PRs/issues automatically when the underlying finding resolves | epic01, epic04, epic09 | Not Started |
| epic27 — PR Feedback Iteration | [epic27-pr-feedback-iteration/](epic27-pr-feedback-iteration/) | Poll open sinks for reviewer comments; dispatch follow-up agent Jobs to address feedback | epic26 | Not Started |
| epic28 — Manual Investigation Triggers | [epic28-manual-trigger/](epic28-manual-trigger/) | TriggerProvider interface + Webhook, GitHub issue, and Slack backends for on-demand investigations | epic01, epic02, epic26 | Not Started |

## Implementation Order

All foundation and v0.3.x epics are complete. The remaining unstarted epics are
independent and can be worked in any order after the current `main` baseline.

```
epic00-foundation          ✓
  epic00.1-interfaces      ✓
    epic01-controller      ✓
      epic02-jobbuilder    ✓
        epic04-deploy      ✓
    epic03-agent-image     ✓
      epic05-prompt        ✓
    epic06-ci-cd           ✓

epic08-pluggable-agent     ✓
epic09-native-provider     ✓
epic10-helm-chart          ✓
epic11-cascade-prevention  ✓
epic12-security-review     ✓ (+ pentest 2026-02-24 + epic25 tool-output-redaction)
epic14-test-infrastructure ✓
epic15-namespace-filtering ✓
epic16-annotation-control  ✓
epic17-dead-letter-queue   ✓
epic19-secret-redaction    ✓ (gap check vs epic12 — complete)
epic21-kubernetes-events   ✓
epic23-structured-audit-log ✓ (gap check — complete)
epic24-severity-tiers      ✓
epic25-tool-output-redaction ✓

Remaining (can be parallelised):
epic13-multi-signal-correlation  DEFERRED (epic11-13-deferred branch; needs epic14 first)
epic18-manifest-validation       Not Started
epic20-dry-run-mode              Not Started
epic22-token-expiry-guard        Not Started
epic26-auto-close-resolved       Not Started (depends on epic01, epic04, epic09)
epic27-pr-feedback-iteration     Not Started (depends on epic26)
epic28-manual-trigger            Not Started (depends on epic01, epic02, epic26)
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
