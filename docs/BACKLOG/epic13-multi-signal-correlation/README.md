# Epic 13: Multi-Signal Correlation (Related Findings)

## Purpose

When multiple findings originate from the same root cause — a PVC failing and its
dependent pod crashing, or three pods all evicted from the same failing node — mendabot
today dispatches one independent `RemediationJob` per finding. Each agent is blind to
the others. The result is contradictory PRs, wasted LLM budget, and operator confusion.

This epic implements a **CorrelationWindow** mechanism: a short hold period during which
newly-created `RemediationJob` objects are checked for correlation before dispatch. Correlated
findings are grouped, labelled with a shared `CorrelationGroupID`, and handed to a single
agent Job that investigates the full group context.

## Status: Deferred — moved to `feature/epic11-13-deferred`

## Dependencies

- epic01-controller complete (`SourceProviderReconciler` in `internal/provider/provider.go`,
  `RemediationJobReconciler` in `internal/controller/remediationjob_controller.go`)
- epic02-jobbuilder complete (`internal/jobbuilder/job.go` — env var injection)
- epic09-native-provider complete (all providers in `internal/provider/native/` — the
  sources whose findings need correlation)
- epic11-self-remediation-cascade complete (cascade suppression logic must not conflict
  with correlation grouping)

## Blocks

- FT-A5 Recurrence memory (prior investigation context per correlated group is more
  valuable than per individual finding)

## Success Criteria

- [x] `domain.CorrelationRule` interface exists in `internal/domain/correlation.go`
- [x] Three built-in rules implemented: `SameNamespaceParentRule`, `PVCPodRule`,
      `MultiPodSameNodeRule`
- [x] `RemediationJobReconciler` holds `RemediationJob` objects in `Pending` phase for
      `CORRELATION_WINDOW_SECONDS` (default: 30) before dispatching
- [x] `Correlator` struct exists in `internal/correlator/correlator.go` with method
      `Evaluate(ctx, candidate, peers, client) (CorrelationGroup, bool, error)`
      (returns the group, a found bool, and an error — idiomatic Go "found" pattern)
- [x] After the window, the correlator runs all rules; when a match is found, matching
      objects receive a `mendabot.io/correlation-group-id` label and all but the primary
      are transitioned to `Suppressed` phase
- [x] `JobBuilder.Build()` accepts a `[]v1alpha1.FindingSpec` slice and injects
      `FINDING_CORRELATED_FINDINGS` as a JSON-encoded env var when the slice has > 1 entry
- [x] `go test -timeout 30s -race ./...` passes with correlation tests
- [x] `DISABLE_CORRELATION=true` env var disables the window and all correlation rules,
      reverting to current dispatch-immediately behaviour

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| Correlation domain types and rule interface | [STORY_00_domain_types.md](STORY_00_domain_types.md) | Complete | High | 2h |
| Built-in correlation rules | [STORY_01_builtin_rules.md](STORY_01_builtin_rules.md) | Complete | High | 3h |
| CorrelationWindow in RemediationJobReconciler | [STORY_02_correlation_window.md](STORY_02_correlation_window.md) | Complete | Critical | 4h |
| JobBuilder multi-finding support | [STORY_03_jobbuilder_multi_finding.md](STORY_03_jobbuilder_multi_finding.md) | Complete | High | 2h |
| Prompt template update for correlated context | [STORY_04_prompt_update.md](STORY_04_prompt_update.md) | Complete | Medium | 1h |
| Integration tests and DISABLE_CORRELATION escape hatch | [STORY_05_integration_escape_hatch.md](STORY_05_integration_escape_hatch.md) | Complete | High | 3h |

## Correlation Rules

Three rules are implemented in priority order. The first matching rule wins.

### Rule 1 — SameNamespaceParentRule

**Trigger:** Two findings share the same namespace and one's `ParentObject` is a prefix
of or equal to the other's `ParentObject`.

**Rationale:** A Deployment named `my-app` and a Pod named `my-app-7d9f-xyz` are the
same application. Findings from both should be correlated.

**Primary selection:** The finding whose `Kind` is higher in the ownership hierarchy
(Deployment > StatefulSet > Pod) is the primary. If equal, the older `RemediationJob`
(by `CreationTimestamp`) is primary.

### Rule 2 — PVCPodRule

**Trigger:** A `PVCProvider` finding and a `PodProvider` finding exist in the same
namespace, and the pod's `volumes` list references the PVC name.

**Rationale:** A pod stuck in `Pending` because its PVC is unbound is the same root
cause as the PVC's `ProvisioningFailed` finding. Opening two PRs ("fix the pod" and
"fix the PVC") is contradictory.

**Primary selection:** The `PVCProvider` finding is always primary — it is the root
cause. The pod finding is suppressed.

**Implementation note:** Requires one `client.Get` call to read the Pod spec and inspect
`spec.volumes[*].persistentVolumeClaim.claimName`. This is the only rule that requires
a live API call during correlation evaluation.

### Rule 3 — MultiPodSameNodeRule

**Trigger:** Three or more pod findings all ran on the same node within the correlation
window. The threshold is `>= CORRELATION_MULTI_POD_THRESHOLD` (default: 3, so 3 pods on
the same node triggers the rule).

**Rationale:** If 4+ pods on `node-abc` are all failing simultaneously, the node is the
root cause, not the individual pods. This ties into the FT-A4 cascade check, but operates
as a correlation rule rather than a suppression rule — the investigation still happens,
but as a single agent with full group context.

**Primary selection:** A synthetic finding is created with `Kind=Node`,
`ParentObject=<node-name>`, representing the node-level root cause. The pod findings
become the correlated context.

**Threshold:** Configurable via `CORRELATION_MULTI_POD_THRESHOLD` env var (default: 3).
The rule fires when the count of pod findings on a single node is `>= threshold`.

## CorrelationWindow Behaviour

```
RemediationJob created (phase: Pending)
      │
      ▼
Wait CORRELATION_WINDOW_SECONDS (default: 30s)
      │
      ├── Correlator runs all rules against all Pending RJobs in namespace
      │
      ├── No correlation found ──> dispatch immediately (phase: Dispatched)
      │
      └── Correlation found ─────> label all with same CorrelationGroupID
                                   primary: phase Dispatched (with full group context)
                                   others:  phase Suppressed (reason: correlated)
```

The hold is implemented using `ctrl.Result{RequeueAfter: window}` in the reconciler,
not a goroutine sleep. This preserves the reconciler's idempotency and restart safety.

A `RemediationJob`'s `CreationTimestamp` is used as the anchor for the window start.
On reconcile, if `time.Since(rjob.CreationTimestamp) < window`, requeue. Otherwise,
run correlation and dispatch.

## Suppressed Phase

A new `Suppressed` phase is added to `RemediationJobStatus` alongside the existing
`Pending`, `Dispatched`, `Succeeded`, `Failed`, and `Cancelled` phases.

`Suppressed` is a terminal phase. A `Suppressed` `RemediationJob` is never dispatched.
It holds the `CorrelationGroupID` label so the relationship to the primary investigation
is traceable.

`SourceProviderReconciler` treats `Suppressed` the same as `Succeeded` for dedup
purposes — a finding whose `RemediationJob` is `Suppressed` is not re-triggered.

## Technical Overview

### New files

| File | Purpose |
|------|---------|
| `internal/domain/correlation.go` | `CorrelationRule` interface, `CorrelationResult` type, `CorrelationGroupID` generator |
| `internal/domain/correlation_test.go` | Unit tests for rule logic |
| `internal/correlator/rules.go` | `SameNamespaceParentRule`, `PVCPodRule`, `MultiPodSameNodeRule` implementations |
| `internal/correlator/rules_test.go` | Rule unit tests with table-driven cases |
| `internal/correlator/correlator.go` | `Correlator` struct — applies rules to a slice of `RemediationJob` objects |
| `internal/correlator/correlator_test.go` | Correlator unit tests |

### Modified files

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add `Suppressed` phase constant; add `CorrelationGroupID` to status |
| `internal/controller/remediationjob_controller.go` | Add window hold logic; call `Correlator` before dispatch |
| `internal/controller/remediationjob_controller_test.go` | Tests for window, correlation, suppression |
| `internal/jobbuilder/job.go` | Accept correlated findings; inject `FINDING_CORRELATED_FINDINGS` env var |
| `internal/jobbuilder/job_test.go` | Tests for multi-finding env injection |
| `internal/config/config.go` | Add `CorrelationWindowSeconds`, `DisableCorrelation`, `MultiPodThreshold` |
| `internal/config/config_test.go` | Config parsing tests for new fields |
| `deploy/kustomize/configmap-prompt.yaml` | Add `FINDING_CORRELATED_FINDINGS` handling instructions |

### Story execution order

STORY_00 must run first — all other stories depend on the domain types. STORY_01 and
STORY_02 can follow in parallel (rules are independent of the reconciler plumbing).
STORY_03 depends on STORY_00. STORY_04 depends on STORY_03. STORY_05 closes the epic
with integration tests that require all prior stories complete.

```
STORY_00 (domain types) ─┬──> STORY_01 (rules)   ──┐
                          └──> STORY_02 (window)  ──┤
                          └──> STORY_03 (builder) ──┼──> STORY_05 (integration)
                                    └──> STORY_04 (prompt) ──┘
```

## Definition of Done

- [x] All unit tests pass: `go test -timeout 30s -race ./...`
- [x] `go build ./...` succeeds
- [x] `go vet ./...` clean
- [x] `kubectl apply -k deploy/kustomize/ --dry-run=client` passes
- [x] `DISABLE_CORRELATION=true` reverts to pre-epic dispatch behaviour (verified by test)
- [x] Worklog entry created in `docs/WORKLOGS/`

## New Configuration Variables

```bash
# Correlation window duration in seconds (default: 30)
CORRELATION_WINDOW_SECONDS=30

# Disable all correlation logic and dispatch immediately (default: false)
DISABLE_CORRELATION=false

# Minimum pod count on same node to trigger MultiPodSameNodeRule (default: 3)
CORRELATION_MULTI_POD_THRESHOLD=3
```
