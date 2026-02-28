# Story: Stabilisation Window

**Epic:** [epic09-native-provider](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator**, I want mechanic to wait a configurable period after a finding
is first detected before creating a `RemediationJob`, so that transient failures (e.g. a
pod that crashes once during a rolling restart) do not trigger unnecessary investigations.

---

## Background

Without a stabilisation window, every transient pod failure immediately produces a
`RemediationJob`. The deduplication fingerprint prevents duplicate Jobs for the *same*
failure, but a transient failure (pod crashes once, recovers within 30 seconds) still
triggers a Job before the pod has had a chance to recover.

The stabilisation window adds a configurable observation period: a finding must be
continuously present for the full window before a `RemediationJob` is created. If the
finding clears (i.e. the next reconcile returns `nil` finding), the window resets.

---

## Design: Option C — in-memory map in SourceProviderReconciler

`SourceProviderReconciler` gains a `firstSeen map[string]time.Time` field, keyed by
finding fingerprint. On each reconcile:

1. If `ExtractFinding` returns `nil` — the resource is healthy. If the fingerprint was
   in `firstSeen`, delete it. Return `ctrl.Result{}`.
2. If `ExtractFinding` returns a finding:
   a. Compute the fingerprint via `domain.FindingFingerprint`.
   b. **Fast path for `StabilisationWindow == 0`:** if `cfg.StabilisationWindow == 0`,
      skip the `firstSeen` map entirely and proceed directly to the dedup + Job creation
      logic. This preserves the original immediate-dispatch behaviour for operators who
      set the window to zero.
   c. If fingerprint is not in `firstSeen`, record `time.Now()` and return
      `ctrl.Result{RequeueAfter: cfg.StabilisationWindow}`.
   d. If `time.Since(firstSeen[fp]) < cfg.StabilisationWindow`, return
      `ctrl.Result{RequeueAfter: remaining}` where `remaining = window - elapsed`.
   e. Window has elapsed — proceed with the existing dedup + `RemediationJob` creation
      logic. Do **not** delete from `firstSeen` yet (leave it so repeated reconciles
      after Job creation do not restart the window).
3. If the watched object is deleted (not-found branch), clear `firstSeen` entirely
   (acceptable approximation per design rationale above).

**Trade-off accepted:** The map is in-memory. A watcher restart clears it, resetting
any active window. At worst this means a previously observed transient failure restarts
its window after a restart. This is not a correctness failure — it only delays `RemediationJob`
creation by at most one window duration. For a default 2-minute window this is acceptable.

**Concurrency and thread safety:** `SourceProviderReconciler` uses a `firstSeen` map
without a mutex. This is safe because:
- controller-runtime runs each controller with a **single worker goroutine** by default.
  The reconciler for one `ObjectType` is never called concurrently.
- The watcher binary does not set `MaxConcurrentReconciles` anywhere in `main.go` —
  it relies on the default of 1.

**Important:** if a future change adds `MaxConcurrentReconciles > 1` to any
`SourceProviderReconciler`, the `firstSeen` map **must** be protected by a
`sync.Mutex`. Add a comment in the implementation code:

```go
// firstSeen is not mutex-protected: controller-runtime guarantees a single
// worker goroutine per controller (MaxConcurrentReconciles defaults to 1).
// If MaxConcurrentReconciles is ever set > 1 for this reconciler, replace
// this map with a sync.Map or add a sync.Mutex.
firstSeen map[string]time.Time
```

---

## Acceptance Criteria

- [ ] `config.Config` gains a `StabilisationWindow time.Duration` field
- [ ] `config.FromEnv` reads `STABILISATION_WINDOW_SECONDS` (integer, seconds); default
  `120`; must be `>= 0` (zero means no window — findings create Jobs immediately); invalid
  values return an error
- [ ] `SourceProviderReconciler` gains a `firstSeen map[string]time.Time` field,
  initialised to an empty map when the reconciler is constructed in `main.go`
- [ ] On each reconcile where `ExtractFinding` returns a non-nil finding:
  - Fingerprint is computed via `domain.FindingFingerprint` (STORY_01 must be complete)
  - If fingerprint is not in `firstSeen`, record `time.Now()`, return
    `ctrl.Result{RequeueAfter: cfg.StabilisationWindow}`
  - If window has not elapsed, return
    `ctrl.Result{RequeueAfter: remaining}` where `remaining = window - elapsed`
  - If window has elapsed (or `StabilisationWindow == 0`), proceed to dedup + Job creation
- [ ] On each reconcile where `ExtractFinding` returns nil, delete the resource's
  fingerprint from `firstSeen` if present
- [ ] On not-found (deleted object), clear `firstSeen` entirely (acceptable approximation
  per design rationale above)
- [ ] When `StabilisationWindow == 0`, window logic is skipped entirely using an explicit
  fast path **before** consulting `firstSeen`:
  ```go
  if r.Cfg.StabilisationWindow == 0 {
      // fast path: no window, proceed directly to dedup + Job creation
  } else {
      // window logic: consult firstSeen map
  }
  ```
  This preserves the original immediate-dispatch behaviour for operators who set the window
  to zero, and avoids the ambiguity of `time.Since(t) >= 0` being always true.
- [ ] `config_test.go` covers the new field: zero, default, valid non-zero, and invalid
  (non-integer, negative)

---

## Test Cases (all must be written before implementation)

### config_test.go additions

| Test Name | Input | Expected |
|-----------|-------|----------|
| `StabilisationWindowDefault` | `STABILISATION_WINDOW_SECONDS` not set | `cfg.StabilisationWindow == 120 * time.Second` |
| `StabilisationWindowZero` | `STABILISATION_WINDOW_SECONDS=0` | `cfg.StabilisationWindow == 0` |
| `StabilisationWindowCustom` | `STABILISATION_WINDOW_SECONDS=300` | `cfg.StabilisationWindow == 300 * time.Second` |
| `StabilisationWindowNegative` | `STABILISATION_WINDOW_SECONDS=-1` | error |
| `StabilisationWindowInvalid` | `STABILISATION_WINDOW_SECONDS=abc` | error |

### provider_test.go additions (SourceProviderReconciler)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `WindowNotElapsed` | Finding present; reconciler has `firstSeen` entry with `time.Now()` | Returns `RequeueAfter > 0`; no `RemediationJob` created |
| `WindowElapsed` | Finding present; reconciler has `firstSeen` entry with `time.Now().Add(-3*time.Minute)`; window is 2 min | `RemediationJob` created |
| `WindowZeroImmediate` | `StabilisationWindow == 0`; finding present | `RemediationJob` created immediately (no requeue) |
| `FindingClearsResetsWindow` | Finding present, then nil on next reconcile | `firstSeen` entry evicted; subsequent finding restarts the window |
| `NotFoundClearsMap` | Object deleted | `firstSeen` map cleared |

---

## Tasks

- [ ] Write `StabilisationWindow` config tests **in the existing**
  `internal/config/config_test.go` (add rows to the existing env-var test table; TDD
  — verify they fail before adding the field)
- [ ] Add `StabilisationWindow time.Duration` to `config.Config`; update `FromEnv` to
  read `STABILISATION_WINDOW_SECONDS` with default `120` and validation
- [ ] Write stabilisation window reconciler tests in `internal/provider/provider_test.go`
  (TDD — must fail first)
- [ ] Add `firstSeen map[string]time.Time` to `SourceProviderReconciler`; update
  `Reconcile` with window logic including the explicit `window == 0` fast path
- [ ] Update `cmd/watcher/main.go`: initialise `firstSeen` map on each reconciler
  construction; pass `cfg.StabilisationWindow` via `Cfg`
- [ ] Run full test suite: `go test -timeout 120s -race ./...`

---

## Dependencies

**Depends on:** STORY_01 (`domain.FindingFingerprint` — the window logic uses the
fingerprint as the map key, and the reconciler must call `domain.FindingFingerprint`
rather than `r.Provider.Fingerprint` for this to work correctly)
**Depends on:** STORY_02 (slim `SourceProvider` interface — `r.Provider.Fingerprint`
must be removed before this story is implemented)
**Note on ordering with STORY_08:** STORY_12 modifies `internal/provider/provider.go`
and `cmd/watcher/main.go`. STORY_08 also modifies `cmd/watcher/main.go` (adding native
provider registrations). These two stories are independent branches in the implementation
graph and can be worked in either order. However, `main.go` must have the final provider
registration loop from STORY_08 before STORY_12 adds the `firstSeen` map — if STORY_08
is done first, the `firstSeen` map is added to each provider's reconciler in the loop
written by STORY_08. If STORY_12 is done before STORY_08, the single-provider loop in
`main.go` gets `firstSeen` added, and STORY_08 then extends the loop without disrupting it.
Either order works; just ensure both changes are present before STORY_09.
**Blocks:** Nothing (this story must complete before STORY_09)

---

## Definition of Done

- [ ] All new config and reconciler tests pass with `-race`
- [ ] Full test suite passes with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
- [ ] `STABILISATION_WINDOW_SECONDS` documented in `deploy/kustomize/deployment-watcher.yaml`
  env block (as a commented-out optional variable with its default)
