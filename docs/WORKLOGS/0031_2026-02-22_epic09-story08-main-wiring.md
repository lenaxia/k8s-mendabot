# Worklog: Epic 09 STORY_08 — Wire Native Providers into main.go

**Date:** 2026-02-22
**Session:** Wire all six native providers into cmd/watcher/main.go alongside the existing K8sGPTProvider
**Status:** Complete

---

## Objective

Register all six native providers (Pod, Deployment, PVC, Node, StatefulSet, Job) in the
`enabledProviders` slice in `cmd/watcher/main.go` so that the watcher starts watching
native Kubernetes resources at startup.

---

## Work Completed

### 1. Read constructors in internal/provider/native/

Confirmed all six constructors and their signatures:
- `NewPodProvider(c client.Client)` — takes client, panics on nil
- `NewDeploymentProvider(c client.Client)` — takes client, panics on nil
- `NewPVCProvider(c client.Client)` — takes client, panics on nil
- `NewNodeProvider(c client.Client)` — takes client, panics on nil (story instructions noted to check)
- `NewStatefulSetProvider(c client.Client)` — takes client, panics on nil
- `NewJobProvider(c client.Client)` — takes client, panics on nil

All six return `domain.SourceProvider`.

### 2. Modified cmd/watcher/main.go

- Added import: `"github.com/lenaxia/k8s-mendabot/internal/provider/native"`
- Extracted `nativeClient := mgr.GetClient()` before the slice literal
- Expanded `enabledProviders` from 1 entry (K8sGPTProvider) to 7:
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
- The existing `SourceProviderReconciler` registration loop is unchanged (iterates the
  slice and calls `SetupWithManager` for each provider).
- The struct literal for `SourceProviderReconciler` in main.go sets only `Client`,
  `Scheme`, `Log`, `Cfg`, `Provider` — `firstSeen` is left uninitialised (lazily
  initialised in `Reconcile`).

---

## Key Decisions

- `nativeClient` local variable used to avoid repeating `mgr.GetClient()` seven times.
  This is a single stable reference; controller-runtime's client is safe to pass to
  multiple providers.
- K8sGPTProvider kept as first entry per story requirement (STORY_09 handles its removal).
- Registration order is deterministic (slice literal, not map iteration): K8sGPT, Pod,
  Deployment, PVC, Node, StatefulSet, Job.

---

## Blockers

None.

---

## Tests Run

```
go build ./...          — clean
go vet ./...            — clean
go test -timeout 60s -race ./cmd/watcher/...   — ok (1.085s)
go test -timeout 60s -race ./...               — all 10 packages ok
```

---

## Next Steps

STORY_09: Remove K8sGPTProvider from main.go (now blocked by this story being complete).

---

## Files Modified

- `cmd/watcher/main.go` — added native import and six provider registrations
- `docs/WORKLOGS/0031_2026-02-22_epic09-story08-main-wiring.md` — this file
- `docs/WORKLOGS/README.md` — index updated
