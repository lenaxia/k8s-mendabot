# mechanic

![Version: 0.3.36](https://img.shields.io/badge/Version-0.3.36-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.3.36](https://img.shields.io/badge/AppVersion-v0.3.36-informational?style=flat-square)

Kubernetes-native SRE remediation bot — watches cluster failures, spawns an LLM agent, and opens GitOps pull requests with proposed fixes.

## TL;DR

```sh
helm install mechanic charts/mechanic/ \
  --namespace mechanic \
  --set gitops.repo=myorg/my-gitops-repo \
  --set gitops.manifestRoot=kubernetes
```

## Prerequisites

- Kubernetes >= 1.28
- Helm >= 3.14
- A GitHub App with Contents (write), Pull Requests (write), and Issues (write) permissions installed on your GitOps repository
- An OpenAI-compatible LLM API key

## Installing the Chart

Create the required Secrets before installing:

```sh
kubectl create namespace mechanic

# GitHub App credentials
kubectl create secret generic github-app \
  --namespace mechanic \
  --from-literal=app-id=<APP_ID> \
  --from-literal=installation-id=<INSTALLATION_ID> \
  --from-literal=private-key="$(cat /path/to/private-key.pem)"

# LLM credentials (OpenAI example)
kubectl create secret generic llm-credentials-opencode \
  --namespace mechanic \
  --from-literal=provider-config='{"$schema":"https://opencode.ai/config.json","provider":{"openai":{"apiKey":"sk-..."}},"model":"openai/gpt-4o"}'
```

Then install:

```sh
helm install mechanic charts/mechanic/ \
  --namespace mechanic \
  --set gitops.repo=myorg/my-gitops-repo \
  --set gitops.manifestRoot=kubernetes
```

## Upgrading

```sh
helm upgrade mechanic charts/mechanic/ --namespace mechanic
```

## Uninstalling

```sh
helm uninstall mechanic --namespace mechanic
kubectl delete crd remediationjobs.remediation.mechanic.io
```

## Configuration

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agent.image.repository | string | `"ghcr.io/lenaxia/mechanic-agent"` | Agent image repository. Watcher and agent images are always released together — do not pin them to different versions. |
| agent.image.tag | string | `""` | Agent image tag. Defaults to `Chart.appVersion` when empty. |
| agent.resources | object | `{"limits":{"cpu":"500m","memory":"512Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Resource requests and limits applied to all three agent Job containers (git-token-clone, dry-run-gate, mendabot-agent). |
| agent.resources.limits.cpu | string | `"500m"` | CPU limit for each agent Job container. |
| agent.resources.limits.memory | string | `"512Mi"` | Memory limit for each agent Job container. |
| agent.resources.requests.cpu | string | `"100m"` | CPU request for each agent Job container. |
| agent.resources.requests.memory | string | `"128Mi"` | Memory request for each agent Job container. |
| agentType | string | `"opencode"` | Agent runner type. Controls which AI agent binary runs inside the Job. Supported values: `"opencode"` (functional), `"claude"` (stub — not yet functional). Each type requires a corresponding Secret named `llm-credentials-<agentType>`. See `NOTES.txt` for the exact `kubectl create secret` commands. |
| createNamespace | bool | `false` | Create `Release.Namespace` if it does not exist. Most operators pre-create namespaces; default is `false`. |
| gitops.manifestRoot | string | `""` | Path within the repository containing Kubernetes manifests. **Required.** |
| gitops.repo | string | `""` | GitHub repository in `org/repo` format (e.g. `"myorg/my-gitops-repo"`). **Required.** |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/lenaxia/mechanic-watcher","tag":""}` | Watcher image repository |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"ghcr.io/lenaxia/mechanic-watcher"` | Watcher image repository |
| image.tag | string | `""` | Watcher image tag. Defaults to `Chart.appVersion` when empty. |
| metrics.enabled | bool | `false` | Expose a metrics Service on port `8080`. Required for ServiceMonitor. |
| metrics.serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator `ServiceMonitor` CR. Requires `metrics.enabled: true` and Prometheus Operator installed. |
| metrics.serviceMonitor.interval | string | `"30s"` | Prometheus scrape interval. |
| metrics.serviceMonitor.labels | object | `{}` | Additional labels to add to the ServiceMonitor (e.g. for a Prometheus label selector). |
| metrics.serviceMonitor.scrapeTimeout | string | `"10s"` | Prometheus scrape timeout. |
| networkPolicy.additionalEgressRules | list | `[]` | Optional additional egress rules appended verbatim to the NetworkPolicy spec. Use to restrict the LLM endpoint to a known CIDR. |
| networkPolicy.apiServerPort | int | `6443` | Port used by the Kubernetes API server. Standard is `6443`; some distributions use `443`. |
| networkPolicy.enabled | bool | `false` | Enable NetworkPolicy to restrict agent Job egress to the cluster API server, GitHub (`443/tcp`), and the LLM endpoint. Requires a CNI that enforces NetworkPolicy (Cilium, Calico, etc.). |
| prompt.agentOverride | string | `""` | Full agent-specific prompt content override. When non-empty, replaces the built-in agent prompt for the selected `agentType`. |
| prompt.coreOverride | string | `""` | Full core prompt content override. When non-empty, replaces the built-in core prompt mounted from the ConfigMap. |
| rbac.create | bool | `true` | Create RBAC resources. Set to `false` if you manage RBAC externally. |
| secrets | object | `{}` | Pre-existing Secrets that must be created before `helm install`. The chart never creates Secret content. Secret names are derived from `agentType` at runtime: `llm-credentials-<agentType>`. Required Secrets: `github-app` (keys: `app-id`, `installation-id`, `private-key`), `llm-credentials-opencode` (key: `provider-config`). |
| selfRemediation | object | `{"cascadeNamespaceThreshold":50,"cascadeNodeCacheTTLSeconds":30,"cooldownSeconds":300,"disableCascadeCheck":false,"disableUpstreamContributions":false,"maxDepth":2,"upstreamRepo":""}` | Self-remediation configuration. |
| selfRemediation.cascadeNamespaceThreshold | int | `50` | Cascade namespace threshold. Default is 50. |
| selfRemediation.cascadeNodeCacheTTLSeconds | int | `30` | Cascade node cache TTL in seconds. Default is 30. |
| selfRemediation.cooldownSeconds | int | `300` | Minimum time between allowed self-remediations. 0 disables circuit breaker.    Default is 300 (5 minutes). |
| selfRemediation.disableCascadeCheck | bool | `false` | Disable cascade check. Default is false. |
| selfRemediation.disableUpstreamContributions | bool | `false` | Disable upstream contributions (legacy, unused in current codebase). |
| selfRemediation.maxDepth | int | `2` | Maximum self-remediation chain depth. Findings with ChainDepth > maxDepth are suppressed.    0 disables self-remediation entirely. Default is 2. |
| selfRemediation.upstreamRepo | string | `""` | Upstream repository for contributions (legacy, unused in current codebase). |
| watcher.agentRBACScope | string | `"cluster"` | RBAC scope for the agent Job. One of `"cluster"` (default) or `"namespace"`. When `"namespace"`, `agentWatchNamespaces` must also be set. |
| watcher.agentWatchNamespaces | string | `""` | Comma-separated list of namespaces for namespace-scoped agent RBAC. Required when `agentRBACScope=namespace`. Example: `"production,staging"`. |
| watcher.correlationWindowSeconds | int | `30` | Seconds to hold Pending jobs before dispatching (correlation window). Set to 0 to    dispatch immediately without correlation. Default is 30. |
| watcher.disableCorrelation | bool | `false` | Disable all correlation logic and dispatch immediately. Default is false. |
| watcher.dryRun | bool | `false` | Enable dry-run shadow mode. When true, write operations are blocked and investigation reports are stored in RemediationJob.status.message. |
| watcher.excludeNamespaces | string | `""` | Comma-separated list of namespaces the watcher ignores. Empty means no exclusions. Example: `"kube-system,monitoring"`. |
| watcher.injectionDetectionAction | string | `"log"` | Action when a prompt injection heuristic fires. One of `"log"` (default) or `"suppress"` (silently drops the finding). |
| watcher.llmProvider | string | `"openai"` | LLM readiness gate. Set to `"openai"` to block RemediationJob creation until the `llm-credentials-<agentType>` Secret exists. Leave empty to disable. |
| watcher.logLevel | string | `"info"` | Zap log level. One of `debug`, `info`, `warn`, `error`. |
| watcher.maxConcurrentJobs | int | `3` | Maximum number of agent Jobs running concurrently. |
| watcher.maxInvestigationRetries | string | `"3"` | Maximum number of investigation retries per RemediationJob before permanently failing. |
| watcher.multiPodThreshold | int | `3` | Minimum number of pods failing on the same node to trigger MultiPodSameNodeRule.    Default is 3. |
| watcher.prAutoClose | bool | `true` | Automatically close open GitHub PRs/issues when the underlying finding resolves.    Set to false to disable auto-close and leave PRs open for manual review. |
| watcher.remediationJobTTLSeconds | int | `604800` | Time-to-live for completed RemediationJob objects in seconds. Default is 7 days. |
| watcher.sinkType | string | `"github"` | Sink type for PR creation. Currently only `"github"` is supported. |
| watcher.stabilisationWindowSeconds | int | `120` | Seconds to wait after first detecting a failure before creating a RemediationJob. Set to `0` to dispatch immediately. |
| watcher.watchNamespaces | string | `""` | Comma-separated list of namespaces the watcher monitors for failures. Empty means all namespaces. |

## Source Code

## Source Code

* <https://github.com/lenaxia/k8s-mechanic>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| lenaxia |  | <https://github.com/lenaxia> |
