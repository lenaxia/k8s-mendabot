# Worklog: LLM Configuration Attempts and Debugging

**Date:** 2026-02-24
**Session:** Debugging OpenCode CLI config schema and watcher RBAC to unblock agent execution
**Status:** Complete

---

## Objective

Two blockers prevented agent execution:

1. The `OPENCODE_CONFIG_CONTENT` JSON in `llm-credentials-opencode` used an incorrect
   schema — all formats tried were rejected by OpenCode CLI at startup.
2. The watcher pod could not start because `secrets` was missing from the
   `mechanic-watcher` ClusterRole, causing controller-runtime REST mapper initialisation
   to fail with `v1: Unauthorized`.

---

## Work Completed

### 1. Re-enabled LLM Readiness Check

Patched the watcher deployment to set `LLM_PROVIDER=openai`, re-enabling the LLM
readiness gate. Verified via watcher logs that RemediationJobs are dispatched without
readiness errors.

```bash
kubectl patch deployment mechanic -n default \
  -p '{"spec":{"template":{"spec":{"containers":[{"name":"watcher","env":[{"name":"LLM_PROVIDER","value":"openai"}]}]}}}}'
```

### 2. Iterated Through Incorrect OpenCode Config Formats (All Rejected)

Three formats were attempted, all rejected by OpenCode CLI at `OPENCODE_CONFIG_CONTENT`:

| Format tried | Error |
|---|---|
| `{"provider":{"openai":{"model":"glm-4.7","options":{...}}}}` | `Unrecognized key: "model" provider.openai` |
| `{"provider":{"openai":{"options":{"baseURL":...,"apiKey":...}}}}` | No `model` key — nothing selected |
| `{"model":"glm-4.7","options":{"baseURL":...,"apiKey":...}}` | `Unrecognized key: "options"` |

### 3. Identified OpenCode Config Root Cause

Consulted `opencode.ai/docs/providers` and `opencode.ai/docs/config`. All attempted
formats violated the actual schema:

1. **`model` is a top-level config key**, not a key inside `provider.<name>`. The value
   must be `"<provider-id>/<model-id>"`.
2. **`options` belongs inside `provider.<name>`**, not at the config root.
3. For a non-standard model at a custom endpoint, a **custom provider** with
   `"npm": "@ai-sdk/openai-compatible"` is required. Reusing the built-in `openai`
   provider for `glm-4.7` does not work — its model registry does not know this name.

### 4. Applied Correct OpenCode Config and Cleared RemediationJobs

Created the secret with the correct schema:

```bash
kubectl delete secret llm-credentials-opencode -n default

cat > /tmp/config.json << 'EOF'
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "thekao-cloud": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "TheKao Cloud",
      "options": {
        "baseURL": "https://ai.thekao.cloud/v1",
        "apiKey": "<redacted>"
      },
      "models": {
        "glm-4.7": {
          "name": "GLM-4.7"
        }
      }
    }
  },
  "model": "thekao-cloud/glm-4.7"
}
EOF

kubectl create secret generic llm-credentials-opencode -n default \
  --from-file=provider-config=/tmp/config.json
```

Deleted all RemediationJobs to trigger fresh agent runs with the corrected secret:

```bash
kubectl delete remediationjobs --all -n default
```

### 5. Diagnosed and Fixed Watcher `Unauthorized` Error

Watcher pod `mechanic-68d6d55795-llw4n` failed to start with:

```
unable to start manager: failed to determine if *v1.Secret is namespaced:
failed to get restmapping: failed to get API group resources:
unable to retrieve the complete list of server APIs: v1: Unauthorized
```

controller-runtime tries to register a `*v1.Secret` informer at startup (for LLM
credential watching). `secrets` was absent from the `mechanic-watcher` ClusterRole,
so the REST mapper initialisation failed before the manager could start.

Patched the live ClusterRole:

```bash
kubectl patch clusterrole mechanic-watcher --type='json' \
  -p='[{"op":"add","path":"/rules/0/resources/-","value":"secrets"}]'
```

Deleted the failing pod to trigger an immediate restart:

```bash
kubectl delete pod mechanic-68d6d55795-llw4n -n default
```

New pod `mechanic-68d6d55795-pwx7t` started cleanly. All controllers came up:

```
Starting Controller  controller=remediationjob
Starting Controller  controller=deployment
Starting Controller  controller=pod
Starting Controller  controller=node
Starting Controller  controller=persistentvolumeclaim
Starting Controller  controller=job
Starting Controller  controller=statefulset
Starting workers     (all controllers)
```

RemediationJobs were immediately dispatched and 3 agent pods reached `Running`:
- `mechanic-agent-22693f928816-ftsld`
- `mechanic-agent-2167deb6ac69-gvkg6`
- `mechanic-agent-0cd2345e0966-5t4wz`

Agent logs confirmed the gitops repo was cloned and opencode is executing.

### 6. Persisted RBAC Fix to Helm Chart

The live `kubectl patch` would be lost on the next `helm upgrade`. Added `secrets`
permanently to `charts/mechanic/templates/clusterrole-watcher.yaml`:

```yaml
- apiGroups: [""]
  resources: ["pods", "persistentvolumeclaims", "nodes", "namespaces", "secrets"]
  verbs: ["get", "list", "watch"]
```

---

## Key Decisions

**Custom provider ID instead of `openai`:** The built-in `openai` provider in OpenCode
uses `@ai-sdk/openai` (not `@ai-sdk/openai-compatible`) and its model registry does not
accept arbitrary model names. A custom provider entry with
`"npm": "@ai-sdk/openai-compatible"` is required for any non-standard endpoint or model.

**`model` format is `"<provider-id>/<model-id>"`:** The provider-id must exactly match
the key under `"provider"` in the config, and the model-id must match a key under
`"models"` within that provider.

**`secrets` in ClusterRole is required for controller-runtime startup:** Any resource
type registered with the controller-runtime scheme or cache must have a corresponding
RBAC rule. Missing it causes REST mapper initialisation to fail before the manager starts.

---

## Blockers

None.

---

## Tests Run

No automated tests run. This session was live cluster debugging only.

---

## Next Steps

1. **Verify agent PRs are opened** — confirm at least one agent completes successfully
   and opens a PR against the gitops repo for `test-broken-image` or `test-crashloop`:
   ```bash
   kubectl get remediationjobs -n default
   kubectl logs -n default -l app=mechanic-agent --tail=50
   ```

2. **Commit and release Helm chart** — the `clusterrole-watcher.yaml` change needs to
   be committed and a new chart version cut so the fix is included in future deployments.

---

## Files Modified

- `charts/mechanic/templates/clusterrole-watcher.yaml` — added `secrets` to core API
  group read rule
