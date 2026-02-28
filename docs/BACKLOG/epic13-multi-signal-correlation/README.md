# Epic 13: Multi-Signal Correlation (Related Findings)

## Purpose

When multiple findings originate from the same root cause — a PVC failing and its
dependent pod crashing, or three pods all evicted from the same failing node — mechanic
today dispatches one independent `RemediationJob` per finding. Each agent is blind to
the others. The result is contradictory PRs, wasted LLM budget, and operator confusion.

This epic implements a **CorrelationWindow** mechanism: a short hold period during which
newly-created `RemediationJob` objects are checked for correlation before dispatch. Correlated
findings are grouped, labelled with a shared `CorrelationGroupID`, and handed to a single
agent Job that investigates the full group context.

## Status: Complete (v0.3.23 / v0.3.24)

## Dependencies

- epic01-controller complete (`SourceProviderReconciler` in `internal/provider/provider.go`,
  `RemediationJobReconciler` in `internal/controller/remediationjob_controller.go`)
- epic02-jobbuilder complete (`internal/jobbuilder/job.go` — STORY_03 partially applied:
  `Build()` already accepts two args and injects `FINDING_CORRELATED_FINDINGS`; the
  `FINDING_CORRELATION_GROUP_ID` injection and all other story work remains)
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
- [x] After the window, when the candidate is the **primary**: it suppresses all correlated
      peers in the same reconcile call, then dispatches with the full group context
- [x] After the window, when the candidate is **not** the primary and the primary is still
      `Pending`: it requeues after 5s (never self-suppresses); the primary's reconcile will
      suppress it
- [x] After the window, when the candidate is **not** the primary and the primary is already
      `PhaseDispatched` or `PhaseRunning`: the non-primary immediately suppresses itself
      using the primary's `CorrelationGroupID` (v0.3.24 bug fix — previously looped forever
      at 5s intervals and eventually solo-dispatched)
- [x] All jobs in a correlation group receive `mechanic.io/correlation-group-id` and
      `mechanic.io/correlation-role` labels
- [x] Non-primary jobs are transitioned to `Suppressed` phase (either by the primary's
      reconcile call, or by self-suppression when the primary is already dispatched)
- [x] `JobBuilder.Build()` accepts a `[]v1alpha1.FindingSpec` slice and injects
      `FINDING_CORRELATED_FINDINGS` as a JSON-encoded env var when the slice has > 1 entry
- [x] `JobBuilder.Build()` injects `FINDING_CORRELATION_GROUP_ID` when the primary
      `RemediationJob` carries a `mechanic.io/correlation-group-id` label at dispatch time
- [x] `go test -timeout 30s -race ./...` passes with correlation tests
- [x] `DISABLE_CORRELATION=true` env var disables the window and all correlation rules,
      reverting to current dispatch-immediately behaviour

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| Correlation domain types and rule interface | [STORY_00_domain_types.md](STORY_00_domain_types.md) | Complete | High | 2h |
| Built-in correlation rules | [STORY_01_builtin_rules.md](STORY_01_builtin_rules.md) | Complete | High | 3h |
| CorrelationWindow in RemediationJobReconciler | [STORY_02_correlation_window.md](STORY_02_correlation_window.md) | Complete | Critical | 4h |
| JobBuilder multi-finding support | [STORY_03_jobbuilder_multi_finding.md](STORY_03_jobbuilder_multi_finding.md) | Complete | High | 1h |
| Prompt template update for correlated context | [STORY_04_prompt_update.md](STORY_04_prompt_update.md) | Complete | Medium | 1h |
| Integration tests and DISABLE_CORRELATION escape hatch | [STORY_05_integration_escape_hatch.md](STORY_05_integration_escape_hatch.md) | Complete | High | 3h |

## Correlation Rules

Three rules are implemented in priority order. The first matching rule wins.

### Rule 1 — SameNamespaceParentRule

**Trigger:** Two findings share the same namespace and one's `ParentObject` is a prefix
of or equal to the other's `ParentObject`.

**Rationale:** A `StatefulSet` named `my-app` (from the native provider) and a `PVC`
named `my-app-data` (from the PVC provider) both have `ParentObject=my-app`. Findings
from both should be grouped into a single investigation.

**Note on deduplication:** Within a single provider, a Deployment finding and its owned
Pod finding share the same `ParentObject` value and therefore the same fingerprint — they
are deduplicated by `SourceProviderReconciler` before two `RemediationJob` objects are
ever created. This rule is most effective for cross-provider scenarios where the same
application surfaces findings from two different resource types.

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

**Primary selection:** The oldest pod `RemediationJob` by `CreationTimestamp` in the
group becomes the primary. All remaining pod findings are suppressed and included in the
primary's correlated-findings context. There is no synthetic node finding — the
investigation agent receives all pod findings and the shared node name via the group
context, giving it full visibility to diagnose the node-level root cause.

**Threshold:** Configurable via `CORRELATION_MULTI_POD_THRESHOLD` env var (default: 3).
The rule fires when the count of pod findings on a single node is `>= threshold`.

## CorrelationWindow Behaviour

```
RemediationJob created (phase: Pending)
      │
      ▼
Wait CORRELATION_WINDOW_SECONDS (default: 30s) using RequeueAfter
      │
      ├── No correlation found ──> dispatch immediately (phase: Dispatched)
      │
      └── Correlation found
             │
             ├── Candidate IS the primary
             │      ├── Suppress all correlated peers (same reconcile call)
             │      ├── Label all (including self) with CorrelationGroupID
             │      └── Dispatch with full group findings (phase: Dispatched)
             │
             └── Candidate is NOT the primary
                    │
                    ├── Primary is Pending ──> Requeue after 5s
                    │   (primary's reconcile will suppress this job when its window elapses)
                    │
                    └── Primary is Dispatched/Running ──> Self-suppress immediately
                        (uses primary's CorrelationGroupID; no dispatch; terminal)
                        [v0.3.24 fix — previously looped at 5s until TTL then solo-dispatched]
```

The hold is implemented using `ctrl.Result{RequeueAfter: remaining}` in the reconciler,
not a goroutine sleep. This preserves the reconciler's idempotency and restart safety.

**Why non-primaries wait when the primary is still Pending:** If a non-primary
self-suppresses before the primary's window has elapsed, the primary's later
`correlationPeers` call would exclude the now-Suppressed non-primary. The primary
dispatches as a solo job and the non-primary's finding is permanently lost. Instead,
non-primaries requeue and wait for the primary to suppress them, ensuring the primary
always sees all correlated findings as eligible peers when its window elapses.

**Why non-primaries self-suppress when the primary is already Dispatched (v0.3.24):**
`correlationPeers()` includes `PhaseDispatched` and `PhaseRunning` peers (not just
`PhasePending`), so a late-arriving non-primary can see a primary that has already
dispatched. In that case, the primary will never re-enter the correlation block to
suppress the non-primary, so the non-primary suppresses itself using the primary's
`CorrelationGroupID`. This prevents the pre-v0.3.24 bug where the non-primary would
loop at 5s indefinitely and eventually solo-dispatch.

A `RemediationJob`'s `CreationTimestamp` is used as the anchor for the window start.
On reconcile, if `time.Since(rjob.CreationTimestamp) < window`, requeue. Otherwise,
run correlation and dispatch.

## Suppressed Phase

A new `Suppressed` phase is added to `RemediationJobStatus` alongside the existing
`Pending`, `Dispatched`, `Running`, `Succeeded`, `Failed`, `Cancelled`, and
`PermanentlyFailed` phases.

`Suppressed` is a terminal phase. A `Suppressed` `RemediationJob` is never dispatched.
It holds the `CorrelationGroupID` label so the relationship to the primary investigation
is traceable.

`SourceProviderReconciler` already treats `Suppressed` correctly for dedup purposes:
the existing `default:` case in the dedup switch at `internal/provider/provider.go:383`
returns early for any phase that is not `Failed` or `PermanentlyFailed`, which includes
`Suppressed`. No code change is required in the provider.

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
| `api/v1alpha1/remediationjob_types.go` | Add `Suppressed` phase constant; add `CorrelationGroupID` to status; update `DeepCopyInto`; update enum marker |
| `internal/controller/remediationjob_controller.go` | Add `Correlator` field; add window hold logic; add `correlationPeers` helper (includes Pending/Dispatched/Running peers); primary suppresses peers then dispatches; non-primary self-suppresses if primary already dispatched; add `case PhaseSuppressed` |
| `internal/controller/remediationjob_controller_test.go` | Tests for window, correlation, suppression |
| `internal/jobbuilder/job.go` | Inject `FINDING_CORRELATION_GROUP_ID` env var (partially done: `FINDING_CORRELATED_FINDINGS` already injected) |
| `internal/jobbuilder/job_test.go` | Tests for `FINDING_CORRELATION_GROUP_ID` injection |
| `internal/config/config.go` | Add `CorrelationWindowSeconds`, `DisableCorrelation`, `MultiPodThreshold` |
| `internal/config/config_test.go` | Config parsing tests for new fields |
| `internal/domain/provider.go` | Add `NodeName string` to `Finding` struct |
| `internal/provider/native/pod.go` | Populate `NodeName` from `pod.Spec.NodeName` |
| `internal/provider/provider.go` | Write `mechanic.io/node-name` annotation on `RemediationJob` when `finding.NodeName != ""` |
| `charts/mechanic/files/prompts/core.txt` | Add `=== CORRELATED GROUP ===` section and HARD RULE 11 |
| `charts/mechanic/templates/deployment-watcher.yaml` | Add three correlation env vars as Helm-controlled values |
| `charts/mechanic/values.yaml` | Add `correlationWindowSeconds`, `disableCorrelation`, `multiPodThreshold` under `watcher:` |
| `testdata/crds/remediationjob_crd.yaml` | Add `Suppressed` to `status.phase` enum; add `correlationGroupID` field |

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
- [x] `helm template mechanic charts/mechanic | kubectl apply --dry-run=client -f -` passes
- [x] `DISABLE_CORRELATION=true` reverts to pre-epic dispatch behaviour (verified by test)
- [x] Worklog entry created in `docs/WORKLOGS/`

## Post-Ship Fix (v0.3.24)

After initial release in v0.3.23, a bug was identified and fixed in v0.3.24:

**Bug:** A non-primary `RemediationJob` whose correlation window elapsed *after* its
primary had already transitioned to `PhaseDispatched` would solo-dispatch instead of
suppressing itself. Root cause: `pendingPeers()` excluded non-Pending peers, making
the dispatched primary invisible to the correlator. The non-primary saw no peers, found
no correlation match, and dispatched independently — producing redundant agent jobs.

**Fix (v0.3.24):**
- Renamed `pendingPeers()` → `correlationPeers()` and expanded the phase filter to
  include `PhaseDispatched` and `PhaseRunning` in addition to `PhasePending`.
- In the non-primary reconcile path, if the correlated primary is already
  `PhaseDispatched` or `PhaseRunning`, the non-primary immediately calls
  `transitionSuppressed()` on itself using the primary's `CorrelationGroupID`.

Regression test: `TestCorrelationBug_PodDispatchesSolo_RegressionTest` in
`internal/controller/remediationjob_controller_test.go` — confirmed to fail on pre-fix
code and pass on fixed code.

Validated on live cluster: after deleting stale rjobs and triggering fresh findings,
all Deployment+Pod pairs produced exactly one dispatched Deployment rjob and one
Suppressed Pod rjob, sharing the same `CorrelationGroupID`.

**Out of scope for this epic:**
- End-to-end tests (`test/e2e/`) — these require a real cluster or `kind` and are deferred
  to a dedicated e2e testing epic. The correlation integration tests in
  `internal/controller/correlation_integration_test.go` use `envtest` and cover the
  controller logic end-to-end at the API level. The remaining gap (agent consuming
  `FINDING_CORRELATED_FINDINGS` correctly, full PR workflow) is deferred.

## New Configuration Variables

```bash
# Correlation window duration in seconds (default: 30).
# Set to 0 to skip the hold period — the correlator evaluates on the first reconcile
# after phase initialisation without waiting. This is useful for testing or for
# environments where findings arrive nearly simultaneously and no hold is needed.
# To bypass correlation entirely (no hold, no grouping, immediate dispatch), use
# DISABLE_CORRELATION=true instead.
CORRELATION_WINDOW_SECONDS=30

# Disable all correlation logic and dispatch immediately (default: false)
DISABLE_CORRELATION=false

# Minimum pod count on same node to trigger MultiPodSameNodeRule (default: 3)
CORRELATION_MULTI_POD_THRESHOLD=3
```
