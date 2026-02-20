# Story: Go Module and Directory Structure

**Epic:** [Foundation](README.md)
**Priority:** Critical
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want a properly initialised Go module with the full project directory
structure so that all subsequent epics have a consistent base to build on.

---

## Acceptance Criteria

- [x] `go.mod` declares module `github.com/lenaxia/k8s-mendabot`
- [x] Go version is 1.24 or later
- [x] Core dependencies added: `sigs.k8s.io/controller-runtime`, `k8s.io/api`,
  `k8s.io/apimachinery`, `k8s.io/client-go`, `go.uber.org/zap`
- [x] `go mod tidy` runs without errors
- [x] `go build ./...` succeeds (stubs only at this stage)
- [x] All directories from the README-LLM.md structure exist
- [x] `.gitignore` covers binaries, secrets, editor files, and test output

---

## Directory Structure to Create

```
api/v1alpha1/
cmd/watcher/
internal/controller/
internal/jobbuilder/
deploy/kustomize/
docker/scripts/
docs/          (already exists)
.github/workflows/
```

---

## Tasks

- [x] Verify `go.mod` exists with correct module name (already initialised)
- [x] Run `go get` for all required dependencies at pinned versions
- [x] Run `go mod tidy`
- [x] Create all missing directories
- [x] Create stub `cmd/watcher/main.go` (package main, empty main func)
- [x] Create `.gitignore`
- [x] Verify `go build ./...` compiles

---

## .gitignore Contents

```
bin/
*.test
*.out
coverage.out
.env
.env.*
*.pem
*.key
/deploy/kustomize/secret-*.yaml
!deploy/kustomize/secret-*-placeholder.yaml
.vscode/
.idea/
*.swp
.DS_Store
```

---

## Dependencies

**Depends on:** None
**Blocks:** All other stories

---

## Definition of Done

- [x] `go build ./...` succeeds
- [x] `go mod tidy` produces no diff
- [x] `go vet ./...` is clean
- [x] All directories present
- [ ] Committed to git
