# Epic: Interfaces, Data Structures, and Test Infrastructure

## Purpose

Define all shared interfaces, data structures, and test infrastructure before any
functional epic begins. This epic produces no observable runtime behaviour — it produces
the skeleton that epics 01 and 02 fill in. Getting these right first prevents costly
refactoring later.

## Status: Not Started

## Dependencies

- epic00-foundation complete (module compiles, config works, logging works, CRD types exist)

## Blocks

- epic01-controller (uses SourceProvider interface, fingerprintFor, RemediationJob CRD)
- epic02-jobbuilder (uses JobBuilder interface, jobbuilder.Config)

## Stories

| Story | File | Status |
|-------|------|--------|
| Core domain types | [STORY_01_domain_types.md](STORY_01_domain_types.md) | Not Started |
| Builder interface | [STORY_02_builder_interface.md](STORY_02_builder_interface.md) | Not Started |
| Reconciler interface and struct skeleton | [STORY_03_reconciler_skeleton.md](STORY_03_reconciler_skeleton.md) | Not Started |
| envtest suite setup | [STORY_04_envtest_suite.md](STORY_04_envtest_suite.md) | Not Started |
| Fake/stub implementations | [STORY_05_fakes.md](STORY_05_fakes.md) | Not Started |

## Success Criteria

- [ ] All interfaces compile and are reachable from the packages that will use them
- [ ] `SourceProvider`, `Finding`, `SourceRef` defined in `internal/domain/provider.go`
- [ ] `JobBuilder` interface defined in `internal/domain/interfaces.go`
- [ ] envtest suite bootstraps a real API server and tears it down cleanly
- [ ] Fake `JobBuilder` exists and is usable in controller unit tests without a cluster
- [ ] `go build ./...` and `go vet ./...` are clean
- [ ] No functional logic implemented — only types, interfaces, and test plumbing

## Definition of Done

- [ ] All tests in this epic pass with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
- [ ] Downstream epics can reference these types without circular imports
