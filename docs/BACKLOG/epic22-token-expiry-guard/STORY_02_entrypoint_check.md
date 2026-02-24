# Story 02: agent-entrypoint.sh — Pre-flight Expiry Check

**Epic:** epic22-token-expiry-guard (FT-R3)
**Status:** Not Started
**Depends on:** STORY_01_token_expiry_file.md

---

## Context

`agent-entrypoint.sh` is the main container's entrypoint. Today it reads the token written
by the init container at line 93:

```bash
gh auth login --with-token < /workspace/github-token
```

If the init container was slow and the token is already old by the time the main container
starts (e.g. pod was pending in a queue), `gh` authentication will appear to succeed but
any subsequent `gh pr list` or `git push` will receive a GitHub 401. The agent then runs
for up to the full `activeDeadlineSeconds` (900 s per `job.go:258`) before the job times
out, burning LLM tokens with no chance of success.

STORY_01 adds `/workspace/github-token-expiry` (a Unix timestamp). This story adds a
pre-flight check that reads that file and exits 1 immediately if the token is expired or
within 60 seconds of expiry, before any `gh` auth or LLM work begins.

---

## What does the entrypoint do today?

Relevant section of `docker/scripts/agent-entrypoint.sh` (lines 90–97):

```bash
# Authenticate gh CLI using the token written by the init container.
# Validate that authentication succeeds — a bad token would otherwise only be
# discovered mid-investigation when gh pr list fails.
gh auth login --with-token < /workspace/github-token
if ! gh auth status > /dev/null 2>&1; then
    echo "ERROR: gh authentication failed — check /workspace/github-token" >&2
    exit 1
fi
```

The `gh auth login` at line 93 is the **first line that does real work** — everything
before it (lines 1–88) is env validation and kubeconfig construction that does not touch
GitHub.

---

## What needs to change

Insert a pre-flight block **immediately before** the `gh auth login` call (before line 93).
The insertion point is after the kubeconfig section ends (line 88) and before the comment
on line 90.

### Exact change — diff format

```diff
 # Authenticate gh CLI using the token written by the init container.
 # Validate that authentication succeeds — a bad token would otherwise only be
 # discovered mid-investigation when gh pr list fails.
+
+# Pre-flight: check that the GitHub App token has not expired (or is not about
+# to expire within the next 60 seconds).  The expiry file is written by the init
+# container via get-github-app-token.sh.  If the file is absent (e.g. an older
+# init container image that pre-dates STORY_01), emit a warning and continue —
+# the existing gh auth status check below still catches a truly bad token.
+EXPIRY_FILE=/workspace/github-token-expiry
+if [ -f "$EXPIRY_FILE" ]; then
+    EXPIRY=$(cat "$EXPIRY_FILE")
+    NOW=$(date +%s)
+    if [ "$NOW" -ge "$((EXPIRY - 60))" ]; then
+        echo "ERROR: GitHub App token is expired or expiring imminently." >&2
+        echo "  EXPIRY=${EXPIRY}  NOW=${NOW}  (threshold: EXPIRY-60=$((EXPIRY - 60)))" >&2
+        echo "  Re-queue the RemediationJob to obtain a fresh token." >&2
+        exit 1
+    fi
+else
+    echo "WARNING: /workspace/github-token-expiry not found — skipping expiry pre-flight check." >&2
+fi
+
 gh auth login --with-token < /workspace/github-token
```

### Full modified section (lines 88–121 after the change)

```bash
fi

# Pre-flight: check that the GitHub App token has not expired (or is not about
# to expire within the next 60 seconds).  The expiry file is written by the init
# container via get-github-app-token.sh.  If the file is absent (e.g. an older
# init container image that pre-dates STORY_01), emit a warning and continue —
# the existing gh auth status check below still catches a truly bad token.
EXPIRY_FILE=/workspace/github-token-expiry
if [ -f "$EXPIRY_FILE" ]; then
    EXPIRY=$(cat "$EXPIRY_FILE")
    NOW=$(date +%s)
    if [ "$NOW" -ge "$((EXPIRY - 60))" ]; then
        echo "ERROR: GitHub App token is expired or expiring imminently." >&2
        echo "  EXPIRY=${EXPIRY}  NOW=${NOW}  (threshold: EXPIRY-60=$((EXPIRY - 60)))" >&2
        echo "  Re-queue the RemediationJob to obtain a fresh token." >&2
        exit 1
    fi
else
    echo "WARNING: /workspace/github-token-expiry not found — skipping expiry pre-flight check." >&2
fi

# Authenticate gh CLI using the token written by the init container.
# Validate that authentication succeeds — a bad token would otherwise only be
# discovered mid-investigation when gh pr list fails.
gh auth login --with-token < /workspace/github-token
if ! gh auth status > /dev/null 2>&1; then
    echo "ERROR: gh authentication failed — check /workspace/github-token" >&2
    exit 1
fi

# Substitute environment variables into the prompt template.
# envsubst only replaces ${VAR} patterns it knows about. To avoid corrupting
# content in FINDING_ERRORS or FINDING_DETAILS that may contain literal $ signs
# (e.g. from Helm templates or shell variables in log output), we restrict
# envsubst to only the known variable names.
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}${IS_SELF_REMEDIATION}${CHAIN_DEPTH}${TARGET_REPO_OVERRIDE}'
envsubst "$VARS" < /prompt/prompt.txt > /tmp/rendered-prompt.txt

# Log self-remediation context if applicable
if [ "$IS_SELF_REMEDIATION" = "true" ]; then
    echo "=== SELF-REMEDIATION MODE ==="
    echo "Chain depth: $CHAIN_DEPTH"
    echo "Target repo override: ${TARGET_REPO_OVERRIDE:-<none>}"
    if [ "$CHAIN_DEPTH" -gt 2 ]; then
        echo "WARNING: Deep cascade detected (depth > 2). Proceeding with caution."
    fi
fi

# Run opencode with the rendered prompt. The prompt is passed as a single
# quoted string argument — word-splitting is not a concern because the shell
# expands "$(cat ...)" as one argument to `opencode run`.
exec opencode run "$(cat /tmp/rendered-prompt.txt)"
```

---

## Exact insertion point

| Where | Line reference (before this change) |
|-------|-------------------------------------|
| After kubeconfig block closes (`fi`, line 88) | `agent-entrypoint.sh:88` |
| Before the `# Authenticate gh CLI` comment | `agent-entrypoint.sh:90` |

The block is inserted between the closing `fi` of the kubeconfig section and the existing
authentication comment. No other lines move.

---

## Logic walkthrough

```
EXPIRY_FILE=/workspace/github-token-expiry

file exists?
  NO  → print WARNING to stderr, continue (backwards-compatible)
  YES → read EXPIRY (integer from file)
        compute NOW=$(date +%s)
        NOW >= EXPIRY - 60?
          YES → print ERROR with EXPIRY and NOW values, exit 1
          NO  → continue to gh auth login
```

The threshold `EXPIRY - 60` means the check fires when fewer than 60 seconds remain on
the token. Combined with STORY_01 writing `NOW_at_mint + 3500`, the guard triggers at
`3500 - 60 = 3440 s` after the token was minted, well before the true 3600 s expiry.

---

## Error message format

When the guard fires the `kubectl logs` output will be:

```
ERROR: GitHub App token is expired or expiring imminently.
  EXPIRY=1740350700  NOW=1740350650  (threshold: EXPIRY-60=1740350640)
  Re-queue the RemediationJob to obtain a fresh token.
```

Both `EXPIRY` and `NOW` are printed as decimal Unix timestamps on the same line, making
it unambiguous without requiring the operator to consult a separate log source.

---

## Backwards compatibility

If the init container image has not been updated to STORY_01 (i.e. it does not write
`/workspace/github-token-expiry`), the `[ -f "$EXPIRY_FILE" ]` test is false and a
`WARNING` is printed to stderr. The existing `gh auth status` check at the lines that
follow still catches an invalid token — behaviour is identical to pre-epic22.

---

## Why `exit 1` causes the Job to fail

The `batch/v1 Job` is created with `restartPolicy: Never` and `backoffLimit: 1`
(`job.go:257`). When the main container exits with a non-zero code:

1. Kubernetes marks the pod as `Failed`.
2. The Job controller checks `backoffLimit`. With `backoffLimit: 1` the Job retries once;
   if the retry also exits non-zero the Job enters `Failed` state.
3. `Failed` state is observable via `kubectl get job` and controller watches, making it
   actionable without inspecting container logs.

---

## Tools required

| Tool | Already present? | Dockerfile source |
|------|------------------|-------------------|
| `date +%s` | Yes | `bash` / coreutils (Debian bookworm-slim, line 27) |
| `cat` | Yes | coreutils |
| `[ -f ... ]` | Yes | bash built-in |
| arithmetic `$(( ))` | Yes | bash built-in |

No Dockerfile changes are needed.

---

## Acceptance criteria

- [ ] When `/workspace/github-token-expiry` is absent, the script prints a `WARNING` to
      stderr and continues normally — the `gh auth status` check still runs.
- [ ] When `NOW >= EXPIRY - 60`, the script prints an error message containing both
      `EXPIRY=<value>` and `NOW=<value>` to stderr and exits with code 1.
- [ ] When `NOW < EXPIRY - 60`, the script proceeds to `gh auth login` and beyond
      unchanged.
- [ ] `exit 1` causes the `batch/v1 Job` to enter `Failed` state (verified by checking
      job status after the container exits).
- [ ] No LLM API calls are made when the guard fires (the `exec opencode run` line is
      never reached).
