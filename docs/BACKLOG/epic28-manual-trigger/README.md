# Epic 28: Manual Investigation Triggers

## Purpose

All current `RemediationJob` sources are automatic — a provider watches a Kubernetes
resource and emits a finding when it detects a problem. There is no way for an operator
to say "investigate this resource now" without touching the cluster directly.

This epic introduces a **`TriggerProvider` interface**: a pluggable abstraction for
external systems that can request a mechanic investigation on-demand. Any system that
can POST a webhook, send a Slack message, create a GitHub issue with a specific label,
or transition a Jira ticket can become a trigger source. The watcher converts trigger
events into `RemediationJob` objects using the same deduplication and dispatch pipeline
as automatic providers.

The interface deliberately mirrors the existing `SourceProvider` pattern so trigger
backends are consistent, testable, and can be added without touching core controller
logic.

## Status: Not Started

## Dependencies

- epic01-controller complete (`SourceProviderReconciler` — trigger-created
  `RemediationJob` objects flow through the same reconciler)
- epic02-jobbuilder complete (agent Job construction is reused unchanged)
- epic26-auto-close-resolved complete (`internal/github/token.go` reused for GitHub
  trigger backend; `SinkRef` model used to link the trigger source to the opened PR)

## Blocks

Nothing downstream depends on this epic.

## Success Criteria

- [ ] `TriggerProvider` interface exists in `internal/domain/trigger.go`
- [ ] Three reference implementations: `WebhookTrigger`, `GitHubIssueTrigger`,
      `SlackTrigger`
- [ ] `RemediationJobSpec` gains a `TriggerRef` field recording the trigger source
      (type, ID, URL) for traceability
- [ ] `RemediationJob` created by a trigger has `spec.source = "manual"` to distinguish
      it from automatic findings in metrics and audit logs
- [ ] Each backend is independently enable/disable-able via env var
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] Worklog entry created

## TriggerProvider Interface

```go
// TriggerProvider is a source of on-demand investigation requests.
// Implementations poll or receive push events from an external system and
// convert them into FindingRequest objects. The watcher creates a RemediationJob
// for each unique FindingRequest, using the standard deduplication pipeline.
type TriggerProvider interface {
    // Name returns a stable identifier for this provider (e.g. "github-issue", "slack").
    Name() string

    // Poll returns any pending trigger requests that have not yet been acknowledged.
    // Implementations must be idempotent — duplicate requests for the same resource
    // must return the same FindingRequest so deduplication suppresses them.
    Poll(ctx context.Context) ([]FindingRequest, error)

    // Acknowledge marks a trigger request as processed so it is not returned by
    // future Poll calls. Called by the watcher after a RemediationJob is created.
    Acknowledge(ctx context.Context, requestID string) error
}

// FindingRequest is an on-demand investigation request from an external trigger.
type FindingRequest struct {
    // RequestID is a stable, unique ID from the trigger source (e.g. GitHub issue number,
    // Slack message timestamp, Jira ticket key). Used for acknowledgement and dedup.
    RequestID string

    // The finding to investigate. Kind, Namespace, Name, and ParentObject are
    // required. Errors may be empty — the agent will investigate from first principles.
    Finding domain.Finding

    // TriggerRef records the source of the request for traceability.
    TriggerRef TriggerRef
}

// TriggerRef identifies the external object that requested the investigation.
type TriggerRef struct {
    // Type is the backend name: "webhook", "github-issue", "slack", "jira", "pagerduty".
    Type   string `json:"type"`
    // ID is the external object's identifier (issue number, message ID, ticket key).
    ID     string `json:"id"`
    // URL is the human-navigable link to the trigger source, if available.
    URL    string `json:"url,omitempty"`
    // Title is a short human-readable summary of the trigger (issue title, alert name).
    Title  string `json:"title,omitempty"`
}
```

## Reference Implementations

### 1 — WebhookTrigger

A lightweight HTTP server (port `8083`, separate goroutine) that accepts `POST /trigger`
with a JSON body:

```json
{
  "kind": "Deployment",
  "namespace": "production",
  "name": "my-app",
  "errors": ["optional: pre-known error context"],
  "requestId": "caller-assigned-stable-id"
}
```

Authentication: bearer token from `TRIGGER_WEBHOOK_TOKEN` env var (required; server
returns 401 if unset or token mismatch).

`requestId` is caller-assigned and must be stable across retries. The watcher uses it
as the deduplication key — sending the same `requestId` twice creates one
`RemediationJob`, not two.

This backend requires no external dependency and works with any system that can issue
an HTTP request: custom scripts, runbooks, CI pipelines, PagerDuty webhooks, Grafana
alert webhooks.

### 2 — GitHubIssueTrigger

Polls a configured GitHub repository for issues carrying a specific label (default:
`mechanic-investigate`). Each matching issue is converted to a `FindingRequest`:

- `Kind`, `Namespace`, `Name` are parsed from the issue title using a structured
  convention: `[investigate] Kind/namespace/name`
- The issue body becomes the `Finding.Errors` context (first 2000 chars)
- `TriggerRef.ID` = issue number; `TriggerRef.URL` = issue HTML URL

After a `RemediationJob` is created, the trigger acknowledges by adding a comment to
the issue: `"Investigation dispatched. RemediationJob: <name>. Follow progress with
\`kubectl describe rjob <name>\`."` and applying a `mechanic-dispatched` label.

Polling interval: `GITHUB_TRIGGER_POLL_INTERVAL` (default: `2m`).
Repository: `GITHUB_TRIGGER_REPO` (e.g. `org/ops-repo`).
Label: `GITHUB_TRIGGER_LABEL` (default: `mechanic-investigate`).

Uses the same `internal/github/token.go` token provider from epic26.

### 3 — SlackTrigger

Listens for Slack slash commands or app mentions via the Slack Events API
(push model — no polling). Requires a Slack App with:
- `commands` scope (for `/investigate Kind/namespace/name`)
- `app_mentions:read` scope (for `@mechanic investigate Kind/namespace/name`)
- `chat:write` scope (to acknowledge in-channel)

The Slack App sends an HTTP POST to the watcher's `/slack/events` endpoint (same HTTP
server as `WebhookTrigger`, different path). The watcher validates the request using
`SLACK_SIGNING_SECRET` (Slack's HMAC-SHA256 request signing).

Parsed from the slash command or mention:
- `Kind/namespace/name` (required)
- Optional `--errors "..."` flag for pre-known context

Acknowledgement: the watcher replies in the same channel with a thread message:
`"Investigation started for Deployment/production/my-app. I'll update this thread when
a PR is opened."`

`SLACK_TRIGGER_ENABLED=true` enables this backend. `SLACK_BOT_TOKEN` and
`SLACK_SIGNING_SECRET` are required when enabled.

### 4 — JiraTrigger (future, not in this epic)

Documented here as a design note. A Jira webhook or polling backend would follow the
same `TriggerProvider` interface. `TriggerRef.Type = "jira"`, `TriggerRef.ID` = Jira
ticket key. Deferred — the interface is designed to accommodate it without any changes
to the core.

## RemediationJobSpec extension

```go
type RemediationJobSpec struct {
    // ... existing fields ...

    // Source indicates how this RemediationJob was created.
    // "automatic" = created by a SourceProvider reconciler (default, backwards compatible)
    // "manual"    = created from a TriggerProvider request
    Source string `json:"source,omitempty"`

    // TriggerRef records the external trigger that created this job, if Source = "manual".
    TriggerRef *TriggerRef `json:"triggerRef,omitempty"`
}
```

`Source = "manual"` is surfaced in:
- Prometheus metrics label `source` on `mechanic_remediationjobs_created_total`
- Structured audit log `event="remediationjob_created"` entry
- `kubectl get rjob` print column (added to CRD printer columns)

## Watcher HTTP Server

Both `WebhookTrigger` and `SlackTrigger` share a single embedded HTTP server in the
watcher binary, managed via `internal/triggerserver/server.go`. Routes:

| Path | Handler |
|------|---------|
| `POST /trigger` | `WebhookTrigger` handler (token auth) |
| `POST /slack/events` | `SlackTrigger` handler (HMAC auth) |

The server starts only when at least one HTTP-based trigger backend is enabled. It binds
to `TRIGGER_HTTP_PORT` (default: `8083`) and is gracefully shut down alongside the
controller manager.

## Configuration

```bash
# --- WebhookTrigger ---
# Required bearer token for POST /trigger (if unset, WebhookTrigger is disabled)
TRIGGER_WEBHOOK_TOKEN=

# --- GitHubIssueTrigger ---
# Enable GitHub issue polling trigger (default: false)
GITHUB_TRIGGER_ENABLED=false
# Repository to watch for trigger issues (owner/repo format)
GITHUB_TRIGGER_REPO=
# Issue label that marks a trigger request (default: mechanic-investigate)
GITHUB_TRIGGER_LABEL=mechanic-investigate
# Poll interval for trigger issues (default: 2m)
GITHUB_TRIGGER_POLL_INTERVAL=2m

# --- SlackTrigger ---
# Enable Slack Events API trigger (default: false)
SLACK_TRIGGER_ENABLED=false
SLACK_BOT_TOKEN=
SLACK_SIGNING_SECRET=

# --- HTTP server for webhook/slack ---
# Port for the trigger HTTP server (default: 8083)
TRIGGER_HTTP_PORT=8083
```

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
| TriggerProvider domain types (interface, FindingRequest, TriggerRef) | [STORY_00_domain_types.md](STORY_00_domain_types.md) | Not Started | High | 1h |
| RemediationJobSpec Source + TriggerRef fields | [STORY_01_remediationjob_types.md](STORY_01_remediationjob_types.md) | Not Started | High | 1h |
| TriggerProviderLoop: poll all backends, create RemediationJobs | [STORY_02_trigger_loop.md](STORY_02_trigger_loop.md) | Not Started | Critical | 3h |
| WebhookTrigger implementation + HTTP server | [STORY_03_webhook_trigger.md](STORY_03_webhook_trigger.md) | Not Started | High | 3h |
| GitHubIssueTrigger implementation | [STORY_04_github_issue_trigger.md](STORY_04_github_issue_trigger.md) | Not Started | High | 3h |
| SlackTrigger implementation | [STORY_05_slack_trigger.md](STORY_05_slack_trigger.md) | Not Started | Medium | 4h |
| Config, deploy manifests, Helm chart values | [STORY_06_config_deploy.md](STORY_06_config_deploy.md) | Not Started | Medium | 2h |

## Story execution order

STORY_00 and STORY_01 must run first. STORY_02 depends on both. STORY_03, STORY_04,
and STORY_05 can be parallelised — each is an independent `TriggerProvider` backend.
STORY_06 closes the epic.

```
STORY_00 (domain types)
STORY_01 (CRD types)
    └──> STORY_02 (trigger loop)
              ├──> STORY_03 (webhook)
              ├──> STORY_04 (github-issue)
              └──> STORY_05 (slack)
                        └──> STORY_06 (config + deploy)
```

## Technical Overview

### New files

| File | Purpose |
|------|---------|
| `internal/domain/trigger.go` | `TriggerProvider` interface, `FindingRequest`, `TriggerRef` types |
| `internal/domain/trigger_test.go` | Interface contract tests |
| `internal/trigger/loop.go` | `TriggerProviderLoop` — polls all registered providers, creates `RemediationJob` objects |
| `internal/trigger/loop_test.go` | Loop unit tests (mock providers) |
| `internal/trigger/webhook/trigger.go` | `WebhookTrigger` HTTP handler |
| `internal/trigger/webhook/trigger_test.go` | Unit tests (token auth, request parsing) |
| `internal/trigger/github/trigger.go` | `GitHubIssueTrigger` polling implementation |
| `internal/trigger/github/trigger_test.go` | Unit tests (mock `gh` output) |
| `internal/trigger/slack/trigger.go` | `SlackTrigger` Events API handler |
| `internal/trigger/slack/trigger_test.go` | Unit tests (HMAC validation, command parsing) |
| `internal/triggerserver/server.go` | Shared HTTP server, route registration, graceful shutdown |
| `internal/triggerserver/server_test.go` | Server lifecycle tests |

### Modified files

| File | Change |
|------|--------|
| `api/v1alpha1/remediationjob_types.go` | Add `Source`, `TriggerRef` to `RemediationJobSpec` |
| `cmd/watcher/main.go` | Instantiate and register trigger backends; start trigger loop and HTTP server |
| `internal/config/config.go` | Add all trigger configuration fields |
| `internal/config/config_test.go` | Config parsing tests |
| `deploy/kustomize/deployment-watcher.yaml` | Add trigger env vars; expose port `8083` |
| `deploy/kustomize/service-watcher.yaml` | New: Service exposing port `8083` for webhook ingress |
| `charts/mechanic/templates/deployment-watcher.yaml` | Same for Helm chart |
| `charts/mechanic/values.yaml` | New `trigger.*` values section |
| `testdata/crds/remediationjob_crd.yaml` | Add `source` and `triggerRef` to spec schema |

## Definition of Done

- [ ] All unit tests pass: `go test -timeout 30s -race ./...`
- [ ] `go build ./...` succeeds
- [ ] All three backends disabled by default; none start unless explicitly configured
- [ ] `WebhookTrigger`: a `curl -X POST` with the correct token creates a `RemediationJob`
      (manual verification in dev cluster)
- [ ] `GitHubIssueTrigger`: a GitHub issue with `mechanic-investigate` label is
      acknowledged and creates a `RemediationJob` (manual verification)
- [ ] `SlackTrigger`: `/investigate Deployment/production/my-app` creates a
      `RemediationJob` and posts a thread reply (manual verification)
- [ ] Duplicate `requestId` does not create a second `RemediationJob`
- [ ] `source=manual` visible in `kubectl get rjob` and audit log
- [ ] Worklog entry created in `docs/WORKLOGS/`
