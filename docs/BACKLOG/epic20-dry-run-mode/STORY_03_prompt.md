# Story: Prompt — dry-run HARD RULE and entrypoint restructuring

**Epic:** [epic20-dry-run-mode](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator** evaluating mendabot in shadow mode, I want the agent prompt to
inform the LLM that it is in dry-run mode and explain that write operations are blocked,
and I want the investigation report to appear in the agent Job's stdout so the watcher can
read it via the Kubernetes pod logs API.

---

## Background

### Enforcement vs. notification

The dry-run HARD RULE added to the prompt is **not** the enforcement mechanism — that is
STORY_03b (the `gh` and `git` wrappers). Even if the LLM ignores the prompt rule, writes
are physically blocked. The prompt rule is informational: it tells the LLM what mode it
is in so it can produce a useful investigation report instead of attempting (and silently
failing) PR creation.

### Entrypoint file structure

The entrypoint is split across four files. Changes must go to the correct files:

| File | Role |
|------|------|
| `docker/scripts/agent-entrypoint.sh` | 4-line dispatcher — do not modify |
| `docker/scripts/entrypoint-common.sh` | Shared: kubeconfig, gh auth, prompt rendering, `envsubst`, **report-cat block (new)** |
| `docker/scripts/entrypoint-opencode.sh` | OpenCode path — `exec opencode run ...` must be restructured |
| `docker/scripts/entrypoint-claude.sh` | Claude stub — same restructuring needed for consistency |

### `entrypoint-common.sh` VARS list (current state)

Line 106 of `entrypoint-common.sh`:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${FINDING_SEVERITY}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
```

`${DRY_RUN}` is not present. This story adds it here — not in `agent-entrypoint.sh`.

### Prompt template location

The shared prompt template lives at `charts/mendabot/files/prompts/core.txt`. It is
packaged into the `agent-prompt-core` ConfigMap by `charts/mendabot/templates/configmap-prompt.yaml`
and mounted at `/prompt/core.txt` in every agent Job. **There is no `configmap-prompt.yaml`
in `deploy/kustomize/` — that path does not exist.**

### Existing HARD RULES in `core.txt`

The `=== HARD RULES ===` section currently has rules numbered:
1, 2, 3, 4, 5, 6, 7, 9, 10 — **there is no rule 8**.

The new dry-run rule is **11**, appended after the existing rule 10 (kubeconform).
No existing rules are renumbered.

### `exec` issue in per-agent entrypoints

`entrypoint-opencode.sh` ends with:
```bash
exec opencode run "$(cat /tmp/rendered-prompt.txt)"
```

`exec` replaces the shell process — any code after this line never runs. To emit the
investigation report to stdout after the agent exits in dry-run mode, the `exec` must be
conditioned: only use `exec` in the normal path; in dry-run mode run `opencode` without
`exec`, then let the shell continue to the report-cat block.

The report-cat block itself goes in `entrypoint-common.sh` (shared path — not in
`entrypoint-opencode.sh` or `entrypoint-claude.sh`). Each per-agent entrypoint calls the
agent binary, returns to `entrypoint-common.sh`, and the common code emits the report.

**Design:** `entrypoint-common.sh` is `source`d (not exec'd) by both per-agent
entrypoints, so code that runs after the `source` in the per-agent script can call back
into variables/functions set by the common script. However, the simplest and most
maintainable pattern is: after the `source entrypoint-common.sh` line in each per-agent
script, the dry-run branch is handled entirely in the per-agent script using a shared
function or inline block that the common script exports. The approach used here: add the
report-cat logic as a shell function `emit_dry_run_report` in `entrypoint-common.sh`,
call it from each per-agent entrypoint after the agent binary returns.

---

## Acceptance Criteria

- [x] `${DRY_RUN}` is added to the `VARS` line in `docker/scripts/entrypoint-common.sh:106`
- [x] `DRY_RUN="${DRY_RUN:-false}"` default assignment added to
  `docker/scripts/entrypoint-common.sh` optional-variables block
- [x] `emit_dry_run_report` shell function defined in `entrypoint-common.sh` — emits the
  sentinel `=== DRY_RUN INVESTIGATION REPORT ===` followed by the file contents to stdout
  when `DRY_RUN=true` and `/workspace/investigation-report.txt` exists
- [x] `entrypoint-opencode.sh` restructured: normal path uses `exec`; dry-run path does not
  use `exec`, calls `emit_dry_run_report` after opencode returns
- [x] `entrypoint-claude.sh` receives the same structural change for consistency (claude path
  is a stub that currently exits 1, but the dry-run pattern should be present)
- [x] HARD RULE 11 appended to `charts/mendabot/files/prompts/core.txt` after rule 10
- [x] Decision tree in `core.txt` gains a dry-run branch prepended at the top
- [x] No other parts of the prompt are changed by this story

---

## Implementation

### 1. Update `docker/scripts/entrypoint-common.sh`

**Add `DRY_RUN` to the optional-variables block** (after `FINDING_SEVERITY` default,
before the kubeconfig section):

```bash
# DRY_RUN is optional — defaults to "false"
DRY_RUN="${DRY_RUN:-false}"
```

**Add `${DRY_RUN}` to the VARS line** (line 106):

```bash
# Before:
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${FINDING_SEVERITY}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'

# After:
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${FINDING_SEVERITY}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}${DRY_RUN}'
```

**Add `emit_dry_run_report` function** at the end of `entrypoint-common.sh`, after the
`envsubst` line:

```bash
# emit_dry_run_report — called by per-agent entrypoints after the agent binary
# returns in dry-run mode. Emits the sentinel and report content to stdout so
# the watcher can extract the report via the Kubernetes pod logs API.
emit_dry_run_report() {
    if [ "${DRY_RUN:-false}" = "true" ]; then
        echo "=== DRY_RUN INVESTIGATION REPORT ==="
        if [ -f /workspace/investigation-report.txt ]; then
            cat /workspace/investigation-report.txt
        else
            echo "(investigation-report.txt not found — agent may have exited without writing the report)"
        fi
    fi
}
```

### 2. Restructure `docker/scripts/entrypoint-opencode.sh`

Replace the final `exec opencode run ...` line with a dry-run-aware branch:

```bash
# Run opencode. In dry-run mode, do not use exec so the shell continues to
# emit_dry_run_report after opencode exits. In normal mode, exec replaces the
# shell (no overhead; correct exit code forwarding).
if [ "${DRY_RUN:-false}" = "true" ]; then
    opencode run "$(cat /tmp/rendered-prompt.txt)"
    emit_dry_run_report
else
    exec opencode run "$(cat /tmp/rendered-prompt.txt)"
fi
```

### 3. Update `docker/scripts/entrypoint-claude.sh`

Apply the same structural change after the existing stub error block, replacing the
`exit 1` with:

```bash
# TODO: replace this stub with the real claude CLI invocation once verified.
# Dry-run pattern is in place for when this is implemented.
if [ "${DRY_RUN:-false}" = "true" ]; then
    # claude run "$(cat /tmp/rendered-prompt.txt)"   # TODO: verify invocation
    echo "ERROR: Claude Code entrypoint is not yet implemented." >&2
    exit 1
    emit_dry_run_report
else
    echo "ERROR: Claude Code entrypoint is not yet implemented." >&2
    exit 1
    # exec claude run "$(cat /tmp/rendered-prompt.txt)"   # TODO: verify invocation
fi
```

> **Note:** The `emit_dry_run_report` and `exec claude` lines are intentionally unreachable
> (after `exit 1`) until the claude invocation is implemented. This keeps the dry-run
> structure ready without enabling the broken stub. Remove the `exit 1` and uncomment the
> claude invocation as part of whichever epic implements the real claude entrypoint.

### 4. Add HARD RULE 11 to `charts/mendabot/files/prompts/core.txt`

Append after rule 10 (the kubeconform block), still inside `=== HARD RULES ===`:

```
11. DRY_RUN mode: ${DRY_RUN}
    If the value above is "true", you are running in dry-run shadow mode:
    a. Write operations are blocked at the tool level — git push, git commit,
       gh pr create, gh pr comment, and related commands will be silently
       rejected by the environment. Do not attempt to work around this.
    b. Complete your full investigation (all INVESTIGATION STEPS) as normal.
    c. Write your complete investigation report — root cause, evidence, and
       proposed fix — to /workspace/investigation-report.txt.
       Use: printf '%s\n' "<your report>" > /workspace/investigation-report.txt
    d. Exit 0 after writing the report.
    e. Rules 3 and 5 (PR outcomes, fingerprint in PR body) do NOT apply in
       dry-run mode. Writing the report file is the required outcome.
```

### 5. Update the `=== DECISION TREE ===` section in `core.txt`

Prepend a dry-run branch at the very top of the decision tree:

```
DRY_RUN is "true" → complete investigation → write /workspace/investigation-report.txt → exit 0 (rules 3 and 5 do not apply; write operations are blocked by the environment)
```

---

## Report file format

The agent writes `/workspace/investigation-report.txt` in plain text. The watcher reads
this from Job logs by finding the `=== DRY_RUN INVESTIGATION REPORT ===` sentinel and
extracting everything after it (see STORY_04). The most important information should
appear early in the file.

Suggested structure (the agent is free to vary this):

```
## Finding
Kind: <FINDING_KIND>
Resource: <FINDING_NAME> / <FINDING_NAMESPACE>
Parent: <FINDING_PARENT>
Fingerprint: <FINDING_FINGERPRINT>

## Root Cause
<one-paragraph assessment>

## Proposed Fix
<what would have been changed>

## Confidence
<high / medium / low — and why>

## Evidence
<kubectl output excerpts>
```

The watcher truncates to 10,000 bytes after sentinel extraction before storing in
`status.message` (see STORY_04).

---

## Tasks

- [x] Add `DRY_RUN="${DRY_RUN:-false}"` default to `entrypoint-common.sh` optional-variables block
- [x] Add `${DRY_RUN}` to the VARS line in `entrypoint-common.sh:106`
- [x] Add `emit_dry_run_report` function to `entrypoint-common.sh`
- [x] Restructure `exec opencode` in `entrypoint-opencode.sh` for dry-run path
- [x] Apply equivalent structural change to `entrypoint-claude.sh` (stub remains `exit 1`)
- [x] Append HARD RULE 11 to `charts/mendabot/files/prompts/core.txt`
- [x] Prepend dry-run branch to `=== DECISION TREE ===` in `core.txt`
- [x] Manual smoke test: render the prompt with `envsubst` and confirm `${DRY_RUN}` is
  substituted correctly with both `DRY_RUN=true` and `DRY_RUN=false`
- [x] Verify `emit_dry_run_report` is visible from `entrypoint-opencode.sh` after source
  (test: `source entrypoint-common.sh && type emit_dry_run_report`)

---

## Dependencies

**Depends on:** STORY_02 (the `DRY_RUN=true` env var must be injected into the Job before
the agent can read it at runtime)
**No Go compilation dependency** — this story only modifies shell scripts and prompt text.

---

## Definition of Done

- [x] `entrypoint-common.sh` has `${DRY_RUN}` in VARS, default assignment, and `emit_dry_run_report`
- [x] `entrypoint-opencode.sh` has the dry-run/normal branch replacing bare `exec opencode`
- [x] `entrypoint-claude.sh` has the structural dry-run/normal branch (stub still exits 1)
- [x] HARD RULE 11 is present verbatim in `charts/mendabot/files/prompts/core.txt`
- [x] Decision tree has the dry-run branch prepended
- [x] `git diff --stat` shows only `entrypoint-common.sh`, `entrypoint-opencode.sh`,
  `entrypoint-claude.sh`, and `charts/mendabot/files/prompts/core.txt` changed
- [x] Full test suite passes: `go test -timeout 120s -race ./...` (no Go changes in this story)
