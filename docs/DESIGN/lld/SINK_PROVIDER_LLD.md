# Domain: Sink Provider — Low-Level Design

**Version:** 1.0
**Date:** 2026-02-20
**Status:** Informational (v1 sink is prompt-layer only; Go interface deferred to post-v1)
**HLD Reference:** [Section 5.7](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

A `SinkProvider` is the output side of the provider pattern: it defines how the agent
delivers its result once an investigation is complete. In v1 the only sink is a GitHub
PR opened via the `gh` CLI — implemented entirely in the agent prompt and entrypoint
script, not in Go code.

This document defines:
1. What the sink abstraction means conceptually
2. How the v1 GitHub sink is implemented (prompt-layer)
3. The Go interface that will formalise this post-v1
4. How future sinks (Jira, Slack, GitLab) would integrate

### 1.2 Design Principles

- **Sink = agent behaviour** — in v1, the sink is a prompt instruction. The agent follows
  the prompt steps and calls the appropriate CLI tool (`gh`, `curl`, etc.).
- **No Go sink interface in v1** — the agent is a black box from the Go controller's
  perspective. The only feedback from the sink back to the controller is the optional
  `status.prRef` patch.
- **One PR per invocation** — enforced by the prompt's Hard Rules, not by Go code.
- **Sink configuration is prompt configuration** — adding a new sink means adding steps
  to the prompt (or a new prompt variant) and building a new agent image if new CLI tools
  are needed.

---

## 2. v1 Sink: GitHub PR

The v1 sink is fully specified in [PROMPT_LLD.md](PROMPT_LLD.md). Summary:

| Step | Action | Tool |
|---|---|---|
| Check for existing PR | `gh pr list --json ... --jq ...` | `gh` CLI |
| Create branch | `git checkout -b fix/k8sgpt-<fp>` | `git` |
| Commit changes | `git commit -m "fix(...)"` | `git` |
| Push branch | `git push origin ...` | `git` |
| Open PR | `gh pr create --repo ... --base main ...` | `gh` CLI |
| Add label (low confidence) | `gh pr edit --add-label needs-human-review` | `gh` CLI |
| Comment on existing PR | `gh pr comment <number> --body ...` | `gh` CLI |

**Authentication:** GitHub App installation token written by the init container to
`/workspace/github-token`. The entrypoint script authenticates `gh` with it before
passing control to the agent.

**Feedback to controller:** On completion, the agent patches `RemediationJob.status.prRef`
with the PR URL. This is best-effort — the agent exits 0 even if the patch fails.

---

## 3. Post-v1 Go Interface

When a second sink is needed, the sink abstraction will be formalised in Go. The
anticipated interface:

```go
// internal/sink/interface.go  (post-v1)
type SinkProvider interface {
    // Name returns a short identifier, e.g. "github-pr", "jira", "slack"
    Name() string

    // Deliver sends the investigation result to the sink.
    // result contains the full agent output (PR URL, summary, etc.)
    // It is called by the agent entrypoint script via a sidecar or by the
    // agent itself, not by the Go controller.
    Deliver(ctx context.Context, result AgentResult) error
}

type AgentResult struct {
    Fingerprint  string
    FindingKind  string
    FindingName  string
    Namespace    string
    PRUrl        string   // empty if no PR was opened
    Summary      string
    Confidence   string   // "high", "medium", "low"
}
```

This interface would be implemented by:
- `GitHubPRSinkProvider` — wraps the current `gh`-based prompt behaviour
- `JiraSinkProvider` — creates a Jira issue via REST API
- `SlackSinkProvider` — posts a message to a channel via webhook

### 3.1 Why Not in v1

Formalising a Go sink interface in v1 would require:
- A structured output format from the agent back to Go (JSON stdout or file)
- The agent entrypoint knowing which sink to call
- Wiring the sink configuration into the `RemediationJob` spec

All of this complexity is deferred. The v1 prompt already handles the one sink we need.

---

## 4. SinkType Field on RemediationJob (v1)

`RemediationJobSpec` already includes `sinkType` in v1. This is not a post-v1 addition:

```go
type RemediationJobSpec struct {
    // ...existing fields...
    SourceType string `json:"sourceType"` // "k8sgpt"
    SinkType   string `json:"sinkType"`   // "github" (default in v1)
}
```

The `RemediationJobReconciler` passes `SinkType` to the job builder, which injects it as the
`SINK_TYPE` env var into the agent Job. The agent entrypoint uses this to select the
appropriate sink behaviour. In v1 the only valid value is `"github"`.

Post-v1, additional `SinkType` values (e.g. `"gitlab"`, `"jira"`) would be added alongside
new agent entrypoint branches and prompt variants. The field and injection plumbing already
exist — only the agent-side implementation would be new.

---

## 5. Agent Image Sink Requirements (v1)

The following tools must be present in the `mendabot-agent` image for the GitHub PR sink:

| Tool | Purpose |
|---|---|
| `gh` | Create PRs, list PRs, add labels, comment |
| `git` | Branch, commit, push |

Both are already included in the v1 agent image. See [AGENT_IMAGE_LLD.md](AGENT_IMAGE_LLD.md).
