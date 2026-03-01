# Worklog: Epic 29 STORY_00 — kubectl Tier 1 Write Blocking

**Date:** 2026-02-28
**Session:** Implement always-on kubectl write-subcommand blocking in the redact wrapper
**Status:** Complete

---

## Objective

Add Tier 1 always-on write-subcommand blocking to `docker/scripts/redact-wrappers/kubectl`
so that the agent cannot mutate cluster state via kubectl regardless of RBAC, and extend
`docker/scripts/wrapper-test.sh` with test cases that verify blocked and non-blocked calls.

---

## Work Completed

### 1. kubectl wrapper — Tier 1 write-block logic

- Updated `docker/scripts/redact-wrappers/kubectl` to insert write-block logic between
  the `mktemp`/`trap` setup and the `kubectl.real` invocation.
- The block extracts `$1` as `_subcmd` and uses a `case` statement covering all 14 flat
  write subcommands plus a nested `case` on `${2:-}` for `rollout restart` and `rollout undo`.
- Blocked calls write `[KUBECTL] kubectl $* blocked — write operations are not permitted
  in the mendabot agent` to stderr and exit 1.
- The `redact` pipeline, `mktemp` setup, and exit-code propagation are all unchanged.
- Updated the shebang comment to reflect the new blocking capability.

### 2. wrapper-test.sh — write-block test cases

- Extended `docker/scripts/wrapper-test.sh` with two new helpers:
  - `check_write_blocked`: asserts exit 1 + `[KUBECTL]` message in output
  - `check_write_allowed`: asserts exit 0 + no `[KUBECTL]` message
- Both helpers stub `kubectl.real` (exits 0) and `redact` (cat passthrough) via
  `/tmp/stub/` injected at the front of PATH — matching the existing `check_exit_code`
  pattern in the file.
- 16 blocked subcommand tests: apply, create, delete, edit, patch, replace, scale, set,
  label, annotate, taint, drain, cordon, uncordon, rollout restart, rollout undo.
- 6 read pass-through tests: get pods, describe deployment, logs, diff, rollout status,
  rollout history.

### 3. shellcheck validation

- `shellcheck docker/scripts/redact-wrappers/kubectl` — zero errors/warnings (via Docker).
- `shellcheck docker/scripts/wrapper-test.sh` — zero errors/warnings (via Docker).

---

## Key Decisions

- **Placement of block logic:** After `mktemp`/`trap` and before `kubectl.real`, as
  specified in the story. The `_tmpfile` is created before the block check because STORY_01
  will add Tier 2 logic in the same position; keeping the trap setup early avoids a leaked
  tmpfile edge case if `mktemp` succeeds but the block check code path hits an error.
- **No `set -e`:** Consistent with all other wrappers — not added.
- **stderr only for block message:** Preserves stdout cleanliness for the redact pipeline.
  The `_tmpfile` is empty on blocked calls; `trap` cleans it up on exit.
- **Stub pattern in tests:** `kubectl.real` stubbed to exit 0 and `redact` stubbed as `cat`
  so that for non-blocked calls the wrapper completes successfully with exit 0, making it
  straightforward to distinguish blocked (exit 1 + `[KUBECTL]`) from allowed (exit 0).

---

## Blockers

None.

---

## Tests Run

```
shellcheck docker/scripts/redact-wrappers/kubectl   → exit 0 (zero errors)
shellcheck docker/scripts/wrapper-test.sh            → exit 0 (zero errors)
```

Full functional test of wrapper-test.sh requires a built agent image (Docker). The
structural and logic correctness has been verified by shellcheck and code review.

---

## Next Steps

STORY_01 — kubectl Tier 2 hardened mode + sentinel detection + Helm flag. This extends
the same wrapper file, inserting Tier 2 logic after the Tier 1 block.

Prerequisite: STORY_04 (config + jobbuilder wiring) should be completed before STORY_01.

---

## Files Modified

- `docker/scripts/redact-wrappers/kubectl` — added Tier 1 write-block logic (lines 13–34)
- `docker/scripts/wrapper-test.sh` — added `check_write_blocked`, `check_write_allowed`
  helpers and 22 test cases (16 blocked + 6 pass-through)
- `docs/WORKLOGS/0094_2026-02-28_epic29-story00-kubectl-write-block.md` — this file
