# Epic 24: Severity Tiers on Findings

**Feature Tracker:** FT-A3
**Area:** Accuracy & Precision

## Purpose

Add a `Severity` field to `domain.Finding` and `RemediationJobSpec` so that findings are
classified by impact before any investigation is dispatched. Each native provider assigns
a severity based on the detected condition. A `MIN_SEVERITY` env var on the watcher
Deployment suppresses findings below the configured threshold, and the agent prompt
receives `FINDING_SEVERITY` to calibrate how aggressively to propose a fix.

Without severity tiers, a single crashed pod and a cluster-wide network failure are
treated identically — same 2-minute stabilisation window, same queue priority, same agent
confidence threshold. Severity lets the system differentiate and act proportionally.

## Status: Complete

## Severity Values

| Value | Meaning |
|-------|---------|
| `critical` | Immediate service impact; investigation should be dispatched at the earliest opportunity |
| `high` | Significant degradation; likely user-visible |
| `medium` | Partial degradation or elevated error rate; investigation warranted but not urgent |
| `low` | Minor or intermittent issue; useful signal but low dispatch priority |

Severity values are ordered `critical > high > medium > low` for threshold comparisons.

## Provider Severity Table

| Provider | Condition | Severity |
|----------|-----------|----------|
| PodProvider | CrashLoopBackOff (> 5 restarts) | `critical` |
| PodProvider | OOMKilled | `high` |
| PodProvider | ImagePullBackOff / ErrImagePull | `high` |
| PodProvider | Unschedulable | `high` |
| PodProvider | Non-zero exit code (other) | `medium` |
| DeploymentProvider | 0 ready replicas | `critical` |
| DeploymentProvider | < 50% ready replicas | `high` |
| DeploymentProvider | `Available=False` condition | `medium` |
| StatefulSetProvider | 0 ready replicas | `critical` |
| StatefulSetProvider | < 50% ready replicas | `high` |
| NodeProvider | `NotReady` condition | `critical` |
| NodeProvider | Any pressure condition (`MemoryPressure`, `DiskPressure`, `PIDPressure`) | `high` |
| JobProvider | Exhausted backoff limit | `medium` |
| PVCProvider | `ProvisioningFailed` or `Pending` with no progress | `high` |

## Architecture

### domain.Finding

```go
type Finding struct {
    Namespace    string
    Kind         string
    Name         string
    ParentObject string
    Errors       string   // JSON-encoded []errorEntry; existing field, unchanged
    Severity     Severity // new field — impact tier; zero value "" is treated as SeverityLow by the filter
}
```

### domain.Severity

```go
// Severity is the impact tier of a Finding.
type Severity string

const (
    SeverityCritical Severity = "critical"
    SeverityHigh     Severity = "high"
    SeverityMedium   Severity = "medium"
    SeverityLow      Severity = "low"
)
```

> **Implementation note:** `Severity` must be a named type (not a bare string) to
> prevent accidental assignment of free-form strings. Comparison for threshold filtering
> uses `SeverityLevel(s int)` helper — see STORY_01.

### RemediationJobSpec

`RemediationJobSpec` gains a `Severity string` field. This is stored as a plain `string`
in the CRD (not a typed Go type) to keep the CRD schema simple.

### MIN_SEVERITY filtering

`config.Config` gains a `MinSeverity Severity` field populated from the `MIN_SEVERITY`
env var (default: `low`, meaning all findings pass). The `SourceProviderReconciler`
drops findings below the minimum severity before fingerprinting and before creating a
`RemediationJob`.

### Agent prompt

`JobBuilder` injects `FINDING_SEVERITY` into the agent Job's environment. The prompt
uses it in the HARD RULES section:

```
FINDING_SEVERITY=${FINDING_SEVERITY}
A critical severity finding requires maximum investigation depth and a confident fix.
A low severity finding warrants a conservative, minimal-change proposal.
```

## Dependencies

- epic09-native-provider complete (all six providers in `internal/provider/native/`)
- epic00.1-interfaces complete (`RemediationJobSpec` is defined)

## Blocks

- epic13-multi-signal-correlation (severity is a correlation input: same-severity correlated findings are grouped)
- epic23-structured-audit-log (audit log entries must include severity)

## Stories

| Story | File | Status |
|-------|------|--------|
| Domain — Severity type, constants, and level helper | [STORY_01_severity_domain.md](STORY_01_severity_domain.md) | Complete |
| CRD — Add Severity field to RemediationJobSpec | [STORY_02_crd_severity_field.md](STORY_02_crd_severity_field.md) | Complete |
| Providers — Assign severity in ExtractFinding | [STORY_03_provider_severity.md](STORY_03_provider_severity.md) | Complete |
| Config — MIN_SEVERITY env var and reconciler filter | [STORY_04_min_severity_filter.md](STORY_04_min_severity_filter.md) | Complete |
| JobBuilder — Inject FINDING_SEVERITY into agent Job | [STORY_05_jobbuilder_severity.md](STORY_05_jobbuilder_severity.md) | Complete |
| Prompt — Use FINDING_SEVERITY in agent instructions | [STORY_06_prompt_severity.md](STORY_06_prompt_severity.md) | Complete |

## Implementation Order

```
STORY_01 (domain) ──> STORY_02 (CRD)
                  ──> STORY_03 (providers)
                  ──> STORY_04 (config + filter)
                        └──> STORY_05 (jobbuilder)
                               └──> STORY_06 (prompt)
```

STORY_02, STORY_03, and STORY_04 are independent once STORY_01 is complete.
STORY_05 depends on STORY_04 (severity flows through the RemediationJob into the Job spec).
STORY_06 depends on STORY_05 (env var must exist before the prompt can reference it).

## Definition of Done

- [ ] `domain.Severity` typed constant defined; `SeverityLevel` comparison helper implemented
- [ ] `domain.Finding.Severity` field present and populated by all six providers
- [ ] `RemediationJobSpec.Severity string` field present in Go types and CRD schema
- [ ] `testdata/crds/remediationjob_crd.yaml` updated with `severity: {type: string}` in spec properties
- [ ] Each provider assigns severity per the Provider Severity Table above
- [ ] `config.Config.MinSeverity` populated from `MIN_SEVERITY` env var; defaults to `low`
- [ ] `SourceProviderReconciler` drops findings below `MinSeverity` before fingerprinting
- [ ] `JobBuilder` injects `FINDING_SEVERITY` into the agent Job's env
- [ ] Prompt configmap updated to reference `FINDING_SEVERITY` in context and hard rules
- [ ] All unit tests pass with `-race`
- [ ] Worklog written
