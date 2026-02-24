# Epic 20: Dry-Run Mode

**Feature Tracker:** FT-U8
**Area:** Usability & Operability

## Purpose

Add a `DRY_RUN=true` environment variable to the watcher Deployment. When set:
- `RemediationJob` objects are created and deduplication works as normal
- The agent Job's prompt is augmented with a HARD RULE prohibiting PR creation
- The agent writes its investigation report to `/workspace/investigation-report.txt` and exits 0
- The watcher reads the report from the Job logs and stores it in `RemediationJob.status.message`

This lets operators evaluate mendabot in shadow mode on production clusters before enabling
live PR creation.

## Status: Not Started

## Deep-Dive Findings (2026-02-23)

### `RemediationJobStatus.Message` — already exists (no CRD change needed)
`api/v1alpha1/remediationjob_types.go` line 173: `Message string json:"message,omitempty"`
already present with a doc comment. `DeepCopyInto` copies it at line 212.
**STORY_04 requires no CRD type additions.**

### `exec opencode` issue in `agent-entrypoint.sh`
The current entrypoint ends with `exec opencode run "$(cat /tmp/rendered-prompt.txt)"`.
`exec` **replaces the shell process** — any code after this line never runs. To cat the
report to stdout in dry-run mode, STORY_03 and STORY_04 must coordinate:

STORY_03 restructures the entrypoint:
```bash
if [ "${DRY_RUN:-false}" = "true" ]; then
    opencode run "$(cat /tmp/rendered-prompt.txt)"   # no exec
    echo "=== DRY_RUN INVESTIGATION REPORT ==="
    cat /workspace/investigation-report.txt
else
    exec opencode run "$(cat /tmp/rendered-prompt.txt)"
fi
```

### Log-fetch approach (STORY_04)
- Report is read via Kubernetes CoreV1 Pods GetLogs API — not kubectl exec or shell.
- `RemediationJobReconciler` needs a new `KubeClient kubernetes.Interface` field.
- Pod is identified by label `batch.kubernetes.io/job-name: <job-name>` in `cfg.AgentNamespace`.
- Dry-run Job is detected by annotation `mendabot.io/dry-run: "true"` (set by STORY_02).
- Report truncated to `maxReportBytes = 10_000` before storing in `status.message`.
- `main.go` must create a `kubernetes.Clientset` from in-cluster config and wire it.
- RBAC marker needed: `//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get`

### `jobbuilder.Config` (STORY_02)
- `internal/jobbuilder/job.go`: `type Config struct { AgentNamespace string }` at line 17.
- Must gain `DryRun bool` field.
- `main.go` construction must pass `DryRun: cfg.DryRun`.
- Annotation `mendabot.io/dry-run: "true"` added to Job `ObjectMeta.Annotations` conditionally.
- `DRY_RUN=true` env var appended to main container `Env` slice only (not init container).

### Prompt (STORY_03)
- `${DRY_RUN}` must be added to the `VARS` list in `agent-entrypoint.sh` (line 104).
- `DRY_RUN="${DRY_RUN:-false}"` added to optional-variables block.
- New HARD RULE 9 in `configmap-prompt.yaml` — fires only when `DRY_RUN == "true"`:
  prohibits all PR/git-push; mandates writing report to `/workspace/investigation-report.txt`.
- Decision tree gets a dry-run branch prepended.

## Dependencies

- epic00-foundation complete (`internal/config/config.go`)
- epic02-jobbuilder complete (`internal/jobbuilder/job.go`)
- epic01-controller complete (`internal/controller/remediationjob_controller.go`)
- epic05-prompt complete (`deploy/kustomize/configmap-prompt.yaml`)

## Blocks

- epic23 (dry_run_report_stored audit event deferred to this epic's STORY_04)

## Stories

| Story | File | Status |
|-------|------|--------|
| Config — DRY_RUN env var | [STORY_01_config.md](STORY_01_config.md) | Not Started |
| JobBuilder — inject dry-run annotation and augment prompt env var | [STORY_02_jobbuilder.md](STORY_02_jobbuilder.md) | Not Started |
| Prompt — dry-run HARD RULE variant | [STORY_03_prompt.md](STORY_03_prompt.md) | Not Started |
| RemediationJobReconciler — read investigation report from Job logs | [STORY_04_reconciler_report.md](STORY_04_reconciler_report.md) | Not Started |

## Implementation Order

```
STORY_01 (config) ──> STORY_02 (jobbuilder) ──> STORY_04 (reconciler)
                 ──> STORY_03 (prompt)       [coordinates with STORY_04 on exec/cat change]
```

## Key Integration Points

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `DryRun bool`; parse `DRY_RUN` env var |
| `internal/jobbuilder/job.go` | Add `DryRun bool` to `Config`; inject annotation + env var |
| `cmd/watcher/main.go` | Pass `DryRun: cfg.DryRun` to jobbuilder; create + wire `KubeClient` |
| `internal/controller/remediationjob_controller.go` | Add `KubeClient` field; `fetchDryRunReport` method |
| `docker/scripts/agent-entrypoint.sh` | Add `${DRY_RUN}` to VARS; restructure `exec opencode` for dry-run |
| `deploy/kustomize/configmap-prompt.yaml` | Add HARD RULE 9; update decision tree |
| `api/v1alpha1/remediationjob_types.go` | **No change needed** — `status.message` already at line 173 |

## Definition of Done

- [ ] `config.Config` gains `DryRun bool`; `FromEnv` parses `DRY_RUN`
- [ ] `JobBuilder.Build()` adds `mendabot.io/dry-run: "true"` annotation when `cfg.DryRun == true`
- [ ] When dry-run, agent Job env includes `DRY_RUN=true` (main container only)
- [ ] `agent-entrypoint.sh` restructured so report is catted to stdout in dry-run mode (no `exec` in dry-run path)
- [ ] Prompt includes HARD RULE 9: no PR creation; write report to `/workspace/investigation-report.txt`
- [ ] `RemediationJobReconciler` detects dry-run Jobs, fetches logs via KubeClient, stores truncated report in `status.message`
- [ ] `KubeClient` wired in `main.go` from in-cluster config
- [ ] All unit and integration tests pass with `-race`
- [ ] Worklog written
