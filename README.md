# k8s-mendabot

A Kubernetes controller that watches `Result` CRDs produced by the
[k8sgpt-operator](https://github.com/k8sgpt-ai/k8sgpt-operator), deduplicates findings by
logical parent resource, and spawns an in-cluster [OpenCode](https://opencode.ai) agent that
investigates the problem and opens a pull request on your GitOps repository with a proposed fix.

## How it works

```
k8sgpt-operator → Result CRDs → mendabot-watcher → Job → mendabot-agent → PR
```

1. The k8sgpt-operator continuously analyses your cluster and writes `Result` CRDs
2. `mendabot-watcher` watches those CRDs, deduplicates by parent resource fingerprint,
   and spawns one Kubernetes Job per unique finding
3. The Job runs the `mendabot-agent` image in-cluster with read-only RBAC
4. The agent clones your GitOps repo, investigates the cluster with `kubectl` and `k8sgpt`,
   and opens a PR with a proposed fix — or comments on an existing PR if one already exists

## Components

| Component | Description |
|---|---|
| `mendabot-watcher` | Go controller (controller-runtime) that watches Result CRDs and creates Jobs |
| `mendabot-agent` | Docker image containing opencode + kubectl + k8sgpt + helm + flux + gh |

## Prerequisites

- k8sgpt-operator installed and producing `Result` CRDs
- A GitHub App installed on your GitOps repository (for PR creation)
- An LLM API key supported by OpenCode (OpenAI-compatible endpoint)
- Kubernetes 1.28+

## Deployment

```bash
# Clone this repo
git clone https://github.com/lenaxia/k8s-mendabot

# Fill in secret placeholders
cp deploy/kustomize/secret-github-app.yaml.example deploy/kustomize/secret-github-app.yaml
cp deploy/kustomize/secret-llm.yaml.example deploy/kustomize/secret-llm.yaml
# Edit both files with your real values

# Apply
kubectl apply -k deploy/kustomize/
```

## Configuration

All configuration is via environment variables on the watcher Deployment and Kubernetes
Secrets for credentials. See [`deploy/kustomize/`](deploy/kustomize/) for the full manifest
set and [`docs/DESIGN/lld/DEPLOY_LLD.md`](docs/DESIGN/lld/DEPLOY_LLD.md) for configuration
reference.

## Documentation

- [`docs/DESIGN/HLD.md`](docs/DESIGN/HLD.md) — Architecture and design decisions
- [`docs/DESIGN/lld/`](docs/DESIGN/lld/) — Component-level low-level designs
- [`docs/BACKLOG/`](docs/BACKLOG/) — Implementation backlog
- [`README-LLM.md`](README-LLM.md) — LLM implementation guide

## License

Apache 2.0
