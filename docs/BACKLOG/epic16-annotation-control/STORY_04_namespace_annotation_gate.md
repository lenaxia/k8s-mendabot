# Story 04: Reconciler — Namespace-Level Annotation Gate

**Epic:** [epic16-annotation-control](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator**, I want to annotate a `Namespace` object with
`mechanic.io/enabled: "false"` or `mechanic.io/skip-until: "YYYY-MM-DD"` to suppress
all mechanic investigations for every resource in that namespace, so that I can silence
an entire namespace (e.g. a load-test namespace or a namespace under active manual
investigation) without having to annotate every individual resource inside it.

---

## Background

Epic 16 (STORY_01–03) implemented annotation control at the **resource level**: each
provider's `ExtractFinding` calls `domain.ShouldSkip(obj.GetAnnotations(), time.Now())`
on the watched resource (Pod, Deployment, etc.). Annotating the `Namespace` object itself
has no effect on the current implementation.

This story extends the gate to the **namespace level** by adding a check in
`SourceProviderReconciler.Reconcile` (`internal/provider/provider.go`) that fetches the
`Namespace` object and calls `domain.ShouldSkip` on its annotations. Because the
reconciler already holds a `client.Client` (embedded in `SourceProviderReconciler`),
no structural changes are required — the `Get` call is a single additional API request
gated behind `if finding.Namespace != ""`.

### Why the reconciler, not the providers

`domain.ShouldSkip` is pure (takes only `map[string]string`). The domain layer must not
import `client.Client`. The six native providers already call `ShouldSkip` on the
resource's own annotations; the namespace fetch requires a client and therefore belongs
one layer up — in the reconciler — alongside the existing namespace-filter logic location
defined in STORY_02 of epic15.

### Exact insertion point

After `ExtractFinding` returns a non-nil finding (line 125) and after both
`domain.DetectInjection` blocks (lines 134–164), **before** `domain.FindingFingerprint`
(line 166). This matches the location specified for the epic15 namespace env-var filter,
ensuring all namespace-level gates are collocated:

```
line 164: end of DetectInjection(finding.Details) block
          ← INSERT namespace annotation gate here
line 166: fp, err := domain.FindingFingerprint(finding)
```

### Cluster-scoped exemption

`NodeProvider` sets `finding.Namespace = ""` (node.go line 102). The gate must be
wrapped in `if finding.Namespace != ""` so that cluster-scoped resources are
unconditionally exempt — there is no `Namespace` object to look up for them.

### Namespace object not found

If the `Namespace` object does not exist (e.g. it was deleted and a final reconcile is
in flight), the gate must treat it as "not annotated" and allow the finding to proceed.
Use `apierrors.IsNotFound` to distinguish a genuine missing namespace from an API error.
A genuine API error (network partition, RBAC issue) must be returned as an error so the
reconciler retries.

---

## Acceptance Criteria

- [x] `SourceProviderReconciler.Reconcile` fetches the `corev1.Namespace` object for
  `finding.Namespace` when `finding.Namespace != ""`, after both injection-detection
  blocks and before `domain.FindingFingerprint`.
- [x] When `domain.ShouldSkip(ns.GetAnnotations(), time.Now())` returns `true`, the
  reconciler returns `ctrl.Result{}, nil` without creating a `RemediationJob`.
- [x] When the `Namespace` object is not found (`apierrors.IsNotFound`), the gate is a
  no-op and the finding proceeds normally.
- [x] When the `Namespace` `Get` returns any other error, the reconciler returns
  `ctrl.Result{}, err` so controller-runtime retries.
- [x] Cluster-scoped findings (`finding.Namespace == ""`) bypass the gate entirely.
- [x] Suppressed findings are logged at `Debug` level (with `r.Log` nil-guard) with
  structured fields: `provider`, `namespace`, `kind`, `name`.
- [x] All new tests are table-driven and pass with `-race`.
- [x] Full test suite passes: `go test -timeout 120s -race ./...`

---

## Technical Implementation

### Gate block to insert in `Reconcile`

Insert after the `domain.DetectInjection(finding.Details)` block (after line 164),
before `fp, err := domain.FindingFingerprint(finding)` (line 166):

```go
if finding.Namespace != "" {
    var ns corev1.Namespace
    if err := r.Get(ctx, client.ObjectKey{Name: finding.Namespace}, &ns); err != nil {
        if !apierrors.IsNotFound(err) {
            return ctrl.Result{}, fmt.Errorf("fetching namespace %s: %w", finding.Namespace, err)
        }
    } else if domain.ShouldSkip(ns.GetAnnotations(), time.Now()) {
        if r.Log != nil {
            r.Log.Debug("namespace annotation gate: skipping finding",
                zap.String("provider", r.Provider.ProviderName()),
                zap.String("namespace", finding.Namespace),
                zap.String("kind", finding.Kind),
                zap.String("name", finding.Name),
            )
        }
        return ctrl.Result{}, nil
    }
}
```

No imports need to be added — `corev1`, `apierrors`, `client`, `fmt`, `time`, and `zap`
are already imported in `internal/provider/provider.go`.

---

## Test Cases

All cases are table-driven and belong in `internal/provider/provider_test.go`.
The existing `fakeSourceProvider` and fake client infrastructure is used throughout.

| Test name | Namespace annotation | `finding.Namespace` | Expected |
|---|---|---|---|
| `NSAnnotation_NoAnnotation_Proceeds` | none | `"production"` | `RemediationJob` created |
| `NSAnnotation_EnabledFalse_Suppressed` | `mechanic.io/enabled: "false"` | `"production"` | no `RemediationJob` |
| `NSAnnotation_SkipUntilFuture_Suppressed` | `mechanic.io/skip-until: <future date>` | `"production"` | no `RemediationJob` |
| `NSAnnotation_SkipUntilPast_Proceeds` | `mechanic.io/skip-until: <past date>` | `"production"` | `RemediationJob` created |
| `NSAnnotation_NamespaceNotFound_Proceeds` | (namespace object absent) | `"production"` | `RemediationJob` created |
| `NSAnnotation_ClusterScoped_Exempt` | `mechanic.io/enabled: "false"` on any ns | `""` | `RemediationJob` created (gate bypassed) |

For `NSAnnotation_EnabledFalse_Suppressed` and `NSAnnotation_SkipUntilFuture_Suppressed`:
create a `corev1.Namespace` object in the fake client with the appropriate annotation
before constructing the reconciler. The finding must come from an unhealthy resource
(e.g. a CrashLoopBackOff pod) to ensure the finding would otherwise proceed.

For `NSAnnotation_NamespaceNotFound_Proceeds`: do not create any `Namespace` object in
the fake client. The `Get` will return NotFound; the gate must allow the finding through.

For `NSAnnotation_ClusterScoped_Exempt`: set `finding.Namespace = ""` via a fake
provider that returns a finding with empty namespace (simulating a Node finding).
Even if a `Namespace` object with suppresssion annotation exists, it must not be
consulted.

---

## Dependencies

**Depends on:** STORY_01 (`domain.ShouldSkip` and annotation constants exist in
`internal/domain/annotations.go`).

**Depends on:** epic09-native-provider complete (`SourceProviderReconciler` in
`internal/provider/provider.go`).

**Relates to:** epic15 STORY_02 — the epic15 namespace env-var filter will be inserted
at the same location. These two gates are independent but collocated. If epic15 is
implemented first, this gate goes immediately after the epic15 block.

---

## Definition of Done

- [x] Namespace annotation gate inserted in `Reconcile` at the correct location
- [x] Cluster-scoped findings (`finding.Namespace == ""`) are unconditionally exempt
- [x] `apierrors.IsNotFound` treated as "no annotation" (gate is a no-op)
- [x] Other `Get` errors returned as errors (retried by controller-runtime)
- [x] Suppressed findings logged at `Debug` level with nil-guard on `r.Log`
- [x] All six table-driven tests pass with `-race`
- [x] Full test suite passes: `go test -timeout 120s -race ./...`
- [x] `go vet ./...` clean
- [x] `go build ./...` clean
