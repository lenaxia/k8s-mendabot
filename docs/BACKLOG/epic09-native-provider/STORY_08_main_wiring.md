# Story: Wire Native Providers into main.go

**Epic:** [epic09-native-provider](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want the watcher to start watching Pods, Deployments, PVCs,
Nodes, StatefulSets, and Jobs at startup so that native findings trigger remediation Jobs
from the moment the watcher is running.

---

## Provider struct architecture

Each native provider is an **independent unexported struct** in its own file (e.g.
`type podProvider struct { client client.Client }` in `internal/provider/native/pod.go`).
There is **no** shared `NativeProvider` wrapper struct. Each provider's constructor is an
exported function in the same file:

```go
// internal/provider/native/pod.go
type podProvider struct {
    client client.Client
}

func NewPodProvider(c client.Client) *podProvider {
    if c == nil {
        panic("NewPodProvider: client must not be nil")
    }
    return &podProvider{client: c}
}
```

The `main.go` slice is built by calling each constructor individually:

```go
nativeClient := mgr.GetClient()
enabledProviders := []domain.SourceProvider{
    &k8sgpt.K8sGPTProvider{},
    native.NewPodProvider(nativeClient),
    native.NewDeploymentProvider(nativeClient),
    native.NewPVCProvider(nativeClient),
    native.NewNodeProvider(nativeClient),
    native.NewStatefulSetProvider(nativeClient),
    native.NewJobProvider(nativeClient),
}
```

This design keeps each provider entirely self-contained — `PodProvider` can be read,
tested, and reasoned about in isolation without knowledge of what other providers exist.
There is no `NativeProvider` factory struct, no `Providers() []domain.SourceProvider`
method, and no shared state between providers (each holds its own `client.Client`
reference, all pointing to the same underlying manager client).

**Registration order** in the slice is deterministic (a slice literal, not a map
iteration). The order shown above is also the recommended implementation order within
the epic.

---

## Acceptance Criteria

- [ ] `cmd/watcher/main.go` registers all six native providers alongside (not replacing)
  `K8sGPTProvider` in the `enabledProviders` slice, using the architecture shown above
- [ ] Each provider exposes a constructor (`NewPodProvider`, `NewDeploymentProvider`,
  `NewPVCProvider`, `NewNodeProvider`, `NewStatefulSetProvider`, `NewJobProvider`) that
  accepts a `client.Client` and panics if `client` is nil
- [ ] The existing `SourceProviderReconciler` registration loop in `main.go` is unchanged —
  it already iterates over `[]domain.SourceProvider` and calls `SetupWithManager` for each
- [ ] `go build ./...` succeeds with all seven providers registered
- [ ] The watcher starts without error in the envtest suite when all six native providers are
  registered (add a smoke-test integration test to `internal/provider/native/` that
  bootstraps the manager with all six providers and verifies it starts cleanly)

---

## Tasks

- [ ] Add `New*Provider(client.Client)` constructors to each native provider file, with
  nil-client validation (this may require minor edits to STORY_04–07 and STORY_10–11
  implementations)
- [ ] Write `internal/provider/native/suite_test.go` with envtest suite setup in
  `package native_test` (modelled on `internal/provider/k8sgpt/suite_test.go`;
  CRD path will be `"../../../testdata/crds"`)
- [ ] Write integration smoke test in `internal/provider/native/integration_test.go`
  using the envtest suite: start manager with all six native providers, confirm no
  startup error
- [ ] **Makefile / CI note:** the integration tests in `internal/provider/native/`
  require envtest binaries (`KUBEBUILDER_ASSETS`). The existing Makefile `go test ./...`
  target already handles this via the `suite_test.go` `TestMain` pattern (skip when
  `KUBEBUILDER_ASSETS` is not set). No Makefile changes are needed unless a new CI
  job is added for native integration tests. Verify that `make test` passes in CI
  before and after this story.
- [ ] Update `cmd/watcher/main.go` to import and register all six native providers
- [ ] Run `go build ./...` and full test suite

---

## Dependencies

**Depends on:** STORY_04, STORY_05, STORY_06, STORY_07, STORY_10, STORY_11 (all six providers complete)
**Blocks:** STORY_09 (k8sgpt removal)

---

## Definition of Done

- [ ] Integration smoke test passes with `-race`
- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] Full test suite passes: `go test -timeout 120s -race ./...`
