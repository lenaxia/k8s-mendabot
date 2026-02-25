# Story 01: get-github-app-token.sh — Write Expiry Timestamp Alongside Token

**Epic:** epic22-token-expiry-guard (FT-R3)
**Status:** Not Started

---

## Context

`get-github-app-token.sh` is the script that mints a GitHub App installation token. It
runs inside the `git-token-clone` init container. Today it only prints the raw token
string to stdout; the caller (`initScript` in `internal/jobbuilder/job.go`) captures that
output and writes it to `/workspace/github-token`.

The script already computes `NOW=$(date +%s)` at line 8 to build the JWT. GitHub App
installation tokens are valid for **one hour (3600 s)**. A 3500 s guard window is used so
that a token received near the end of its lifetime does not pass the write step but then
expire before the main container checks it.

The expiry timestamp must be written **as a side effect on disk** by the same script that
knows when `NOW` was captured. This avoids any clock skew between the write of the token
file and a separate timestamp computation in the caller.

---

## What does the script do today?

Full current file (`docker/scripts/get-github-app-token.sh`):

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_APP_ID:?GITHUB_APP_ID must be set}"
: "${GITHUB_APP_INSTALLATION_ID:?GITHUB_APP_INSTALLATION_ID must be set}"
: "${GITHUB_APP_PRIVATE_KEY:?GITHUB_APP_PRIVATE_KEY must be set}"

NOW=$(date +%s)                               # <-- line 8: epoch seconds
IAT=$((NOW - 60))
EXP=$((NOW + 540))

b64url() { base64 -w0 | tr '+/' '-_' | tr -d '='; }

HEADER=$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)
# iss must be a JSON number (integer), not a string
PAYLOAD=$(printf '{"iat":%d,"exp":%d,"iss":%d}' "$IAT" "$EXP" "$GITHUB_APP_ID" | b64url)
UNSIGNED="${HEADER}.${PAYLOAD}"

KEY_FILE=$(mktemp)
trap 'rm -f "$KEY_FILE"' EXIT
printf '%s' "$GITHUB_APP_PRIVATE_KEY" > "$KEY_FILE"

SIGNATURE=$(printf '%s' "$UNSIGNED" \
  | openssl dgst -sha256 -sign "$KEY_FILE" \
  | b64url)

JWT="${UNSIGNED}.${SIGNATURE}"

curl -sf \
  -X POST \
  -H "Authorization: Bearer ${JWT}" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/app/installations/${GITHUB_APP_INSTALLATION_ID}/access_tokens" \
  | jq -r '.token'          # <-- line 37: prints token to stdout, nothing written to disk
```

**Key facts:**
- `NOW` is already available at line 8 — no additional `date` call needed.
- The final `jq -r '.token'` at line 37 pipes the GitHub API response and prints the token
  string to stdout. The caller in `job.go` captures this via `TOKEN=$(get-github-app-token.sh)`
  (inside the `initScript` constant at `job.go:47–58`, specifically line 50).
- No file is written anywhere inside this script today.
- The `/workspace` volume (an `emptyDir`) is declared at `internal/jobbuilder/job.go:188–193`
  and mounted at `/workspace` for the init container at `job.go:111–116`. It is writable
  when this script runs.

---

## What needs to change

Add two lines **after** the curl pipeline, replacing the bare `jq -r '.token'` with a form
that (a) captures the API response, (b) writes the expiry file, and then (c) prints the
token to stdout.

### Exact change — diff format

```diff
-curl -sf \
-  -X POST \
-  -H "Authorization: Bearer ${JWT}" \
-  -H "Accept: application/vnd.github+json" \
-  -H "X-GitHub-Api-Version: 2022-11-28" \
-  "https://api.github.com/app/installations/${GITHUB_APP_INSTALLATION_ID}/access_tokens" \
-  | jq -r '.token'
+RESPONSE=$(curl -sf \
+  -X POST \
+  -H "Authorization: Bearer ${JWT}" \
+  -H "Accept: application/vnd.github+json" \
+  -H "X-GitHub-Api-Version: 2022-11-28" \
+  "https://api.github.com/app/installations/${GITHUB_APP_INSTALLATION_ID}/access_tokens")
+
+# Write expiry timestamp: NOW (captured at JWT mint time) + 3500 s.
+# GitHub installation tokens are valid for 3600 s; the 3500 s window gives the
+# main container a 100 s head-start before the pre-flight guard triggers.
+printf '%d' "$((NOW + 3500))" > /workspace/github-token-expiry
+
+printf '%s\n' "$(printf '%s' "$RESPONSE" | jq -r '.token')"
```

### Full file after the change

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_APP_ID:?GITHUB_APP_ID must be set}"
: "${GITHUB_APP_INSTALLATION_ID:?GITHUB_APP_INSTALLATION_ID must be set}"
: "${GITHUB_APP_PRIVATE_KEY:?GITHUB_APP_PRIVATE_KEY must be set}"

NOW=$(date +%s)
IAT=$((NOW - 60))
EXP=$((NOW + 540))

b64url() { base64 -w0 | tr '+/' '-_' | tr -d '='; }

HEADER=$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)
# iss must be a JSON number (integer), not a string — GitHub rejects string iss values.
PAYLOAD=$(printf '{"iat":%d,"exp":%d,"iss":%d}' "$IAT" "$EXP" "$GITHUB_APP_ID" | b64url)
UNSIGNED="${HEADER}.${PAYLOAD}"

# Write the private key to a temp file to avoid process substitution (<(…)) which
# requires /dev/fd and may not be available in hardened container environments.
KEY_FILE=$(mktemp)
trap 'rm -f "$KEY_FILE"' EXIT
printf '%s' "$GITHUB_APP_PRIVATE_KEY" > "$KEY_FILE"

SIGNATURE=$(printf '%s' "$UNSIGNED" \
  | openssl dgst -sha256 -sign "$KEY_FILE" \
  | b64url)

JWT="${UNSIGNED}.${SIGNATURE}"

RESPONSE=$(curl -sf \
  -X POST \
  -H "Authorization: Bearer ${JWT}" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/app/installations/${GITHUB_APP_INSTALLATION_ID}/access_tokens")

# Write expiry timestamp: NOW (captured at JWT mint time) + 3500 s.
# GitHub installation tokens are valid for 3600 s; the 3500 s window gives the
# main container a 100 s head-start before the pre-flight guard triggers.
printf '%d' "$((NOW + 3500))" > /workspace/github-token-expiry

printf '%s\n' "$(printf '%s' "$RESPONSE" | jq -r '.token')"
```

---

## Why this approach

| Decision | Rationale |
|----------|-----------|
| Reuse `NOW` from line 8 | `NOW` is already set. Using the same value means the expiry timestamp is consistent with the JWT that was just minted — no second clock read. |
| `NOW + 3500` not `NOW + 3600` | 100 s safety margin. The token is valid for 3600 s but the guard in STORY_02 also checks `EXPIRY - 60`, so the combined guard fires at `NOW_at_mint + 3500 - 60 = NOW_at_mint + 3440 s` before the token actually expires. |
| Write expiry before printing token | If the write fails (e.g. disk full, path not writable) `set -euo pipefail` aborts the script and `printf` never runs, so the caller never gets a token. This is intentionally fail-fast: better to surface a disk error than to have the main container start with a token but no expiry file. |
| Capture full API response into `RESPONSE` | Allows future stories to extract `expires_at` from the response JSON if needed without a second API call. |
| `printf '%d'` for the expiry file | Writes a plain integer with no trailing newline issues; `%d` enforces numeric format. |
| `printf '%s\n'` for stdout | Matches the shell idiom used elsewhere in the project; ensures the caller's `TOKEN=$(...)` captures a clean string. |

---

## File written

| Path | Content | Example |
|------|---------|---------|
| `/workspace/github-token-expiry` | Unix timestamp (integer, no newline) | `1740350700` |

The `/workspace` path is the `emptyDir` volume named `shared-workspace` declared in
`internal/jobbuilder/job.go:188–193` and mounted at `/workspace` for the init container
at `job.go:111–116`.

---

## Tools required

All tools used in this change are already present in `docker/Dockerfile.agent`:

| Tool | Dockerfile source |
|------|-------------------|
| `date +%s` | `bash` package (Debian coreutils), line 27 |
| `jq` | installed explicitly, line 33 |
| `printf` | bash built-in |

No Dockerfile changes are needed.

---

## Acceptance criteria

- [ ] After the init container runs, `/workspace/github-token-expiry` exists and contains
      a decimal integer equal to `(epoch seconds at token mint time) + 3500`.
- [ ] `/workspace/github-token` still contains the raw token string (caller behaviour in
      `job.go` is unchanged).
- [ ] If the curl call fails (non-zero exit), the script exits non-zero and neither file is
      written (because `set -euo pipefail` aborts before the `printf` lines).
- [ ] The script continues to print exactly the token string to stdout so the existing
      caller `TOKEN=$(get-github-app-token.sh)` in `job.go:50` is unaffected.
