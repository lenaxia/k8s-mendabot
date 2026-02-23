# Worklog: Epic 12 STORY_00 — Security Infrastructure

**Date:** 2026-02-23
**Session:** STORY_00 infrastructure-only setup for epic12 security review
**Status:** Complete

---

## Objective

Create the security infrastructure scaffolding for epic12: Makefile with security targets, kind cluster config, Trivy CVE scan steps in CI workflows, and gosec baseline placeholder.

---

## Work Completed

### 1. Makefile at repository root
- Created `Makefile` with targets: `build`, `test`, `lint`, `lint-security`, `lint-security-report`, `docker-build-watcher`, `docker-build-agent`, `scan-watcher`, `scan-agent`, `dev-cluster`, `dev-cluster-destroy`, `help`
- All recipe lines use tabs (verified with `cat -A`)
- `lint-security` uses gosec with `-severity high -confidence medium` to fail on HIGH/CRITICAL
- `scan-watcher` / `scan-agent` chain from docker build targets and invoke trivy with `--exit-code 1 --severity CRITICAL`
- `dev-cluster` provisions a kind cluster with Cilium CNI disabled default CNI for security testing

### 2. hack/kind-config.yaml
- Created `hack/kind-config.yaml` disabling default CNI, setting podSubnet `10.244.0.0/16`, with one control-plane and one worker node
- Referenced by `make dev-cluster`

### 3. CI workflow updates
- Added `Scan watcher image for CVEs` step to `.github/workflows/build-watcher.yaml` using `aquasecurity/trivy-action@0.20.0`, inserted before the existing Smoke test step
- Added `Scan agent image for CVEs` step to `.github/workflows/build-agent.yaml` using `aquasecurity/trivy-action@0.20.0`, inserted before the existing Smoke test step
- Both steps: `exit-code: '1'`, `severity: CRITICAL`, `ignore-unfixed: true`

### 4. gosec-baseline.json placeholder
- Created `docs/BACKLOG/epic12-security-review/gosec-baseline.json` as a placeholder because gosec is not installed in this environment
- Contains a `_comment` field documenting how to regenerate via `make lint-security-report`
- GosecVersion set to `2.20.0`, Stats all zero, Issues empty array

---

## Key Decisions

- Trivy scan step placed before Smoke test in CI: if there are CRITICAL CVEs, the smoke test is moot — fail fast
- `lint-security-report` uses `|| true` so it never blocks CI; it is a reporting target only
- `lint-security` (without `|| true`) is the enforcing target that fails the build
- gosec baseline is a placeholder stub — zero issues means it will be overwritten on first real run; the `_comment` field makes intent clear without polluting the Issues array

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./...
```

All 13 packages pass (cached). No Go code was added; this is infrastructure-only.

```
make build
```

Succeeds — `go build ./...` exits 0.

---

## Next Steps

STORY_01: Implement secret redaction in the agent prompt pipeline. Start by reading `docs/BACKLOG/epic12-security-review/STORY_01_secret_redaction.md` and the relevant LLD sections before writing any code.

---

## Files Modified

- `Makefile` — created
- `hack/kind-config.yaml` — created
- `docs/BACKLOG/epic12-security-review/gosec-baseline.json` — created
- `.github/workflows/build-watcher.yaml` — added Trivy scan step
- `.github/workflows/build-agent.yaml` — added Trivy scan step
