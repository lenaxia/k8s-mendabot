# Worklog: Wrap curl/jq/openssl in Main Agent Container

**Date:** 2026-03-02
**Session:** Design and planning for EX-001/EX-005/EX-006 remediation via per-container PATH override
**Status:** In Progress

---

## Objective

Remediate EX-001 (curl → K8s API), EX-005 (jq + curl extraction), and EX-006 (openssl key
material stdout) by wrapping `curl`, `jq`, and `openssl` output through `redact` in the main
agent container, while leaving the init container unaffected so `get-github-app-token.sh`
continues to work.

---

## Work Completed

### 1. Security review and findings triage

Reviewed all 9 entries in `docs/SECURITY/EXFIL_LEAK_REGISTRY.md`. Current state:

| ID | Severity | Status |
|----|----------|--------|
| EX-001 | HIGH | accepted |
| EX-002 | MEDIUM | accepted |
| EX-003 | LOW | accepted |
| EX-004 | LOW | accepted |
| EX-005 | MEDIUM | accepted |
| EX-006 | HIGH | accepted |
| EX-007 | HIGH | remediated |
| EX-008 | HIGH | remediated |
| EX-009 | MEDIUM | remediated |

EX-001, EX-005, and EX-006 were accepted due to `curl`, `jq`, `openssl` being needed
unwrapped in `get-github-app-token.sh` (init container). This session designs a way to
close them without breaking the init container.

### 2. Architecture investigation

Read and analysed:
- `docker/Dockerfile.agent` — existing wrapper shadowing mechanism
- `docker/scripts/redact-wrappers/` — all 13 existing wrappers
- `docker/scripts/get-github-app-token.sh` — uses `curl`, `jq`, `openssl`, `base64`, `git`
- `internal/controller/jobbuilder.go` — how init and main containers are constructed
- `docker/scripts/entrypoint-common.sh` and `entrypoint-opencode.sh` — main container setup

Key finding: the existing mechanism renames real binaries to `<tool>.real` and installs
wrapper scripts at `/usr/local/bin/<tool>`. There is no per-container PATH differentiation
today — both init and main containers use the same image PATH.

### 3. Design decision

**Chosen approach: separate wrapper directory + PATH env var on main container only.**

- New wrappers for `curl`, `jq`, `openssl` installed to `/usr/local/bin/agent-wrapped/`
  in the image (real binaries remain at `/usr/bin/curl`, `/usr/bin/jq`, `/usr/bin/openssl` — no renaming)
- Main container gets `PATH=/usr/local/bin/agent-wrapped:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`
  via an env var set in `jobbuilder.go`
- Init container PATH is untouched — resolves real binaries via default Debian PATH
- `cat` is explicitly excluded: used by entrypoint scripts for their own file reads; wrapping
  it would risk silent data corruption or breaking prompt rendering

**Why not rename the real binaries?** Renaming `/usr/bin/curl` → `/usr/bin/curl.real` would
break the init container because `get-github-app-token.sh` calls `curl` by name, and after
rename the only `curl` in PATH would be the wrapper. Keeping real binaries at their original
paths and using a separate overlay directory avoids this entirely.

**Why always-on (not hardened-mode-only)?** Redacting curl/jq/openssl output in the main
container is desirable regardless of dry-run state. A live run that fetches K8s API
responses should redact credentials from those responses just as much as a dry run.

### 4. Scope definition for implementation

Files to create:
- `docker/scripts/redact-wrappers/curl`
- `docker/scripts/redact-wrappers/jq`
- `docker/scripts/redact-wrappers/openssl`

Files to modify:
- `docker/Dockerfile.agent` — add `mkdir /usr/local/bin/agent-wrapped` + 3 COPY lines
- `internal/controller/jobbuilder.go` — add PATH env var on main container spec
- `internal/controller/jobbuilder_test.go` — add tests: main container has agent-wrapped in PATH, init container does not
- `docker/scripts/tests/` — wrapper unit tests for curl/jq/openssl
- `docs/SECURITY/EXFIL_LEAK_REGISTRY.md` — EX-001, EX-005, EX-006 → remediated

### 5. Phase 11 adversarial verification run (EX-009 follow-up)

Before starting this new work, completed the open item from the previous session:

- Applied `agent-prompt-core-redteam` ConfigMap to cluster (in-cluster only previously)
- Ran adversarial Agent B against `ghcr.io/lenaxia/mechanic-agent:v0.3.39`
- `env | sort` output confirmed: `AGENT_PROVIDER_CONFIG` absent, no `sk-`/`apiKey` in env
- EX-009 fix confirmed working
- Updated Phase 11 report with section 11.8 (full run log, per-path verdicts)
- Committed `agent-prompt-core-redteam` ConfigMap YAML to
  `deploy/overlays/security/configmap-agent-prompt-core-redteam.yaml`
- Pushed — blocked by GitHub secret scanning (`ghs_` token in report table), redacted,
  amended commit, re-pushed successfully

---

## Key Decisions

1. **Separate wrapper directory (`/usr/local/bin/agent-wrapped/`) over PATH reorder on init
   container.** The reorder approach (put `/usr/bin` before `/usr/local/bin` on the init
   container) is simpler but affects ALL tools in the init container, not just curl/jq/openssl.
   The separate directory is an explicit opt-in that only activates the three new wrappers
   and leaves everything else unchanged.

2. **Real binaries stay at `/usr/bin/{curl,jq,openssl}` — no renaming.** Renaming would
   break the init container. The wrapper scripts call the real binary by absolute path
   (`/usr/bin/curl "$@"` etc.) so there is no PATH ambiguity.

3. **`cat` not wrapped.** Wrapping `cat` would affect entrypoint scripts that read
   `/tmp/rendered-prompt.txt`, kubeconfig, and other internal files. The risk does not
   justify the operational fragility. EX-002 remains accepted.

4. **Always-on (not keyed to HARDEN_KUBECTL).** The wrappers activate whenever the main
   container runs. No additional flag needed.

---

## Blockers

None.

---

## Tests Run

None yet — implementation not started.

---

## Next Steps

1. Create `docker/scripts/redact-wrappers/curl` — call `/usr/bin/curl "$@"` → tmpfile → `redact`
2. Create `docker/scripts/redact-wrappers/jq` — call `/usr/bin/jq "$@"` → tmpfile → `redact`
3. Create `docker/scripts/redact-wrappers/openssl` — call `/usr/bin/openssl "$@"` → tmpfile → `redact`
4. Edit `docker/Dockerfile.agent`:
   - Add `RUN mkdir -p /usr/local/bin/agent-wrapped` before the COPY block
   - Add three `COPY --chmod=755` lines installing wrappers to `/usr/local/bin/agent-wrapped/`
5. Edit `internal/controller/jobbuilder.go` — in the main container env var block, add:
   `{Name: "PATH", Value: "/usr/local/bin/agent-wrapped:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}`
6. Write tests in `internal/controller/jobbuilder_test.go` FIRST (TDD):
   - Assert main container has `PATH` env var starting with `/usr/local/bin/agent-wrapped`
   - Assert init container does NOT have a `PATH` env var override
7. Write wrapper unit tests (same pattern as existing wrapper tests)
8. Run `go test -timeout 30s -race ./...` — all must pass
9. Update `docs/SECURITY/EXFIL_LEAK_REGISTRY.md`: EX-001, EX-005, EX-006 → remediated
10. Bump minor version, commit, push, tag to trigger CI

---

## Files Modified

- `deploy/overlays/security/configmap-agent-prompt-core-redteam.yaml` — created (committed to source control)
- `docs/SECURITY/2026-03-02_security_report/phase11_exfil.md` — added section 11.8, updated summary table
