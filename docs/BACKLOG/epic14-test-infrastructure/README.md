# Epic 14: Test Infrastructure Correctness

## Purpose

The integration test suite in `internal/controller/` has two structural defects that
cause non-deterministic test failures and false test confidence. These are not flaky
tests in the general sense — they are precise, reproducible bugs in the test
infrastructure itself.

**Defect 1 — CRD schema drift:** `testdata/crds/remediationjob_crd.yaml` is a manually
maintained copy of the CRD schema. It is never regenerated automatically. Whenever a
field is added to `RemediationJobStatus` or `RemediationJobSpec` in Go, someone must
also update the testdata YAML. When they don't, envtest's real Kubernetes API server
silently strips the unknown fields during status patches, causing integration tests to
observe empty values for fields that the code correctly set. Unit tests using the fake
client are unaffected (the fake client does no schema validation), which gives false
confidence that the feature works.

Two fields are currently missing from `spec.properties`: `isSelfRemediation` (bool) and
`chainDepth` (int), both added during epic11. These are not causing current test failures
but represent documented drift that must be corrected before it does. (`correlationGroupID`
in status was already added to the CRD and is not missing.)

**Defect 2 — Integration test isolation:** Five tests in `integration_test.go` create
`batch/v1` Jobs with deterministic names derived from fixed fingerprints. They register
cleanup via `t.Cleanup`, which runs *after* the test. If a previous test run (or a
`-count=N` invocation) did not clean up — or if cleanup failed silently — the next run
finds a stale job immediately via the label-based `waitFor` poll. That job's
`OwnerReference.UID` (or status) reflects the prior run, causing assertions to fail
non-deterministically.

The five affected tests:
- `TestRemediationJobReconciler_CreatesJob` (job `mendabot-agent-aaaa0000bbbb`)
- `TestRemediationJobReconciler_SyncsStatus_Running` (job `mendabot-agent-bbbb1111cccc`)
- `TestRemediationJobReconciler_SyncsStatus_Succeeded` (job `mendabot-agent-cccc2222dddd`)
- `TestRemediationJobReconciler_SyncsStatus_Failed` (job `mendabot-agent-dddd3333eeee`)
- `TestRemediationJobReconciler_OwnerReference` (job `mendabot-agent-ffff5555aaaa`)

**Why these keep recurring:** There is no written rule requiring testdata CRD updates
when types change, and no pre-test cleanup convention for envtest tests that create
deterministically named objects. Both gaps are corrected here: by fixing the immediate
bugs and by documenting the rules in `README-LLM.md` so future sessions don't re-introduce
the same drift.

## Status: Complete

## Dependencies

- No hard dependencies. STORY_00 and STORY_01 are standalone fixes that can be
  implemented immediately.
- Note: `correlationGroupID` was added to `RemediationJobStatus` during epic13 planning,
  but it is already present in the CRD (line 91). The integration test that exercises it
  (`TestCorrelationIntegration_TC02b_SecondaryIsSuppressed`) has not been written yet —
  that is epic13 work (`multi-signal-correlation`, Not Started). When epic13 is
  implemented, the CRD will already be correct for that field.

## Blocks

- Nothing — this is maintenance work. However, all epics that add integration tests
  benefit from the documented rules established here.

## Success Criteria

- [x] `testdata/crds/remediationjob_crd.yaml` `spec` schema includes `isSelfRemediation`
      and `chainDepth`
- [x] All five affected tests pass deterministically under `-count=3`
- [x] `README-LLM.md` documents the CRD testdata maintenance rule and the pre-test
      cleanup convention
- [x] `go test -count=3 -timeout 300s -race ./internal/controller/...` passes

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| Fix CRD schema drift | [STORY_00_crd_schema_drift.md](STORY_00_crd_schema_drift.md) | Complete | Critical | 30m |
| Fix integration test isolation | [STORY_01_integration_test_isolation.md](STORY_01_integration_test_isolation.md) | Complete | High | 45m |
| Document test infrastructure rules | [STORY_02_document_rules.md](STORY_02_document_rules.md) | Complete | Medium | 30m |

## Story Execution Order

STORY_00 and STORY_01 are independent and can be worked in parallel. STORY_02 should be
completed last so the documentation reflects the final state of the fixes.

```
STORY_00 (CRD schema drift)    ──┐
STORY_01 (test isolation)      ──┼──> STORY_02 (document rules)
```

## Definition of Done

- [x] All three stories complete
- [x] `go test -count=3 -timeout 300s -race ./internal/controller/...` passes (validates
      both determinism fixes)
- [x] `go test -timeout 90s -race ./...` passes (full suite, no regressions)
- [x] Worklog entry created in `docs/WORKLOGS/`
