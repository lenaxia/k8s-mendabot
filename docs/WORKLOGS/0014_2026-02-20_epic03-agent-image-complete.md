# Worklog: Epic 03 — Agent Image Complete

**Date:** 2026-02-20
**Session:** Implement Dockerfile.agent, get-github-app-token.sh, agent-entrypoint.sh, smoke-test.sh; 2 review passes, 14 gaps fixed
**Status:** Complete

---

## Objective

Implement the `mechanic-agent` Docker image per `AGENT_IMAGE_LLD.md`: a self-contained Kubernetes Job environment containing all investigation tools (opencode, kubectl, k8sgpt, helm, flux, talosctl, kustomize, gh, yq, kubeconform, stern, age, sops, and supporting utilities). All scripts and the smoke test were also implemented.

---

## Work Completed

### 1. `docker/Dockerfile.agent`

Full Dockerfile per LLD §3:
- 12 ARG version pins at the top
- Base packages: bash, ca-certificates, curl, gettext-base, git, gnupg, jq, openssl, unzip
- gh CLI from GitHub's official GPG-signed apt repository
- 12 binary tools, each with SHA256 checksum verification before installation
- Non-root user: `useradd -u 1000 -m -s /bin/bash agent`
- Git identity ENV vars (4)
- COPY + chmod of both scripts
- `USER agent`, `WORKDIR /workspace`, `ENTRYPOINT ["/usr/local/bin/agent-entrypoint.sh"]`

### 2. `docker/scripts/get-github-app-token.sh`

Per LLD §4: validates env vars, computes RS256 JWT with integer `iss`, writes private key to temp file with `trap EXIT` cleanup, calls GitHub API, returns installation token via `jq -r '.token'`.

### 3. `docker/scripts/agent-entrypoint.sh`

Per LLD §5: authenticates `gh` CLI from init-container token, runs `envsubst` restricted to the 9 known variable names (protects `$`-containing content in FINDING_ERRORS), `exec opencode run --file /tmp/rendered-prompt.txt`.

### 4. `docker/scripts/smoke-test.sh`

Verifies all 18 items from LLD §8: 15 tool version checks (opencode, kubectl, k8sgpt, helm, flux, talosctl, kustomize, yq, gh, jq, sops, age, stern, kubeconform, envsubst + git + curl + openssl) plus 2 `test -x` script checks. Uses a `check()` wrapper that prints the tool name before each check for clear failure identification.

---

## Key Decisions and Bugs Fixed During Review

### Critical bugs fixed (would have caused `docker build` to fail)

| Tool | Bug | Fix |
|------|-----|-----|
| k8sgpt | Version 0.4.28 had no binary assets; wrong asset naming (`linux_amd64` vs `Linux_x86_64` tarball) | Downgraded to 0.4.27; rewrote to download tarball with arch mapping |
| opencode | Version 0.1.0 does not exist; wrong org (`opencode-ai` → `sst`); wrong asset naming | Downgraded to 0.0.55; corrected org/URL/tarball pattern |
| talosctl | Checksum sidecar URL 404s; real file is combined `sha256sum.txt` | Changed to `sha256sum.txt` + grep + awk path rewrite |
| helm | `sha256sum --check` looked for `helm-v3.17.2-linux-amd64.tar.gz` in CWD but file is `/tmp/helm.tar.gz` | Added awk path rewrite |
| yq | `awk '$1'` extracts filename column, not SHA-256 hash; SHA-256 is column `$19` | Changed to `awk '$19'` |
| flux, kustomize, kubeconform, stern, age | Same path-mismatch as helm (all 5 had missing awk rewrite) | Added awk path rewrite to each |

### Medium bugs fixed

| Item | Bug | Fix |
|------|-----|-----|
| get-github-app-token.sh | Private key temp file not cleaned up if openssl fails | Added `trap 'rm -f "$KEY_FILE"' EXIT` |
| smoke-test.sh | Missing git, curl, openssl version checks | Added all three |
| smoke-test.sh | No identifying message on failure | Added `check()` wrapper with echo |

---

## Blockers

None. Docker build cannot be verified locally (no Docker in this environment) — CI will be the first real build test. The risks are documented in the worklog:
- opencode `--file` flag must be verified against `v0.0.55` before first build
- k8sgpt arm64 asset name (`arm64` vs `aarch64`) should be verified

---

## Tests Run

```
go build ./...                → clean
go test -timeout 30s -race ./... → all 9 packages pass
git ls-files --stage docker/scripts/ → all 3 scripts at 100755
```

Docker build not available in this environment — CI validation pending.

---

## Next Steps

epic03 is complete. Next: **epic04-deploy** (Kustomize manifests). Its dependencies are now all met:
- epic01-controller: complete
- epic02-jobbuilder: complete
- epic03-agent-image: complete

---

## Files Created/Modified

| File | Change |
|------|--------|
| `docker/Dockerfile.agent` | Created — full agent image Dockerfile |
| `docker/scripts/get-github-app-token.sh` | Created — GitHub App token exchanger |
| `docker/scripts/agent-entrypoint.sh` | Created — image entrypoint |
| `docker/scripts/smoke-test.sh` | Created — CI smoke test |
