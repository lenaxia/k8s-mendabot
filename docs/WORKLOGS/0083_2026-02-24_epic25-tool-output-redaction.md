# Worklog: Epic 25 — Tool Call Output Redaction

**Date:** 2026-02-24
**Session:** Implement all five stories of epic25: cmd/redact binary, shell wrappers, Dockerfile.agent integration, wrapper integration tests, security documentation
**Status:** Complete

---

## Objective

Implement the tool-call output redaction layer for the mendabot-agent image. All tool
output from the LLM agent (kubectl, helm, flux, gh, and 8 more) must pass through the
`redact` filter before being returned to the LLM context, preventing raw credentials
from reaching the external LLM API.

---

## Work Completed

### 1. cmd/redact filter binary (STORY_01)

`cmd/redact/main.go` — standalone Go filter binary that reads all of stdin, applies
`domain.RedactSecrets`, writes redacted output to stdout. Exits 0 on success, exits 1
on I/O error. Imports `internal/domain` directly — same compiled regex patterns as the
source-level redaction, zero drift.

`cmd/redact/main_test.go` — 13 table-driven test cases using `run(io.Reader, io.Writer)`
directly (no process spawning, no `os.Exit` risk). All 13 pass under `go test -race`.

Key TDD fix applied during review: the struct field `wantExact string` was guarded by
`if tt.wantExact != ""` — this made the "Empty input" case vacuously true (the assertion
was never executed). Fixed by adding `wantExactSet bool` sentinel field; all exact-match
cases now set `wantExactSet: true`.

Additional fix: the story spec's 13th test case used `dGhpcyBpcyBhIHNlY3JldA==AAAAAAAAAAAAAAAAAAA`
which contains `==` in the middle — this breaks the base64 run into two segments both
under 40 chars, so the regex correctly does not match. Test updated to use a valid
44-char single-run base64 value: `dGhpc2lzYXNlY3JldHZhbHVlMTIzNDU2Nzg5MGFiY2Q=`.

### 2. Shell wrapper scripts (STORY_02)

12 wrapper scripts in `docker/scripts/redact-wrappers/`:
`kubectl`, `helm`, `flux`, `gh`, `sops`, `talosctl`, `yq`, `stern`,
`kubeconform`, `kustomize`, `age`, `age-keygen`.

All wrappers: hard-fail guard if `redact` not in PATH, `mktemp` failure guard, `trap`
for temp file cleanup, `2>&1` merge, `_rc=$?` capture, `exit $_rc` passthrough. No
`set -e`. `gh` calls `/usr/bin/gh` by absolute path. All others call `<tool>.real`.

### 3. Dockerfile.agent integration (STORY_03)

Three changes to `docker/Dockerfile.agent`:
- `redact-builder` stage added after `age-builder`, before runtime image. Uses identical
  Go image digest (`sha256:0b5f101af6e4f905da4e1c5885a76b1e7a9fbc97b1a41d971dbcab7be16e70a1`).
  `COPY go.mod go.sum` first (cache layer), then `COPY . .`.
- `RUN test -x /usr/bin/gh` assertion added after `gh` apt install block.
- Rename+copy block after `opencode` install, before `# Non-root user`: 11 `.real`
  renames (all tools except `gh`) then `COPY --chmod=755` for `redact` binary and all
  12 wrapper scripts. No `ENV PATH` change needed.

### 4. Wrapper integration tests (STORY_04)

`docker/scripts/wrapper-test.sh` — post-build CI script that verifies:
- `redact` binary present and executable
- Functional redaction: `password=hunter2` redacted, `ghs_` token redacted,
  `CrashLoopBackOff` not falsely redacted
- All 12 wrappers present and executable, all 11 `.real` binaries present,
  `/usr/bin/gh` present
- Structural checks: all wrappers contain `trap`, `_rc=$?`, `redact < `, `command -v redact`
- Exit code passthrough: all 11 PATH-interceptable tools tested with a stub exiting 42

`docker/scripts/smoke-test.sh` — one line added: `check_binary redact`.

`.github/workflows/build-agent.yaml` — "Wrapper test" step added after "Smoke test" step.

### 5. Security documentation (STORY_05)

- `docs/SECURITY/2026-02-24_pentest_report/findings.md` — P-010 (HIGH, Remediated)
  added. Header updated: 15 total findings, 3 HIGH. Heading format fixed to match file
  convention (`2026-02-24-P-010:`).
- `docs/SECURITY/2026-02-24_pentest_report/phase03_redaction.md` — epic25 addendum appended.
- `docs/SECURITY/2026-02-24_security_report/phase03_redaction.md` — epic25 addendum appended.
- `docs/SECURITY/THREAT_MODEL.md` — AV-02 updated with tool-call output path and epic25 mitigation.

---

## Key Decisions

**`wantExactSet bool` sentinel instead of `wantExact != ""` guard:** The `!= ""` guard
on the exact-match assertion made the "Empty input" test vacuously true. Using a separate
boolean field is the only correct fix — it makes the intent explicit and removes the edge
case entirely.

**Mixed-charset base64 test input corrected:** The story spec example `dGhpcyBpcyBhIHNlY3JldA==AAAAAAAAAAAAAAAAAAA`
was a broken test — the `==` in the middle breaks the run into two segments both under
40 chars, neither of which matches the `{40,}` threshold. Replaced with
`dGhpc2lzYXNlY3JldHZhbHVlMTIzNDU2Nzg5MGFiY2Q=` (44 chars, valid single-run base64).
This is the correct test for the mixed-charset case.

**No `shellcheck` in CI:** `shellcheck` is not installed in the dev environment. The
wrapper scripts follow the exact templates from the story spec. A future epic could add
a `shellcheck` step to CI.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./cmd/redact/...
# 13/13 PASS

go test -timeout 60s -race ./...
# 14/14 packages PASS

go build ./...
# Clean

go vet ./cmd/redact/...
# Clean
```

---

## Next Steps

1. Merge `feature/epic25-tool-output-redaction` to `main` when ready.
2. Build and push agent image to trigger wrapper-test.sh in CI.
3. Consider adding `shellcheck` to CI as a separate epic.

---

## Files Modified

- `cmd/redact/main.go` — new: filter binary
- `cmd/redact/main_test.go` — new: 13 table-driven tests
- `docker/scripts/redact-wrappers/age` — new
- `docker/scripts/redact-wrappers/age-keygen` — new
- `docker/scripts/redact-wrappers/flux` — new
- `docker/scripts/redact-wrappers/gh` — new
- `docker/scripts/redact-wrappers/helm` — new
- `docker/scripts/redact-wrappers/kubeconform` — new
- `docker/scripts/redact-wrappers/kubectl` — new
- `docker/scripts/redact-wrappers/kustomize` — new
- `docker/scripts/redact-wrappers/sops` — new
- `docker/scripts/redact-wrappers/stern` — new
- `docker/scripts/redact-wrappers/talosctl` — new
- `docker/scripts/redact-wrappers/yq` — new
- `docker/Dockerfile.agent` — redact-builder stage, gh assertion, rename+copy block
- `docker/scripts/wrapper-test.sh` — new
- `docker/scripts/smoke-test.sh` — check_binary redact added
- `.github/workflows/build-agent.yaml` — Wrapper test step added
- `docs/SECURITY/2026-02-24_pentest_report/findings.md` — P-010 added, header updated
- `docs/SECURITY/2026-02-24_pentest_report/phase03_redaction.md` — epic25 addendum
- `docs/SECURITY/2026-02-24_security_report/phase03_redaction.md` — epic25 addendum
- `docs/SECURITY/THREAT_MODEL.md` — AV-02 updated
