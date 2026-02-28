# Worklog: Epic 12 — Security Remediation Complete

**Date:** 2026-02-23
**Session:** Orchestrator session — remediate all 11 open findings from the 2026-02-23 security report
**Status:** Complete

---

## Objective

Address every open finding from the first mechanic security review (`docs/SECURITY/2026-02-23_security_report/`). Two findings (002, 004) were accepted during the review; the remaining 11 were Open and required remediation.

Branch: `feature/epic12-security-remediation`

---

## Work Completed

Each fix was delegated individually following the Orchestrator workflow. TDD was enforced for all Go changes: tests written and confirmed failing before implementation.

### Fix 001 — Go toolchain CVEs (MEDIUM)

- `go.mod`: `go 1.23.0` → `go 1.23.12`
- Fixes GO-2026-4341 (net/url), GO-2026-4340 (crypto/tls), GO-2026-4337 (crypto/tls)
- No test changes required — build and govulncheck verification sufficient
- Committed: `2bb6347`

### Fix 003 — FINDING_DETAILS injection path (MEDIUM)

- `internal/provider/provider.go`: added `domain.DetectInjection(finding.Details)` block immediately after the existing `finding.Errors` check; event key `finding.injection_detected_in_details`
- `deploy/kustomize/configmap-prompt.yaml`: added `BEGIN/END FINDING DETAILS (UNTRUSTED INPUT)` envelope around `${FINDING_DETAILS}`; updated HARD RULE 8 to cover both envelope blocks
- 4 new TDD tests in `internal/provider/provider_test.go` using `zaptest/observer`: logs event, suppresses when configured, clean Details no event, nil logger no panic

### Fix 005 — ClusterRole ConfigMap least-privilege (MEDIUM)

- `deploy/kustomize/clusterrole-watcher.yaml`: split first rule — ConfigMaps now `get/list/watch` only; other core resources unchanged
- `deploy/kustomize/role-watcher.yaml`: added ConfigMaps rule `get/list/watch/create/update/patch` (namespace-scoped)
- No Go tests — verified with `go build ./...` and grep

### Fix 006 — Binary checksums for yq, age, opencode (MEDIUM)

- `docker/Dockerfile.agent`: added 6 `ARG` variables (`YQ_SHA256_AMD64/ARM64`, `AGE_SHA256_AMD64/ARM64`, `OPENCODE_SHA256_AMD64/ARM64`) with pre-computed SHA256 values
- Each of the three tool install blocks now includes `echo "${EXPECTED}  <path>" | sha256sum --check`
- SHA256 values computed by downloading the actual release artifacts at v4.45.1 / v1.3.1 / v1.2.10

### Fix 007 — Base image digest pinning (LOW)

- `docker/Dockerfile.agent` line 1: `FROM debian:bookworm-slim@sha256:6458e6ce...`
- `docker/Dockerfile.watcher` build stage: `FROM golang:1.23-bookworm@sha256:e87b2a5f...`
- `docker/Dockerfile.watcher` runtime stage: `FROM debian:bookworm-slim@sha256:6458e6ce...`
- Digests fetched from Docker Hub API (linux/amd64)

### Fix 008 — GitHub Actions SHA pinning (LOW)

All three workflow files (`.github/workflows/build-watcher.yaml`, `build-agent.yaml`, `chart-test.yaml`) updated. Every `uses:` now references the commit SHA with the original tag as a comment:

| Action | SHA |
|--------|-----|
| actions/checkout | `34e114876b0b11c390a56381ad16ebd13914f8d5` |
| docker/setup-qemu-action | `c7c53464625b32c7a7e944ae62b3e17d2b600130` |
| docker/setup-buildx-action | `8d2750c68a42422c14e847fe6c8ac0403b4cbd6f` |
| docker/login-action | `c94ce9fb468520275223c153574b00df6fe4bcc9` |
| docker/metadata-action | `c299e40c65443455700f0fdfc63efafe5b349051` |
| docker/build-push-action | `ca052bb54ab0790a636c9b5f226502c73d547a25` |
| aquasecurity/trivy-action | `b2933f565dbc598b29947660e66259e3c7bc8561` |
| azure/setup-helm | `1a275c3b69536ee54be43f2070a358922e12c8d4` |

### Fix 009 — Trivy severity threshold (LOW)

- `build-watcher.yaml` and `build-agent.yaml`: `severity: CRITICAL` → `severity: CRITICAL,HIGH`

### Fix 010 — JWT Bearer token redaction (MEDIUM)

- `internal/domain/redact.go`: added `(?i)(bearer )\S+` pattern, positioned before the base64 sweep (order-critical)
- 2 new TDD tests: uppercase `Authorization: Bearer` header, lowercase `bearer` prefix

### Fix 011 — JSON password field redaction (LOW)

- `internal/domain/redact.go`: added `(?i)("password"\s*:\s*)"[^"]*"` pattern before generic password pattern
- 3 new TDD tests: no space, space after colon, case-insensitive key

### Fix 012 — Redis empty-username URL (LOW)

- `internal/domain/redact.go`: URL pattern quantifier `[^:@\s]+` → `[^:@\s]*` (allows zero chars before `:`)
- 1 new TDD test: `redis://:s3cr3tpassword@host` → `redis://[REDACTED]@host`

### Fix 013 — Stop-following/obeying injection variant (INFO)

- `internal/domain/injection.go`: added fifth pattern `(?i)stop\s+(following|obeying)\s+((the|these|all)\s+)?(rules?|instructions?|guidelines?|prompts?)`
- 5 new TDD tests: 4 positive matches, 1 negative guard (`stop running the pod`)

---

## Key Decisions

- **Bearer pattern ordering**: Must precede the base64 pattern. JWT header sections are valid base64 ≥40 chars; if base64 ran first, the `bearer ` prefix would survive but the token would be redacted as `[REDACTED-BASE64]` instead of `[REDACTED]`.
- **Age checksums**: No upstream CHECKSUMS file exists for age — only SLSA provenance `.proof` files. SHA256 values were computed by downloading the actual release tarballs and running `sha256sum`. This means the ARG values must be updated manually on each version bump.
- **Worklogs 0050–0055**: Delegation agents wrote per-fix worklogs during implementation. This entry (0056) is the orchestrator-level summary. Both are kept — the per-fix logs are more detailed; this one provides the full-session picture.
- **`toolchain` directive**: `go mod tidy` drops a `toolchain go1.23.12` line when it matches the `go` directive — this is correct Go 1.21+ behaviour; the `go 1.23.12` directive alone enforces the minimum.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race -count=1 ./...

ok  github.com/lenaxia/k8s-mechanic/api/v1alpha1          1.150s
ok  github.com/lenaxia/k8s-mechanic/cmd/watcher           1.282s
ok  github.com/lenaxia/k8s-mechanic/internal              1.285s
ok  github.com/lenaxia/k8s-mechanic/internal/cascade      1.219s
ok  github.com/lenaxia/k8s-mechanic/internal/circuitbreaker  1.657s
ok  github.com/lenaxia/k8s-mechanic/internal/config       1.219s
ok  github.com/lenaxia/k8s-mechanic/internal/controller   11.908s
ok  github.com/lenaxia/k8s-mechanic/internal/domain       1.287s
ok  github.com/lenaxia/k8s-mechanic/internal/jobbuilder   1.144s
ok  github.com/lenaxia/k8s-mechanic/internal/logging      1.110s
ok  github.com/lenaxia/k8s-mechanic/internal/metrics      1.230s
ok  github.com/lenaxia/k8s-mechanic/internal/provider     10.056s
ok  github.com/lenaxia/k8s-mechanic/internal/provider/native  2.251s
```

13/13 packages pass. Race detector enabled throughout.

---

## Next Steps

- Push `feature/epic12-security-remediation` and open a PR into `main`
- Update the security report findings to `Remediated` status (done in this session — see report update)
- Next security review requires a live cluster to complete the deferred Phase 3.3, 4, 5, 6.2, and 7 tests
- Consider adding `staticcheck` to CI to close Phase 1.1 gap
- When upgrading yq/age/opencode versions in future, recompute the SHA256 ARGs in `Dockerfile.agent`

---

## Files Modified

**Go source:**
- `internal/domain/redact.go` — 3 new/modified patterns
- `internal/domain/redact_test.go` — 6 new test cases
- `internal/domain/injection.go` — 1 new pattern
- `internal/domain/injection_test.go` — 5 new test cases
- `internal/provider/provider.go` — DetectInjection block for finding.Details
- `internal/provider/provider_test.go` — 4 new test cases

**Manifests:**
- `deploy/kustomize/clusterrole-watcher.yaml` — ConfigMaps write removed
- `deploy/kustomize/role-watcher.yaml` — ConfigMaps write added
- `deploy/kustomize/configmap-prompt.yaml` — FINDING_DETAILS envelope + HARD RULE 8 update

**Dockerfiles:**
- `docker/Dockerfile.agent` — 6 SHA256 ARGs, checksum steps for yq/age/opencode, debian digest pin
- `docker/Dockerfile.watcher` — golang and debian digest pins

**CI:**
- `.github/workflows/build-watcher.yaml` — actions SHA pinned, Trivy CRITICAL,HIGH
- `.github/workflows/build-agent.yaml` — actions SHA pinned, Trivy CRITICAL,HIGH
- `.github/workflows/chart-test.yaml` — actions SHA pinned

**Module:**
- `go.mod` — go 1.23.12

**Docs:**
- `docs/SECURITY/2026-02-23_security_report/README.md` — finding summary and counts updated
- `docs/SECURITY/2026-02-23_security_report/findings.md` — all 11 findings marked Remediated
- `README-LLM.md` — branch table updated
- `docs/WORKLOGS/0056_2026-02-23_epic12-security-remediation-complete.md` — this file
- `docs/WORKLOGS/README.md` — index updated
