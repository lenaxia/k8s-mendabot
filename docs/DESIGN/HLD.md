# High-Level Design

**Version:** 1.3
**Date:** 2026-02-19
**Status:** Authoritative Specification

---

## Document Control

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-19 | Initial HLD | LLM / Human |
| 1.1 | 2026-02-19 | Design review fixes: init image, token path, FINDING_NAMESPACE, fingerprint, AGENT_NAMESPACE constraint, config table, RBAC, data flow | LLM / Human |
| 1.2 | 2026-02-20 | Operator pattern: introduce RemediationJob CRD, split controller into ResultReconciler + RemediationJobReconciler, replace in-memory map with CRD state | LLM / Human |
| 1.3 | 2026-02-20 | Provider/plugin pattern: SourceProvider interface, SinkProvider concept, ResultReconciler moves to internal/provider/k8sgpt/, sourceType field on RemediationJob | LLM / Human |

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Goals and Non-Goals](#2-goals-and-non-goals)
3. [System Overview](#3-system-overview)
4. [Component Design](#4-component-design)
5. [Provider Pattern](#5-provider-pattern)
6. [Data Flow](#6-data-flow)
7. [Deduplication Strategy](#7-deduplication-strategy)
8. [RBAC Design](#8-rbac-design)
9. [GitHub Authentication](#9-github-authentication)
10. [Agent Investigation Strategy](#10-agent-investigation-strategy)
11. [Security Constraints](#11-security-constraints)
12. [Failure Modes](#12-failure-modes)
13. [Configuration Reference](#13-configuration-reference)
14. [Deployment Model](#14-deployment-model)
15. [Upstream Contribution Path](#15-upstream-contribution-path)
16. [v1 Scope](#16-v1-scope)
17. [Success Criteria](#17-success-criteria)

---

## 1. Problem Statement

The k8sgpt-operator already analyses Kubernetes clusters and writes `Result` CRDs describing
problems it finds. These results include an AI-generated explanation of the problem. However,
nothing acts on them — a human must still read the results, investigate the cluster, locate
the relevant GitOps manifests, determine a fix, and open a PR.

This project automates that entire loop: from Result CRD to a PR on the GitOps repository
containing a proposed fix, with the investigation and reasoning documented inline.

---

## 2. Goals and Non-Goals

### Goals

- Watch `Result` CRDs cluster-wide and react to new findings
- Deduplicate findings so multiple pods from the same bad Deployment produce one investigation
- Spawn an isolated, short-lived Kubernetes Job per unique finding
- Give the agent read-only cluster access via in-cluster ServiceAccount
- Have the agent clone the GitOps repo, investigate, and open a PR with a proposed fix
- Avoid creating duplicate PRs — detect existing open PRs for the same finding
- Be self-contained: deploy via Kustomize, no external state store required

### Non-Goals

- Automatically merging PRs (human review required)
- Remediating cluster state directly (no `kubectl apply` from the agent)
- Replacing the k8sgpt-operator (this project depends on it)
- Supporting GitOps repos other than Flux + Kustomize/Helm (out of v1 scope)
- Persisting deduplication state across watcher restarts (acceptable limitation in v1)

---

## 3. System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                                  │
│                                                                      │
│  ┌──────────────────┐  writes   ┌──────────────────────────────┐   │
│  │  k8sgpt-operator │ ────────▶ │  Result CRDs                 │   │
│  │  (pre-existing)  │           │  (results.core.k8sgpt.ai)    │   │
│  └──────────────────┘           └──────────┬───────────────────┘   │
│                                             │ watch                 │
│                                  ┌──────────▼───────────────────┐  │
│                                  │  mendabot-watcher             │  │
│                                  │  (Deployment, 1 replica)      │  │
│                                  │                               │  │
│                                  │  K8sGPTSourceProvider         │  │
│                                  │  (internal/provider/k8sgpt/)  │  │
│                                  │  - ResultReconciler           │  │
│                                  │  - watches Result CRDs        │  │
│                                  │  - creates RemediationJob     │  │
│                                  │                               │  │
│                                  │  RemediationJobReconciler     │  │
│                                  │  (internal/controller/)       │  │
│                                  │  - watches RemediationJob     │  │
│                                  │  - creates batch/v1 Jobs      │  │
│                                  │  - syncs Job status back      │  │
│                                  └──────────┬──────────┬─────────┘  │
│                                             │ creates  │ watches    │
│                              ┌──────────────▼──┐  ┌────▼──────────┐│
│                              │ RemediationJob  │  │ batch/v1 Job  ││
│                              │ CRDs            │  │ (agent Job)   ││
│                              │ (remediation.   │  │               ││
│                              │  k8sgpt.ai)     │  │ initContainer ││
│                              │                 │  │ + mendabot-   ││
│                              │ spec.sourceType │  │   agent image ││
│                              │ status.phase    │  └───────────────┘│
│                              │ status.jobRef   │                    │
│                              │ status.prRef    │                    │
│                              └─────────────────┘                    │
└─────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼ opens PR (or comments)
                          ┌────────────────────────────────┐
                          │  lenaxia/talos-ops-prod         │
                          │  (Flux + Kustomize/Helm GitOps) │
                          └────────────────────────────────┘
```

---

## 4. Component Design

### 4.1 mendabot-watcher

A single-binary Go controller built on `controller-runtime`. It runs as a single-replica
Deployment in the `mendabot` namespace. It contains:

**K8sGPTSourceProvider** (`internal/provider/k8sgpt/`) — the v1 source provider:
- Owns a `ResultReconciler` that watches `results.core.k8sgpt.ai` across all namespaces
- Computes the parent-resource fingerprint of each Result
- Creates a `RemediationJob` CRD per unique fingerprint, setting `spec.sourceType: "k8sgpt"`
- Delegates all Job creation and status tracking to the RemediationJobReconciler

**RemediationJobReconciler** (`internal/controller/`) — provider-agnostic sink for all
`RemediationJob` objects regardless of which source created them:
- Watches `RemediationJob` objects in the `mendabot` namespace, and also watches owned
  `batch/v1 Jobs` via `Owns()`
- Creates `batch/v1 Jobs` for `RemediationJob` objects in `Pending` phase
- Enforces `MAX_CONCURRENT_JOBS` before creating each Job
- Re-triggers when the owned Job status changes, and patches `RemediationJob.Status`
- Sets `ownerReferences` on the Job so deletion cascades

**State:** Entirely in the `RemediationJob` CRD. Watcher restarts are fully safe — all
state is reconstructed by re-listing existing `RemediationJob` objects.

### 4.2 RemediationJob CRD

The project's own custom resource. Lives in group `remediation.mendabot.io/v1alpha1`.
One object per unique finding fingerprint. Tracks the full lifecycle:

| Field | Purpose |
|---|---|
| `spec.fingerprint` | Deduplication key — immutable after creation |
| `spec.sourceType` | Which provider created this object — e.g. `"k8sgpt"` |
| `spec.finding` | Extracted finding context (kind, name, errors, details) |
| `spec.sourceResultRef` | Back-reference to the k8sgpt Result that triggered this |
| `status.phase` | `Pending` → `Dispatched` → `Running` → `Succeeded` / `Failed` |
| `status.jobRef` | Name of the owned `batch/v1 Job` |
| `status.prRef` | GitHub PR URL (written by the agent on exit, best-effort) |

See [`REMEDIATIONJOB_LLD.md`](lld/REMEDIATIONJOB_LLD.md) for the full spec.

### 4.3 mendabot-agent Job

A `batch/v1 Job` created dynamically by the `RemediationJobReconciler` per
`RemediationJob`. Owned by the `RemediationJob` via `ownerReferences`.

**Init container** (`ghcr.io/lenaxia/mendabot-agent`):
- Calls `get-github-app-token.sh` to exchange the GitHub App private key for a
  short-lived installation token
- Writes the token to a shared `emptyDir` volume at `/workspace/github-token`
- Clones the GitOps repo using the token into `/workspace/repo`

**Main container** (`ghcr.io/lenaxia/mendabot-agent`):
- Receives the finding as environment variables (kind, name, errors, details, fingerprint)
- Reads the rendered prompt from a mounted ConfigMap
- Runs `opencode run --file <path>` with in-cluster kubeconfig (automatic, via ServiceAccount)
- On completion, patches `RemediationJob.status.prRef` with the opened PR URL (best-effort)

**Job settings:**

| Setting | Value | Reason |
|---|---|---|
| `restartPolicy` | `Never` | Failed investigations should not silently retry |
| `backoffLimit` | `1` | Allow one retry on container crash only |
| `activeDeadlineSeconds` | `900` | 15 min hard cap; prevents runaway LLM sessions |
| `ttlSecondsAfterFinished` | `86400` | Clean up after 1 day |
| Name | `mendabot-agent-<12-char-fingerprint>` | Deterministic, collision-resistant |

### 4.4 mendabot-agent Docker image

Built on `debian:bookworm-slim`. Unchanged from the original design — see
[`AGENT_IMAGE_LLD.md`](lld/AGENT_IMAGE_LLD.md).

---

## 5. Provider Pattern

### 5.1 Motivation

The operator pattern (§4) defines a stable internal contract — the `RemediationJob` CRD.
The provider pattern defines how external signals arrive at that contract (sources) and how
the agent's outputs reach their destination (sinks). Separating these layers means new
signal types can be added without touching the core reconciliation logic.

### 5.2 SourceProvider Interface

A `SourceProvider` is responsible for:
1. Watching an external signal source (e.g. k8sgpt `Result` CRDs, Prometheus alerts)
2. Translating each signal into a `RemediationJob` object and creating it via the
   Kubernetes API

The interface is defined in `internal/provider/`:

```go
// internal/provider/interface.go
type SourceProvider interface {
    // SetupWithManager registers the provider's controller(s) with the manager.
    // Called once at startup from main.go.
    SetupWithManager(mgr ctrl.Manager) error
}
```

This is deliberately minimal. Each provider owns its own reconciler and registers it
with the manager. The `RemediationJob` CRD is the single handoff point.

### 5.3 Built-in Providers (v1)

| Provider | Package | Signal source | Status |
|---|---|---|---|
| `K8sGPTSourceProvider` | `internal/provider/k8sgpt/` | `results.core.k8sgpt.ai` CRDs | v1 |

Future providers (post-v1, tracked as separate epics):
- `PrometheusSourceProvider` — alert rules firing in Alertmanager
- `DatadogSourceProvider` — Datadog events via webhook receiver

### 5.4 Package Structure

```
internal/
├── provider/
│   ├── interface.go               # SourceProvider interface
│   └── k8sgpt/
│       ├── provider.go            # K8sGPTSourceProvider struct + SetupWithManager
│       ├── reconciler.go          # ResultReconciler (watches Result CRDs)
│       └── reconciler_test.go
```

The `ResultReconciler` moves from `internal/controller/` to
`internal/provider/k8sgpt/`. It is no longer a "controller" in the generic sense —
it is the k8sgpt source provider's implementation detail.

`internal/controller/` retains only the `RemediationJobReconciler`, which is
provider-agnostic and handles all `RemediationJob` objects regardless of source.

### 5.5 Provider Registration in main.go

```go
// cmd/watcher/main.go (provider registration block)
providers := []provider.SourceProvider{
    k8sgpt.NewProvider(cfg, logger),
}
for _, p := range providers {
    if err := p.SetupWithManager(mgr); err != nil {
        log.Fatal("provider setup failed", zap.Error(err))
    }
}
```

This makes the set of active providers explicit and auditable at startup. Adding a
new provider requires one line here and nothing else in `main.go`.

### 5.6 sourceType Field on RemediationJob

`RemediationJob.Spec` gains a `sourceType` field that records which provider created
the object. This is informational only (does not affect reconciliation) but aids
debugging and future filtering.

```go
type RemediationJobSpec struct {
    // ...existing fields...
    SourceType string `json:"sourceType"` // e.g. "k8sgpt", "prometheus"
}
```

The `K8sGPTSourceProvider` always sets `SourceType: "k8sgpt"`.

### 5.7 SinkProvider Concept (Prompt Layer)

A `SinkProvider` is the output side: where the agent delivers its result. v1 has one
sink: GitHub PR via the `gh` CLI. The sink is not a Go interface — it is implemented
entirely in the agent prompt and the `agent-entrypoint.sh` script. The agent is
instructed to use `gh` commands; future sinks (Jira ticket, Slack message) would be
additional steps appended to the prompt.

A formal `SinkProvider` Go interface is deferred to post-v1. The agent prompt is
the extensibility point for sinks in v1. See [PROMPT_LLD.md](lld/PROMPT_LLD.md) and
[SINK_PROVIDER_LLD.md](lld/SINK_PROVIDER_LLD.md) for details.

---

## 6. Data Flow

```
1. k8sgpt-operator writes Result CRD
      result.spec.kind         = "Pod"
      result.spec.parentObject = "my-deployment"
      result.spec.error[]      = [{text: "Back-off restarting failed container"}]
      result.spec.details      = "<LLM explanation>"

2. ResultReconciler triggered
      fingerprint = sha256("Pod" + "my-deployment" + sorted(["Back-off..."]))
      list RemediationJobs with label remediation.mendabot.io/fingerprint=<fp>
      → none found → create RemediationJob "mendabot-a3f9c2b14d8e"
        spec.fingerprint = "<full 64-char fp>"
        spec.finding.kind = "Pod"
        spec.finding.parentObject = "my-deployment"
        ...
        status.phase = "Pending"

3. RemediationJobReconciler triggered (by RemediationJob creation)
      check MAX_CONCURRENT_JOBS → under limit
      jobBuilder.Build(rjob) → Job "mendabot-agent-a3f9c2b14d8e"
        ownerReference → RemediationJob "mendabot-a3f9c2b14d8e"
        env: FINDING_KIND=Pod
             FINDING_NAME=my-deployment-abc12-xyz34
             FINDING_NAMESPACE=default
             FINDING_PARENT=my-deployment
             FINDING_ERRORS=[{"text":"Back-off restarting failed container"}]
             FINDING_DETAILS=<LLM text>
             FINDING_FINGERPRINT=a3f9c2b14d8e...
             GITOPS_REPO=lenaxia/talos-ops-prod
             GITOPS_MANIFEST_ROOT=kubernetes
      patch RemediationJob.status.phase = "Dispatched"
      patch RemediationJob.status.jobRef = "mendabot-agent-a3f9c2b14d8e"

4. Job init container
      → get-github-app-token.sh → writes /workspace/github-token
      → git clone https://x-access-token:<token>@github.com/lenaxia/talos-ops-prod /workspace/repo

5. Job main container
      → opencode run --file /tmp/rendered-prompt.txt
      → OpenCode calls: kubectl describe pod, kubectl get events,
                        k8sgpt analyze, gh pr list, git diff, gh pr create
      → on completion: kubectl patch remediationjob mendabot-a3f9c2b14d8e
                         --subresource=status --patch '{"status":{"prRef":"<url>"}}'

6. RemediationJobReconciler re-triggered (by Job status change via Owns())
      Job.Status.Succeeded > 0 → patch RemediationJob.status.phase = "Succeeded"
      patch RemediationJob.status.completedAt = now

7. Outcome A — fix found:
      → branch: fix/k8sgpt-a3f9c2b14d8e
      → PR title: "fix(Pod/my-deployment): back-off restarting failed container"
      → PR body: investigation findings + proposed change + k8sgpt reference
      → RemediationJob.status.prRef = "<PR URL>"

   Outcome B — existing PR found:
      → gh issue comment on existing PR with updated findings

   Outcome C — no safe fix identified:
      → PR opened with investigation report only, labelled "needs-human-review"
```

---

## 7. Deduplication Strategy

### Fingerprint algorithm

The fingerprint is a deterministic SHA256 hash that incorporates the Result's namespace,
kind, parent object name, and sorted error texts. The authoritative algorithm is defined in
[CONTROLLER_LLD.md §4](docs/DESIGN/lld/CONTROLLER_LLD.md). The HLD does not duplicate it
to avoid divergence — the LLD is the single source of truth for the exact implementation.

**Key properties:**
- Includes `namespace` — prevents cross-namespace collisions between same-named parents
- Uses `parentObject` not the resource name — collapses multiple pods from one Deployment
- Error texts are sorted — ordering in the CRD is non-deterministic
- Full 64-char hex SHA256 used as branch name; first 12 chars used as object name suffix

### Deduplication mechanism

Deduplication is now performed via the Kubernetes API — no in-memory state:

1. `ResultReconciler` lists `RemediationJob` objects in the `mendabot` namespace with the
   label `remediation.mendabot.io/fingerprint=<first-12-of-fp>`
2. If a `RemediationJob` exists and its phase is not `Failed`, skip
3. If no matching object exists (or the existing one is `Failed`), create a new one

This is safe across restarts: the `RemediationJob` objects persist in etcd.

### When a new RemediationJob is triggered despite an existing one

- The error texts change (different fingerprint → new object)
- The existing `RemediationJob` is in `Failed` phase (re-dispatch is safe)
- The existing `RemediationJob` was deleted manually

### Watcher restart safety

On restart, all Result CRDs re-reconcile. The `ResultReconciler` lists existing
`RemediationJob` objects and skips any with a non-Failed phase. No race condition exists
because the list is against the API server, not in-memory state.

---

## 8. RBAC Design

### mendabot-watcher ServiceAccount

| Resource | Verbs | Scope |
|---|---|---|
| `results.core.k8sgpt.ai` | `get`, `list`, `watch` | ClusterRole (all namespaces) |
| `namespaces` | `get`, `list` | ClusterRole |
| `remediationjobs.remediation.mendabot.io` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` | ClusterRole |
| `remediationjobs/status` | `get`, `patch`, `update` | ClusterRole |
| `jobs.batch` | `get`, `list`, `create`, `watch`, `delete` | Role (own namespace only) |
| `pods` | `get`, `list` | Role (own namespace only) |

### mendabot-agent ServiceAccount

| Resource | Verbs | Scope |
|---|---|---|
| `*` (all resources) | `get`, `list`, `watch` | ClusterRole (all namespaces) |
| `remediationjobs/status` | `get`, `patch` | Role (mendabot namespace only) |

The read-only ClusterRole mirrors permissions already granted to the k8sgpt deployment.
The status patch Role allows the agent to write the PR URL back to its `RemediationJob`.

---

## 9. GitHub Authentication

The agent uses a **GitHub App**, not a PAT. A GitHub App:
- Issues short-lived tokens (1 hour expiry) rather than long-lived credentials
- Has granular repository-level permissions
- Does not expose a personal account

**Required GitHub App permissions:**
- `Contents: Write` (create branches, push commits)
- `Pull requests: Write` (create PRs, add comments)
- `Issues: Write` (add comments to issues)

**Secret structure (Kubernetes Secret `github-app`):**

```yaml
data:
  app-id: <base64-encoded App ID>
  installation-id: <base64-encoded Installation ID>
  private-key: <base64-encoded PEM private key>
```

**Token exchange flow:**

```
init container
  → reads app-id, installation-id, private-key from mounted Secret
  → get-github-app-token.sh:
      1. Creates JWT signed with private key (RS256, 10 min expiry)
      2. POST /app/installations/<id>/access_tokens → installation token
      3. Writes token to /workspace/github-token
  → git clone https://x-access-token:<token>@github.com/... /workspace/repo
  → exits

main container
  → reads /workspace/github-token
  → gh auth login --with-token < /workspace/github-token
  → proceeds with investigation
```

---

## 10. Agent Investigation Strategy

The OpenCode agent receives the finding context via environment variables and a rendered
prompt. The prompt instructs OpenCode to follow this investigation sequence:

1. **Search for existing PRs** — `gh pr list --search "fix/k8sgpt-<fingerprint>"`. If found,
   add a comment and exit. Do not create a duplicate.

2. **Inspect the specific resource** — `kubectl describe <kind> <name> -n <namespace>`

3. **Check events** — `kubectl get events -n <namespace> --field-selector involvedObject.name=<name>`

4. **Check logs if Pod** — `kubectl logs <pod> -n <namespace> --previous`

5. **Run k8sgpt** — `k8sgpt analyze --filter <kind> --namespace <namespace> --explain`

6. **Locate GitOps manifests** — search `<GITOPS_MANIFEST_ROOT>/` in the cloned repo for
   the resource name and namespace

7. **Read related manifests** — HelmRelease, Kustomization, values files

8. **Determine root cause** — based on all gathered evidence

9. **Propose a fix** — targeted change to the GitOps manifests

10. **Open PR** — branch `fix/k8sgpt-<fingerprint>`, one PR with the minimum change needed.
    If no safe fix is determinable, open a PR with an investigation report only.

**Hard rules for the agent (enforced in the prompt):**
- Never commit to `main` directly
- Never create, modify, or reference Kubernetes Secrets
- One PR per invocation
- If uncertain, open an investigation-report PR rather than guessing

---

## 11. Security Constraints

| Constraint | Enforcement |
|---|---|
| Agent is read-only on the cluster | ClusterRole with only `get/list/watch` — no mutating verbs |
| Agent cannot write cluster Secrets | Follows from read-only RBAC |
| Agent can read cluster Secrets | `get/list/watch` on `["*"]["*"]` includes Secrets — this is a conscious accepted risk matching the permissions already granted to the k8sgpt-operator itself. Operators who consider this unacceptable must restrict the ClusterRole explicitly. |
| Agent cannot push to GitOps repo main directly | GitHub branch protection rules on target repo |
| GitHub token is short-lived | GitHub App installation token, 1 hour TTL |
| GitHub App private key never leaves the cluster | Mounted as K8s Secret into init container only; never injected into main container env |
| LLM API key never leaves the cluster | Mounted as K8s Secret, never logged or printed |
| Job has a hard deadline | `activeDeadlineSeconds: 900` |
| Job cleans itself up | `ttlSecondsAfterFinished: 86400` |
| Prompt injection risk | `FINDING_DETAILS` and `FINDING_ERRORS` originate from k8sgpt's LLM analysis of cluster state, which may be influenced by application log output or error messages an attacker can control. A crafted error message could attempt to override the agent's hard rules. Mitigations: hard rules are stated prominently in the prompt; GitHub branch protection prevents direct pushes to main; human review is required to merge any PR. |

---

## 12. Failure Modes

| Failure | Behaviour |
|---|---|
| RemediationJob creation fails | ResultReconciler returns error, controller-runtime requeues |
| RemediationJob already exists | ResultReconciler skips (dedup by fingerprint label) |
| Job creation fails (API error) | RemediationJobReconciler returns error, requeues with backoff |
| Job already exists | RemediationJobReconciler re-fetches, syncs status, moves on |
| Agent exceeds 15 min deadline | Job killed → RemediationJob.status.phase = Failed |
| Agent crashes | Job retries once (`backoffLimit: 1`), then Failed → phase = Failed |
| OpenCode finds no fix | Agent opens investigation-report PR; exits 0; phase = Succeeded |
| GitHub token exchange fails | Init container exits non-zero → Job Failed → phase = Failed |
| GitOps repo clone fails | Init container exits non-zero → same as above |
| Watcher restarts | RemediationJob CRDs survive; ResultReconciler skips non-Failed ones |
| Status patch (prRef) fails | Logged; agent exits 0 anyway; PR still exists on GitHub |

---

## 13. Configuration Reference

### Watcher Deployment environment variables

| Variable | Required | Description |
|---|---|---|
| `GITOPS_REPO` | Yes | GitHub repo in `owner/repo` format, e.g. `lenaxia/talos-ops-prod` |
| `GITOPS_MANIFEST_ROOT` | Yes | Path within the cloned repo to the manifests root, e.g. `kubernetes` |
| `AGENT_IMAGE` | Yes | Full image ref for the agent, e.g. `ghcr.io/lenaxia/mendabot-agent:latest` |
| `AGENT_NAMESPACE` | Yes | Namespace where agent Jobs are created — **must equal the watcher's own namespace** |
| `AGENT_SA` | Yes | ServiceAccount name for agent Jobs |
| `LOG_LEVEL` | No | `debug`, `info` (default), `warn`, `error` |
| `MAX_CONCURRENT_JOBS` | No | Max agent Jobs running at once, default `3` — enforced by counting Jobs with `app.kubernetes.io/managed-by: mendabot-watcher` label |

### Agent Job environment variables (injected by watcher)

| Variable | Source |
|---|---|
| `FINDING_KIND` | `result.spec.kind` |
| `FINDING_NAME` | `result.spec.name` (plain name, no namespace prefix) |
| `FINDING_NAMESPACE` | `result.metadata.namespace` (ObjectMeta namespace of the Result) |
| `FINDING_PARENT` | `result.spec.parentObject` |
| `FINDING_ERRORS` | `json(result.spec.error)` — `Sensitive` fields redacted before injection |
| `FINDING_DETAILS` | `result.spec.details` |
| `FINDING_FINGERPRINT` | computed sha256 |
| `GITOPS_REPO` | from watcher env |
| `GITOPS_MANIFEST_ROOT` | from watcher env |
| `OPENAI_API_KEY` | from Secret `llm-credentials`, key `api-key` |
| `OPENAI_BASE_URL` | from Secret `llm-credentials`, key `base-url` (optional) |
| `OPENAI_MODEL` | from Secret `llm-credentials`, key `model` (optional) |

**Note:** `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, and `GITHUB_APP_PRIVATE_KEY` are
injected into the **init container only**. The main container reads the short-lived
installation token from `/workspace/github-token`. The private key must never be present
in the main container's environment.

---

## 14. Deployment Model

All Kubernetes resources are managed via Kustomize in `deploy/kustomize/`. The directory is
designed to be referenced directly from a Flux `Kustomization` resource in the GitOps repo.

Resources created:
- `Namespace: mendabot`
- `CustomResourceDefinition: remediationjobs.remediation.mendabot.io`
- `ServiceAccount: mendabot-watcher` (in `mendabot` namespace)
- `ServiceAccount: mendabot-agent` (in `mendabot` namespace)
- `ClusterRole: mendabot-watcher` (Result + RemediationJob read/write + Namespace read)
- `ClusterRole: mendabot-agent` (cluster-wide read-only)
- `ClusterRoleBinding: mendabot-watcher`
- `ClusterRoleBinding: mendabot-agent`
- `Role: mendabot-watcher` (Job + Pod read/create in own namespace)
- `RoleBinding: mendabot-watcher`
- `Role: mendabot-agent` (RemediationJob status patch in mendabot namespace)
- `RoleBinding: mendabot-agent`
- `ConfigMap: opencode-prompt` (prompt template)
- `Secret: github-app` (placeholder — fill manually)
- `Secret: llm-credentials` (placeholder — fill manually)
- `Deployment: mendabot-watcher`

---

## 15. Upstream Contribution Path

Once v1 is stable and battle-tested:

1. Open a discussion in `k8sgpt-ai/k8sgpt-operator` proposing a `GitOpsRemediation`
   controller as an optional component
2. Refactor the watcher to be configurable (not hardcoded to OpenCode — pluggable agent
   command)
3. Generalise the prompt and GitOps assumptions (not Flux-specific)
4. Submit as a `contrib/` addition initially, with a path to first-class support

---

## 16. v1 Scope

**In scope:**
- Watcher controller watching all namespaces
- In-memory deduplication by parent fingerprint
- One agent Job per unique finding
- Debian-slim agent image with opencode + kubectl + k8sgpt + helm + flux + gh
- GitHub App authentication
- Kustomize manifests
- GitHub Actions CI for both images (ghcr.io)
- Full test coverage for watcher (TDD)

**Out of scope for v1:**
- Persistent deduplication state (Redis, ConfigMap)
- Auto-merging PRs
- Slack/webhook notifications
- Supporting non-Flux GitOps patterns
- Multi-cluster support
- Web UI or dashboard

---

## 17. Success Criteria

- [ ] Watcher starts, connects to cluster, and begins watching Result CRDs
- [ ] A new Result CRD triggers exactly one Job
- [ ] Multiple pods from the same Deployment produce one Job, not many
- [ ] Changed error text on an existing Result produces a new Job
- [ ] Agent Job completes within 15 minutes for a typical finding
- [ ] Agent opens a PR on the GitOps repo with a relevant proposed change
- [ ] Agent comments on an existing PR instead of opening a duplicate
- [ ] All watcher unit tests pass with race detector enabled
- [ ] Both images build and push successfully via GitHub Actions
- [ ] Kustomize manifests apply cleanly to a cluster (`--dry-run=client` passes)
