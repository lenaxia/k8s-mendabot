# Story: Provider and Reconciler Skeletons

**Epic:** [Interfaces and Test Infrastructure](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want the provider and reconciler structs declared (but unimplemented)
so that `cmd/watcher/main.go` can wire the full manager, and `go build ./...` passes
end-to-end before any reconcile logic is written.

---

## Acceptance Criteria

- [x] `internal/provider/k8sgpt/provider.go` defines `K8sGPTProvider` as a plain struct
  implementing `domain.SourceProvider` with stub methods that `panic("not implemented")`:
  ```go
  type K8sGPTProvider struct{}

  func (p *K8sGPTProvider) ProviderName() string          { panic("not implemented") }
  func (p *K8sGPTProvider) ObjectType() client.Object     { panic("not implemented") }
  func (p *K8sGPTProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
      panic("not implemented")
  }
  func (p *K8sGPTProvider) Fingerprint(f *domain.Finding) string { panic("not implemented") }
  ```
  Compile-time assertion: `var _ domain.SourceProvider = (*K8sGPTProvider)(nil)`

- [x] `internal/provider/provider.go` defines `SourceProviderReconciler` with all fields
  from PROVIDER_LLD.md §5, and stub `Reconcile` and `SetupWithManager` methods that
  `panic("not implemented")`:
  ```go
  type SourceProviderReconciler struct {
      client.Client
      Scheme   *runtime.Scheme
      Log      *zap.Logger
      Cfg      config.Config
      Provider domain.SourceProvider
  }
  ```

- [x] `internal/controller/remediationjob_controller.go` defines `RemediationJobReconciler`
  with all fields from CONTROLLER_LLD.md §6.1, and stub methods:
  ```go
  type RemediationJobReconciler struct {
      client.Client
      Scheme     *runtime.Scheme
      Log        *zap.Logger
      JobBuilder domain.JobBuilder
      Cfg        config.Config
  }
  ```

- [x] `cmd/watcher/main.go` is updated from its empty stub to the full manager wiring
  from CONTROLLER_LLD.md §7:
  - Scheme registration (clientgo + batchv1 + v1alpha1)
  - `config.FromEnv()` with fatal on error
  - Logger construction
  - `jobbuilder.New(jobbuilder.Config{AgentNamespace: cfg.AgentNamespace})` with error check
  - `RemediationJobReconciler` registered directly
  - Provider loop: `[]domain.SourceProvider{&k8sgpt.K8sGPTProvider{}}` wrapped by
    `SourceProviderReconciler` per provider
  - Health probes
  - `mgr.Start(ctrl.SetupSignalHandler())`

- [x] `go build ./...` compiles cleanly with stubs in place

---

## Note on `main.go`

The full wiring is included here because it references only types and interfaces — no
logic. It unblocks `go build` verification throughout all remaining epics and will not
need to change when the reconciler bodies are implemented in epic01.

---

## Tasks

- [x] Verify `internal/domain/provider.go` (created in STORY_01) defines `SourceProvider`,
  `Finding`, `SourceRef` exactly as specified in PROVIDER_LLD.md §3
- [x] Create `internal/provider/provider.go` with `SourceProviderReconciler` + stub methods
- [x] Create `internal/provider/k8sgpt/provider.go` with `K8sGPTProvider` + stub methods
- [x] Create `internal/provider/k8sgpt/reconciler.go` as an empty file (no struct — it will
  hold only the `fingerprintFor()` function, added in epic01-controller/STORY_02)
- [x] Create `internal/controller/remediationjob_controller.go` with struct + stub methods
- [x] Rewrite `cmd/watcher/main.go` with full manager wiring
- [x] Verify `go build ./...` compiles

---

## Dependencies

**Depends on:** STORY_01 (RemediationJob types + domain interfaces)
**Depends on:** STORY_02 (JobBuilder interface)
**Depends on:** epic00-foundation/STORY_03 (logging)
**Depends on:** epic00-foundation/STORY_04 (CRD types + AddToScheme)
**Blocks:** epic01-controller (fills in stub bodies)

---

## Definition of Done

- [x] `go build ./...` clean
- [x] `go vet ./...` clean
- [x] `main.go` contains full manager wiring with provider loop, not just `func main() {}`
