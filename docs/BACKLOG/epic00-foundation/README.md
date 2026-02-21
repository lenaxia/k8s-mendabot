# Epic: Foundation

## Purpose

Establish the Go module, directory structure, typed configuration, logging setup, and
CI skeleton so all subsequent epics have a solid base to build on.

## Status: In Progress

## Dependencies

None — this is the first epic.

## Blocks

All other epics.

## Success Criteria

- [ ] Go module compiles with `go build ./...`
- [ ] All required dependencies in `go.mod`
- [ ] Typed `Config` struct reads from environment variables with validation at startup
- [ ] Structured logging (zap) initialised and usable from any package
- [ ] `.gitignore` covers binaries, secrets, and editor files
- [ ] GitHub Actions test workflow runs `go test ./...` on push

## Stories

| Story | File | Status |
|-------|------|--------|
| Go module and directory structure | [STORY_01_module_setup.md](STORY_01_module_setup.md) | Complete |
| Typed configuration | [STORY_02_config.md](STORY_02_config.md) | Not Started |
| Structured logging | [STORY_03_logging.md](STORY_03_logging.md) | Not Started |
| Vendored CRD types | [STORY_04_crd_types.md](STORY_04_crd_types.md) | Not Started |

## Technical Overview

The foundation epic produces no user-facing behaviour. It exists to ensure every
subsequent epic starts from a consistent, well-structured base.

Key outcomes:
- `cmd/watcher/main.go` exists and compiles (stub only — wired up in Controller epic)
- `internal/config/config.go` defines a `Config` struct with `FromEnv()` constructor
- Logging is initialised in `main.go` and passed down; no global logger
- `go.mod` pins all core dependencies to the same versions used by k8sgpt itself

## Definition of Done

- [ ] `go build ./...` succeeds
- [ ] `go test -timeout 30s -race ./...` succeeds
- [ ] `go vet ./...` is clean
- [ ] `go fmt ./...` produces no diff
