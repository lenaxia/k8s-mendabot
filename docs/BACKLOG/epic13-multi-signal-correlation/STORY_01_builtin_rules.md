# Story 01: Built-in Correlation Rules

**Epic:** [epic13-multi-signal-correlation](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot operator**, I want the three built-in correlation rules
(`SameNamespaceParentRule`, `PVCPodRule`, `MultiPodSameNodeRule`) implemented and tested,
so that the most common multi-signal root causes are automatically grouped into a single
investigation.

---

## Background

The three rules cover the majority of real-world correlation scenarios:

1. Multiple findings for the same application (same parent, same namespace)
2. A PVC failure and the pod that depends on it
3. Multiple pods failing because their shared node is degraded

Each rule implements the `domain.CorrelationRule` interface from STORY_00. Rules are
stateless — all state lives in the `RemediationJob` objects passed in.

---

## Acceptance Criteria

- [ ] `internal/correlator/rules.go` exists with all three rules implementing `domain.CorrelationRule`
- [ ] `SameNamespaceParentRule.Evaluate` returns `Matched=true` when two `RemediationJob`
      objects share a namespace and one's `parentObject` is a prefix of the other's
- [ ] `PVCPodRule.Evaluate` returns `Matched=true` when a PVC finding and a pod finding
      share a namespace and the pod's volumes reference the PVC (requires one `client.Get`).
      The rule must handle both orientations: candidate=Pod (PVC in peers) and
      candidate=PVC (Pod in peers). The PVC is always the primary.
- [ ] `MultiPodSameNodeRule.Evaluate` returns `Matched=true` when `>= threshold` pod
      findings ran on the same node
- [ ] `internal/correlator/rules_test.go` covers:
  - Happy path for each rule
  - No-match cases (different namespace, different parent prefix, pod count below threshold)
  - `PVCPodRule` with no matching volume reference
  - `PVCPodRule` with candidate=Pod and PVC in peers (forward orientation)
  - `PVCPodRule` with candidate=PVC and Pod in peers (reverse orientation) — both must match
  - `MultiPodSameNodeRule` at exactly threshold - 1 (no match) and threshold (match)
- [ ] `go test -timeout 30s -race ./internal/correlator/...` passes

---

## Technical Implementation

### Package location

`internal/correlator/` — separate from `internal/domain/` to keep domain types free of
rule logic. The correlator package imports domain; domain does not import correlator.

### `SameNamespaceParentRule`

```go
type SameNamespaceParentRule struct{}

func (r SameNamespaceParentRule) Name() string { return "SameNamespaceParent" }

func (r SameNamespaceParentRule) Evaluate(
    ctx context.Context,
    candidate *v1alpha1.RemediationJob,
    peers []*v1alpha1.RemediationJob,
    c client.Client,
) (domain.CorrelationResult, error) {
    cNS := candidate.Spec.Finding.Namespace
    cParent := candidate.Spec.Finding.ParentObject

    var matched []*v1alpha1.RemediationJob
    for _, p := range peers {
        if p.UID == candidate.UID {
            continue
        }
        if p.Spec.Finding.Namespace != cNS {
            continue
        }
        pParent := p.Spec.Finding.ParentObject
        if strings.HasPrefix(cParent, pParent) || strings.HasPrefix(pParent, cParent) {
            matched = append(matched, p)
        }
    }
    if len(matched) == 0 {
        return domain.CorrelationResult{}, nil
    }
    primary := selectPrimary(candidate, matched)
    return domain.CorrelationResult{
        Matched:    true,
        GroupID:    domain.NewCorrelationGroupID(),
        PrimaryUID: primary.UID,
        Reason:     "same-namespace-parent-prefix",
    }, nil
}
```

`selectPrimary` picks the `RemediationJob` whose finding `Kind` is highest in the
ownership hierarchy (Deployment > StatefulSet > Pod > others). On a tie, the oldest
`CreationTimestamp` wins.

**Important — rule applicability:** `ParentObject` is computed by `getParent()` in the
native providers (`internal/provider/native/`), which walks owner references to find the
top-level owning resource. For a Pod owned by a ReplicaSet owned by a Deployment, both
the Deployment finding and the Pod finding will store `ParentObject = "<deployment-name>"`.
Because they share the same parent name, they also share the same fingerprint and are
deduplicated by `SourceProviderReconciler` before two `RemediationJob` objects are ever
created — meaning the correlator never sees them together.

The `SameNamespaceParentRule` is therefore most useful for cross-provider scenarios where
the same application surfaces findings from two different providers (e.g. a `StatefulSet`
finding from one provider and a `PVC` finding from another, both with the same
`ParentObject`). Note: in single-provider deployments, same-provider findings for the same
parent are fingerprint-deduplicated before correlation runs — this rule fires primarily in
multi-provider deployments. Write tests that reflect this — do not write tests using Pod + Deployment
from the same native provider, as those will be deduplicated before reaching the correlator.

### `PVCPodRule`

Requires reading the Pod object from the API to inspect `spec.volumes`. The rule
receives a `client.Client` for this purpose. If the `client.Get` call fails (pod gone),
the rule returns `Matched=false, nil` — a non-fatal miss.

```go
type PVCPodRule struct{}

func (r PVCPodRule) Name() string { return "PVCPod" }
```

Logic:
1. Determine the roles: if `candidate.Spec.Finding.Kind == "PersistentVolumeClaim"`, the
   candidate is the PVC and peers are searched for Pod findings. If
   `candidate.Spec.Finding.Kind == "Pod"`, peers are searched for PVC findings. Handle
   both orientations so the rule fires regardless of which job reaches the correlator first.
2. **candidate = Pod orientation:**
   Filter `peers` for findings where `peer.Spec.Finding.Kind == "PersistentVolumeClaim"`
   and `peer.Spec.Finding.Namespace == candidate.Spec.Finding.Namespace`.
   For each PVC peer, call `client.Get` to fetch the live Pod object using
   `candidate.Spec.Finding.Namespace` + `candidate.Spec.Finding.Name` as the key.
   Iterate `pod.Spec.Volumes`: if any volume has a `PersistentVolumeClaimVolumeSource`
   whose `ClaimName` equals the PVC peer's `Spec.Finding.Name`, the rule matches.
   Primary is the PVC finding (the PVC peer).
3. **candidate = PVC orientation:**
   Filter `peers` for findings where `peer.Spec.Finding.Kind == "Pod"`
   and `peer.Spec.Finding.Namespace == candidate.Spec.Finding.Namespace`.
   For each Pod peer, call `client.Get` to fetch the live Pod object using
   `peer.Spec.Finding.Namespace` + `peer.Spec.Finding.Name` as the key.
   Iterate `pod.Spec.Volumes`: if any volume has a `PersistentVolumeClaimVolumeSource`
   whose `ClaimName` equals `candidate.Spec.Finding.Name` (the candidate PVC), the rule
   matches. Primary is the candidate (the PVC finding).
4. If `client.Get` fails (pod gone), skip that peer — non-fatal miss.
5. If neither orientation finds a match: `Matched=false, nil`.

### `MultiPodSameNodeRule`

```go
type MultiPodSameNodeRule struct {
    Threshold int // default 3; set from config
}

func (r MultiPodSameNodeRule) Name() string { return "MultiPodSameNode" }
```

Logic:
1. Collect all pod findings (Kind == "Pod") across candidate + peers
2. Group by the `nodeName` annotation (`mendabot.io/node-name`) set on the `RemediationJob`
3. If any node has >= threshold pod findings: `Matched=true`

**Known limitation — pending/unschedulable pods:** `spec.nodeName` is only populated for
pods that have been *scheduled* to a node. Pods in `Pending/Unschedulable` state (e.g.
waiting for PVC, waiting for resources) have an empty `spec.nodeName`. The
`MultiPodSameNodeRule` will not fire for these pods. It only correlates pods that were
*running* on a node and are now crashing (e.g. after a node enters `NotReady` mid-run).
This limitation should be documented in the rule's `Name()` docstring and acknowledged
in tests.

**Note on nodeName:** `PodProvider.ExtractFinding` must be updated (in this story) to
populate `Finding.NodeName` from `pod.Spec.NodeName`. The `SourceProviderReconciler`
writes this into `RemediationJob` annotations as `mendabot.io/node-name` only when the
value is non-empty. Pods in `Pending` state will have no annotation and will be excluded
from this rule's grouping.

---

## Tasks

- [ ] Write `internal/correlator/rules_test.go` with table-driven tests for all three rules (TDD).
      **Note on `SameNamespaceParentRule` test cases:** use two `RemediationJob` objects from
      *different providers* (e.g. a `StatefulSet` finding and a `PVC` finding with the same
      `ParentObject`). Do not use Pod + Deployment from the same native provider — those would
      share a fingerprint and be deduplicated before reaching the correlator.
- [ ] Add `NodeName string` to `domain.Finding` in `internal/domain/provider.go`
- [ ] Update `PodProvider.ExtractFinding` in `internal/provider/native/pod.go` to populate
      `NodeName` from `pod.Spec.NodeName` (empty for unscheduled/pending pods — that is correct)
- [ ] Update `SourceProviderReconciler` to write the `mendabot.io/node-name` annotation on the
      `RemediationJob` when `finding.NodeName != ""`. The exact location is
      `internal/provider/provider.go` in the `RemediationJob` construction block (around line 266),
      in the `Annotations` map. Add:
      ```go
      if finding.NodeName != "" {
          annotations["mendabot.io/node-name"] = finding.NodeName
      }
      ```
- [ ] Implement `internal/correlator/rules.go` with all three rules
- [ ] Run `go test -timeout 30s -race ./...` — must pass

---

## Dependencies

**Depends on:** STORY_00 (`domain.CorrelationRule` interface, `domain.CorrelationResult`)
**Blocks:** STORY_02 (correlator needs rules to apply)

---

## Definition of Done

- [ ] All three rules compile and pass their unit tests
- [ ] `PodProvider` populates `NodeName` and the reconciler writes the annotation
- [ ] No existing tests broken
