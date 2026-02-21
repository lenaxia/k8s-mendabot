# Story: envtest Suite Setup

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **developer**, I want a working envtest suite that starts a real Kubernetes API server,
installs both the k8sgpt Result CRD and our `RemediationJob` CRD, and tears everything
down cleanly after each test suite so that controller integration tests run without a live
cluster.

---

## Acceptance Criteria

- [ ] `internal/provider/k8sgpt/suite_test.go` contains a `TestMain` that:
  - Starts `envtest.Environment` with CRD paths pointing at both:
    - `testdata/crds/result_crd.yaml` (k8sgpt Result CRD, vendored)
    - `testdata/crds/remediationjob_crd.yaml` (our CRD — from DEPLOY_LLD §2.1)
  - Creates a `ctrl.Manager` backed by the test API server
  - Registers both schemes (Result + RemediationJob)
  - Tears down cleanly after all tests complete

- [ ] `internal/controller/suite_test.go` contains an equivalent `TestMain` for
  `RemediationJobReconciler` integration tests (same CRDs, same setup pattern)

- [ ] `go test -timeout 300s ./internal/provider/k8sgpt/...` and
  `go test -timeout 300s ./internal/controller/...` both pass with no test cases yet —
  the suites just start and stop

- [ ] A smoke test `TestSuite_StartsAndStops` in each package verifies the envtest API
  server is reachable by listing `RemediationJob` objects:
  ```go
  var rjobList v1alpha1.RemediationJobList
  err := k8sClient.List(ctx, &rjobList)
  // expect no error — CRD is registered
  ```
- [ ] The envtest binary path is resolved via `setup-envtest` — no hardcoded paths
- [ ] A comment at the top of each `suite_test.go` explains how to install envtest binaries:
  ```
  # go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
  # setup-envtest use --bin-dir /usr/local/kubebuilder/bin
  # export KUBEBUILDER_ASSETS=$(setup-envtest use -p path)
  ```

---

## CRD Sources

| File | Source |
|---|---|
| `testdata/crds/result_crd.yaml` | Vendored from `k8sgpt-ai/k8sgpt-operator` — `config/crd/bases/core.k8sgpt.ai_results.yaml` |
| `testdata/crds/remediationjob_crd.yaml` | Copied from `deploy/kustomize/crd-remediationjob.yaml` (same file, different path) |

Both files must be committed. They are treated as vendored/pinned dependencies — update
deliberately, never automatically.

---

## Framework Choice

Standard `controller-runtime` envtest with Go's `testing` package and `TestMain`. No
Ginkgo/Gomega — the rest of the test suite uses plain `testing` and table-driven tests.

---

## Tasks

- [ ] Create `testdata/crds/result_crd.yaml` (source from k8sgpt-operator repo)
- [ ] Create `testdata/crds/remediationjob_crd.yaml` (copy from deploy/kustomize/)
- [ ] Create `internal/provider/k8sgpt/suite_test.go` with `TestMain`, envtest start/stop,
  and a `k8sClient` variable accessible to all integration tests in that package
- [ ] Create `internal/controller/suite_test.go` (same pattern as above)
- [ ] Write `TestSuite_StartsAndStops` smoke test in each package

---

## Dependencies

**Depends on:** STORY_01 (RemediationJob types — suite uses `v1alpha1.RemediationJobList`)
**Depends on:** STORY_03 (provider + reconciler skeletons — suites start them)
**Depends on:** DEPLOY_LLD.md §2.1 (hand-write the CRD YAML from the schema defined there
  — do not wait for epic04; the CRD manifest is committed under `testdata/crds/` as part
  of this story)
**Blocks:** epic01-controller/STORY_07 (integration tests)

---

## Definition of Done

- [ ] `go test -timeout 300s -race ./internal/provider/k8sgpt/...` passes
- [ ] `go test -timeout 300s -race ./internal/controller/...` passes
- [ ] `go vet` clean
- [ ] Suite starts and stops without leaving stale processes
- [ ] Both CRD YAML files committed under `testdata/crds/`
