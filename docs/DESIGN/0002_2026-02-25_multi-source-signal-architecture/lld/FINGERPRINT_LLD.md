# Fingerprint Redesign — Low-Level Design

**Version:** 1.1
**Date:** 2026-02-25
**Status:** Proposed
**HLD Reference:** [§6, §7](../HLD.md)

---

## 0. Prerequisite: Extend `domain.Finding`

Before any fingerprint or adapter code is written, `internal/domain/provider.go` must be
extended with v2 fields. This is the **first task** in any implementation plan — every other
v2 component depends on it.

```go
// internal/domain/provider.go

type Finding struct {
    // ... existing fields unchanged ...

    // AlertName is the name of the alert (e.g. "KubeDeploymentReplicasMismatch").
    // Empty for native-sourced findings.
    AlertName string

    // AlertLabels is the complete raw label set from the originating alert.
    // Empty for native-sourced findings.
    AlertLabels map[string]string

    // SourceType identifies which provider/adapter created this finding.
    // "native" for all native providers. The adapter's TypeName() for external sources.
    SourceType string

    // SourceCRName is the name of the AlertSource CR that generated this finding.
    // Used for status counter attribution so counters are correctly mapped back
    // to the AlertSource CR in flushCountersForSource.
    // Empty for native-sourced findings.
    SourceCRName string

    // SourcePriority is the deduplication priority of the source.
    // Native default: value of NATIVE_PROVIDER_PRIORITY env var (default 10).
    // External: AlertSource.Spec.Priority.
    SourcePriority int

    // SkipStabilisation, when true, bypasses the stabilisation window.
    // Set to true by all external alert source adapters; false for native providers.
    SkipStabilisation bool

    // PreviousPRURL is the GitHub PR URL from a prior RemediationJob for the same
    // resource. Set by AlertSourceReconciler.handlePendingAlert when the prior RJ
    // succeeded. Empty when not applicable.
    PreviousPRURL string
}
```

**Note on `SourceType` for native providers:** The existing `domain.SourceProvider.ProviderName()`
returns `"native"` for all native providers. The `Finding.SourceType` field should be populated
with this value by `SourceProviderReconciler` when building the RJ, the same way
`rjob.Spec.SourceType = r.Provider.ProviderName()` already works.

---

## 1. Overview

The v1 fingerprint encodes `{namespace, kind, parentObject, sorted(errorTexts)}`. This
ties deduplication to a specific observed error state, which causes two problems in v2:

1. **Cross-source collision is impossible.** The native Deployment provider produces
   `"replica mismatch: desired=3 ready=1"` while Alertmanager produces
   `"KubeDeploymentReplicasMismatch: deployment=foo"`. These are the same problem described
   differently — they will never share a fingerprint.

2. **Dynamic error states create spurious RJs.** As a deployment recovers from `0/3` to
   `1/3` to `2/3` ready, the error text changes and v1 would create a new `RemediationJob`
   at each step if the previous one completed in the interim.

The v2 fingerprint encodes only the resource identity: `{namespace, kind, parentObject}`.
Error texts are stored in the `RemediationJob` spec and surfaced to the agent but are not
part of the deduplication key.

---

## 2. Algorithm

### 2.1 Resource Fingerprint (v2)

```go
// internal/domain/provider.go

// FindingFingerprint computes the resource-level deduplication key for a Finding.
//
// Algorithm:
//  1. Build a payload struct: {Namespace, Kind, ParentObject}
//  2. JSON-encode with SetEscapeHTML(false)
//  3. Return lowercase hex SHA256 of the encoded bytes (always 64 chars)
//
// The fingerprint identifies the affected resource, not the specific error state.
// Error texts are stored in RemediationJob.Spec.Finding.Errors but are not hashed.
func FindingFingerprint(f *Finding) (string, error) {
    payload := struct {
        Namespace    string `json:"namespace"`
        Kind         string `json:"kind"`
        ParentObject string `json:"parentObject"`
    }{
        Namespace:    f.Namespace,
        Kind:         f.Kind,
        ParentObject: f.ParentObject,
    }

    var buf bytes.Buffer
    enc := json.NewEncoder(&buf)
    enc.SetEscapeHTML(false)
    if err := enc.Encode(payload); err != nil {
        return "", fmt.Errorf("FindingFingerprint: encode: %w", err)
    }

    sum := sha256.Sum256(buf.Bytes())
    return fmt.Sprintf("%x", sum), nil
}
```

### 2.2 Properties

| Property | v1 | v2 |
|---|---|---|
| Input fields | namespace + kind + parentObject + sorted(errorTexts) | namespace + kind + parentObject |
| Stability across error text changes | No — new fingerprint for each error state | Yes — same resource = same fingerprint |
| Cross-source deduplication | Not possible | Natural — same resource from any source = same fingerprint |
| Fingerprint length | 64 hex chars (SHA256) | 64 hex chars (SHA256) |
| First 12 chars used as label | Yes | Yes (unchanged) |
| Minimum length guard (≥12) | Yes (`provider.go:166`) | Yes (unchanged) |

### 2.3 ParentObject Normalisation

`ParentObject` is set by providers as `"Kind/name"` (e.g. `"Deployment/my-app"`). For
alert-sourced findings, the adapter's `resolveResource` helper produces the same canonical
form. This ensures fingerprints match across sources for the same resource.

**Examples of equivalent fingerprints across sources:**

| Source | Kind | Namespace | ParentObject | Fingerprint |
|---|---|---|---|---|
| Native Deployment provider | Deployment | default | Deployment/test-broken-image | `sha256(...)` |
| Alertmanager (KubeDeploymentReplicasMismatch) | Deployment | default | Deployment/test-broken-image | same `sha256(...)` |

| Source | Kind | Namespace | ParentObject | Fingerprint |
|---|---|---|---|---|
| Native Pod provider | Pod | media | Deployment/subgen-worker | `sha256(...)` |
| Alertmanager (OomKilled) | Pod | media | Deployment/subgen-worker | same `sha256(...)` |

---

## 3. RemediationJob Labels and Annotations

### 3.1 Labels (indexed, for fast K8s queries)

```
mechanic.io/resource-fingerprint:   <fp[:12]>     # NEW: resource-level, cross-source
mechanic.io/source-priority:        "90"           # NEW: cross-source priority comparison
remediation.mechanic.io/fingerprint: <fp[:12]>    # RETAINED: backward compat
```

### 3.2 Annotations

```
mechanic.io/resource-fingerprint-full: <64-char sha256>   # NEW: exact resource match
mechanic.io/source-type:               "alertmanager"      # NEW: human-readable source
mechanic.io/error-summary:             "KubeDeployment..."  # NEW: quick human inspection
mechanic.io/pending-alert:             <JSON Finding>       # NEW: see PENDING ALERT LLD
```

### 3.3 RemediationJob Name

Unchanged: `"mechanic-" + fp[:12]`

With the v2 fingerprint, the name is now stable for a given resource (same resource =
same first 12 chars of fingerprint = same RJ name suffix). This is an improvement: a
`RemediationJob` for `Deployment/my-app` in `default` will always be named
`mechanic-<consistent-12-chars>` regardless of source or error text.

---

## 4. Deduplication Query

### 4.1 Dual-fingerprint query strategy

Both the v1 (`Spec.Fingerprint`) and v2 (`Spec.ResourceFingerprint`) fields are relevant
during the migration window. The dedup query must handle three cases:

1. **New RJ created by v2** — has both `mechanic.io/resource-fingerprint` label and
   `mechanic.io/resource-fingerprint-full` annotation, and `Spec.ResourceFingerprint`.
2. **Old RJ created by v1** — has only `remediation.mechanic.io/fingerprint` label and
   `Spec.Fingerprint` (includes error texts).

The dedup check queries on `mechanic.io/resource-fingerprint` (new label) rather than
the v1 `remediation.mechanic.io/fingerprint` label. Both labels are set on every RJ during
the transition period.

**IMPORTANT — `SourceProviderReconciler` (`internal/provider/provider.go`) must also be
updated as part of this change.** The current dedup query at `provider.go:252-261` uses only
`remediation.mechanic.io/fingerprint`. After v2, it must be updated to:

1. Use `mechanic.io/resource-fingerprint` label for the primary query (v2 dedup).
2. Include `Spec.ResourceFingerprint` and all new labels/annotations when building RJs
   at `provider.go:292-327`. Specifically add:
   - Label `mechanic.io/resource-fingerprint`: `rfp[:12]`
   - Label `mechanic.io/source-priority`: `strconv.Itoa(cfg.NativeProviderPriority)`
   - Annotation `mechanic.io/resource-fingerprint-full`: `rfp`
   - `Spec.ResourceFingerprint`: `rfp`
3. Retain the existing `remediation.mechanic.io/fingerprint` label and `Spec.Fingerprint`
   set to the v1 fingerprint value (via `FindingFingerprintV1`) for backward compat.

Without these changes, native-sourced RJs will not carry the v2 labels and the
`AlertSourceReconciler.resolveDedup` function will be unable to find active native RJs
when checking for cross-source collisions — defeating the entire purpose of v2 dedup.

```go
// internal/provider/provider.go (updated dedup logic)

// Step 1: compute resource fingerprint (v2 — no error texts)
rfp, err := domain.FindingFingerprint(finding)

// Step 2: query by resource fingerprint label (new)
var rjList v1alpha1.RemediationJobList
if err := r.List(ctx, &rjList,
    client.InNamespace(r.Cfg.AgentNamespace),
    client.MatchingLabels{"mechanic.io/resource-fingerprint": rfp[:12]},
); err != nil {
    return ctrl.Result{}, err
}

// Step 3: exact match on full resource fingerprint annotation
for i := range rjList.Items {
    rj := &rjList.Items[i]
    fullRFP := rj.Annotations["mechanic.io/resource-fingerprint-full"]
    if fullRFP != rfp {
        continue
    }

    // Active v2 RJ found for this resource. Dispatch depends on its phase.
    // NOTE: SourceProviderReconciler has no cross-source priority concept.
    // A native provider always produces exactly one RJ per resource at a time.
    switch rj.Status.Phase {
    case v1alpha1.PhaseFailed:
        // Delete the failed RJ so a fresh investigation can start from the
        // current error state. Ignore NotFound (already deleted concurrently).
        if delErr := r.Delete(ctx, rj); delErr != nil && !apierrors.IsNotFound(delErr) {
            return ctrl.Result{}, delErr
        }
        // Fall through to create a new RJ below.

    case v1alpha1.PhaseSucceeded:
        // Prior investigation completed. The resource is still or again unhealthy.
        // Delete the succeeded RJ before creating a new one. With v2 resource-level
        // fingerprinting, the replacement RJ has the SAME name ("mechanic-" + rfp[:12]).
        // If the old RJ is not deleted first, r.Create returns AlreadyExists and the
        // new investigation is silently never started. In v1, error texts were part of
        // the fingerprint, so a new error state produced a different name and this was
        // not a problem. v2 resource fingerprints are stable — deletion is required.
        if delErr := r.Delete(ctx, rj); delErr != nil && !apierrors.IsNotFound(delErr) {
            return ctrl.Result{}, delErr
        }
        // Fall through to create a new RJ below.
        // (No PreviousPRURL injection on the native path — that is only used in the
        // alert-source two-PR flow via handlePendingAlert.)

    default:
        // Pending, Dispatched, Running, Suppressed, Cancelled.
        // An active investigation already exists — suppress the incoming finding
        // and requeue so the native provider re-evaluates after the current RJ
        // reaches a terminal state.
        return ctrl.Result{}, nil
    }
}

// Step 4: FALLBACK — check v1 fingerprint for RJs created before v2 rollout.
// This prevents creating a duplicate RJ for a resource that already has an
// active v1 RJ (the v1 Spec.Fingerprint exact-match check passes for v1 RJs).
//
// IMPORTANT: the v1 dedup check at provider.go:260 is:
//   rjob.Spec.Fingerprint != fp
// where fp is the v1 fingerprint (includes error texts). After the v2 change,
// FindingFingerprint returns the RESOURCE fingerprint (no error texts), so
// the v1 check would NEVER match a v1-created RJ. This fallback prevents
// creating a second concurrent investigation for a resource that already has
// an active v1 RJ.
//
// Implementation: compute the v1 fingerprint separately for fallback only:
v1fp, _ := domain.FindingFingerprintV1(finding) // legacy function retained during migration
var v1RJList v1alpha1.RemediationJobList
if err := r.List(ctx, &v1RJList,
    client.InNamespace(r.Cfg.AgentNamespace),
    client.MatchingLabels{"remediation.mechanic.io/fingerprint": v1fp[:12]},
); err != nil {
    // Treat a List error as "no v1 RJs found" — a transient API error here
    // is not worth blocking the finding. Worst case: we create a duplicate RJ
    // alongside an active v1 RJ during the migration window, which resolves
    // naturally when the v1 TTL expires.
    r.Log.Warn("v1 fallback dedup List failed; proceeding without v1 check", zap.Error(err))
}
for _, rj := range v1RJList.Items {
    if rj.Spec.Fingerprint == v1fp {
        if rj.Status.Phase != v1alpha1.PhaseFailed && rj.Status.Phase != v1alpha1.PhaseSucceeded {
            // V1 RJ is still active — suppress incoming finding
            return ctrl.Result{}, nil
        }
    }
}

// Step 5: No active RJ found (neither v2 nor v1) → proceed to create
```

### 4.2 Legacy `FindingFingerprintV1` function

The existing `FindingFingerprint` function in `internal/domain/provider.go` (which includes
error texts) is **renamed** to `FindingFingerprintV1` and kept alongside the new
`FindingFingerprint` (resource-only). Both are retained until all v1-created RJs have
reached terminal state and their 7-day TTL has expired.

```go
// FindingFingerprintV1 is the v1 algorithm: includes namespace + kind + parentObject
// + sorted(errorTexts). Retained for migration fallback dedup only.
// Do not use for new RJ creation in v2.
func FindingFingerprintV1(f *Finding) (string, error) { ... }

// FindingFingerprint is the v2 algorithm: namespace + kind + parentObject only.
// Use this for all v2 RJ creation and cross-source deduplication.
func FindingFingerprint(f *Finding) (string, error) { ... }
```

All existing callers of `FindingFingerprint` that are setting `Spec.Fingerprint` on RJs
must be updated to also set `Spec.ResourceFingerprint` with the v2 result. The
`Spec.Fingerprint` field continues to be set with the **v1** fingerprint value during the
migration period so that any existing monitoring, tooling, or tests keyed on it continue to
work unchanged.

---

## 5. Previous PR URL Propagation

When creating a new RJ for a resource that has a Succeeded prior RJ:

```go
// internal/provider/provider.go

func (r *SourceProviderReconciler) buildRemediationJob(
    finding *domain.Finding,
    fp string,
    previousPRURL string,
) *v1alpha1.RemediationJob {
    rj := &v1alpha1.RemediationJob{ ... }

    if previousPRURL != "" {
        rj.Spec.Finding.PreviousPRURL = previousPRURL
    }

    return rj
}
```

`PreviousPRURL` is then injected into the agent Job by `jobbuilder` as
`FINDING_PREVIOUS_PR_URL`. See `AGENT_CONTEXT_LLD.md` for prompt handling.

---

## 6. Migration from v1

The v1 fingerprint field (`spec.fingerprint` and `remediation.mechanic.io/fingerprint`
label) is **retained unchanged** during migration. Existing `RemediationJob` objects
created by v1 will not have the new `mechanic.io/resource-fingerprint` label or annotation.

**Migration strategy:**

1. `FindingFingerprint` (error-text-inclusive) is **renamed** to `FindingFingerprintV1` and
   retained. A new `FindingFingerprint` function (resource-only) is added alongside it.

2. For every new RJ created by v2, set **both**:
   - `Spec.Fingerprint` = v1 fingerprint (for backward compat with existing tooling/tests)
   - `Spec.ResourceFingerprint` = v2 resource fingerprint (for cross-source dedup)
   - Label `remediation.mechanic.io/fingerprint` = `Spec.Fingerprint[:12]` (v1 label, retained)
   - Label `mechanic.io/resource-fingerprint` = `Spec.ResourceFingerprint[:12]` (v2 label)
   - Annotation `mechanic.io/resource-fingerprint-full` = `Spec.ResourceFingerprint`

3. The v2 dedup query checks v2 labels first (§4.1). If no v2 RJ is found, it falls back to
   a v1 label query using `FindingFingerprintV1`. This prevents creating a duplicate RJ for
   a resource that has an active v1 RJ.

4. **Critically:** the v1 dedup check in `provider.go:260` is:
   ```go
   rjob.Spec.Fingerprint != fp
   ```
   where `fp` was previously the v1 fingerprint. After renaming `FindingFingerprintV1`, the
   native provider reconciler must be updated to compute and compare the v1 fingerprint for
   this legacy exact-match check, and to additionally set `Spec.ResourceFingerprint` on new
   RJs using the v2 fingerprint. Both values must be computed and stored — not just one.

5. After all v1-created RJs have reached a terminal state and their 7-day TTL has expired,
   the v1 fallback query (step 3), `FindingFingerprintV1`, and the `Spec.Fingerprint` field
   can be removed in a subsequent release.

**No data migration job is required.** Old RJs expire naturally via the 7-day TTL.

---

## 7. Test Cases

### Unit tests — `domain.FindingFingerprint`

| Test | Expected behaviour |
|---|---|
| Same namespace + kind + parentObject → same fingerprint | Deterministic |
| Different error texts, same resource → same fingerprint | Error texts excluded |
| Different namespace, same kind + parentObject → different fingerprint | Namespace included |
| Different kind, same namespace + parentObject → different fingerprint | Kind included |
| Different parentObject, same namespace + kind → different fingerprint | ParentObject included |
| Empty namespace → valid fingerprint (no panic) | Edge case |
| Characters `<`, `>`, `&` in parentObject → same fingerprint regardless of escaping | SetEscapeHTML(false) guard |
| Output is always exactly 64 hex characters | SHA256 length |

### Cross-source equivalence test

```go
// TestFingerprintCrossSourceEquivalence
//
// For the same underlying resource (Deployment/test-broken-image in default):
// - Compute fingerprint from a native Finding (errors = replica mismatch text)
// - Compute fingerprint from an alert Finding (errors = KubeDeploymentReplicasMismatch text)
// - Both must produce the same fingerprint because only resource identity is hashed.
func TestFingerprintCrossSourceEquivalence(t *testing.T) {
    nativeFinding := &domain.Finding{
        Kind: "Deployment", Namespace: "default",
        ParentObject: "Deployment/test-broken-image",
        Errors: `[{"text":"replica mismatch: desired=3 ready=1"}]`,
    }
    alertFinding := &domain.Finding{
        Kind: "Deployment", Namespace: "default",
        ParentObject: "Deployment/test-broken-image",
        Errors: `[{"text":"KubeDeploymentReplicasMismatch: deployment=test-broken-image"}]`,
        AlertName: "KubeDeploymentReplicasMismatch",
    }
    fpNative, _ := domain.FindingFingerprint(nativeFinding)
    fpAlert, _ := domain.FindingFingerprint(alertFinding)
    assert.Equal(t, fpNative, fpAlert)
}
```

### Regression test — v1 error-text sensitivity

```go
// TestFingerprintV1Regression
//
// Confirms that the v2 fingerprint does NOT change when error texts change
// for the same resource. This is the key behavioural difference from v1.
func TestFingerprintV1Regression(t *testing.T) {
    base := &domain.Finding{
        Kind: "Deployment", Namespace: "default",
        ParentObject: "Deployment/my-app",
        Errors: `[{"text":"replica mismatch: desired=3 ready=0"}]`,
    }
    recovering := &domain.Finding{
        Kind: "Deployment", Namespace: "default",
        ParentObject: "Deployment/my-app",
        Errors: `[{"text":"replica mismatch: desired=3 ready=2"}]`,
    }
    fp1, _ := domain.FindingFingerprint(base)
    fp2, _ := domain.FindingFingerprint(recovering)
    assert.Equal(t, fp1, fp2, "error text changes must not change the fingerprint")
}
```
