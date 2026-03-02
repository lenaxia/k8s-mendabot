# k8s-mechanic

> **Name change:** This project was previously known as **k8s-mendabot**. It has been
> renamed to **k8s-mechanic**. The CRD API group has changed from `remediation.mendabot.io`
> to `remediation.mechanic.io`. Existing installations will require migration.

k8s-mechanic is a Kubernetes controller that watches your cluster for failures,
investigates them automatically, and opens pull requests on your GitOps repository
with proposed fixes — all without leaving your cluster.

When a Pod is crash-looping, a Deployment is degraded, or a Node goes NotReady,
mechanic spawns an in-cluster [OpenCode](https://opencode.ai) agent that inspects
the live cluster, locates the relevant manifests in your GitOps repo, determines the
root cause, and opens a PR. You review and merge. No external operators, no
external databases, no persistent services outside your cluster.

## What it does

1. **Detects failures** — watches Pods, Deployments, StatefulSets, PVCs, Nodes, and
   Jobs natively via the Kubernetes API
2. **Deduplicates by parent** — repeated pod restarts from the same Deployment produce
   one investigation, not one per pod restart
3. **Stabilises before acting** — a configurable window (default: 120s) filters
   transient blips before dispatching
4. **Investigates in-cluster** — an agent Job runs with read-only RBAC, clones your
   GitOps repo, and inspects the live cluster
5. **Opens a PR** — with a structured body: summary, evidence, root cause, proposed
   fix, and confidence level

**Three possible outcomes per invocation:**

| Outcome | When | Action |
|---|---|---|
| Fix PR | Root cause identified, confidence medium or high | Opens a PR with a targeted manifest change |
| Investigation PR | Root cause unclear or confidence low | Opens a PR with an investigation report, labelled `needs-human-review` |
| Comment | An open PR already exists for this fingerprint | Comments with updated findings; no new PR |

Hard constraints enforced in the agent prompt: never commit directly to `main`; never
touch Kubernetes Secrets in the GitOps repo; exactly one outcome per invocation.

## Features

**[OpenCode agentic workflow](docs/WORKLOGS/0071_2026-02-23_epic08-pluggable-agent-complete.md)** — investigations are driven by [OpenCode](https://opencode.ai)
running inside your cluster. Works with any OpenAI-compatible LLM endpoint. Additional agent backends are planned.

**[Detection](docs/WORKLOGS/0033_2026-02-22_epic09-native-provider-complete.md)** — watches Pods, Deployments, StatefulSets, PVCs, Nodes, and Jobs natively.
Covers `CrashLoopBackOff`, `ImagePullBackOff`, `OOMKilled`, degraded Deployments, unschedulable pods, failed Jobs, PVC provisioning failures, and unhealthy Nodes.

**[Deduplication](docs/WORKLOGS/0011_2026-02-20_epic01-controller-core-logic.md)** — findings are deduplicated by parent resource fingerprint
(`sha256(namespace + kind + parentObject + sorted errors)`). Repeated pod restarts from
the same Deployment produce one investigation. State is stored in `RemediationJob` CRD
objects — survives watcher restarts, no external store required.

**[Severity tiers](docs/WORKLOGS/0075_2026-02-24_epic24-severity-tiers-complete.md)** — every finding is classified as `critical`, `high`, `medium`, or `low`
based on the detected condition (e.g. CrashLoopBackOff >5 restarts → critical; OOMKilled → high;
degraded-but-available Deployment → medium). A `MIN_SEVERITY` env var on the watcher Deployment
suppresses findings below the configured threshold. The agent receives the severity at runtime
and calibrates its investigation depth accordingly — maximum thoroughness for critical, conservative
minimal-change proposals for low.

**[Stabilisation window](docs/WORKLOGS/0030_2026-02-22_epic09-story12-stabilisation-window.md)** — a configurable hold period (default: 120s) suppresses
transient blips before an investigation is dispatched.

**[Concurrency throttling](docs/WORKLOGS/0011_2026-02-20_epic01-controller-core-logic.md)** — `maxConcurrentJobs` (default: 3) caps simultaneous agent
Jobs. Excess findings queue as `Pending` and dispatch as slots become available.

**[Customisable agent prompt](docs/WORKLOGS/0071_2026-02-23_epic08-pluggable-agent-complete.md)** — the investigation prompt is mounted from a ConfigMap and
can be fully overridden via `prompt.coreOverride` / `prompt.agentOverride` in
`values.yaml`.

**[Prometheus metrics](docs/WORKLOGS/0038_2026-02-23_epic10-helm-chart-implementation.md)** — optional metrics Service and Prometheus Operator
`ServiceMonitor` for watcher health observability.

**[Auto-close resolved findings](docs/WORKLOGS/0091_2026-02-26_epic26-auto-close-resolved.md)** — when a Kubernetes finding clears (Deployment recovers,
PVC is provisioned, Node returns Ready), the watcher automatically closes the GitHub PR the agent
opened. Works for both in-flight jobs (Pending/Dispatched/Running) and already-succeeded jobs whose
PR became stale after the cluster self-healed. Uses the GitHub App installation token directly via
REST API — no `gh` CLI required in the watcher. Opt out with `watcher.prAutoClose: false`.

**[PR-merge-aware deduplication](https://github.com/lenaxia/k8s-mechanic/pull/23)** — when a PR opened by mechanic is merged, the
`RemediationJob` is tombstoned with a short TTL (1 hour) rather than the default 7-day TTL. This
prevents the same finding from being re-investigated immediately after a merge while the GitOps
reconciler applies the fix and the cluster stabilises.

**[Dry-run mode](docs/WORKLOGS/0089_2026-02-25_epic20-dry-run-mode.md)** — set `watcher.dryRun: true` to run the full investigation
pipeline without opening any PRs. The agent produces an investigation report written to a
`mechanic-dryrun-<fingerprint>` ConfigMap instead. Useful for validating mechanic behaviour in
staging or validating a new LLM model before enabling it in production.

**[GitHub App token expiry guard](docs/WORKLOGS/0088_2026-02-25_epic22-token-expiry-guard.md)** — the main agent container checks the
installation token's expiry before proceeding. If the token has expired or is within 60 seconds of
expiry the job fails fast with a clear error rather than silently failing deep into the investigation.

**[Mandatory manifest validation](docs/WORKLOGS/0087_2026-02-25_epic18-manifest-validation.md)** — before committing any change to the GitOps
repo, the agent runs `kubeconform` (and `kustomize build` for overlay changes) on every modified
manifest. If validation fails the agent opens a placeholder PR labelled `validation-failed` with
the full error output rather than committing a schema-invalid manifest.

### Security

**[Secret redaction](docs/WORKLOGS/0054_2026-02-23_story01-secret-redaction.md)** — error text extracted from cluster state (pod `Waiting.Message`,
node condition messages, etc.) is passed through a redaction filter before being stored
in `RemediationJob` or injected into the agent. Patterns include URL credentials,
base64-encoded values ≥ 40 chars, and common secret key prefixes (`password=`,
`token=`, `api-key=`, etc.).

**[Prompt injection detection](docs/WORKLOGS/0055_2026-02-23_story05-prompt-injection-defence.md)** — `Finding.Errors` is bounded to 500 characters per
field and wrapped in an explicit untrusted-data envelope in the prompt. Injection
heuristics (`ignore.*previous.*instructions`) are detected and logged; configurable to
suppress the finding entirely (`INJECTION_DETECTION_ACTION=suppress`).

**[Agent network policy](docs/WORKLOGS/0057_2026-02-23_story02-network-policy.md)** — an opt-in `NetworkPolicy` restricts agent Job egress to the
cluster API server, GitHub, and the LLM endpoint. Enabled via `networkPolicy.enabled: true`
in `values.yaml`. Requires a CNI that enforces `NetworkPolicy` (Cilium, Calico, etc.).

**[Read-only agent RBAC](docs/WORKLOGS/0016_2026-02-20_epic04-deploy-manifests.md)** — the agent holds only `get/list/watch` verbs cluster-wide.
It cannot create, modify, or delete any Kubernetes resource. All cluster changes go
through Git and your GitOps reconciler.

**[Namespace-scoped agent RBAC](docs/WORKLOGS/0058_2026-02-23_story04-agent-rbac-scoping.md)** — `AGENT_RBAC_SCOPE=namespace` switches the agent from
a cluster-wide `ClusterRole` to a namespace-scoped `Role`, limiting what the agent can
read to the namespaces you specify.

**[Structured audit log](docs/WORKLOGS/0056_2026-02-23_story03-audit-log.md)** — all suppression and dispatch decisions emit structured log
lines with `audit: true`, queryable from any log aggregation system (Loki,
Elasticsearch, Datadog) for post-incident forensics.

**[Trivy CVE scanning](https://github.com/lenaxia/k8s-mechanic/actions)** — both `mechanic-watcher` and `mechanic-agent` images are scanned on every release with [Trivy](https://trivy.dev) (`CRITICAL` and `HIGH`, ignore-unfixed). The build fails if any fixable vulnerability is detected. Unfixable CVEs in upstream pre-built binaries (tools not yet released with the required Go version) are tracked in [`.trivyignore`](.trivyignore) with mandatory expiry dates for re-evaluation.

**[Short-lived GitHub credentials](docs/WORKLOGS/0014_2026-02-20_epic03-agent-image-complete.md)** — the agent never holds a long-lived PAT. A GitHub
App installation token (1-hour TTL) is exchanged in the init container and never
exposed to the main agent container.

#### Hardened mode

Hardened mode (`agent.hardenKubectl: true` in `values.yaml`, on by default) adds
additional read restrictions on top of the always-on defaults below.

**Always on (regardless of hardened mode)**

| Control | What it does |
|---|---|
| **kubectl write blocking** | `apply`, `create`, `delete`, `edit`, `patch`, `replace`, `scale`, `label`, `annotate`, `taint`, `drain`, `cordon`, `uncordon`, `rollout restart/undo` — all exit 1. All cluster changes go through Git and your GitOps reconciler. |
| **kubectl output redaction** | All `kubectl` output is piped through the `redact` binary. Any value matching a known secret pattern (`base64 ≥ 40 chars`, `password=…`, `token=…`, etc.) is replaced with `[REDACTED]`. The wrapper hard-fails if `redact` is missing. |
| **Tool output redaction** | `helm`, `flux`, `sops`, `talosctl`, `yq`, `stern`, `kubeconform`, `kustomize`, `age`, `age-keygen`, and `gh` all have PATH-shadowing wrappers that pipe output through `redact`. Wrappers fail closed — if the `redact` binary is absent the tool exits 1. |
| **No secrets in environment variables** | No credentials or API keys are present in the agent process environment. All secret material is written to files before the agent starts and removed from the environment entirely. |

**Hardened mode only**

| Control | What it adds |
|---|---|
| **kubectl secret blocking** | `get secret(s)`, `describe secret(s)`, and `get all` are blocked. Kubernetes Secrets never reach the LLM context via `kubectl`. |
| **kubectl exec / port-forward blocking** | `exec` and `port-forward` are blocked. The agent cannot open interactive sessions or forward ports to cluster workloads. |

**Exfiltration testing**

The security controls are validated in regular red-team runs. Results and the full exfiltration leak registry are in
[`docs/SECURITY/EXFIL_LEAK_REGISTRY.md`](docs/SECURITY/EXFIL_LEAK_REGISTRY.md).

## Quick Start

### Prerequisites

- Kubernetes >= 1.28
- Helm >= 3.14
- A GitHub App installed on your GitOps repository with: Contents (write), Pull Requests (write), Issues (write)
- An OpenAI-compatible LLM API key

#### GitHub App permissions

| Permission | Level | Purpose |
|---|---|---|
| Contents | Write | Clone repository, create branches, push changes |
| Pull requests | Write | Create and comment on pull requests |
| Issues | Write | Reference issues in PR descriptions |

### 1. Create required Secrets

```sh
kubectl create namespace mechanic
```

#### github-app

The `github-app` Secret must contain three keys:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-app
  namespace: mechanic
stringData:
  app-id: "<App ID>"             # numeric ID from https://github.com/settings/apps/<your-app-name>
  installation-id: "12345678"   # numeric ID from the installation URL (see below)
  private-key: |
    <contents of the .pem file downloaded from your GitHub App settings>
```

The **App ID** is shown on the settings page for your GitHub App at `https://github.com/settings/apps/<your-app-name>`. Each user creates their own GitHub App; the project author has no visibility into your credentials, tokens, or repository.

The **Installation ID** is the numeric suffix in the URL when you view your app's
installation: `https://github.com/organizations/<org>/settings/installations/<id>`
(personal accounts: `https://github.com/settings/installations/<id>`).
It is also returned by `GET https://api.github.com/app/installations` authenticated
with the App JWT.

The private key is used only in the agent Job's init container to exchange a
short-lived installation token (1-hour TTL). It is never injected into the main
agent container.

#### llm-credentials-opencode

The `llm-credentials-opencode` secret holds the full
[OpenCode config](https://opencode.ai/docs) as its `provider-config` key.
The correct schema has `model` as a **top-level** key (format: `"<provider-id>/<model-id>"`);
`options` belongs **inside** `provider.<name>`, not at the root.

Opencode is the only agentic provider available at the moment, more options will be coming later.

**Native OpenAI (`api.openai.com`)**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials-opencode
  namespace: mechanic
stringData:
  provider-config: |
    {
      "$schema": "https://opencode.ai/config.json",
      "provider": {
        "openai": {
          "apiKey": "sk-<your-openai-api-key>"
        }
      },
      "model": "openai/gpt-4o"
    }
```

**Custom OpenAI-compatible endpoint (self-hosted, Ollama, Azure, etc.)**

For any endpoint that is not `api.openai.com`, or that uses a model name not
registered in the built-in OpenAI provider, you must define a custom provider
with `"npm": "@ai-sdk/openai-compatible"`. You cannot reuse the built-in
`openai` provider for a different base URL.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials-opencode
  namespace: mechanic
stringData:
  provider-config: |
    {
      "$schema": "https://opencode.ai/config.json",
      "provider": {
        "myprovider": {
          "npm": "@ai-sdk/openai-compatible",
          "name": "My Provider",
          "options": {
            "baseURL": "https://my-llm-endpoint/v1",
            "apiKey": "sk-<your-api-key>"
          },
          "models": {
            "my-model-id": {
              "name": "My Model Name"
            }
          }
        }
      },
      "model": "myprovider/my-model-id"
    }
```

> **Note:** The agent also accepts the config via the `OPENCODE_CONFIG_CONTENT`
> environment variable (the full JSON string). This is the highest-precedence
> config layer and overrides the secret. All standard OpenCode schema keys are
> valid (`model`, `provider`, `$schema`, etc.).

**Other providers**

OpenCode supports 75+ providers. Any provider with an OpenAI-compatible API
(Ollama, LM Studio, llama.cpp, Azure OpenAI, Groq, Together AI, OpenRouter,
DeepSeek, and many more) works with the custom-provider pattern shown above.
For built-in providers (Anthropic, Amazon Bedrock, Google Vertex AI, GitHub
Copilot, etc.) the config structure differs slightly — consult the full
provider directory in the OpenCode docs:

- **Provider directory** — [opencode.ai/docs/providers](https://opencode.ai/docs/providers/)
- **Built-in providers** (Anthropic, Bedrock, Vertex, Groq, …) — config examples for each
- **Custom provider pattern** — [opencode.ai/docs/providers#custom-provider](https://opencode.ai/docs/providers/#custom-provider)

### 2. Install with Helm

```sh
helm install mechanic charts/mechanic/ \
  --namespace mechanic \
  --set gitops.repo=myorg/my-gitops-repo \
  --set gitops.manifestRoot=kubernetes
```

### 3. Verify

```sh
kubectl get deployment -n mechanic
kubectl get rjob -n mechanic
# Show lifecycle events for a specific RemediationJob:
kubectl describe rjob <name> -n mechanic
```

## Configuration

### Helm values reference

All `values.yaml` keys and their defaults:

| Key | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/lenaxia/mechanic-watcher` | Watcher image repository |
| `image.tag` | `""` (uses `Chart.appVersion`) | Watcher image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `agent.image.repository` | `ghcr.io/lenaxia/mechanic-agent` | Agent image repository |
| `agent.image.tag` | `""` (uses `Chart.appVersion`) | Agent image tag |
| `gitops.repo` | **required** | GitOps repository in `org/repo` format |
| `gitops.manifestRoot` | **required** | Path within repo to manifests root |
| `watcher.stabilisationWindowSeconds` | `120` | Seconds a finding must persist before dispatching |
| `watcher.maxConcurrentJobs` | `3` | Maximum simultaneous agent Jobs |
| `watcher.minSeverity` | `low` | Minimum severity to dispatch: `critical`, `high`, `medium`, or `low` |
| `watcher.remediationJobTTLSeconds` | `604800` | TTL for completed RemediationJob objects (7 days) |
| `watcher.sinkType` | `github` | Sink type for PR creation |
| `watcher.logLevel` | `info` | Log level: debug, info, warn, error |
| `watcher.llmProvider` | `openai` | LLM readiness gate: `openai` enables it; empty disables |
| `watcher.injectionDetectionAction` | `log` | What to do when a prompt injection heuristic fires: `log` or `suppress` |
| `watcher.maxInvestigationRetries` | `3` | Maximum Job retries per `RemediationJob` before permanently failing |
| `watcher.agentRBACScope` | `cluster` | RBAC scope for the agent: `cluster` or `namespace` |
| `watcher.agentWatchNamespaces` | `""` | Comma-separated namespaces for the agent RBAC scope. Required when `agentRBACScope=namespace` |
| `watcher.watchNamespaces` | `""` | Comma-separated namespaces the watcher monitors for failures. Empty = all namespaces |
| `watcher.excludeNamespaces` | `""` | Comma-separated namespaces the watcher ignores. Empty = no exclusions |
| `agentType` | `opencode` | Agent runner type: `opencode` (functional) or `claude` (stub, not yet functional). Controls which `llm-credentials-<agentType>` Secret is consumed. Secret names are compile-time constants — they cannot be overridden via Helm values. |
| `prompt.coreOverride` | `""` | Full core prompt override (replaces built-in `files/prompts/core.txt`) |
| `prompt.agentOverride` | `""` | Full agent prompt override (replaces built-in `files/prompts/<agentType>.txt`) |
| `rbac.create` | `true` | Create RBAC resources |
| `createNamespace` | `false` | Create `Release.Namespace` if it does not exist |
| `metrics.enabled` | `false` | Expose metrics Service on port 8080 |
| `metrics.serviceMonitor.enabled` | `false` | Create Prometheus Operator ServiceMonitor |
| `metrics.serviceMonitor.interval` | `30s` | Prometheus scrape interval |
| `metrics.serviceMonitor.scrapeTimeout` | `10s` | Prometheus scrape timeout |
| `metrics.serviceMonitor.labels` | `{}` | Additional labels for the ServiceMonitor |
| `networkPolicy.enabled` | `false` | Restrict agent Job egress to API server, GitHub, and LLM endpoint |
| `networkPolicy.apiServerPort` | `6443` | Kubernetes API server port (some distributions use `443`) |
| `networkPolicy.additionalEgressRules` | `[]` | Extra egress rules appended verbatim (e.g. to restrict LLM endpoint by CIDR) |
| `watcher.prAutoClose` | `true` | Automatically close the GitHub PR when the underlying finding resolves. Set to `false` to leave PRs open for manual review |

### Configuration validation

The watcher validates configuration at startup with clear error messages.

**Numeric validations:**
- `MAX_CONCURRENT_JOBS`: must be > 0
- `REMEDIATION_JOB_TTL_SECONDS`: must be > 0
- `STABILISATION_WINDOW_SECONDS`: must be ≥ 0

**Enum validations:**
- `MIN_SEVERITY`: must be one of `critical`, `high`, `medium`, `low` (absent defaults to `low`)

**Format validations:**
- `GITOPS_REPO`: must be in `owner/repo` format

## How it works

```mermaid
%%{init: {'flowchart': {'curve': 'linear'}}}%%
flowchart TD
    subgraph watcher["mechanic-watcher — Deployment"]
        SPR["SourceProviderReconcilers<br/>one per resource type<br/>─────────────────────<br/>watches Pods, Deployments,<br/>StatefulSets, PVCs, Nodes, Jobs<br/>extracts findings<br/>deduplicates by fingerprint"]
        RJR["RemediationJobReconciler<br/>─────────────────────<br/>watches RemediationJob CRDs<br/>enforces MAX_CONCURRENT_JOBS<br/>syncs Job status back"]
    end

    RJ["RemediationJob CRDs<br/>rjob<br/>─────────────────────<br/>durable dedup state<br/>survives restarts"]

    AJ["mechanic-agent Job<br/>one per finding<br/>─────────────────────<br/>init: git clone repo<br/>main: opencode run<br/>  kubectl read-only<br/>  gh pr create"]

    GH["GitOps repository<br/>GitHub"]

    SPR -->|creates| RJ
    RJ -->|watched by| RJR
    RJR -->|creates| AJ
    AJ -->|opens PR| GH
```

### What the agent does

The agent runs [OpenCode](https://opencode.ai) inside the cluster with read-only RBAC
and follows a structured investigation:

1. Check for an existing open PR for this fingerprint — if found, comment on it and exit
2. `kubectl describe` and `kubectl get events` on the failing resource
3. Inspect related resources (owning Deployment, Endpoints, PVs, etc.)
4. Locate the relevant manifests in the cloned GitOps repository
5. Inspect Flux/Helm state with `flux get all` and `helm list`
6. Determine root cause and assign a confidence level (high / medium / low)
7. Validate proposed changes with `kubeconform` and `kustomize build`
8. Open a pull request with a structured body: summary, evidence, root cause, fix, confidence

### The `RemediationJob` CRD

Every unique finding is tracked by a `RemediationJob` object (`rjob`).

```bash
kubectl get rjob -n mechanic
```

```
NAME                          PHASE       KIND         PARENT                  JOB                                   AGE
mechanic-a3f9c2b14d8e         Succeeded   Pod          Deployment/my-app       mechanic-agent-a3f9c2b14d8e           8m
mechanic-7bc1d3e90f21         Dispatched  Deployment   Deployment/api-server   mechanic-agent-7bc1d3e90f21           2m
mechanic-f4e2a1c85b67         Failed      Node         Node/worker-03                                                1h
```

#### RemediationJob lifecycle

```mermaid
stateDiagram-v2
    [*] --> Pending : finding detected

    Pending --> Dispatched : concurrent-job slot available
    Pending --> Cancelled : source object deleted

    Dispatched --> Running : Job pod scheduled

    Running --> Succeeded : agent Job completed
    Running --> Failed : exit non-zero or deadline exceeded
    Running --> Cancelled : source object deleted

    Failed --> Dispatched : retry (RetryCount < MaxRetries)
    Failed --> PermanentlyFailed : RetryCount >= MaxRetries
    Succeeded --> [*]
    Cancelled --> [*]
    PermanentlyFailed --> [*]
```

- **Pending** — finding detected, waiting for a concurrent-job slot
- **Dispatched** — `batch/v1 Job` created, waiting for pod scheduling
- **Running** — agent pod is executing
- **Succeeded** — agent Job completed; `status.prRef` holds the PR URL if one was opened
- **Failed** — agent Job failed (exit non-zero or deadline exceeded); re-queued if `RetryCount < MaxRetries`
- **PermanentlyFailed** — `RetryCount` has reached `MaxRetries`; no further dispatch; visible via `kubectl describe rjob <name>`
- **Cancelled** — source object was deleted while the investigation was in progress

### Per-resource annotation control

Three annotations gate mechanic's behaviour on any watched resource (Pod, Deployment,
StatefulSet, PVC, Node, Job) or on an entire Namespace:

| Annotation | Value | Effect |
|---|---|---|
| `mechanic.io/enabled` | `"false"` | Permanently suppress all findings from this resource |
| `mechanic.io/skip-until` | `"YYYY-MM-DD"` | Suppress findings until end-of-day UTC on this date |
| `mechanic.io/priority` | `"critical"` | Bypass the stabilisation window — dispatch immediately |

**Examples:**

```sh
# Disable investigations on a deployment permanently
kubectl annotate deployment my-app mechanic.io/enabled=false

# Silence a noisy node until after a maintenance window
kubectl annotate node worker-03 mechanic.io/skip-until=2026-03-15

# Dispatch immediately on a critical deployment (no stabilisation window)
kubectl annotate deployment api-server mechanic.io/priority=critical
```

**Namespace-level gate:** Annotating the `Namespace` object itself applies to all resources
in that namespace. This suppresses every finding regardless of the resource's own annotations:

```sh
# Disable all mechanic activity in the kube-system namespace
kubectl annotate namespace kube-system mechanic.io/enabled=false

# Suppress all findings in staging until a date
kubectl annotate namespace staging mechanic.io/skip-until=2026-04-01
```

The `skip-until` date is inclusive: findings are suppressed until midnight UTC at the
start of the day *after* the specified date.

### Components

| Component | Description |
|---|---|
| `mechanic-watcher` | Go controller (controller-runtime) that watches Kubernetes resources, manages `RemediationJob` CRDs, and creates agent Jobs |
| `mechanic-agent` | Docker image containing opencode + kubectl + helm + flux + gh and supporting investigation tools |

### Agent image tools

| Tool | Version | Purpose |
|---|---|---|
| `opencode` | `1.2.10` | AI agent driver |
| `kubectl` | `1.35.1` | Cluster inspection (read-only) |
| `helm` | `3.20.0` | Chart metadata, template rendering |
| `flux` | `2.8.0` | Flux status, trace, diff |
| `kustomize` | `5.8.1` | Render and validate Kustomize overlays |
| `gh` | latest stable | PR creation, listing, commenting |
| `kubeconform` | `0.7.0` | Kubernetes manifest schema validation |
| `yq` | `4.52.4` | YAML processing |
| `jq` | apt | JSON processing |
| `stern` | `1.33.1` | Multi-pod log tailing |
| `sops` | `3.12.1` | Decrypt SOPS-encrypted secrets |
| `age` | `1.3.1` | Decrypt age-encrypted files |
| `talosctl` | `1.12.4` | Talos node inspection (requires `talosconfig` mount) |

All binaries are fetched from official releases with SHA256 checksum verification.
The agent runs as non-root (`uid=1000`).

## Roadmap

Features under active development or planned:

| Area | Feature | Status |
|---|---|---|
| Operability | Kubernetes Events on `RemediationJob` (`kubectl describe rjob` shows lifecycle) | Shipped |
| Operability | Dry-run mode — investigate without opening PRs | Shipped |
| Reliability | `PermanentlyFailed` phase — retry cap with dead-letter tombstone | Shipped |
| Reliability | GitHub App token expiry fast-fail guard | Shipped |
| Accuracy | Namespace-scoped provider filtering (`WATCH_NAMESPACES`, `EXCLUDE_NAMESPACES`) | Shipped |
| Accuracy | Per-resource opt-out annotations (`mechanic.io/enabled`, `mechanic.io/skip-until`, `mechanic.io/priority`) | Shipped |
| Accuracy | Multi-signal correlation (related findings grouped into one investigation) | Planned |
| Accuracy | Mandatory pre-PR manifest validation | Shipped |
| Impact | PR auto-close when finding resolves | Shipped |
| Impact | GitLab and Gitea sink support | Evaluated |
| Signal sources | Prometheus / Alertmanager source provider | Evaluated |
| Signal sources | cert-manager certificate expiry provider | Evaluated |

See [`docs/BACKLOG/FEATURE_TRACKER.md`](docs/BACKLOG/FEATURE_TRACKER.md) for the
full product backlog with value/complexity ratings and implementation notes.

## Documentation

- [`docs/DESIGN/HLD.md`](docs/DESIGN/HLD.md) — Architecture and design decisions
- [`docs/DESIGN/lld/`](docs/DESIGN/lld/) — Component-level low-level designs
- [`docs/BACKLOG/`](docs/BACKLOG/) — Implementation backlog and feature tracker
- [`README-LLM.md`](README-LLM.md) — LLM implementation guide

## Development

### Prerequisites

- Go 1.24+
- [`golangci-lint`](https://golangci-lint.run/usage/install/) — extended linter suite
- [`gitleaks`](https://github.com/zricethezav/gitleaks) — secrets scanner

Install both with `go install`:

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/zricethezav/gitleaks/v8@latest
```

### Git hooks

After cloning, install the pre-commit hook once:

```sh
make install-hooks
```

The hook runs on every `git commit` and enforces two checks:

| Check | Tool | What it catches |
|---|---|---|
| Secrets scan | `gitleaks` | API keys, tokens, credentials in staged files |
| Lint | `golangci-lint` | Type errors, unused code, security issues, formatting |

To bypass in an emergency: `git commit --no-verify`

### Useful make targets

| Target | Description |
|---|---|
| `make lint` | Quick `go vet` check |
| `make lint-full` | Full `golangci-lint` run (same as pre-commit) |
| `make lint-secrets` | Full repo secrets scan with `gitleaks` |
| `make lint-security` | `gosec` HIGH/CRITICAL security check |
| `make test` | Full test suite with race detector |
| `make install-hooks` | (Re-)install git hooks after cloning |

## Community

- **GitHub Discussions** — [github.com/lenaxia/k8s-mechanic/discussions](https://github.com/lenaxia/k8s-mechanic/discussions)
  — questions, ideas, architecture discussions, and show-and-tell
- **GitHub Issues** — bugs and feature requests

### Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding standards,
the DCO requirement, and how to find good first issues.

### Governance

See [GOVERNANCE.md](GOVERNANCE.md) for the project's contributor ladder, decision-making
process, and how to become a maintainer. The current maintainer list is in
[MAINTAINERS.md](MAINTAINERS.md).

### GitHub Repository Topics

The following topics are set on this repository to aid discoverability. If you
are maintaining a fork or related project, you may want to apply the same set:

`kubernetes` `gitops` `devops` `cloud-native` `operator` `controller`
`remediation` `self-healing` `argocd` `flux` `automation`

Topics are configured via the GitHub web UI: repository page → gear icon next
to "About" → Topics.

---

## License

Apache 2.0

---

## Development Practices

This project follows structured SDLC practices throughout:

- **Backlog-driven** — all features and epics are tracked in [`docs/BACKLOG/`](docs/BACKLOG/),
  with explicit story breakdowns, acceptance criteria, and value/complexity ratings before
  implementation begins.
- **Security-reviewed** — [Epic 12](docs/BACKLOG/epic12-security-review/README.md) ran a
  structured security audit against mechanic's full attack surface: secret redaction,
  prompt injection detection, network policy, RBAC scoping, structured audit logging, and
  a formal penetration test plan with documented findings. All HIGH/CRITICAL findings were
  remediated before the epic was closed.
- **Documented** — every significant session is recorded in [`docs/WORKLOGS/`](docs/WORKLOGS/),
  capturing design decisions, implementation notes, and the rationale behind changes as they
  happen.

Development is AI-accelerated (primarily [OpenCode](https://opencode.ai)), which allows the
project to move quickly without compromising process rigour. The process keeps the work
accountable.
