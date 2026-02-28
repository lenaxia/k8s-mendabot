# Epic 26: Auto-Close Sinks on Remediation Resolution

## Purpose

When a finding clears — the deployment recovers, the PVC is provisioned, the node
returns to Ready — the `SourceProviderReconciler` already cancels `Pending`,
`Dispatched`, and `Running` `RemediationJob` objects. But if the agent opened a PR
before the finding cleared, that PR remains open indefinitely. A human must manually
close it. For clusters with frequent transient failures this produces a backlog of
stale PRs that obscures genuinely important ones.

This epic implements automatic sink closure when a finding resolves — covering both
jobs that were cancelled in-flight (Pending/Dispatched/Running) **and** jobs that
already completed successfully (PhaseSucceeded). The latter is the most common
production scenario: the agent opens a PR, the cluster self-heals before the PR is
merged, and the PR becomes permanently stale.

## Status: Complete

## Dependencies

- epic01-controller complete (`SourceProviderReconciler`, `RemediationJobReconciler`)
- epic04-deploy complete (watcher Deployment has access to GitHub App credentials)
- epic09-native-provider complete (native source providers that emit the findings)

## Blocks

- epic27-pr-feedback-iteration (feedback loop depends on the watcher being able to
  interact with open PRs — shares the GitHub REST client and token provider built here)

## Success Criteria

- [ ] `SinkRef` type exists in `api/v1alpha1/remediationjob_types.go` with fields
      `Type`, `URL`, `Number`, `Repo`
- [ ] `SinkCloser` interface exists in `internal/domain/sink.go`
- [ ] `GitHubSinkCloser` implementation in `internal/sink/github/` uses the GitHub
      REST API directly (no `gh` CLI subprocess)
- [ ] `SourceProviderReconciler` calls `SinkCloser.Close` for **both**:
      - Jobs cancelled in-flight (Pending/Dispatched/Running/Suppressed) that have a
        non-empty `SinkRef.URL`
      - Jobs already in `PhaseSucceeded` that have a non-empty `SinkRef.URL`
- [ ] `PhaseSucceeded` rjobs are **not deleted** after auto-close — they remain as
      dedup tombstones until TTL expires
- [ ] Closure comment is human-readable and explains why the sink was closed
- [ ] `PR_AUTO_CLOSE=false` env var disables auto-close (default: `true`)
- [ ] Agent writes `SinkRef` (Type, URL, Number, Repo) to `status.sinkRef` via status
      patch after opening a PR (STORY_05)
- [ ] Watcher Deployment manifest mounts the GitHub App credentials Secret
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] Worklog entry created

## Design

### SinkRef model

`RemediationJob.status` gains a `SinkRef` field (set by the agent via a status patch
after it opens a PR or issue):

```go
type RemediationJobStatus struct {
    // ... existing fields ...
    SinkRef SinkRef `json:"sinkRef,omitempty"`
}

type SinkRef struct {
    // Type is "pr" or "issue"
    Type string `json:"type"`
    // URL is the full URL to the PR or issue
    // (e.g. https://github.com/org/repo/pull/42)
    URL string `json:"url"`
    // Number is the numeric ID required for GitHub API calls
    Number int `json:"number"`
    // Repo is "owner/repo" format, e.g. "lenaxia/talos-ops-prod"
    Repo string `json:"repo"`
}
```

The existing `status.prRef` string field is kept unchanged for display purposes
(`kubectl get rjobs` column). `SinkRef` is the canonical representation used for
API-based closure. New rjobs set both fields; old rjobs (pre-epic26) that only have
`prRef` and no `sinkRef` are silently skipped by the auto-close logic (see backwards
compatibility note below).

#### How SinkRef is populated (STORY_05)

The **agent** writes `SinkRef` to `status.sinkRef` immediately after `gh pr create`
succeeds. The agent parses the PR URL and number from `gh pr create --json url,number`
output, derives `Repo` from the `GITOPS_REPO` env var, and performs a
`kubectl patch` on the `RemediationJob` status subresource before it exits. The watcher
reads this field when the finding later clears.

### SinkCloser interface

```go
// SinkCloser closes an open sink (PR or issue) when the underlying finding resolves.
type SinkCloser interface {
    // Close closes the sink referenced by the RemediationJob's SinkRef.
    // reason is a human-readable explanation included in the closing comment.
    // Returns nil if SinkRef is empty (no sink to close).
    // Returns nil if the sink is already closed (idempotent).
    Close(ctx context.Context, rjob *v1alpha1.RemediationJob, reason string) error
}
```

### GitHub REST API (not gh CLI)

`GitHubSinkCloser` calls the GitHub REST API directly using `net/http`. The watcher
container does **not** contain the `gh` CLI binary — only the agent container does.
Using the REST API avoids subprocess execution in a tight reconcile loop, removes the
need to parse CLI output, and provides proper connection reuse via `http.Client`.

Operations performed per `Close()` call:
1. `POST /repos/{owner}/{repo}/issues/{number}/comments` — posts the human-readable
   closure reason as a comment
2. `PATCH /repos/{owner}/{repo}/pulls/{number}` with `{"state":"closed"}` — closes
   the PR (for `SinkRef.Type == "pr"`)
   or `PATCH /repos/{owner}/{repo}/issues/{number}` with `{"state":"closed"}` — closes
   the issue (for `SinkRef.Type == "issue"`)

**Idempotency:** GitHub returns `422 Unprocessable Entity` when attempting to close an
already-closed PR or issue via the REST API. `GitHubSinkCloser` treats `422` as success
— the sink is already in the desired state. This is not an error.

**Authentication:** The installation token from `internal/github/token.go` is passed as
`Authorization: Bearer <token>` on every request. No `gh` binary is required.

### GitHub App token in the watcher

The watcher currently does not mount the GitHub App credentials — only the agent Job
does. This epic adds a mounted Secret and a `GitHubTokenProvider` that the watcher uses:

1. `internal/github/token.go` — exchanges App private key for installation token,
   caches it with a 55-minute TTL (refreshes before the 1-hour GitHub App expiry)
2. Token is injected into `GitHubSinkCloser` at construction time in `cmd/watcher/main.go`

The same token provider is reused by epic27.

**Restart behaviour:** The token cache is in-memory. On watcher restart the cache is
empty. The first close call after restart exchanges a fresh token. The GitHub App Secret
must be mounted at pod startup — it cannot be lazily loaded. The deployment manifest
(STORY_04) declares the Secret as a required volume so Kubernetes will refuse to start
the watcher pod if the Secret does not exist.

### Trigger conditions in SourceProviderReconciler

Auto-close fires in **two places** in `SourceProviderReconciler.Reconcile`:

**Path A — finding cleared, in-flight jobs (Pending/Dispatched/Running/Suppressed):**

```go
for i := range rjobList.Items {
    rjob := &rjobList.Items[i]
    // ... existing source ref match check ...
    phase := rjob.Status.Phase
    if phase is non-terminal {
        if r.Cfg.PRAutoClose && rjob.Status.SinkRef.URL != "" {
            reason := fmt.Sprintf(
                "Closing automatically: the underlying issue (%s/%s %s) has resolved. No manual fix is required.",
                rjob.Spec.Finding.Kind, rjob.Spec.Finding.Name, rjob.Spec.Finding.Namespace)
            if err := r.SinkCloser.Close(ctx, rjob, reason); err != nil {
                log.Error(err, "failed to close sink", "sinkRef", rjob.Status.SinkRef.URL)
                // log and continue — must not block cancellation
            }
        }
        // existing cancel + delete logic unchanged
    }
}
```

**Path B — finding cleared, already-Succeeded jobs:**

After processing non-terminal cancellations in the same finding-cleared path, also
iterate `PhaseSucceeded` rjobs matching the same `SourceResultRef`:

```go
for i := range rjobList.Items {
    rjob := &rjobList.Items[i]
    if rjob.Spec.SourceResultRef.Name != req.Name ||
        rjob.Spec.SourceResultRef.Namespace != req.Namespace {
        continue
    }
    if rjob.Status.Phase == v1alpha1.PhaseSucceeded && rjob.Status.SinkRef.URL != "" {
        if r.Cfg.PRAutoClose {
            reason := fmt.Sprintf(
                "Closing automatically: the underlying issue (%s/%s %s) has resolved. No manual fix is required.",
                rjob.Spec.Finding.Kind, rjob.Spec.Finding.Name, rjob.Spec.Finding.Namespace)
            if err := r.SinkCloser.Close(ctx, rjob, reason); err != nil {
                log.Error(err, "failed to close succeeded sink", "sinkRef", rjob.Status.SinkRef.URL)
            }
        }
        // Do NOT delete the PhaseSucceeded rjob.
        // It is the dedup tombstone — removing it would allow re-dispatch before TTL.
    }
}
```

Sink closure failure is logged and ignored in both paths. It must not block any
`RemediationJob` state transition or deletion.

### Flapping findings and dedup interaction

**Known interaction (by design, not a bug):**

1. Finding fires → agent opens PR → rjob reaches `PhaseSucceeded`
2. Finding clears → watcher auto-closes PR via Path B ✓
3. Finding fires again (same fingerprint, before TTL expires) →
   `SourceProviderReconciler` finds the existing `PhaseSucceeded` rjob →
   **suppressed as duplicate** — no new agent is dispatched

This is intentional. The `PhaseSucceeded` rjob is the dedup tombstone. Auto-closing
the PR does NOT delete the tombstone, so the dedup guard still fires. If the finding
is genuinely new (different error text → different fingerprint) or returns after TTL,
a new investigation is dispatched normally.

Operators who have a workload that frequently self-heals and want re-investigation on
each occurrence should reduce `REMEDIATION_JOB_TTL_SECONDS` to a value shorter than
the flap period (e.g. `3600` = 1 hour). The default is 7 days, which is conservative.

### Backwards compatibility

`RemediationJob` objects created before epic26 have `status.prRef` set but
`status.sinkRef` empty. The auto-close logic checks `SinkRef.URL != ""` — old objects
are silently skipped. Their open PRs are not auto-closed. This is acceptable: the
migration path is natural (old rjobs expire via TTL; new ones created after the upgrade
populate `SinkRef`). Retroactive closure of pre-epic26 PRs requires a one-time
out-of-band script and is out of scope for this epic.

### Configuration

```bash
# Disable automatic sink closure when a finding resolves (default: true)
PR_AUTO_CLOSE=true
```

`PR_AUTO_CLOSE=false` disables all `SinkCloser.Close` calls. The `SourceProviderReconciler`
checks `r.Cfg.PRAutoClose` before calling the closer. Cancellation and deletion of
rjobs proceed normally regardless of this flag.

**Granularity:** `PR_AUTO_CLOSE` is an operator-level binary flag. Per-namespace or
per-resource overrides are not in scope for v1. If needed in a future version, the
annotation control system (epic16) can be extended with a
`mechanic.io/auto-close: false` annotation on the resource or namespace.

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| SinkRef domain type + SinkCloser interface | [STORY_00_domain_types.md](STORY_00_domain_types.md) | Complete | High | 1h |
| GitHub App token provider in watcher | [STORY_01_watcher_github_token.md](STORY_01_watcher_github_token.md) | Complete | High | 2h |
| GitHubSinkCloser implementation (REST API) | [STORY_02_github_sink_closer.md](STORY_02_github_sink_closer.md) | Complete | High | 2h |
| Wire SinkCloser into SourceProviderReconciler | [STORY_03_wire_reconciler.md](STORY_03_wire_reconciler.md) | Complete | Critical | 2h |
| Deploy manifest: Secret mount + PR_AUTO_CLOSE | [STORY_04_deploy.md](STORY_04_deploy.md) | Complete | Medium | 1h |
| Agent: write SinkRef after gh pr create | [STORY_05_agent_sinkref_patch.md](STORY_05_agent_sinkref_patch.md) | Complete | High | 1h |

## Technical Overview

### New files

| File | Purpose |
|------|---------|
| `internal/domain/sink.go` | `SinkCloser` interface |
| `internal/domain/sink_test.go` | Unit tests |
| `internal/github/token.go` | GitHub App → installation token exchange + 55-min cache |
| `internal/github/token_test.go` | Token provider tests (mock HTTP server) |
| `internal/sink/github/closer.go` | `GitHubSinkCloser` — REST API, no gh CLI |
| `internal/sink/github/closer_test.go` | Unit tests with mock HTTP server |

### Modified files

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add `SinkRef` struct + `SinkRef` field to `RemediationJobStatus`; update `DeepCopyInto` |
| `internal/provider/provider.go` | Add Path A + Path B auto-close calls; accept `SinkCloser` and read `PRAutoClose` |
| `internal/config/config.go` | Add `PRAutoClose bool` |
| `deploy/kustomize/deployment-watcher.yaml` | Mount GitHub App Secret; add `PR_AUTO_CLOSE` env var |
| `charts/mechanic/templates/deployment-watcher.yaml` | Same as above for Helm chart |
| `testdata/crds/remediationjob_crd.yaml` | Add `sinkRef` to status schema |
| `docker/scripts/agent-entrypoint.sh` | Parse `gh pr create --json url,number` output; `kubectl patch` status |

## Definition of Done

- [ ] All unit tests pass: `go test -timeout 30s -race ./...`
- [ ] `go build ./...` succeeds
- [ ] `PR_AUTO_CLOSE=false` disables closure (verified by test)
- [ ] Closure comment appears on the GitHub PR (manual verification in dev cluster)
- [ ] `SinkCloser` failure does not block `RemediationJob` cancellation (tested)
- [ ] `PhaseSucceeded` rjobs with `SinkRef.URL` are auto-closed but not deleted (tested)
- [ ] 422 response from GitHub REST API is treated as success, not an error (tested)
- [ ] Old rjobs with only `prRef` and no `sinkRef` are silently skipped (tested)
- [ ] Worklog entry created in `docs/WORKLOGS/`
