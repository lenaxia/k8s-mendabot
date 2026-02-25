# Phase 8: Supply Chain Integrity

**Date run:** 2026-02-24
**Reviewer:** automated (orchestrator + static analysis)

---

## 8.1 Docker Image Binary Checksum Coverage

| Binary | Download | Checksum Verified? | Method |
|--------|----------|-------------------|--------|
| kubectl | dl.k8s.io | Yes | `kubectl.sha256` sidecar file + `sha256sum --check` |
| helm | get.helm.sh | Yes | `.sha256sum` sidecar from get.helm.sh |
| flux | github releases | Yes | `checksums.txt` from fluxcd/flux2 releases |
| talosctl | github releases | Yes | `sha256sum.txt` from siderolabs/talos |
| kustomize | github releases | Yes | `checksums.txt` from kubernetes-sigs/kustomize |
| yq | github releases | Yes | Embedded SHA256 ARG per arch (`YQ_SHA256_AMD64`, `YQ_SHA256_ARM64`) |
| kubeconform | github releases | Yes | `CHECKSUMS` file from yannh/kubeconform |
| stern | github releases | Yes | `checksums.txt` from stern/stern |
| sops | github releases | Yes | `.checksums.txt` from getsops/sops |
| opencode | github releases | Yes | Embedded SHA256 ARG per arch (`OPENCODE_SHA256_AMD64`, `OPENCODE_SHA256_ARM64`) |
| age / age-keygen | source build | Yes | Two-stage build with `golang:1.25.7` — compiled from source, no download |
| gh CLI | apt (GPG-signed) | Yes | `githubcli-archive-keyring.gpg` + signed apt repository |

**All binaries verified.** No download without a checksum step.

---

## 8.2 GitHub Actions Pin Audit

```
grep -r 'uses:' .github/workflows/
```

All action `uses:` references are pinned to commit SHAs:

| Action | SHA Ref | Comment Tag | Trusted Org? |
|--------|---------|-------------|--------------|
| `actions/checkout` | `34e114876b0b11c390a56381ad16ebd13914f8d5` | v4 | Yes |
| `docker/setup-qemu-action` | `c7c53464625b32c7a7e944ae62b3e17d2b600130` | v3 | Yes |
| `docker/setup-buildx-action` | `8d2750c68a42422c14e847fe6c8ac0403b4cbd6f` | v3 | Yes |
| `docker/login-action` | `c94ce9fb468520275223c153574b00df6fe4bcc9` | v3 | Yes |
| `docker/metadata-action` | `c299e40c65443455700f0fdfc63efafe5b349051` | v5 | Yes |
| `docker/build-push-action` | `ca052bb54ab0790a636c9b5f226502c73d547a25` | v5 | Yes |
| `aquasecurity/trivy-action` | `b2933f565dbc598b29947660e66259e3c7bc8561` | 0.20.0 | Yes |

**Result:** All actions pinned. No unpinned `@main`, `@master`, or floating tag references.

---

## 8.3 Base Image Currency

**Base images:**
```
docker/Dockerfile.agent:  FROM golang:1.25.7-bookworm@sha256:0b5f101af6e4f905...  (age builder)
docker/Dockerfile.agent:  FROM debian:bookworm-slim@sha256:6458e6ce2b6448e31b...  (runtime)
docker/Dockerfile.watcher: FROM golang:1.25.7-bookworm@sha256:0b5f101af6e4f905...  (builder)
docker/Dockerfile.watcher: FROM debian:bookworm-slim@sha256:6458e6ce2b6448e31b...  (runtime)
```

Both Dockerfiles pin base images to digests, not just tags. Go builder uses 1.25.7.

**Trivy scan:** SKIPPED — local Docker build not executed. CI Trivy scan runs on every tag push.

**`.trivyignore` review:**
8 CVE suppressions present for third-party tool binaries (helm, kubectl, kustomize, stern, flux, talosctl, sops, yq) that have not yet been released with Go 1.25.7. Suppressions have expiry `2026-06-01` and are documented with reasoning. No suppressions for code owned by mendabot.

---

## 8.4 Go Module Integrity

```
go mod verify → all modules verified
```

**Result:** PASS.

**Note:** `golang.org/x/net v0.30.0` has 4 CVEs in the module tree (per Phase 1 govulncheck). Module is not directly used in code paths calling vulnerable functions, per govulncheck analysis. Upgrade to v0.45.0 recommended.

---

## Phase 8 Summary

Supply chain integrity is strong. All binary downloads are checksum-verified. All GitHub Actions are SHA-pinned. Base images are digest-pinned. Go modules verified. Trivy scan runs in CI on every release tag.

**Total findings:** 0
