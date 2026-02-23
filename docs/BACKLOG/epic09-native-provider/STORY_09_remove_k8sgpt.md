# Story: Remove k8sgpt

**Epic:** [epic09-native-provider](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator**, I want to deploy mendabot without installing k8sgpt-operator
so that there is one fewer prerequisite and one fewer binary in the agent image.

---

## Background

This story is a deletion story — its only job is to remove everything that depended on
k8sgpt once the native providers are confirmed working via STORY_08. No new functionality
is added. Every deletion must be verified by confirming `go build ./...` and the full
test suite pass after the change.

---

## Acceptance Criteria

**Go code deletions:**
- [ ] `internal/provider/k8sgpt/` package deleted entirely (all `.go` files)
- [ ] `api/v1alpha1/result_types.go` deleted
- [ ] `api/v1alpha1/remediationjob_types.go`:
  - `SourceTypeK8sGPT` constant replaced with `SourceTypeNative = "native"`; all
    references updated
  - `NewScheme()` function updated to remove the `AddResultToScheme` call (which
    will no longer exist once `result_types.go` is deleted); `NewScheme()` must
    still register `RemediationJob` and `RemediationJobList` under
    `remediation.mendabot.io/v1alpha1` and all standard Kubernetes types required
    by the test suite and envtest
- [ ] `cmd/watcher/main.go`: `k8sgpt.K8sGPTProvider{}` entry removed from `enabledProviders`;
  `AddResultToScheme` call removed from scheme registration
- [ ] `go mod tidy` run; any k8sgpt-specific `go.mod`/`go.sum` entries removed

**Integration test migration (critical — do not skip):**

`internal/provider/k8sgpt/integration_test.go` contains 6 end-to-end integration test
scenarios using envtest that test `SourceProviderReconciler` behaviour: create
RemediationJob, dedup on same fingerprint, re-dispatch on Failed phase, skip on no
errors, cancel Pending on source delete, cancel Dispatched on source delete. When
`k8sgpt/` is deleted, these tests are deleted with it.

These scenarios are not covered by the `internal/provider/native/integration_test.go`
added in STORY_08 (which only tests that the manager starts cleanly). Before deleting
the `k8sgpt/` package, the reconciler integration test scenarios must be migrated.

**Migrate by creating two new files in the `provider` package test suite:**

1. `internal/provider/suite_test.go` — a new envtest suite setup file, modelled on
   `internal/provider/k8sgpt/suite_test.go`. It must be in `package provider_test`,
   declare the same `k8sClient`, `testEnv`, `suiteReady` vars, and use `TestMain`.
   The CRD path will be `"../../testdata/crds"` (two levels up from `internal/provider/`).
   **Note:** `internal/provider/provider_test.go` already exists and uses
   `package provider_test` with a `newTestScheme()` / `newTestClient()` pattern —
   the new `suite_test.go` must be consistent with this package declaration.
   Check whether a `TestMain` already exists in `provider_test.go` before adding one
   (there can only be one `TestMain` per test binary/package).

2. `internal/provider/provider_integration_test.go` — re-implements the same 6 scenarios
   using a `fakeSourceProvider` backed by `fake.NewClientBuilder()` and the envtest
   `k8sClient` from the new `suite_test.go`.

The 6 scenarios to migrate:
1. `CreateRemediationJob` — provider with finding → RemediationJob created with correct fields
2. `DuplicateFingerprint_Skips` — same fingerprint on second reconcile → no second Job
3. `FailedPhase_ReDispatches` — existing Failed Job → deleted and new one created
4. `NoErrors_Skipped` — nil finding → no Job
5. `ResultDeleted_CancelsPending` — not-found → Pending Job deleted
6. `ResultDeleted_CancelsDispatched` — not-found → Dispatched Job deleted

When migrating, update the `fakeSourceProvider` to match STORY_02's slimmed interface
(no `Fingerprint` method). The `Errors` field on the test finding must be valid JSON
(e.g. `[{"text":"CrashLoopBackOff"}]`) so that `domain.FindingFingerprint` does not
return an error.

**Docker image:**
- [ ] `docker/Dockerfile.agent`: `k8sgpt` installation block removed
- [ ] Agent image smoke test (`docker/scripts/smoke-test.sh`) updated to remove `k8sgpt`
  version check

**Prompt:**
- [ ] `deploy/kustomize/configmap-prompt.yaml`: step referencing `k8sgpt analyze` removed
  from the investigation sequence; the agent's own `kubectl describe` and event inspection
  steps cover the same ground

**Verification:**
- [ ] `go build ./...` clean after all deletions
- [ ] `go vet ./...` clean
- [ ] Full test suite passes: `go test -timeout 120s -race ./...`
- [ ] No remaining references to `k8sgpt` in any `.go` file (verify with a search)
- [ ] No remaining references to `result_types.go` or `SourceTypeK8sGPT`

---

## Migration note for existing deployments

Any cluster that currently has `RemediationJob` objects with `spec.sourceType: "k8sgpt"`
will continue to function — `RemediationJobReconciler` does not branch on `sourceType`.
Those objects will simply retain the `"k8sgpt"` label as historical record. New objects
created after this story will have `spec.sourceType: "native"`.

---

## Tasks

- [ ] Create `internal/provider/suite_test.go` with envtest setup in `package provider_test`
  (modelled on `internal/provider/k8sgpt/suite_test.go`; CRD path: `"../../testdata/crds"`)
- [ ] Create `internal/provider/provider_integration_test.go` with the 6 migrated reconciler
  integration test scenarios (TDD: run against current code to verify they pass before deletion)
- [ ] Delete `internal/provider/k8sgpt/` directory (all files: `provider.go`,
  `provider_test.go`, `integration_test.go`, `suite_test.go`)
- [ ] Delete `api/v1alpha1/result_types.go`
- [ ] Update `api/v1alpha1/remediationjob_types.go`:
  - Replace `SourceTypeK8sGPT` constant with `SourceTypeNative = "native"`
  - Remove `AddResultToScheme` call from `NewScheme()`; update `NewScheme()` to only
    register RemediationJob types
- [ ] Update `cmd/watcher/main.go` — remove k8sgpt provider and `AddResultToScheme`
  scheme registration
- [ ] Run `go build ./...` — confirm clean
- [ ] Run `go mod tidy`
- [ ] Remove k8sgpt from `docker/Dockerfile.agent`
- [ ] Update `docker/scripts/smoke-test.sh`
- [ ] Update `deploy/kustomize/configmap-prompt.yaml`
- [ ] Run full build and test suite — confirm clean
- [ ] Write worklog entry 0021 for epic09 implementation complete

---

## Dependencies

**Depends on:** STORY_08 (all native providers wired and integration smoke test passing)
**Depends on:** STORY_12 (stabilisation window complete — both stories touch `main.go`
and `provider.go`; completing STORY_12 first avoids merge conflicts)
**Blocks:** Nothing — this is the final story of the epic

---

## Definition of Done

- [ ] Zero references to `k8sgpt` in `.go` source files
- [ ] All 6 reconciler integration scenarios covered in `provider_integration_test.go`
- [ ] Full test suite passes with `-race`
- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] `go mod tidy` leaves no k8sgpt dependency entries
