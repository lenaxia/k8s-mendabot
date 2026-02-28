# Story 03: SourceProviderReconciler Wiring, Config, and main.go

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mechanic operator**, I want the watcher to respect a maximum
self-remediation depth and a cooldown period, so that a broken agent image
cannot loop indefinitely or exhaust my LLM quota.

---

## Problem

`SourceProviderReconciler` in `internal/provider/provider.go` currently has no
awareness of `Finding.ChainDepth`. It will create a `RemediationJob` for any
non-nil Finding without checking depth limits. This story adds:

1. Two new config fields and their env-var parsing.
2. Depth enforcement in `SourceProviderReconciler.Reconcile`.
3. Circuit breaker call site in `SourceProviderReconciler.Reconcile`.
4. Circuit breaker injection via a new struct field.
5. `cmd/watcher/main.go` construction of the circuit breaker.

The circuit breaker implementation itself is in STORY_04. This story defines
the interface the reconciler calls and wires it in. STORY_04 provides the
concrete type.

---

## Acceptance Criteria

- [ ] `config.Config` has two new fields:
  - `SelfRemediationMaxDepth int` — parsed from `SELF_REMEDIATION_MAX_DEPTH`;
    default `2`; must be `>= 0`.
  - `SelfRemediationCooldown time.Duration` — parsed from
    `SELF_REMEDIATION_COOLDOWN_SECONDS`; default `300s`; must be `>= 0`.
- [ ] `config.FromEnv()` returns an error for non-integer values; both `0`
  values are valid (they disable the respective feature).
- [ ] `SourceProviderReconciler` has a new field:
  ```go
  CircuitBreaker circuitbreaker.Gater
  ```
  where `circuitbreaker.Gater` is defined in STORY_04 (see interface spec below).
  A `nil` value means the circuit breaker is disabled.
- [ ] `SourceProviderReconciler.Reconcile` enforces depth **before** the
  deduplication list query:
  - If `finding.ChainDepth > 0` and
    `cfg.SelfRemediationMaxDepth > 0` and
    `finding.ChainDepth > cfg.SelfRemediationMaxDepth`:
    log a warning and return `ctrl.Result{}, nil` (discard — do not requeue).
  - If `cfg.SelfRemediationMaxDepth == 0` and `finding.ChainDepth > 0`:
    log a warning and return `ctrl.Result{}, nil` (disabled — ignore all
    self-remediations).
- [ ] `SourceProviderReconciler.Reconcile` calls the circuit breaker **after**
  depth enforcement and **before** deduplication:
  - Only when `finding.ChainDepth > 0` (never for normal findings).
  - Only when `r.CircuitBreaker != nil`.
  - If `allowed == false`: log info and return `ctrl.Result{RequeueAfter: remaining}`.
  - If the circuit breaker returns an error: return the error (requeue).
- [ ] `cmd/watcher/main.go` constructs a `*circuitbreaker.CircuitBreaker` and
  injects it into each `SourceProviderReconciler` via the `CircuitBreaker` field.
  The circuit breaker is constructed only once and shared across all providers.
  Construction is skipped (field left `nil`) when `cfg.SelfRemediationCooldown == 0`.
- [ ] `config_test.go` covers the new fields (defaults, valid values, invalid
  values, zero values).

---

## Technical Implementation

### Circuit breaker interface (defined in `internal/circuitbreaker/`)

This story depends on STORY_04 defining this type. Specify the interface here
so both stories can be implemented independently:

```go
// Gater is the interface SourceProviderReconciler uses to gate self-remediations.
// The concrete implementation is CircuitBreaker.
type Gater interface {
    // ShouldAllow returns (true, 0, nil) if the self-remediation may proceed.
    // Returns (false, remaining, nil) if the cooldown has not elapsed.
    // Returns (false, 0, err) on an unexpected error.
    ShouldAllow(ctx context.Context) (allowed bool, remaining time.Duration, err error)
}
```

### Changes to `internal/config/config.go`

Add two fields to `Config`:

```go
SelfRemediationMaxDepth  int           // SELF_REMEDIATION_MAX_DEPTH — default 2; 0 = disabled
SelfRemediationCooldown  time.Duration // SELF_REMEDIATION_COOLDOWN_SECONDS — default 300s; 0 = disabled
```

Add parsing at the end of `FromEnv()`:

```go
depthStr := os.Getenv("SELF_REMEDIATION_MAX_DEPTH")
if depthStr == "" {
    cfg.SelfRemediationMaxDepth = 2
} else {
    n, err := strconv.Atoi(depthStr)
    if err != nil {
        return Config{}, fmt.Errorf("SELF_REMEDIATION_MAX_DEPTH must be an integer: %w", err)
    }
    if n < 0 {
        return Config{}, fmt.Errorf("SELF_REMEDIATION_MAX_DEPTH must be >= 0, got %d", n)
    }
    cfg.SelfRemediationMaxDepth = n
}

cooldownStr := os.Getenv("SELF_REMEDIATION_COOLDOWN_SECONDS")
if cooldownStr == "" {
    cfg.SelfRemediationCooldown = 300 * time.Second
} else {
    n, err := strconv.Atoi(cooldownStr)
    if err != nil {
        return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS must be an integer: %w", err)
    }
    if n < 0 {
        return Config{}, fmt.Errorf("SELF_REMEDIATION_COOLDOWN_SECONDS must be >= 0, got %d", n)
    }
    cfg.SelfRemediationCooldown = time.Duration(n) * time.Second
}
```

### Changes to `internal/provider/provider.go`

Add `CircuitBreaker circuitbreaker.Gater` to `SourceProviderReconciler`.

Insert the following block in `Reconcile`, **immediately after** `finding` is
confirmed non-nil and the injection-detection checks complete, and **before**
the fingerprint is computed (i.e. before the `domain.FindingFingerprint` call):

```go
// Self-remediation depth gate.
if finding.ChainDepth > 0 {
    maxDepth := r.Cfg.SelfRemediationMaxDepth
    if maxDepth == 0 || finding.ChainDepth > maxDepth {
        if r.Log != nil {
            r.Log.Warn("self-remediation suppressed",
                zap.Bool("audit", true),
                zap.String("event", "self_remediation.depth_exceeded"),
                zap.String("provider", r.Provider.ProviderName()),
                zap.String("kind", finding.Kind),
                zap.String("namespace", finding.Namespace),
                zap.String("name", finding.Name),
                zap.Int("chainDepth", finding.ChainDepth),
                zap.Int("maxDepth", maxDepth),
            )
        }
        return ctrl.Result{}, nil
    }

    if r.CircuitBreaker != nil {
        allowed, remaining, err := r.CircuitBreaker.ShouldAllow(ctx)
        if err != nil {
            return ctrl.Result{}, fmt.Errorf("circuit breaker: %w", err)
        }
        if !allowed {
            if r.Log != nil {
                r.Log.Info("self-remediation suppressed by circuit breaker",
                    zap.Bool("audit", true),
                    zap.String("event", "self_remediation.circuit_breaker"),
                    zap.String("provider", r.Provider.ProviderName()),
                    zap.String("kind", finding.Kind),
                    zap.String("namespace", finding.Namespace),
                    zap.String("name", finding.Name),
                    zap.Int("chainDepth", finding.ChainDepth),
                    zap.Duration("remaining", remaining),
                )
            }
            return ctrl.Result{RequeueAfter: remaining}, nil
        }
    }
}
```

### Changes to `cmd/watcher/main.go`

The circuit breaker must be constructed **after the manager is created**
(`mgr, err := ctrl.NewManager(...)` at `cmd/watcher/main.go:97`) because it
needs `mgr.GetClient()`. Placing it before the manager is a compile error.

Insert the circuit breaker construction **between line 142 (`combinedChecker :=
readiness.All(...)`) and line 155 (`nativeClient := mgr.GetClient()`)**:

```go
var cb circuitbreaker.Gater
if cfg.SelfRemediationCooldown > 0 {
    cb = circuitbreaker.New(mgr.GetClient(), cfg.AgentNamespace, cfg.SelfRemediationCooldown)
}
```

Add `CircuitBreaker: cb` to every `SourceProviderReconciler` literal in the
provider loop.

Add the import `"github.com/lenaxia/k8s-mechanic/internal/circuitbreaker"`.

---

## Placement of the gate in `Reconcile`

The gate must come **after** namespace filtering and severity filtering, and
**before** the fingerprint computation. This ensures:
- A self-remediation finding suppressed by namespace/severity rules never
  reaches the circuit breaker (no wasted state writes).
- The circuit breaker is only consulted for findings that would otherwise
  proceed to create a `RemediationJob`.

The exact insertion point in `internal/provider/provider.go` is **after line
238** (the closing `}` of the severity threshold block) and **before line 240**
(`fp, err := domain.FindingFingerprint(finding)`).

For reference, the surrounding structure at the time of implementation:
```
line 167: }  ← closing } of DetectInjection(finding.Details) outer block
...
line 219: }  ← namespace filter block ends
...
line 238: }  ← severity check block ends
line 239:    ← INSERT DEPTH GATE HERE
line 240: fp, err := domain.FindingFingerprint(finding)
```

---

## Files to modify

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `SelfRemediationMaxDepth`, `SelfRemediationCooldown`; parse from env |
| `internal/config/config_test.go` | Add tests for both new fields |
| `internal/provider/provider.go` | Add `CircuitBreaker` field; add depth gate + circuit breaker call |
| `internal/provider/provider_test.go` | Add tests for depth suppression and circuit breaker gating |
| `cmd/watcher/main.go` | Construct `circuitbreaker.CircuitBreaker`; inject into providers |

---

## Testing Requirements

**`internal/config/config_test.go`:**

| Case | Env | Expected |
|---|---|---|
| Default depth | unset | `SelfRemediationMaxDepth == 2` |
| Default cooldown | unset | `SelfRemediationCooldown == 300s` |
| Zero depth | `SELF_REMEDIATION_MAX_DEPTH=0` | `SelfRemediationMaxDepth == 0` (valid) |
| Zero cooldown | `SELF_REMEDIATION_COOLDOWN_SECONDS=0` | `SelfRemediationCooldown == 0` (valid) |
| Negative depth | `SELF_REMEDIATION_MAX_DEPTH=-1` | error |
| Negative cooldown | `SELF_REMEDIATION_COOLDOWN_SECONDS=-1` | error |
| Non-integer depth | `SELF_REMEDIATION_MAX_DEPTH=foo` | error |

**`internal/provider/provider_test.go`:**

| Case | Finding | Config | CB | Expected result |
|---|---|---|---|---|
| Depth 0 (normal finding) | `ChainDepth: 0` | any | nil | passes through, RJob created |
| Depth within limit | `ChainDepth: 1` | `MaxDepth: 2` | nil | passes through, RJob created |
| Depth at limit | `ChainDepth: 2` | `MaxDepth: 2` | nil | passes through, RJob created |
| Depth exceeds limit | `ChainDepth: 3` | `MaxDepth: 2` | nil | suppressed, no RJob |
| MaxDepth == 0 | `ChainDepth: 1` | `MaxDepth: 0` | nil | suppressed, no RJob |
| CB blocks | `ChainDepth: 1` | `MaxDepth: 2` | blocked (fake Gater) | `RequeueAfter` |
| CB allows | `ChainDepth: 1` | `MaxDepth: 2` | allowed (fake Gater) | RJob created |
| CB error | `ChainDepth: 1` | `MaxDepth: 2` | error (fake Gater) | error returned |
| CB nil, depth > 0 | `ChainDepth: 1` | `MaxDepth: 2` | nil | passes through (no CB) |

Use a `fakeGater` struct implementing `circuitbreaker.Gater` for the test cases
that exercise the circuit breaker path. Do not depend on the concrete
`CircuitBreaker` implementation (that is tested in STORY_04).

---

## Dependencies

**Depends on:** STORY_01 (schema), STORY_04 (interface definition for `Gater`)
**Blocks:** nothing (this is the final integration story; STORY_04 can be
implemented concurrently once the interface is defined)

---

## Definition of Done

- [ ] All tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
- [ ] Config fields tested with defaults, valid values, zero, and invalid values
- [ ] Reconciler tested with fake `Gater` for all gate outcomes
- [ ] `main.go` compiles with circuit breaker wired in
