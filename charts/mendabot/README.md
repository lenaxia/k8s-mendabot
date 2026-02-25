# mendabot

![Version: 0.3.13](https://img.shields.io/badge/Version-0.3.13-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.3.13](https://img.shields.io/badge/AppVersion-v0.3.13-informational?style=flat-square)

Kubernetes-native SRE remediation bot — watches cluster failures, spawns an LLM agent, and opens GitOps pull requests with proposed fixes.

## TL;DR

```sh
helm install mendabot charts/mendabot/ \
  --namespace mendabot \
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
kubectl create namespace mendabot

# GitHub App credentials
kubectl create secret generic github-app \
  --namespace mendabot \
  --from-literal=app-id=<APP_ID> \
  --from-literal=installation-id=<INSTALLATION_ID> \
  --from-literal=private-key="$(cat /path/to/private-key.pem)"

# LLM credentials (OpenAI example)
kubectl create secret generic llm-credentials-opencode \
  --namespace mendabot \
  --from-literal=provider-config='{"$schema":"https://opencode.ai/config.json","provider":{"openai":{"apiKey":"sk-..."}},"model":"openai/gpt-4o"}'
```

Then install:

```sh
helm install mendabot charts/mendabot/ \
  --namespace mendabot \
  --set gitops.repo=myorg/my-gitops-repo \
  --set gitops.manifestRoot=kubernetes
```

## Upgrading

```sh
helm upgrade mendabot charts/mendabot/ --namespace mendabot
```

## Uninstalling

```sh
helm uninstall mendabot --namespace mendabot
kubectl delete crd remediationjobs.remediation.mendabot.io
```

## Configuration

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agent.image.repository | string | `"ghcr.io/lenaxia/mendabot-agent"` | Agent image repository. Watcher and agent images are always released together — do not pin them to different versions. |
| agent.image.tag | string | `""` | Agent image tag. Defaults to `Chart.appVersion` when empty. |
| agentType | string | `"opencode"` | Agent runner type. Controls which AI agent binary runs inside the Job. Supported values: `"opencode"` (functional), `"claude"` (stub — not yet functional). Each type requires a corresponding Secret named `llm-credentials-<agentType>`. See `NOTES.txt` for the exact `kubectl create secret` commands. |
| createNamespace | bool | `false` | Create `Release.Namespace` if it does not exist. Most operators pre-create namespaces; default is `false`. |
| gitops.manifestRoot | string | `""` | Path within the repository containing Kubernetes manifests. **Required.** |
| gitops.repo | string | `""` | GitHub repository in `org/repo` format (e.g. `"myorg/my-gitops-repo"`). **Required.** |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/lenaxia/mendabot-watcher","tag":""}` | Watcher image repository |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"ghcr.io/lenaxia/mendabot-watcher"` | Watcher image repository |
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
| watcher.agentRBACScope | string | `"cluster"` | RBAC scope for the agent Job. One of `"cluster"` (default) or `"namespace"`. When `"namespace"`, `agentWatchNamespaces` must also be set. |
| watcher.agentWatchNamespaces | string | `""` | Comma-separated list of namespaces for namespace-scoped agent RBAC. Required when `agentRBACScope=namespace`. Example: `"production,staging"`. |
| watcher.excludeNamespaces | string | `""` | Comma-separated list of namespaces the watcher ignores. Empty means no exclusions. Example: `"kube-system,monitoring"`. |
| watcher.injectionDetectionAction | string | `"log"` | Action when a prompt injection heuristic fires. One of `"log"` (default) or `"suppress"` (silently drops the finding). |
| watcher.llmProvider | string | `"openai"` | LLM readiness gate. Set to `"openai"` to block RemediationJob creation until the `llm-credentials-<agentType>` Secret exists. Leave empty to disable. |
| watcher.logLevel | string | `"info"` | Zap log level. One of `debug`, `info`, `warn`, `error`. |
| watcher.maxConcurrentJobs | int | `3` | Maximum number of agent Jobs running concurrently. |
| watcher.maxInvestigationRetries | string | `"3"` | Maximum number of investigation retries per RemediationJob before permanently failing. |
| watcher.remediationJobTTLSeconds | int | `604800` | Time-to-live for completed RemediationJob objects in seconds. Default is 7 days. |
| watcher.sinkType | string | `"github"` | Sink type for PR creation. Currently only `"github"` is supported. |
| watcher.stabilisationWindowSeconds | int | `120` | Seconds to wait after first detecting a failure before creating a RemediationJob. Set to `0` to dispatch immediately. |
| watcher.watchNamespaces | string | `""` | Comma-separated list of namespaces the watcher monitors for failures. Empty means all namespaces. |

## Source Code

## Source Code

* <https://github.com/lenaxia/k8s-mendabot>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| lenaxia |  | <https://github.com/lenaxia> |
