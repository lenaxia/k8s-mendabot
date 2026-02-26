# Epic 22: GitHub App Token Expiry Guard

**Feature Tracker:** FT-R3
**Area:** Reliability

## Purpose

Make the agent fail fast with a clear error message when the GitHub App installation
token has expired (or is about to expire) before the main container starts, rather than
failing mid-investigation with opaque GitHub 401 errors after a 15-minute timeout.

The init container already calls `get-github-app-token.sh` and writes the token to
`/workspace/github-token`. This epic adds a companion expiry file and a pre-flight check
in `entrypoint-common.sh`.

This is a **shell script change only** — zero Go code.

## Status: Complete

## Deep-Dive Findings (2026-02-23)

### `get-github-app-token.sh` — STORY_01
Full current file (`docker/scripts/get-github-app-token.sh`):
- `NOW=$(date +%s)` is **already computed at line 8** to build the JWT — no second
  `date` call needed for the expiry timestamp.
- Final line: `| jq -r '.token'` — pipes the GitHub API response to stdout; nothing
  is written to disk today.
- `/workspace` (the `emptyDir` volume at `job.go:188–193`) is writable when this script
  runs inside the init container.

**Change:** Capture the `curl` response into `RESPONSE`, write
`printf '%d' "$((NOW + 3500))" > /workspace/github-token-expiry`, then print the token
to stdout. The expiry file write is placed **before** the stdout print so that
`set -euo pipefail` will abort before the token is emitted if the write fails
(fail-fast on disk error).

**Why 3500 s, not 3600 s:** 100-second safety margin. STORY_02 checks `EXPIRY - 60`,
so the combined guard triggers at `NOW_at_mint + 3440 s` — well before true expiry.

### `entrypoint-common.sh` — STORY_02
Current authentication section (lines 75–82):
```bash
gh auth login --with-token < /workspace/github-token
if ! gh auth status > /dev/null 2>&1; then
    echo "ERROR: gh authentication failed — check /workspace/github-token" >&2
    exit 1
fi
```

**Insertion point:** immediately before `gh auth login` (between line 71 and line 75),
after the kubeconfig `fi` block (kubeconfig section closes at line 71).

**Logic:**
```
EXPIRY_FILE=/workspace/github-token-expiry
file exists?
  NO  → print WARNING to stderr, continue (backwards-compatible)
  YES → read EXPIRY; NOW=$(date +%s)
        NOW >= EXPIRY - 60?
          YES → print ERROR with EXPIRY and NOW values, exit 1
          NO  → continue to gh auth login
```

**Error message format (unambiguous for kubectl logs):**
```
ERROR: GitHub App token is expired or expiring imminently.
  EXPIRY=1740350700  NOW=1740350650  (threshold: EXPIRY-60=1740350640)
  Re-queue the RemediationJob to obtain a fresh token.
```

**Backwards compatibility:** if the init container pre-dates STORY_01 (no expiry file
written), the missing-file path prints a WARNING and continues — the existing
`gh auth status` check still catches a truly bad token.

**Why `exit 1` causes Job failure:** Job has `restartPolicy: Never` and `backoffLimit: 1`
(`job.go:260` and `job.go:266`). Non-zero exit → pod `Failed` → Job `Failed` after one retry.

No Dockerfile changes needed — all tools (`date`, `cat`, arithmetic `$(( ))`) are
bash built-ins or coreutils already present.

## Dependencies

- epic03-agent-image complete (`docker/scripts/get-github-app-token.sh`, `docker/scripts/entrypoint-common.sh`)

## Blocks

- Nothing

## Stories

| Story | File | Status |
|-------|------|--------|
| get-github-app-token.sh — write expiry timestamp alongside token | [STORY_01_token_expiry_file.md](STORY_01_token_expiry_file.md) | Complete |
| entrypoint-common.sh — pre-flight expiry check | [STORY_02_entrypoint_check.md](STORY_02_entrypoint_check.md) | Complete |

## Implementation Order

```
STORY_01 (write expiry file) ──> STORY_02 (entrypoint check)
```

## Definition of Done

- [x] `get-github-app-token.sh` captures API response into `RESPONSE`; writes `$((NOW + 3500))` to `/workspace/github-token-expiry` before printing token to stdout
- [x] Write of expiry file precedes stdout print (fail-fast on disk error via `set -euo pipefail`)
- [x] `entrypoint-common.sh` pre-flight block inserted between kubeconfig `fi` (line 71) and `gh auth login` (line 75)
- [x] When expiry file absent: WARNING to stderr, continue (backwards-compatible)
- [x] When `NOW >= EXPIRY - 60`: ERROR with EXPIRY and NOW values, `exit 1`
- [x] `exit 1` causes `batch/v1 Job` to enter `Failed` state
- [x] No LLM API calls when guard fires (`exec opencode` never reached)
- [x] Error message in `kubectl logs` is unambiguous (includes both timestamps)
- [x] Worklog written
