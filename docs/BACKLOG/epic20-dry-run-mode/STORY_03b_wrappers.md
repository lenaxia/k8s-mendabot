# Story: Enforcement Wrappers — gh and git dry-run blocking

**Epic:** [epic20-dry-run-mode](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator**, I want `gh pr create`, `gh pr comment`, `git push`, and `git commit`
to be physically blocked when `DRY_RUN=true`, regardless of what the LLM decides to do,
so that dry-run mode is enforced deterministically rather than relying on prompt compliance.

---

## Background

### Why this story exists

The prompt HARD RULE 11 (STORY_03) tells the LLM it is in dry-run mode. This is a
probabilistic control — it relies on the LLM following the instruction. It is equivalent to
AR-06 in `docs/SECURITY/THREAT_MODEL.md`: "HARD RULEs are prompt instructions, not technical
controls."

The existing wrapper infrastructure (`docker/scripts/redact-wrappers/`) was built for
credential redaction (AV-02 in the threat model). The same mechanism is the right layer for
dry-run enforcement: the wrappers shadow the real binaries via PATH, intercept every call, and
can gate on environment variables before delegating.

See `docs/SECURITY/THREAT_MODEL.md` AV-13 (added in this epic) for the threat this story
mitigates.

### Existing `gh` wrapper

`docker/scripts/redact-wrappers/gh` already intercepts every `gh` call for output redaction.
Extending it to also block write subcommands when `DRY_RUN=true` adds ~10 lines and requires
no new infrastructure.

The `gh` wrapper calls `/usr/bin/gh` directly (gh is installed by apt to `/usr/bin/gh` and
is not renamed — see `Dockerfile.agent:169`). It does **not** need to be renamed.

### `git` — deliberately not wrapped in epic12/epic25

`THREAT_MODEL.md` "Tools deliberately NOT wrapped" explains: git was not wrapped because
**output redaction would break diff-based PR workflows**. That reason does not apply to a
wrapper that only blocks write *subcommands* and passes all read-only subcommands through
unmodified, without touching stdout. This story adds a targeted dry-run-only gate, not
output redaction.

`git` is installed by apt at `/usr/bin/git`. The Dockerfile rename+COPY pattern used for
all other tools applies here: rename `/usr/bin/git` to `/usr/bin/git.real` and install the
wrapper at `/usr/local/bin/git` (which takes precedence in PATH over `/usr/bin/git`).

### Exit code convention for blocked commands

When a write command is blocked in dry-run mode, the wrapper exits **0** (not 1). This is
intentional: the LLM sees a clean exit and does not enter an error-recovery loop trying to
diagnose why `gh pr create` failed. A `[DRY_RUN]` message is printed to **stderr** so it
appears in the agent's terminal output and in container logs, but does not pollute stdout
(which the LLM may capture for tool-call return values).

---

## Acceptance Criteria

- [x] `docker/scripts/redact-wrappers/gh` blocks write subcommands when `DRY_RUN=true`
  — exits 0 with `[DRY_RUN] gh <subcommand> blocked` on stderr
- [x] `docker/scripts/redact-wrappers/gh` passes all other calls through to `/usr/bin/gh`
  unchanged (including `gh auth`, `gh pr list`, `gh pr view`, etc.)
- [x] New `docker/scripts/redact-wrappers/git` wrapper exists
- [x] `git` wrapper blocks `push`, `commit`, `tag` (with `-a` or `-s` flags creating
  annotated/signed tags), and `push --force` / `push --force-with-lease` when `DRY_RUN=true`
  — exits 0 with `[DRY_RUN] git <subcommand> blocked` on stderr
- [x] `git` wrapper passes all other subcommands (`clone`, `fetch`, `pull`, `log`, `diff`,
  `show`, `status`, `checkout`, `branch`, `add`, `stash`, `merge`, `rebase`, etc.) through
  to `/usr/bin/git.real` unchanged, including stdout
- [x] `docker/Dockerfile.agent` renames `/usr/bin/git` to `/usr/bin/git.real` and installs
  the wrapper at `/usr/local/bin/git`
- [x] When `DRY_RUN` is unset or `false`, both wrappers behave identically to their
  unmodified pre-epic behaviour
- [x] `docker/scripts/wrapper-test.sh` (or equivalent manual test) covers the blocking
  and pass-through cases

---

## Implementation

### 1. Extend `docker/scripts/redact-wrappers/gh`

Replace the entire file content with the updated version:

```bash
#!/usr/bin/env bash
# gh wrapper — gh is installed by apt to /usr/bin/gh; call it by absolute path.
# Does NOT use set -e: the real binary may exit non-zero legitimately.

if ! command -v redact > /dev/null 2>&1; then
    echo "[ERROR] redact binary not found in PATH — aborting to prevent unredacted output" >&2
    exit 1
fi

# Dry-run enforcement: block write subcommands that create, modify, or comment on
# GitHub resources. This is a deterministic control — the LLM cannot bypass it by
# rephrasing a prompt instruction. Exit 0 so the LLM does not enter an error loop.
if [ "${DRY_RUN:-false}" = "true" ]; then
    case "${1:-}" in
        pr|issue|release|gist)
            case "${2:-}" in
                create|edit|close|merge|delete|comment|reopen|label|lock|unlock|transfer)
                    echo "[DRY_RUN] gh $* blocked — write operations are disabled in dry-run mode" >&2
                    exit 0
                    ;;
            esac
            ;;
    esac
fi

_tmpfile=$(mktemp) || { echo "[ERROR] mktemp failed — aborting" >&2; exit 1; }
trap 'rm -f "$_tmpfile"' EXIT

/usr/bin/gh "$@" > "$_tmpfile" 2>&1
_rc=$?

redact < "$_tmpfile"
_rr=$?
[ "$_rr" -ne 0 ] && exit "$_rr"
exit "$_rc"
```

### 2. Create `docker/scripts/redact-wrappers/git`

New file — no output redaction (see Background). The wrapper only gates write subcommands:

```bash
#!/usr/bin/env bash
# git wrapper — blocks write subcommands when DRY_RUN=true.
# Pass-through for all read-only operations; does not redact stdout.
# Does NOT use set -e: the real binary may exit non-zero legitimately.
#
# git is installed by apt to /usr/bin/git; renamed to /usr/bin/git.real by
# Dockerfile.agent. This wrapper is installed at /usr/local/bin/git which
# takes precedence in PATH.

# Dry-run enforcement: block subcommands that write to the remote or local
# repository history. Exit 0 so the LLM does not enter an error-recovery loop.
if [ "${DRY_RUN:-false}" = "true" ]; then
    case "${1:-}" in
        push|commit)
            echo "[DRY_RUN] git $* blocked — write operations are disabled in dry-run mode" >&2
            exit 0
            ;;
        tag)
            # Block annotated (-a) and signed (-s) tags; allow lightweight tags
            # (used internally by some tools for version queries).
            for _arg in "$@"; do
                case "$_arg" in
                    -a|-s|--annotate|--sign)
                        echo "[DRY_RUN] git $* blocked — write operations are disabled in dry-run mode" >&2
                        exit 0
                        ;;
                esac
            done
            ;;
    esac
fi

exec /usr/bin/git.real "$@"
```

### 3. Update `docker/Dockerfile.agent`

**Add git to the rename block** (the `RUN mv ...` layer, around line 173):

```dockerfile
RUN mv /usr/local/bin/kubectl       /usr/local/bin/kubectl.real       \
    && mv /usr/local/bin/helm        /usr/local/bin/helm.real          \
    && mv /usr/local/bin/flux        /usr/local/bin/flux.real          \
    && mv /usr/local/bin/sops        /usr/local/bin/sops.real          \
    && mv /usr/local/bin/talosctl    /usr/local/bin/talosctl.real      \
    && mv /usr/local/bin/yq          /usr/local/bin/yq.real            \
    && mv /usr/local/bin/stern       /usr/local/bin/stern.real         \
    && mv /usr/local/bin/kubeconform /usr/local/bin/kubeconform.real   \
    && mv /usr/local/bin/kustomize   /usr/local/bin/kustomize.real     \
    && mv /usr/local/bin/age         /usr/local/bin/age.real           \
    && mv /usr/local/bin/age-keygen  /usr/local/bin/age-keygen.real    \
    && mv /usr/bin/git               /usr/bin/git.real
```

**Add git wrapper COPY** (in the `COPY --chmod=755 docker/scripts/redact-wrappers/...` block):

```dockerfile
COPY --chmod=755 docker/scripts/redact-wrappers/git         /usr/local/bin/git
```

> `/usr/local/bin` appears before `/usr/bin` in the default Debian PATH, so
> `/usr/local/bin/git` shadows `/usr/bin/git.real` without any PATH manipulation.

---

## Test Cases

Manual test procedure (run inside the agent container or a shell with the wrapper in PATH):

```bash
# --- gh blocking ---
# Should print [DRY_RUN] to stderr, exit 0
DRY_RUN=true gh pr create --title "test" --body "test"
DRY_RUN=true gh issue create --title "test"
DRY_RUN=true gh pr comment 1 --body "test"

# Should pass through to /usr/bin/gh (will fail with auth error in test, but wrapper exits non-zero)
DRY_RUN=true gh pr list
DRY_RUN=true gh auth status

# Should pass through (DRY_RUN unset)
gh pr list

# --- git blocking ---
# Should print [DRY_RUN] to stderr, exit 0
DRY_RUN=true git push
DRY_RUN=true git push origin main
DRY_RUN=true git commit -m "test"
DRY_RUN=true git tag -a v1.0 -m "test"

# Should pass through (read-only)
DRY_RUN=true git log --oneline -5
DRY_RUN=true git diff HEAD
DRY_RUN=true git status
DRY_RUN=true git show HEAD
DRY_RUN=true git branch -a

# Should pass through (DRY_RUN unset)
git log --oneline -5
```

Add the following test cases to `docker/scripts/wrapper-test.sh` if that file supports
automated testing, or document them as manual verification steps.

---

## Tasks

- [x] Update `docker/scripts/redact-wrappers/gh` with dry-run blocking block
- [x] Create `docker/scripts/redact-wrappers/git` (new file)
- [x] Add `mv /usr/bin/git /usr/bin/git.real` to the Dockerfile rename `RUN` layer
- [x] Add `COPY --chmod=755 docker/scripts/redact-wrappers/git /usr/local/bin/git` to Dockerfile
- [x] Build the agent image locally and run the manual test cases above
- [x] Verify `DRY_RUN=false git push` still reaches the real git binary (pass-through)
- [x] Verify `DRY_RUN=true git log` still returns output (pass-through)

---

## Dependencies

**Depends on:** STORY_02 (the `DRY_RUN=true` env var must be injected into the Job for the
wrappers to read it; the wrappers read `$DRY_RUN` from the environment at call time)
**No Go compilation dependency** — this story only modifies shell scripts and Dockerfile.

---

## Definition of Done

- [x] `docker/scripts/redact-wrappers/gh` has dry-run write-blocking section
- [x] `docker/scripts/redact-wrappers/git` exists and is executable
- [x] `docker/Dockerfile.agent` renames `/usr/bin/git` and installs the wrapper
- [x] Manual test: `DRY_RUN=true git push` exits 0 with `[DRY_RUN]` on stderr
- [x] Manual test: `DRY_RUN=true git log` passes through cleanly
- [x] Manual test: `DRY_RUN=true gh pr create` exits 0 with `[DRY_RUN]` on stderr
- [x] Manual test: `DRY_RUN=false git push` (in a repo with no remote) exits non-zero
  from the real git binary — confirming the wrapper does not block in non-dry-run mode
- [x] `THREAT_MODEL.md` AV-13 references this story as the control
