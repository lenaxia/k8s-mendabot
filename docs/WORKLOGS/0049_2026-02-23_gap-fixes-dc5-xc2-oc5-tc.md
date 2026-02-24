# Worklog: Gap Fixes — DC-5, XC-2, OC-5, TC-1b, TC-2a, TC-3a, TC-6

**Date:** 2026-02-23
**Session:** Delegation agent fixing 7 gaps identified in code review
**Status:** Complete

---

## Objective

Fix all gaps assigned by the orchestrator: DC-5 (major), XC-2 (major), OC-5 (minor),
TC-1b (minor), TC-2a (minor), TC-3a (minor), TC-6 (minor). IC-6 is documentation-only
and requires no code change (see Key Decisions).

---

## Work Completed

### 1. DC-5 — `AllFindings` included unmatched peers from other correlation groups

**Root cause:** `CorrelationResult` had no `MatchedUIDs` field, so the correlator had no
way to distinguish which peers were part of a match vs. which happened to be in the peer
list. The correlator assembled `AllFindings` from candidate + ALL peers indiscriminately.

**Fix:**

- `internal/domain/correlation.go`: Added `MatchedUIDs []types.UID` field to
  `CorrelationResult` with a doc comment explaining its contract.

- `internal/correlator/rules.go`:
  - `SameNamespaceParentRule.Evaluate`: Populates `MatchedUIDs` with candidate UID +
    all matched peer UIDs.
  - `PVCPodRule.evaluatePodCandidate`: Populates `MatchedUIDs` with `{candidate.UID, pvcPeer.UID}`.
  - `PVCPodRule.evaluatePVCCandidate`: Populates `MatchedUIDs` with `{candidate.UID, p.UID}`.
  - `MultiPodSameNodeRule.Evaluate`: Populates `MatchedUIDs` with all `nodePods` UIDs
    (exactly those on the matching node that met the threshold).

- `internal/correlator/correlator.go`: Changed `AllFindings` assembly to filter by
  `MatchedUIDs` when the field is non-empty. Falls back to candidate+all-peers when
  `MatchedUIDs` is nil (backward-compatible with stub rules in tests that don't populate
  the field). The candidate is only included if its UID appears in `matchedSet`.

### 2. XC-2 — Testdata CRD missing `enum` on `phase` and `x-kubernetes-preserve-unknown-fields` on `conditions.items`

**Root cause:** `testdata/crds/remediationjob_crd.yaml` diverged from the production CRD
at `deploy/kustomize/crd-remediationjob.yaml`. The envtest API server enforces CRD schema
strictly — missing `enum` means invalid phase values could be stored silently; missing
`x-kubernetes-preserve-unknown-fields` on `conditions.items` means condition fields
(unknown to the schema) get stripped during status patches.

**Fix:** `testdata/crds/remediationjob_crd.yaml`:
- Changed `phase: {type: string}` to multi-line with `enum: [Pending, Dispatched,
  Running, Succeeded, Failed, Cancelled, Suppressed]`.
- Added `x-kubernetes-preserve-unknown-fields: true` to the `conditions.items` object.
- No other changes made.

### 3. OC-5 — `Status.CorrelationGroupID` not set on PRIMARY jobs

**Root cause:** The primary dispatch path in the controller only patched labels (metadata),
never patching `Status.CorrelationGroupID`. The suppressed path correctly set this field
via `transitionSuppressed`, but the primary path did not.

**Fix:** `internal/controller/remediationjob_controller.go` primary dispatch path
(around line 113): Added a status patch that sets `rjob.Status.CorrelationGroupID =
group.GroupID` before patching labels and calling `dispatch()`. The status patch uses a
separate `DeepCopy` base since status and metadata are different subresources.

**Test:** Added assertion to `TestCorrelationWindow_PrimaryIsDispatched` that verifies
`updated.Status.CorrelationGroupID != ""` after dispatch.

### 4. TC-1b — Missing test: multiple nodes where ONE meets threshold, others do not

Added `TestMultiPodSameNodeRule_MultipleNodes_OnlyThresholdNodeMatches` to
`internal/correlator/rules_test.go`:
- 3 pods on node-abc (threshold=3 → matches), 2 pods on node-def (below threshold).
- Asserts `result.Matched == true`, `result.PrimaryUID` is one of the node-abc pods,
  `result.MatchedUIDs` contains exactly the 3 node-abc pod UIDs (not node-def pods).

### 5. TC-2a — `TestCorrelationWindow_SecondaryIsSuppressed` missing label assertion

Added assertion to `internal/controller/remediationjob_controller_test.go` in
`TestCorrelationWindow_SecondaryIsSuppressed` that verifies
`updated.Labels[domain.CorrelationGroupIDLabel]` is present after suppression.

### 6. TC-3a — `TestCorrelationWindow_PrimaryIsDispatched` missing primary finding assertion

Added assertion to `TestCorrelationWindow_PrimaryIsDispatched` that verifies
`primary.Spec.Finding.Name` (`"pod-primary"`) is present in `CorrelatedFindings`.
This ensures the primary's own finding is always included alongside peer findings.

### 7. TC-6 — `TestCorrelationIntegration_TC02b_SecondaryIsSuppressed` missing label assertion

Added assertion to `internal/controller/correlation_integration_test.go` in
`TestCorrelationIntegration_TC02b_SecondaryIsSuppressed` that verifies
`updated2.Labels[domain.CorrelationGroupIDLabel]` is non-empty after suppression.

### 8. IC-6 — Worklog 0047 contains incorrect documentation (documentation-only gap)

Worklog 0047 incorrectly describes `pendingPeers()` behaviour. Per the append-only
worklog discipline rule, worklog 0047 is not modified. The correction is noted here:
`pendingPeers()` in `internal/controller/remediationjob_controller.go` correctly
includes only jobs where `Phase == PhasePending`. Jobs with `Phase == ""` are NOT
included — that was the original undocumented behaviour but was removed when the
controller was updated to always initialise new jobs to `PhasePending` on first reconcile
(added in session 0047 itself). The code is correct; only the documentation was wrong.

### 9. Existing rule tests updated with MatchedUIDs assertions

Added `MatchedUIDs` correctness assertions to:
- `TestSameNamespaceParentRule_Match` — verifies both candidate and peer UIDs present
- `TestPVCPodRule_CandidatePod_PeerPVC_Match` — verifies both UIDs present
- `TestPVCPodRule_CandidatePVC_PeerPod_Match` — verifies both UIDs present
- `TestMultiPodSameNodeRule_AtThreshold_Match` — verifies all 3 matched pod UIDs present

---

## Key Decisions

| Decision | Rationale |
|---|---|
| Fall back to candidate+all-peers when `MatchedUIDs` is nil | Maintains backward-compat with stub rules in unit tests that return `MatchedUIDs: nil`. Production rules always populate it; stubs are test-only. |
| Candidate only included in AllFindings if its UID is in matchedSet | Per DC-5 spec: "candidate always included" only applies when MatchedUIDs is empty (fallback). When MatchedUIDs is set, the candidate's presence is determined by whether its UID appears in the matched set — this handles both primary and secondary candidate roles correctly. |
| Separate status patch before label patch for OC-5 | Status subresource and metadata are separate API resources. A single MergeFrom patch cannot update both; two patches are required. Status is patched first so the field is visible before labels are updated. |
| Do not modify worklog 0047 | Worklogs are append-only history per README-LLM.md rule 7. The correction is recorded here instead. |

---

## Blockers

None.

---

## Tests Run

```
go build ./...                                           → clean
go vet ./...                                             → clean
go test -count=1 -timeout 90s -race ./...               → 17/17 PASS
go test -count=3 -timeout 300s -race ./internal/controller/... → PASS (33s)
```

---

## Next Steps

No immediate next steps from this session. All 7 assigned gaps are fixed. IC-6 is
documentation-only and corrected in this worklog.

---

## Files Modified

- `internal/domain/correlation.go` — Added `MatchedUIDs []types.UID` to `CorrelationResult`
- `internal/correlator/rules.go` — Populated `MatchedUIDs` in all three rules
- `internal/correlator/correlator.go` — `AllFindings` now filters by `MatchedUIDs` when set
- `internal/correlator/rules_test.go` — Added TC-1b test; added `MatchedUIDs` assertions to 4 existing match tests
- `internal/correlator/correlator_test.go` — Added `TestCorrelator_MultiPodSameNodeRule_AllFindingsOnlyMatchedNode`
- `internal/controller/remediationjob_controller.go` — Added status patch for `CorrelationGroupID` on primary
- `internal/controller/remediationjob_controller_test.go` — Added OC-5, TC-2a, TC-3a assertions
- `internal/controller/correlation_integration_test.go` — Added TC-6 label assertion
- `testdata/crds/remediationjob_crd.yaml` — Added `enum` on `phase`, `x-kubernetes-preserve-unknown-fields` on `conditions.items`
