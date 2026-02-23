# Worklog 0021 — epic09 Skeptical Reviewer Finding Fixes

**Date:** 2026-02-22
**Session type:** Backlog specification hardening
**Duration:** ~1 session
**Stories touched:** STORY_01–12, README

---

## What was done

Applied all 24 findings from the skeptical reviewer analysis (worklog 0020) to the epic09
backlog files. No production code was written.

### Critical fixes (C1–C8)

| Finding | File | Change |
|---------|------|--------|
| C1 | STORY_01 | Added exact `FindingFingerprint` Go function signature; listed which fields enter the hash vs which are excluded; added byte-for-byte compatibility requirement vs `K8sGPTProvider.Fingerprint` |
| C2 | STORY_02 | Already fixed in prior session (trackingFakeProvider); confirmed present |
| C3 | STORY_03 | Added max traversal depth (10 levels); circular reference guard (UID tracking); clarified returns root not immediate owner; specified error handling (log debug + fallback, no error return); added `CircularOwnerRefs` test case |
| C4 | STORY_06 | Fixed event field selector to include `involvedObject.kind=PersistentVolumeClaim`; added rationale for all three fields; noted fake client field index limitation |
| C5 | (no change) | Finding was incorrect: `NewScheme()` IS in `remediationjob_types.go` (confirmed line 31); STORY_09 was already correct |
| C6 | STORY_10 | Already fixed: `NoAvailableCondition` test case and Kubernetes 1.26 note were already present |
| C7 | STORY_11 | Added "CronJob exclusion: exact mechanism" section specifying check `ownerReferences[i].Kind == "CronJob"` before calling `getParent`; explained why before (not after) traversal |
| C8 | STORY_12 | Added explicit `window == 0` fast path with code snippet; documented single-worker concurrency assumption with comment template; specified `sync.Mutex` upgrade path |

### High fixes (H1–H7)

| Finding | File | Change |
|---------|------|--------|
| H1 | STORY_01 | Test table now shows explicit input fields and field exclusion notes (already covered by C1 fix) |
| H2 | STORY_04 | Added `Finding` construction example in Go code; corrected to 4-arg `getParent` call |
| H3 | STORY_05 | Added note explaining `AvailableConditionFalse` and `ErrorTextIncludesReason` are intentionally separate assertions on the same code path |
| H4 | STORY_07 | Added explicit "conditions only, no taints" preamble; clarified OR logic per condition |
| H5 | STORY_08 | Added "Provider struct architecture" section with Go code example of independent unexported structs and exported constructors; removed ambiguous `NativeProvider` concept |
| H6 | STORY_09 | Specified migration requires creating `internal/provider/suite_test.go` (new) AND `provider_integration_test.go`; noted `TestMain` uniqueness constraint; mentioned updated `fakeSourceProvider` must match slimmed interface |
| H7 | STORY_12 | Covered by C8 fix (window==0 fast path already explicit) |

### Medium fixes (M1–M6)

| Finding | File | Change |
|---------|------|--------|
| M1 | STORY_12 | Task now explicitly says "add rows to the **existing** `internal/config/config_test.go`" |
| M2 | STORY_04–07, 10–11 | Renamed all provider structs to unexported (`podProvider`, `deploymentProvider`, etc.) throughout acceptance criteria; constructors remain exported (`NewPodProvider`, etc.) |
| M3 | STORY_03 | Covered by C3 fix (error handling: log debug + fallback, no return error) |
| M4 | STORY_06 | Covered by C4 fix (explicit event age / Bound-phase ordering) |
| M5 | STORY_08 | Added Makefile/CI note to tasks: existing `TestMain`-skip pattern handles envtest availability; no Makefile changes needed unless a new CI job is added |
| M6 | STORY_05–07, 10–11 | Added explicit `getParent(ctx, p.client, X.ObjectMeta, "Kind")` call examples in `ParentObject` criteria for each provider story |

### Low fixes (L1–L3)

| Finding | File | Change |
|---------|------|--------|
| L1 | STORY_09 | `suite_test.go` added to the deletion task list |
| L2 | README | Replaced ASCII art dependency tree with a dependency table |
| L3 | STORY_09 | Added "Write worklog entry 0021 for epic09 implementation complete" to tasks |

---

## Key decisions confirmed / locked

- `getParent` signature: 4 args, last arg is `kind string`, no error return
- `domain.FindingFingerprint` hashes: `Namespace`, `Kind`, `ParentObject`, sorted error texts — NOT `Name`, `Details`, `SourceRef`
- Provider structs are unexported (`podProvider`, etc.); constructors are exported (`NewPodProvider`)
- Stabilisation window: explicit `window==0` fast path before map lookup; single-worker assumption documented
- CronJob exclusion: check `ownerReferences[i].Kind == "CronJob"` before `getParent`, not after
- PVC event selector: three fields (`name`, `namespace`, `kind`) — not just name
- Integration test migration: needs both `suite_test.go` AND `provider_integration_test.go` in `internal/provider/`

---

## State at end of session

All 24 findings applied. Epic09 backlog is now ready for implementation.
No production code written. All changes are documentation only.
