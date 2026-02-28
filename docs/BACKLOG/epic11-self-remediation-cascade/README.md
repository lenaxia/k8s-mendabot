# Epic 11: Self-Remediation Cascade Prevention

## Status: Complete — branch `feature/epic11-self-remediation-cascade`

## Problem Statement

The `jobProvider` (`internal/provider/native/job.go`) currently silences all
`batch/v1` Jobs whose `app.kubernetes.io/managed-by: mechanic-watcher` label
is set, returning `(nil, nil)` unconditionally. This guard prevents an infinite
cascade where a failed agent job spawns another agent job which also fails.

The guard is correct. The problem is that it also prevents any investigation
of a *legitimately* failing agent job — for example a misconfigured LLM
credential or a broken agent image. We want to allow a limited number of
self-remediation attempts before backing off.

## Goal

Allow the watcher to investigate a failing mechanic agent job **up to a
configurable depth limit** (`SELF_REMEDIATION_MAX_DEPTH`), then stop
permanently. A per-namespace cooldown (`SELF_REMEDIATION_COOLDOWN_SECONDS`)
prevents burst behaviour.

No upstream repository routing. All PRs always target `GITOPS_REPO`.

## What this epic does NOT do

- Infrastructure cascade suppression (node → pod correlation) — that is
  covered by the Node provider (`internal/provider/native/node.go`) which
  already produces a Finding for `NodeReady=False`, `MemoryPressure=True`,
  etc. Pod findings from a bad node will naturally resolve when the node
  heals, so deduplication handles the noise. A separate cascade-suppression
  layer is not needed.
- Prometheus metrics — the system has no custom metrics infrastructure yet.
- Upstream bug routing — removed; not required.

## Dependencies

- epic01-controller complete (`SourceProviderReconciler`)
- epic02-jobbuilder complete (`JobBuilder`, `batch/v1` Job labels)

## Stories

| # | File | Title | Status | Priority | Effort |
|---|------|-------|--------|----------|--------|
| 1 | [STORY_01_schema_foundations.md](STORY_01_schema_foundations.md) | Schema foundations: ChainDepth in Finding and RemediationJobSpec | Complete | Critical | 1h |
| 2 | [STORY_02_job_provider_detection.md](STORY_02_job_provider_detection.md) | jobProvider: detect mechanic agent jobs and compute chain depth | Complete | Critical | 2h |
| 3 | [STORY_03_reconciler_wiring.md](STORY_03_reconciler_wiring.md) | SourceProviderReconciler: depth gate, circuit breaker wiring, main.go | Complete | Critical | 3h |
| 4 | [STORY_04_circuit_breaker.md](STORY_04_circuit_breaker.md) | Circuit breaker: in-memory cooldown | Complete | High | 1h |

## Technical Overview

### Data flow

```
batch/v1 Job (failed, managed-by=mechanic-watcher)
  │
  ▼
jobProvider.ExtractFinding()           [STORY_02]
  - reads ChainDepth from owning RemediationJob (API call)
  - increments depth → new ChainDepth
  - returns Finding{..., ChainDepth: N}
  │
  ▼
SourceProviderReconciler.Reconcile()   [STORY_03]
  - namespace + severity filters (existing gates)
  - if ChainDepth > 0 AND maxDepth == 0 → suppressed (self-remediation disabled)
  - if ChainDepth > 0 AND ChainDepth > maxDepth → suppressed (depth exceeded)
  - if ChainDepth > 0 → call circuitBreaker.ShouldAllow()
      → if blocked → return RequeueAfter(remaining)
  - creates RemediationJob{..., Spec.Finding.ChainDepth: N}
  │
  ▼
RemediationJob → batch/v1 Job → agent
```

### Self-remediation depth

- Depth `0` means the finding is NOT a self-remediation (normal path).
- Depth `1` means the failed job is a mechanic agent job (first level).
- Depth `N` means a chain of N nested self-remediations.
- When `SELF_REMEDIATION_MAX_DEPTH=2`, depths 1 and 2 are allowed; depth 3
  is blocked.
- Setting `SELF_REMEDIATION_MAX_DEPTH=0` disables self-remediation entirely
  (agent job failures are ignored, matching the old behaviour).

### Chain depth source

The `jobProvider` reads chain depth from the owning `RemediationJob` CRD
via an owner-reference lookup. The `RemediationJob.Spec.Finding.ChainDepth`
field is the authoritative source. There is no annotation fallback — the
code was removed entirely, there is nothing to be backward-compatible with.

### Circuit breaker

A ConfigMap named `mechanic-circuit-breaker` in `AgentNamespace` stores the
RFC3339 timestamp of the last permitted self-remediation. If another
self-remediation arrives within `SELF_REMEDIATION_COOLDOWN_SECONDS`, it is
held with `RequeueAfter`. Zero cooldown disables the circuit breaker.

Only self-remediation findings (`ChainDepth > 0`) pass through the circuit
breaker. Normal findings (depth 0) are never gated by it.

### Node provider and cascade overlap

`internal/provider/native/node.go` already raises a Finding for:
- `NodeReady == False / Unknown`
- `NodeMemoryPressure == True`
- `NodeDiskPressure == True`
- `NodePIDPressure == True`
- `NodeNetworkUnavailable == True`

When a node goes bad, the node provider generates a single Finding for the
node. Pod failures caused by that node will generate separate pod Findings,
but those are deduplicated to a single `RemediationJob` per parent
(Deployment/StatefulSet). There is no need to correlate pod findings to node
conditions in this epic — doing so would duplicate logic already expressed by
the node provider.

## Configuration

```bash
# Maximum self-remediation chain depth (default: 2)
# 0 = disable self-remediation entirely
SELF_REMEDIATION_MAX_DEPTH=2

# Seconds between allowed self-remediations per namespace (default: 300)
# 0 = disable circuit breaker
SELF_REMEDIATION_COOLDOWN_SECONDS=300
```

Both variables are optional. Safe defaults are applied when absent.

## Definition of Done

- [x] `domain.Finding.ChainDepth int` field exists
- [x] `FindingSpec.ChainDepth int32` field exists in `RemediationJobSpec`
- [x] `RemediationJob` CRD testdata YAML updated
- [x] `jobProvider.ExtractFinding` returns non-nil for failed mechanic agent
      jobs with correct `ChainDepth`
- [x] `SourceProviderReconciler.Reconcile` enforces max-depth and calls
      circuit breaker for self-remediations
- [x] `CircuitBreaker` package exists with in-memory cooldown
- [x] `cmd/watcher/main.go` constructs and injects `CircuitBreaker`
- [x] `Config` contains `SelfRemediationMaxDepth` and `SelfRemediationCooldown`
- [x] All tests pass with `-race`
- [ ] ConfigMap RBAC rule added — N/A: implementation is in-memory only, no ConfigMap
