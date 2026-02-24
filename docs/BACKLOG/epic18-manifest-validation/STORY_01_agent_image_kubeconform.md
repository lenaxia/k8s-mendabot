# STORY_01 — Agent Image: Ensure kubeconform Is Installed

**Epic:** epic18-manifest-validation (FT-A9)
**Status:** Done — no change needed
**Blocked by:** Nothing
**Blocks:** STORY_02 (prompt hard rule depends on kubeconform being present)

---

## Goal

Confirm that `kubeconform` is present in `docker/Dockerfile.agent` so that STORY_02 can
safely mandate its use in a HARD RULE. If it were absent, this story would add it.

---

## Finding: kubeconform Is Already Installed

`kubeconform` **is already present** in `docker/Dockerfile.agent`. No Dockerfile change is
required. This story is closed as done by inspection.

### Evidence — Exact Lines from `docker/Dockerfile.agent`

```dockerfile
# Tool versions — bump these to upgrade
ARG KUBECONFORM_VERSION=0.7.0          # line 10
```

```dockerfile
# kubeconform — with checksums (add retries)         # lines 99–106
RUN curl -fsSL --retry 3 --retry-delay 5 "https://github.com/yannh/kubeconform/releases/download/v${KUBECONFORM_VERSION}/kubeconform-linux-${TARGETARCH}.tar.gz" \
      -o /tmp/kubeconform.tar.gz \
    && curl -fsSL --retry 3 --retry-delay 5 "https://github.com/yannh/kubeconform/releases/download/v${KUBECONFORM_VERSION}/CHECKSUMS" \
      | grep "kubeconform-linux-${TARGETARCH}.tar.gz$" \
      | awk '{print $1 "  /tmp/kubeconform.tar.gz"}' | sha256sum --check \
    && tar -xz -C /usr/local/bin -f /tmp/kubeconform.tar.gz kubeconform \
    && rm /tmp/kubeconform.tar.gz
```

**Version pinned:** `0.7.0`
**Install destination:** `/usr/local/bin/kubeconform`
**Checksum verification:** upstream `CHECKSUMS` file, verified with `sha256sum --check`
**Arch handling:** `${TARGETARCH}` ARG (default `amd64`; supports `arm64` via Docker
`--platform` or `TARGETARCH=arm64` build-arg)

### kubeconform Is Also Listed in the Prompt's Tool Inventory

`deploy/kustomize/configmap-prompt.yaml` line 54 already names `kubeconform` in the
environment section:

```
- All tools available: kubectl, helm, flux, talosctl, kustomize, gh, git,
  jq, yq, kubeconform, stern, sops, age.
```

---

## Install Pattern Reference (for future tools)

The pattern used for kubeconform matches the standard used for flux, talosctl, kustomize,
and stern in the same file:

1. `ARG <TOOL>_VERSION=<pinned-version>` in the version block at the top of the file
2. `curl` the tarball to `/tmp/<tool>.tar.gz` with `--retry 3 --retry-delay 5`
3. `curl` the upstream checksum file, `grep` for the arch-specific filename, pipe to
   `awk '{print $1 "  /tmp/<tool>.tar.gz"}'`, pipe to `sha256sum --check`
4. `tar -xz -C /usr/local/bin -f /tmp/<tool>.tar.gz <tool>`
5. `rm /tmp/<tool>.tar.gz`

The kubeconform upstream uses a file named `CHECKSUMS` (uppercase, no extension) rather
than `checksums.txt` — this is already handled correctly in the existing block.

---

## Smoke Test

### Existing smoke-test.sh

`docker/scripts/smoke-test.sh` currently checks only that `agent-entrypoint.sh` and
`get-github-app-token.sh` are executable. It does **not** yet assert that binary tools
installed to `/usr/local/bin` are present and executable.

### Recommended Addition

Add a `kubeconform --version` check to `docker/scripts/smoke-test.sh` alongside the
other tool checks. The pattern to follow:

```bash
check kubeconform --version
```

Where `check` is the helper already defined in that file:

```bash
check() {
    echo "Checking: $*"
    docker run --rm --entrypoint "" "$IMAGE" "$@"
}
```

Adding `check kubeconform --version` will cause the smoke test to fail immediately if
the binary is missing or broken in the built image.

This addition is a **separate, small task** — it does not block STORY_02 because the
binary is already confirmed present.

---

## Definition of Done

- [x] `docker/Dockerfile.agent` includes `kubeconform` at version `0.7.0` (lines 10, 99–106)
- [x] Install uses SHA256 verification against the upstream `CHECKSUMS` file
- [x] Binary lands at `/usr/local/bin/kubeconform`
- [x] Arch-portable via `${TARGETARCH}`
- [ ] (Optional follow-up) `docker/scripts/smoke-test.sh` extended with `check kubeconform --version`

---

## Worklog

| Date | Action |
|------|--------|
| 2026-02-23 | Inspected `docker/Dockerfile.agent`; confirmed kubeconform v0.7.0 installed at lines 99–106; story closed as no-change-needed |
