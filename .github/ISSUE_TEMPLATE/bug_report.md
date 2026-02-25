---
name: Bug report
about: Something is broken or behaving unexpectedly
labels: bug
---

## Describe the bug

<!-- A clear, concise description of what is wrong. -->

## Steps to reproduce

1.
2.
3.

## Expected behaviour

<!-- What should have happened? -->

## Actual behaviour

<!-- What actually happened? Include any error messages verbatim. -->

## Environment

| Field | Value |
|---|---|
| mendabot chart version | <!-- e.g. 0.3.12 --> |
| mendabot image tag | <!-- e.g. v0.3.12 --> |
| Kubernetes version | <!-- kubectl version --short --> |
| CNI | <!-- e.g. Cilium 1.15.5, Calico 3.27 --> |
| `agentType` | <!-- opencode / claude --> |
| `agentRBACScope` | <!-- cluster / namespace --> |

## Relevant logs

<!-- Watcher logs: kubectl logs -n <namespace> deployment/mendabot-watcher --tail=100 -->
<!-- Agent logs:   kubectl logs -n <namespace> <agent-job-pod> -->

```
paste logs here
```

## kubectl describe output

<!-- kubectl describe remediationjob -n <namespace> <name> -->

```
paste output here
```

## Additional context

<!-- Anything else relevant: Helm values used, NetworkPolicy config, GitOps repo layout, etc. -->
