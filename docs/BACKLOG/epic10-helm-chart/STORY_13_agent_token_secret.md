# Story 13: Agent Token Secret

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Critical (agent Jobs fail without this Secret)
**Status:** Not Started
**Estimated Effort:** 30 minutes

---

## User Story

As a **cluster operator**, I want the chart to create the `mechanic-agent-token`
ServiceAccount token Secret so agent Jobs can authenticate to the Kubernetes API
server without relying on the projected short-lived token.

---

## Background

`internal/jobbuilder/job.go` mounts a volume named `agent-token` onto every agent
Job Pod, sourced from a Secret named `mechanic-agent-token`:

```go
{
    Name: "agent-token",
    VolumeSource: corev1.VolumeSource{
        Secret: &corev1.SecretVolumeSource{
            SecretName: "mechanic-agent-token",
        },
    },
},
```

This Secret is mounted at `/var/run/secrets/mechanic/serviceaccount/` inside the
agent container. `docker/scripts/agent-entrypoint.sh` reads it as `LEGACY_TOKEN`:

```bash
LEGACY_TOKEN=/var/run/secrets/mechanic/serviceaccount/token
```

The entrypoint uses this long-lived legacy SA token (no audience claim) to build a
kubeconfig for the agent's `kubectl` calls. This is needed because some cluster
configurations (notably Talos) reject projected tokens from worker nodes due to
issuer mismatches.

**This Secret does not exist in `deploy/kustomize/`** — it is a known gap in the
Kustomize manifests that was never addressed. The Helm chart must create it.

---

## Acceptance Criteria

- [ ] `charts/mechanic/templates/secret-agent-token.yaml` creates a Secret of type
  `kubernetes.io/service-account-token` named `mechanic-agent-token` in
  `{{ .Release.Namespace }}`
- [ ] The Secret has the annotation
  `kubernetes.io/service-account.name: {{ include "mechanic.agentSAName" . }}`
  which causes Kubernetes to automatically populate the `token`, `ca.crt`, and
  `namespace` keys
- [ ] Standard chart labels applied
- [ ] The Secret name `mechanic-agent-token` is hardcoded — it must match
  exactly what `job.go` references (no templating of the name, since it is a
  compile-time constant in the Go code)

---

## How Kubernetes populates this Secret

When a Secret of type `kubernetes.io/service-account-token` is created with the
`kubernetes.io/service-account.name` annotation pointing to an existing SA, the
token controller automatically injects:
- `token` — a long-lived non-expiring SA token
- `ca.crt` — the cluster CA certificate
- `namespace` — the namespace

No `data:` block is needed in the template. Kubernetes fills it.

---

## Tasks

- [ ] Write `templates/secret-agent-token.yaml`
- [ ] Verify annotation is `kubernetes.io/service-account.name`, not `kubectl.kubernetes.io/...`
- [ ] Verify Secret is in `{{ .Release.Namespace }}`
- [ ] Verify Secret name is exactly `mechanic-agent-token`
- [ ] Run `helm lint charts/mechanic/` — passes

---

## Notes

- This is a legacy token mechanism. Kubernetes 1.24+ deprecated auto-created SA
  tokens (the old behaviour of one token Secret per SA), but you can still request
  one manually by creating a Secret with this type and annotation. The token
  controller will still populate it.
- The `deploy/kustomize/` manifests do not include this Secret. That means anyone
  using the Kustomize path must create it manually. The Helm chart fixes this gap.
- The `agent-entrypoint.sh` has a fallback path: if `LEGACY_TOKEN` is absent, it
  falls back to the standard projected token at
  `/var/run/secrets/kubernetes.io/serviceaccount/token`. The chart should still
  create the Secret so operators do not depend on the fallback path silently.

---

## Dependencies

**Depends on:** STORY_04 (agent ServiceAccount must exist before the annotation reference
  is valid — though in practice both are created in the same `helm install`, so ordering
  is handled by Kubernetes)
**Blocks:** nothing

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] `helm template` output includes a Secret of type `kubernetes.io/service-account-token`
  with the correct annotation and name `mechanic-agent-token`
