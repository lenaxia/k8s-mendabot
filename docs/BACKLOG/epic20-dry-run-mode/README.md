# Epic 20: Dry-Run Mode

**Feature Tracker:** FT-U8
**Area:** Usability & Operability

## Purpose

Add a `DRY_RUN=true` environment variable to the watcher Deployment. When set:
- `RemediationJob` objects are created and deduplication works as normal
- The agent Job receives `DRY_RUN=true` in its environment
- **Write operations are blocked programmatically** — `gh` and `git` wrappers refuse
  `pr create`, `git push`, `git commit`, etc., regardless of what the LLM decides to do
- The agent writes its investigation report to `/workspace/investigation-report.txt` and exits 0
- The entrypoint cats the report to stdout after opencode exits (shared path for all agent types)
- The watcher reads the report from the Job logs, extracts the section after the sentinel
  `=== DRY_RUN INVESTIGATION REPORT ===`, and stores it in `RemediationJob.status.message`

This lets operators evaluate mendabot in shadow mode on production clusters before enabling
live PR creation. The key design principle: **dry-run enforcement is deterministic, not
probabilistic**. The prompt is updated to inform the LLM of the mode, but the wrappers
are the actual enforcement layer.

## Status: Not Started

## Design: Why Wrappers, Not Just a Prompt Rule

The original design used a prompt HARD RULE to instruct the LLM not to create PRs. This
is a prompt-only control — equivalent to AR-06 in the threat model ("HARD RULEs are prompt
instructions, not technical controls"). An LLM following a bad instruction, or a prompt
injection attack overriding the rule, could cause PR creation in dry-run mode.

The correct layer for this enforcement is the existing wrapper infrastructure (see
`THREAT_MODEL.md` AV-02 wrapper inventory and the new AV-13). The `gh` wrapper at
`/usr/local/bin/gh` already intercepts every `gh` call for redaction. Extending it to
also block write subcommands when `DRY_RUN=true` makes dry-run enforcement deterministic.

The same logic applies to `git`. `git` was deliberately not wrapped in epic12/epic25 because
output redaction would break diff-based workflows (see `THREAT_MODEL.md` "Tools deliberately
NOT wrapped"). That reason does not apply to a wrapper that only inspects the first argument
and blocks write *subcommands* — no stdout is touched.

## Deep-Dive Findings (2026-02-25, corrected from 2026-02-23)

### `RemediationJobStatus.Message` — already exists (no CRD change needed)
`api/v1alpha1/remediationjob_types.go` line 206: `Message string json:"message,omitempty"`
already present. `DeepCopyInto` copies it at line 251. **STORY_04 requires no CRD type additions.**

### Entrypoint structure
The entrypoint is split across four files:
- `docker/scripts/agent-entrypoint.sh` — 4-line dispatcher, routes to per-agent entrypoint via `AGENT_TYPE`
- `docker/scripts/entrypoint-common.sh` — shared setup: kubeconfig, gh auth, prompt rendering, `envsubst`
- `docker/scripts/entrypoint-opencode.sh` — sources common.sh, then `exec opencode run ...`
- `docker/scripts/entrypoint-claude.sh` — sources common.sh, then `exec claude ...`

The `VARS` list for `envsubst` is in `entrypoint-common.sh:106`:
```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${FINDING_SEVERITY}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
```
`${DRY_RUN}` must be added here (not in `agent-entrypoint.sh`).

The `exec` in per-agent entrypoints must be restructured in both `entrypoint-opencode.sh`
and `entrypoint-claude.sh` to allow the dry-run report cat after opencode/claude exits.
The cat itself goes in `entrypoint-common.sh` (shared path, runs for both agent types),
called from each per-agent entrypoint after the agent process returns.

### Prompt template location
The prompt template no longer lives in `deploy/kustomize/configmap-prompt.yaml`. It is at:
- `charts/mendabot/files/prompts/core.txt` — shared investigation instructions (all agent types)
- `charts/mendabot/files/prompts/opencode.txt` — OpenCode-specific preamble
- `charts/mendabot/files/prompts/claude.txt` — Claude-specific preamble

The new dry-run HARD RULE goes in `core.txt` as **rule 11** (after the existing rule 10,
the kubeconform rule). The existing rules go 1–7, 9, 10 — there is no rule 8; the new rule
is appended as 11 to avoid renumbering.

### `jobbuilder.Config` (STORY_02)
The struct currently has three fields (not one):
```go
type Config struct {
    AgentNamespace string
    AgentType      config.AgentType
    TTLSeconds     int32
}
```
`DryRun bool` is added as the fourth field. The main container `Env` slice currently ends
with `AGENT_TYPE` (not `IS_SELF_REMEDIATION` — that env var does not exist). Annotation
map is at lines 244–247.

### Log-fetch and sentinel extraction (STORY_04)
- Report is read via Kubernetes CoreV1 Pods GetLogs API.
- `fetchDryRunReport` reads the raw log stream, finds the sentinel line
  `=== DRY_RUN INVESTIGATION REPORT ===`, and returns only the text after it.
- If the sentinel is absent (opencode failed silently), returns the raw truncated log with a note.
- Use `io.LimitReader` + `io.ReadAll` (not `io.ReadFull`).
- Truncated to `maxReportBytes = 10_000` after sentinel extraction.
- `RemediationJobReconciler` needs a new `KubeClient kubernetes.Interface` field.

## Dependencies

- epic00-foundation complete (`internal/config/config.go`)
- epic02-jobbuilder complete (`internal/jobbuilder/job.go`)
- epic01-controller complete (`internal/controller/remediationjob_controller.go`)
- epic12-security-review complete (wrapper infrastructure in place)

## Blocks

- epic23 (dry_run_report_stored audit event deferred to this epic's STORY_04)

## Stories

| Story | File | Status |
|-------|------|--------|
| Config — DRY_RUN env var | [STORY_01_config.md](STORY_01_config.md) | Not Started |
| JobBuilder — inject dry-run annotation and env var | [STORY_02_jobbuilder.md](STORY_02_jobbuilder.md) | Not Started |
| Enforcement wrappers — gh and git dry-run blocking | [STORY_03b_wrappers.md](STORY_03b_wrappers.md) | Not Started |
| Prompt — dry-run HARD RULE and entrypoint restructuring | [STORY_03_prompt.md](STORY_03_prompt.md) | Not Started |
| RemediationJobReconciler — read investigation report from Job logs | [STORY_04_reconciler_report.md](STORY_04_reconciler_report.md) | Not Started |

## Implementation Order

```
STORY_01 (config) ──> STORY_02 (jobbuilder) ──> STORY_03b (wrappers) ──> STORY_04 (reconciler)
                  ──> STORY_03 (prompt)      [depends on STORY_02 for DRY_RUN env var]
```

STORY_03b and STORY_03 are independent of each other and can be implemented in parallel
after STORY_02. STORY_04 depends on both STORY_03 (entrypoint cat) and STORY_03b
(wrappers enforce the constraint that the report is the only output).

## Key Integration Points

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `DryRun bool`; parse `DRY_RUN` env var |
| `internal/jobbuilder/job.go` | Add `DryRun bool` to `Config`; inject annotation + env var |
| `cmd/watcher/main.go` | Pass `DryRun: cfg.DryRun` to jobbuilder; create + wire `KubeClient` |
| `internal/controller/remediationjob_controller.go` | Add `KubeClient` field; `fetchDryRunReport` method with sentinel extraction |
| `docker/scripts/redact-wrappers/gh` | Add dry-run write-subcommand blocking |
| `docker/scripts/redact-wrappers/git` | New wrapper — block push/commit/tag in dry-run mode |
| `docker/Dockerfile.agent` | Add git wrapper install (rename real binary; COPY wrapper) |
| `docker/scripts/entrypoint-common.sh` | Add `${DRY_RUN}` to VARS; add default assignment; add report-cat block |
| `docker/scripts/entrypoint-opencode.sh` | Restructure `exec opencode` for dry-run path |
| `docker/scripts/entrypoint-claude.sh` | Same restructuring as opencode |
| `charts/mendabot/files/prompts/core.txt` | Add HARD RULE 11; update decision tree |
| `api/v1alpha1/remediationjob_types.go` | **No change needed** — `status.message` already exists |

## Definition of Done

- [ ] `config.Config` gains `DryRun bool`; `FromEnv` parses `DRY_RUN`
- [ ] `JobBuilder.Build()` adds `mendabot.io/dry-run: "true"` annotation when `cfg.DryRun == true`
- [ ] When dry-run, agent Job env includes `DRY_RUN=true` (main container only)
- [ ] `gh` wrapper blocks all write subcommands (`pr create`, `pr comment`, `pr edit`,
  `issue create`, etc.) when `DRY_RUN=true`; exits 0 with a `[DRY_RUN]` log line to stderr
- [ ] New `git` wrapper blocks `push`, `commit`, `tag -a`, `tag -s` when `DRY_RUN=true`;
  passes all read-only subcommands through unchanged; installed in Dockerfile via rename+COPY
- [ ] `entrypoint-common.sh` has `${DRY_RUN}` in VARS; has report-cat block
- [ ] `entrypoint-opencode.sh` and `entrypoint-claude.sh` restructured to support dry-run path
- [ ] `charts/mendabot/files/prompts/core.txt` has HARD RULE 11; decision tree has dry-run branch
- [ ] `RemediationJobReconciler` detects dry-run Jobs, fetches logs via KubeClient, extracts
  post-sentinel content, stores truncated report in `status.message`
- [ ] `KubeClient` wired in `main.go` from `ctrl.GetConfigOrDie()` (reuse existing REST config)
- [ ] All unit and integration tests pass with `-race`
- [ ] Worklog written
