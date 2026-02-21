# Story: Manager Setup and main.go Wiring

**Epic:** [Controller](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want `cmd/watcher/main.go` to fully wire the controller-runtime
manager, register all schemes, set up health checks, and start the reconciler so the
watcher binary is runnable end-to-end.

---

## Acceptance Criteria

- [ ] Manager created with `ctrl.NewManager(ctrl.GetConfigOrDie(), ...)`
- [ ] `LeaderElection: false` (single replica, no election needed)
- [ ] Metrics server on `:8080`, health probe on `:8081`
- [ ] `healthz.Ping` registered for both `/healthz` and `/readyz`
- [ ] `RemediationJobReconciler` instantiated with config, logger, and jobbuilder, and
  registered via `SetupWithManager`
- [ ] Provider loop registers all source providers:
  `[]domain.SourceProvider{&k8sgpt.K8sGPTProvider{}}` wrapped by `SourceProviderReconciler`
- [ ] `mgr.Start(ctrl.SetupSignalHandler())` called — blocks until signal received
- [ ] `log.Fatal` on any setup error (fail fast, never silently start degraded)
- [ ] Binary responds to `--version` flag by printing the version string and exiting 0
  (controller-runtime does not add this automatically — `main()` must check `os.Args`
  for `"--version"`, print `Version`, and call `os.Exit(0)`)

---

## Tasks

- [ ] Complete `cmd/watcher/main.go` with all wiring
- [ ] Verify binary starts and connects to a cluster (manual test)
- [ ] Verify `/healthz` returns 200

---

## Dependencies

**Depends on:** STORY_04 (reconcile), Foundation STORY_02 (config), Foundation STORY_03 (logging)
**Blocks:** STORY_07 (integration tests), Deploy epic

---

## Definition of Done

- [ ] Binary compiles and starts
- [ ] Health endpoints respond
- [ ] `go vet` clean
