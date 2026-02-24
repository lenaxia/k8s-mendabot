# Phase 8: Supply Chain Integrity

**Date run:** 2026-02-23
**Reviewer:** OpenCode (automated review)

---

## 8.1 Docker Image Binary Checksum Coverage

**Command:**
```bash
grep -E '(curl|sha256sum|sha256)' docker/Dockerfile.agent
```

Output reviewed. Summary:

| Binary | Download Source | Checksum Verified? | Method | Notes |
|--------|----------------|-------------------|--------|-------|
| kubectl | dl.k8s.io | **yes** | `sha256sum --check` vs official `.sha256` file | Pass |
| helm | get.helm.sh | **yes** | `sha256sum --check` vs `.sha256sum` file | Pass |
| flux | github releases | **yes** | `sha256sum --check` vs `checksums.txt` | Pass |
| talosctl | github releases | **yes** | `sha256sum --check` vs `sha256sum.txt` | Pass |
| kustomize | github releases | **yes** | `sha256sum --check` vs `checksums.txt` | Pass |
| kubeconform | github releases | **yes** | `sha256sum --check` vs `CHECKSUMS` | Pass |
| stern | github releases | **yes** | `sha256sum --check` vs `checksums.txt` | Pass |
| sops | github releases | **yes** | `sha256sum --check` vs `.checksums.txt` | Pass |
| yq | github releases | **no** | Comment: "checksums file format is non-standard" | Finding 2026-02-23-006 (already recorded Phase 2) |
| age | github releases | **no** | Comment: "only provenance .proof files" | Finding 2026-02-23-006 (already recorded Phase 2) |
| opencode | github releases | **no** | No checksum step at all | Finding 2026-02-23-006 (already recorded Phase 2) |
| gh CLI | apt (GPG-signed repo) | **yes (apt)** | GPG key + signed apt repo — no manual checksum needed | Pass |

**Binaries without checksum verification:**
```
yq    — comment says checksums file format is non-standard
age   — comment says only provenance .proof files available
opencode — no checksum verification
```

All three are referenced as finding 2026-02-23-006 (MEDIUM) recorded in Phase 2.

---

## 8.2 GitHub Actions Pin Audit

**Command:**
```bash
grep -r 'uses:' .github/workflows/
```
```
.github/workflows/build-agent.yaml:    uses: actions/checkout@v4
.github/workflows/build-agent.yaml:    uses: docker/setup-qemu-action@v3
.github/workflows/build-agent.yaml:    uses: docker/setup-buildx-action@v3
.github/workflows/build-agent.yaml:    uses: docker/login-action@v3
.github/workflows/build-agent.yaml:    uses: docker/metadata-action@v5
.github/workflows/build-agent.yaml:    uses: docker/build-push-action@v5
.github/workflows/build-agent.yaml:    uses: aquasecurity/trivy-action@0.20.0
.github/workflows/build-watcher.yaml:  [same as above]
.github/workflows/chart-test.yaml:     uses: actions/checkout@v4
.github/workflows/chart-test.yaml:     uses: azure/setup-helm@v4
```

| Action | Current Ref | Pinned to SHA? | Trusted Org? | Notes |
|--------|------------|----------------|--------------|-------|
| `actions/checkout` | `@v4` | **no** | yes | Mutable tag |
| `docker/setup-qemu-action` | `@v3` | **no** | yes | Mutable tag |
| `docker/setup-buildx-action` | `@v3` | **no** | yes | Mutable tag |
| `docker/login-action` | `@v3` | **no** | yes | Mutable tag |
| `docker/metadata-action` | `@v5` | **no** | yes | Mutable tag |
| `docker/build-push-action` | `@v5` | **no** | yes | Mutable tag |
| `aquasecurity/trivy-action` | `@0.20.0` | **no** | yes | Version tag (not SHA) |
| `azure/setup-helm` | `@v4` | **no** | yes | Mutable tag |

All third-party actions use mutable version tags. This is finding 2026-02-23-008 (LOW) already recorded in Phase 2.

---

## 8.3 Base Image Currency

**Base images in use:**
```bash
grep 'FROM' docker/Dockerfile.agent docker/Dockerfile.watcher
```
```
docker/Dockerfile.agent:FROM debian:bookworm-slim
docker/Dockerfile.watcher:FROM golang:1.23-bookworm AS builder
docker/Dockerfile.watcher:FROM debian:bookworm-slim
```

`debian:bookworm-slim` — tag only, not a digest. This is finding 2026-02-23-007 (LOW) already recorded in Phase 2.

**Trivy scan — agent image:**

**Status:** SKIPPED — reason: Docker build not available in this review environment

**Trivy scan — watcher image:**

**Status:** SKIPPED — reason: Docker build not available in this review environment

**Note:** Trivy scans are run in CI on every tagged build (`aquasecurity/trivy-action@0.20.0` in both build workflows). However, as noted in Phase 2, the CI scan only fails on `CRITICAL` — HIGH findings are shown but do not fail the build (finding 2026-02-23-009).

**HIGH/CRITICAL CVEs found:** Not assessed in this review — deferred to CI scan results.

---

## 8.4 Go Module Integrity

```bash
go mod verify
```
```
all modules verified
```

**Result:** PASS — all module checksums valid

**Findings:** none

---

## Phase 8 Summary

**Total findings:** 0 new — all supply chain findings were recorded in Phase 2
**Findings already recorded:** 2026-02-23-006 (missing checksums: yq, age, opencode), 2026-02-23-007 (base image tag), 2026-02-23-008 (actions not SHA-pinned), 2026-02-23-009 (Trivy CRITICAL-only fail)
**Trivy image scan:** SKIPPED — deferred; CI runs Trivy on every release build
