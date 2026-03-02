# mechanic

![Version: 0.4.2](https://img.shields.io/badge/Version-0.4.2-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.4.2](https://img.shields.io/badge/AppVersion-v0.4.2-informational?style=flat-square)

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
| affinity | object | `{}` | Affinity rules for the watcher Deployment Pod. |
| agent.image.repository | string | `"ghcr.io/lenaxia/mechanic-agent"` | Agent image repository. Watcher and agent images are always released together — do not pin them to different versions. |
| agent.image.tag | string | `""` | Agent image tag. Defaults to Chart.appVersion when empty. |
| agent.prompt | object | `{"agentOverride":"","coreOverride":""}` | Investigation prompt overrides. When set, replaces the built-in prompts mounted from the ConfigMap. |
| agent.prompt.agentOverride | string | `""` | Full agent-specific prompt content override for the selected agent type. |
| agent.prompt.coreOverride | string | `""` | Full core prompt content override. |
| agent.rbac | object | `{"scope":"cluster","watchNamespaces":""}` | RBAC configuration for agent Jobs. |
| agent.rbac.scope | string | `"cluster"` | RBAC scope for the agent Job. One of "cluster" (default) or "namespace". When "namespace", watchNamespaces must also be set. |
| agent.rbac.watchNamespaces | string | `""` | Comma-separated list of namespaces the agent is permitted to read when scope is "namespace". Example: "production,staging". |
| agent.resources | object | `{"limits":{"cpu":"500m","memory":"512Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Resource requests and limits applied to all agent Job containers. |
| agent.security | object | `{"hardening":{"enabled":false,"extraRedactPatterns":[]},"kyverno":{"allowedImagePrefix":"ghcr.io/lenaxia/mechanic-agent","enabled":false},"networkPolicy":{"additionalEgressRules":[],"apiServerPort":6443,"enabled":false}}` | Security hardening for agent Jobs. |
| agent.security.hardening | object | `{"enabled":false,"extraRedactPatterns":[]}` | Tool output redaction and kubectl restrictions. |
| agent.security.hardening.enabled | bool | `false` | Block kubectl get/describe secret(s), get all, exec, and port-forward in agent Jobs. Off by default — disabling exec limits the agent's ability to inspect running containers. |
| agent.security.hardening.extraRedactPatterns | list | `[]` | Additional RE2 regex patterns applied by both the watcher's finding redaction and the agent redact binary inside every Job. Invalid patterns cause watcher startup failure. Example:   extraRedactPatterns:     - 'CORP-[0-9]{8}'     - 'INT-[A-Z]+-[0-9]+' |
| agent.security.kyverno | object | `{"allowedImagePrefix":"ghcr.io/lenaxia/mechanic-agent","enabled":false}` | Optional Kyverno ClusterPolicy enforcing agent hardening rules at the admission layer: secret read denial, write denial, pod exec/port-forward denial, agent image allowlist, and pod security profile. Requires Kyverno v1.9+ installed in the cluster. If enabled and Kyverno is not installed, helm install will fail with a CRD-not-found error. |
| agent.security.kyverno.allowedImagePrefix | string | `"ghcr.io/lenaxia/mechanic-agent"` | Image prefix enforced on all mechanic-agent Job containers. Jobs whose agent container image does not start with this prefix are denied. Set to "" to disable image enforcement while keeping other rules active. |
| agent.security.networkPolicy | object | `{"additionalEgressRules":[],"apiServerPort":6443,"enabled":false}` | NetworkPolicy restricting agent Job egress to the cluster API server, GitHub (443/tcp), and the LLM endpoint. Requires a CNI that enforces NetworkPolicy (Cilium, Calico, etc.). |
| agent.security.networkPolicy.additionalEgressRules | list | `[]` | Optional additional egress rules appended verbatim to the NetworkPolicy spec. Use to restrict the LLM endpoint to a known CIDR. |
| agent.security.networkPolicy.apiServerPort | int | `6443` | Port used by the Kubernetes API server. Standard is 6443; some distributions use 443. |
| agent.selfRemediation | object | `{"cascadeNamespaceThreshold":50,"cascadeNodeCacheTTLSeconds":30,"cooldownSeconds":300,"disableCascadeCheck":false,"maxDepth":2}` | Self-remediation controls. Governs how mechanic handles findings generated by its own changes (i.e. a PR it opened caused a new failure). |
| agent.selfRemediation.cascadeNamespaceThreshold | int | `50` | Number of failing namespaces that triggers a cascade abort. |
| agent.selfRemediation.cascadeNodeCacheTTLSeconds | int | `30` | How long in seconds node failure data is cached for cascade detection. |
| agent.selfRemediation.cooldownSeconds | int | `300` | Minimum time in seconds between allowed self-remediations. Set to 0 to disable the circuit breaker. |
| agent.selfRemediation.disableCascadeCheck | bool | `false` | Disable cascade check. |
| agent.selfRemediation.maxDepth | int | `2` | Maximum self-remediation chain depth. Findings with ChainDepth > maxDepth are suppressed. Set to 0 to disable self-remediation entirely. |
| agent.type | string | `"opencode"` | Agent runner type. Controls which AI agent binary runs inside the Job. Supported values: "opencode" (functional), "claude" (stub — not yet functional). Each type requires a corresponding Secret named llm-credentials-<type>. |
| createNamespace | bool | `false` | Create Release.Namespace if it does not exist. Most operators pre-create namespaces; default is false. |
| fullnameOverride | string | `""` | Override the fully-qualified app name used in resource names. |
| gitops.manifestRoot | string | `""` | Path within the repository containing Kubernetes manifests. Required. |
| gitops.repo | string | `""` | GitHub repository in org/repo format (e.g. "myorg/my-gitops-repo"). Required. This is the repo that agent checks out during investigations and will open PRs for.  Requires setup of a GitHub App and github-app secret with PEM key in order to open PRs |
| imagePullSecrets | list | `[]` | Image pull secrets for the watcher Deployment. Required when pulling from a private registry. Example: imagePullSecrets: [{ name: my-registry-secret }] |
| metrics | object | `{"enabled":true,"serviceMonitor":{"enabled":false,"interval":"30s","labels":{},"scrapeTimeout":"10s"}}` | Prometheus metrics for the watcher Deployment. |
| metrics.enabled | bool | `true` | Expose a metrics Service on port 8080. Enabled by default so that mechanic_* metrics are immediately scrapeable after install. The watcher always binds :8080/metrics regardless of this flag; setting enabled: false only suppresses the Kubernetes Service resource. |
| metrics.serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor CR. Requires metrics.enabled: true and Prometheus Operator installed. |
| metrics.serviceMonitor.interval | string | `"30s"` | Scrape interval for the ServiceMonitor. |
| metrics.serviceMonitor.labels | object | `{}` | Additional labels for the ServiceMonitor (e.g. for a Prometheus label selector). |
| metrics.serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout for the ServiceMonitor. |
| nameOverride | string | `""` | Override the chart name component used in resource names. |
| nodeSelector | object | `{}` | Node selector for the watcher Deployment Pod. |
| podAnnotations | object | `{}` | Annotations added to the watcher Pod. Use for sidecar injection, APM agents, service mesh opt-outs, etc. |
| podLabels | object | `{}` | Extra labels added to the watcher Pod. |
| rbac | object | `{"create":true}` | RBAC for the watcher Deployment and agent Jobs. |
| rbac.create | bool | `true` | Create RBAC resources. Set to false if you manage RBAC externally. |
| serviceAccount | object | `{"annotations":{}}` | Watcher ServiceAccount configuration. |
| serviceAccount.annotations | object | `{}` | Annotations to add to the watcher ServiceAccount. Use for AWS IRSA, GCP Workload Identity, or Azure Workload Identity. Example: eks.amazonaws.com/role-arn: arn:aws:iam::123456789:role/mechanic |
| tolerations | list | `[]` | Tolerations for the watcher Deployment Pod. |
| watcher.correlation | object | `{"enabled":true,"multiPodThreshold":3,"windowSeconds":30}` | Multi-signal correlation. Groups related findings into a single investigation rather than dispatching one agent Job per finding. |
| watcher.correlation.enabled | bool | `true` | Enable correlation. When false, every finding dispatches immediately. |
| watcher.correlation.multiPodThreshold | int | `3` | Minimum number of pods failing on the same node to trigger MultiPodSameNodeRule. |
| watcher.correlation.windowSeconds | int | `30` | Seconds to hold Pending jobs before dispatching (correlation window). |
| watcher.dryRun | bool | `false` | Enable dry-run mode. When true, write operations are blocked and investigation reports are written to a mechanic-dryrun-<fingerprint> ConfigMap instead of opening PRs. Useful for validating mechanic behaviour before enabling in production. |
| watcher.excludeNamespaces | string | `""` | Comma-separated list of namespaces the watcher ignores. Empty means no exclusions. Example: "kube-system,monitoring". |
| watcher.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. |
| watcher.image.repository | string | `"ghcr.io/lenaxia/mechanic-watcher"` | Watcher image repository. |
| watcher.image.tag | string | `""` | Watcher image tag. Defaults to Chart.appVersion when empty. |
| watcher.injectionDetectionAction | string | `"log"` | Action when a prompt injection heuristic fires. One of "log" (default) or "suppress" (silently drops the finding). |
| watcher.logLevel | string | `"info"` | Zap log level. One of debug, info, warn, error. |
| watcher.maxConcurrentJobs | int | `3` | Maximum number of agent Jobs running concurrently. |
| watcher.maxInvestigationRetries | int | `3` | Maximum number of investigation retries per RemediationJob before permanently failing. |
| watcher.prAutoClose | bool | `true` | Automatically close open GitHub PRs when the underlying finding resolves. Set to false to leave PRs open for manual review. |
| watcher.remediationJobTTLSeconds | int | `604800` | Time-to-live for completed RemediationJob objects in seconds. Default is 7 days. |
| watcher.resources | object | `{"limits":{"cpu":"200m","memory":"128Mi"},"requests":{"cpu":"50m","memory":"64Mi"}}` | Resource requests and limits for the watcher Deployment container. |
| watcher.sinkType | string | `"github"` | Sink type for PR creation. Currently only "github" is supported. |
| watcher.stabilisationWindowSeconds | int | `120` | Seconds to wait after first detecting a failure before creating a RemediationJob. Set to 0 to dispatch immediately. |
| watcher.watchNamespaces | string | `""` | Comma-separated list of namespaces the watcher monitors for failures. Empty means all namespaces. |

## Source Code

## Source Code

* <https://github.com/lenaxia/k8s-mechanic>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| lenaxia |  | <https://github.com/lenaxia> |
