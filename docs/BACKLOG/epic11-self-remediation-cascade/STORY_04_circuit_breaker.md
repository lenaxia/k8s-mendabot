# Story 04: Circuit Breaker — ConfigMap-Backed Cooldown

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot operator**, I want a cooldown period enforced between
self-remediations, backed by a ConfigMap so it survives controller restarts,
so that a repeated agent failure cannot exhaust my LLM quota in a single burst.

---

## Problem

Without a cooldown, every reconcile cycle on a failing mendabot agent job
(within `SELF_REMEDIATION_MAX_DEPTH`) would attempt to create a new
`RemediationJob`. Deduplication prevents redundant RJobs for the same
fingerprint, but once an RJob is deleted (e.g. after it fails and is
re-dispatched), a new one could be created immediately. The circuit breaker
prevents this by enforcing a minimum gap between successive self-remediations.

---

## Acceptance Criteria

- [ ] New package `internal/circuitbreaker/` with files:
  - `circuitbreaker.go` — `Gater` interface + `CircuitBreaker` struct + `New`
  - `circuitbreaker_test.go` — unit tests
- [ ] `Gater` interface:
  ```go
  type Gater interface {
      ShouldAllow(ctx context.Context) (allowed bool, remaining time.Duration, err error)
  }
  ```
- [ ] `CircuitBreaker` struct implements `Gater`. Compile-time assertion:
  ```go
  var _ Gater = (*CircuitBreaker)(nil)
  ```
- [ ] `New(c client.Client, namespace string, cooldown time.Duration) *CircuitBreaker`
  is the constructor. `c` must be non-nil and `namespace` must be non-empty;
  panic with a descriptive message otherwise.
- [ ] On the first call to `ShouldAllow`, the circuit breaker reads the
  ConfigMap `mendabot-circuit-breaker` in `namespace` to load the last
  permitted timestamp. If the ConfigMap does not exist, it treats the last
  timestamp as zero (always allow on first call).
- [ ] On a call to `ShouldAllow` that returns `(true, 0, nil)`:
  - Update the in-memory `lastAllowed` timestamp.
  - Write the timestamp (RFC3339) to the ConfigMap key `last-self-remediation`.
  - Create the ConfigMap if it does not exist; update it if it does.
- [ ] On a call to `ShouldAllow` that returns `(false, remaining, nil)`:
  - Do NOT update the ConfigMap.
  - `remaining = cooldown - time.Since(lastAllowed)`.
- [ ] Concurrent calls are safe (`sync.Mutex` protecting `lastAllowed` and
  `initialized`).
- [ ] ConfigMap metadata:
  ```yaml
  name: mendabot-circuit-breaker
  namespace: <AgentNamespace>
  labels:
    app.kubernetes.io/managed-by: mendabot-watcher
    app.kubernetes.io/component: circuit-breaker
  ```
- [ ] RBAC: `charts/mendabot/templates/role-watcher.yaml` currently grants the
  watcher access to `batch/jobs`, `pods`, and `secrets` only. Add a new rule
  granting `get`, `create`, `update` on `configmaps`:
  ```yaml
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "create", "update"]
  ```

---

## Technical Implementation

### Package: `internal/circuitbreaker/`

```go
package circuitbreaker

import (
    "context"
    "fmt"
    "sync"
    "time"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const configMapName = "mendabot-circuit-breaker"
const timestampKey  = "last-self-remediation"

// Gater is the interface SourceProviderReconciler uses to gate self-remediations.
type Gater interface {
    ShouldAllow(ctx context.Context) (allowed bool, remaining time.Duration, err error)
}

// CircuitBreaker implements Gater with ConfigMap-backed persistence.
type CircuitBreaker struct {
    client    client.Client
    namespace string
    cooldown  time.Duration

    mu          sync.Mutex
    lastAllowed time.Time
    initialized bool
}

// New constructs a CircuitBreaker. Panics if client or namespace are nil/empty.
func New(c client.Client, namespace string, cooldown time.Duration) *CircuitBreaker {
    if c == nil {
        panic("circuitbreaker.New: client must not be nil")
    }
    if namespace == "" {
        panic("circuitbreaker.New: namespace must not be empty")
    }
    return &CircuitBreaker{client: c, namespace: namespace, cooldown: cooldown}
}
```

**`ShouldAllow` logic:**

```go
func (cb *CircuitBreaker) ShouldAllow(ctx context.Context) (bool, time.Duration, error) {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if !cb.initialized {
        if err := cb.loadState(ctx); err != nil {
            return false, 0, err
        }
        cb.initialized = true
    }

    if !cb.lastAllowed.IsZero() {
        elapsed := time.Since(cb.lastAllowed)
        if elapsed < cb.cooldown {
            return false, cb.cooldown - elapsed, nil
        }
    }

    now := time.Now()
    if err := cb.saveState(ctx, now); err != nil {
        return false, 0, err
    }
    cb.lastAllowed = now  // only update in-memory state after successful persist
    return true, 0, nil
}
```

Note: `saveState` is updated to accept the timestamp as a parameter rather
than reading `cb.lastAllowed`, so the two are set atomically only on success.
Update `saveState`'s signature accordingly:
```go
func (cb *CircuitBreaker) saveState(ctx context.Context, t time.Time) error {
    ts := t.UTC().Format(time.RFC3339)
    // ... rest of saveState unchanged
```

**`loadState`** — reads `mendabot-circuit-breaker` ConfigMap; on `IsNotFound`,
leaves `lastAllowed` as zero and returns nil. On any other error, returns the
error. Parses `data["last-self-remediation"]` as RFC3339 into `cb.lastAllowed`.
If the key is absent or empty, leaves `lastAllowed` as zero.

```go
func (cb *CircuitBreaker) loadState(ctx context.Context) error {
    var cm corev1.ConfigMap
    if err := cb.client.Get(ctx, client.ObjectKey{Namespace: cb.namespace, Name: configMapName}, &cm); err != nil {
        if apierrors.IsNotFound(err) {
            return nil
        }
        return fmt.Errorf("circuitbreaker: load state: %w", err)
    }
    ts, ok := cm.Data[timestampKey]
    if !ok || ts == "" {
        return nil
    }
    t, err := time.Parse(time.RFC3339, ts)
    if err != nil {
        return fmt.Errorf("circuitbreaker: parse timestamp %q: %w", ts, err)
    }
    cb.lastAllowed = t
    return nil
}
```

**`saveState`** — `client.Patch` cannot create a non-existent object, so use
a two-path approach: attempt `client.Create`; if the ConfigMap already exists
(`IsAlreadyExists`), fetch the current version with `client.Get` and call
`client.Update` with the updated timestamp. Do not use `client.Patch` here.

**Known limitation:** There is a narrow TOCTOU window between the `Get` in the
`IsAlreadyExists` path and the subsequent `Update` — if the ConfigMap is deleted
by an external actor in that window, `Update` returns Not Found and `saveState`
returns an error. This is acceptable: the error propagates to `ShouldAllow`,
which returns it to `Reconcile`, which requeues. On the next reconcile,
`initialized` is still `true` and `lastAllowed` is still the old value (because
`cb.lastAllowed` is only written after a successful `saveState`), so the
operation is retried cleanly.

```go
func (cb *CircuitBreaker) saveState(ctx context.Context, t time.Time) error {
    ts := t.UTC().Format(time.RFC3339)
    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      configMapName,
            Namespace: cb.namespace,
            Labels: map[string]string{
                "app.kubernetes.io/managed-by": "mendabot-watcher",
                "app.kubernetes.io/component":  "circuit-breaker",
            },
        },
        Data: map[string]string{timestampKey: ts},
    }
    if err := cb.client.Create(ctx, cm); err != nil {
        if !apierrors.IsAlreadyExists(err) {
            return fmt.Errorf("circuitbreaker: create state: %w", err)
        }
        var existing corev1.ConfigMap
        if err := cb.client.Get(ctx, client.ObjectKey{Namespace: cb.namespace, Name: configMapName}, &existing); err != nil {
            return fmt.Errorf("circuitbreaker: get state for update: %w", err)
        }
        if existing.Data == nil {
            existing.Data = make(map[string]string)
        }
        existing.Data[timestampKey] = ts
        if err := cb.client.Update(ctx, &existing); err != nil {
            return fmt.Errorf("circuitbreaker: update state: %w", err)
        }
    }
    return nil
}
```

### ConfigMap structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mendabot-circuit-breaker
  namespace: <AgentNamespace>
  labels:
    app.kubernetes.io/managed-by: mendabot-watcher
    app.kubernetes.io/component: circuit-breaker
data:
  last-self-remediation: "2026-02-24T10:00:00Z"
```

### RBAC

`charts/mendabot/templates/role-watcher.yaml` currently grants `batch/jobs`,
`pods`, and `secrets` only. Add a new rule — ConfigMap access is absent:

```yaml
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "create", "update"]
```

---

## What this story does NOT do

- Does not reset the circuit breaker manually (out of scope).
- Does not expose circuit breaker state as Prometheus metrics (out of scope).
- Does not gate normal (non-self-remediation) findings.

---

## Files to create / modify

| File | Change |
|------|--------|
| `internal/circuitbreaker/circuitbreaker.go` | New file: `Gater` interface + `CircuitBreaker` implementation |
| `internal/circuitbreaker/circuitbreaker_test.go` | New file: unit tests |
| `charts/mendabot/templates/role-watcher.yaml` | Add `configmaps` RBAC rule |

---

## Testing Requirements

All tests use a fake `client.Client` — no envtest required.

**Scheme setup:** The fake client must have `corev1` registered to serve
ConfigMap operations. Use `corev1.AddToScheme` when building the test scheme:

```go
scheme := runtime.NewScheme()
_ = corev1.AddToScheme(scheme)
fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
```

**Error injection:** To test `Create` and `Update` error paths, use
`fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{...})` from
`sigs.k8s.io/controller-runtime/pkg/client/interceptor`. Example:

```go
fakeClient := fake.NewClientBuilder().
    WithScheme(scheme).
    WithInterceptorFuncs(interceptor.Funcs{
        Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
            return fmt.Errorf("injected create error")
        },
    }).
    Build()
```

| Test case | Setup | Expected |
|---|---|---|
| First call, no ConfigMap | Empty fake client | allowed; ConfigMap created with timestamp |
| First call, CM with old timestamp | CM `last-self-remediation` = 1 year ago | allowed; CM updated |
| First call, CM with recent timestamp | CM timestamp = 1 min ago, cooldown = 5 min | blocked; remaining ≈ 4 min |
| Second call within cooldown | Call once (allowed), call again immediately with cooldown = 1h | blocked |
| CM does not exist, Create error | Interceptor injects Create error | error returned |
| CM exists, Get error on update path | Stateful interceptor: Create returns AlreadyExists, Get returns error (requires a call-count closure — see note below) | error returned |
| CM exists, Update error | Stateful interceptor: Create returns AlreadyExists, Get succeeds, Update returns error | error returned |
| loadState parse error | CM with `last-self-remediation: "not-a-timestamp"` | error returned |
| Concurrent calls | Two goroutines; cooldown = 1h | no data race; exactly one allowed |

The concurrent-call test requires `-race` to be meaningful. The "second call
within cooldown" test must not use `time.Sleep` — set cooldown to 1 hour and
call twice in immediate succession.

**Note on stateful interceptors:** The "CM exists, Get error on update path"
and "CM exists, Update error" test cases require the interceptor to behave
differently on the first `Create` call (return `AlreadyExists`) versus the
subsequent `Get` or `Update` call. Implement using a closure over a call
counter:

```go
createCalled := 0
fakeClient := fake.NewClientBuilder().
    WithScheme(scheme).
    WithObjects(existingCM).
    WithInterceptorFuncs(interceptor.Funcs{
        Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
            createCalled++
            return apierrors.NewAlreadyExists(schema.GroupResource{Resource: "configmaps"}, configMapName)
        },
        Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
            return fmt.Errorf("injected get error")
        },
    }).
    Build()
```

---

## Dependencies

**Ordering note:** `provider.go` (STORY_03) imports `internal/circuitbreaker`
for the `Gater` interface. The `Gater` interface definition in
`circuitbreaker.go` must therefore be committed before STORY_03's changes to
`provider.go` will compile. STORY_03 and STORY_04 can be developed
concurrently, but the interface definition must be committed first (or both
committed together in the same PR).

**Depends on:** STORY_01 (no direct code dependency, but ordering convention)
**Blocks:** nothing

---

## Definition of Done

- [ ] All tests pass with `-race`
- [ ] `go vet` clean
- [ ] `go build ./...` clean
- [ ] `var _ Gater = (*CircuitBreaker)(nil)` compile-time assertion present
- [ ] `loadState`, `saveState`, and `ShouldAllow` all tested including all error paths
- [ ] Concurrent test passes clean with `-race`
- [ ] ConfigMap RBAC rule added to `charts/mendabot/templates/role-watcher.yaml`
