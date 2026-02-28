# Worklog: 0077 — LLM Config Root Cause and Fix

**Date**: 2026-02-24

## Root Cause

The worklog `0076_2026-02-24_llm-configuration-attempts-and-debugging.md` documented trial-and-error against an incorrect assumption about `OPENCODE_CONFIG_CONTENT` schema. The actual cause is a schema mismatch: every format tried used keys that don't exist at the top level of OpenCode's config schema.

### What Schema Actually Requires

From official docs (opencode.ai/docs/providers and opencode.ai/docs/config):

For a custom OpenAI-compatible provider with a custom baseURL and apiKey, the correct config shape is:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "my-custom-provider": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "My Provider",
      "options": {
        "baseURL": "https://ai.thekao.cloud/v1",
        "apiKey": "sk-Ba0CTdypBUrbkIdJXXlmhA"
      },
      "models": {
        "glm-4.7": {
          "name": "GLM-4.7"
        }
      }
    }
  },
  "model": "my-custom-provider/glm-4.7"
}
```

### Why Every Attempted Format Failed

| Attempted format | Error | Why |
|----------------|-------|------|
| `{"provider":{"openai":{"model":"glm-4.7","options":{...}}}}` | Unrecognized key: "model" provider.openai | `model` is not a valid key inside `provider.<name>` — model selection goes at the top level as `"model": "<provider-id>/<model-id>"` |
| `{"model":"glm-4.7","options":{"baseURL":...,"apiKey":...}}` | Unrecognized key: "options" | `options` is not a top-level config key — it belongs inside `provider.<name>`, not at the root |
| `{"provider":{"openai":{"options":{"baseURL":...,"apiKey":...}}}}` | Would fail silently | `openai` is a built-in named provider — setting baseURL to a non-OpenAI endpoint overrides it, but `apiKey` is picked up from auth.json, not from `options` in `OPENCODE_CONFIG_CONTENT`. Also there's no `"model"` key so nothing would be selected. |

**Additional Issue**: `glm-4.7` with the built-in `openai` provider
Even if the schema were correct, pointing to the built-in `openai` provider at `https://ai.thekao.cloud/v1` and asking for model `glm-4.7` would likely fail because OpenCode's model registry for `openai` doesn't know `glm-4.7`. A custom provider definition is required so OpenCode knows to treat it as an opaque model name.

## The Fix

Update the Kubernetes secret with the correct config:

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
        "apiKey": "sk-Ba0CTdypBUrbkIdJXXlmhA"
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

# Delete stuck RemediationJobs to trigger fresh agent runs
kubectl delete remediationjobs --all -n default
```

## Key Points

1. **`model` is a top-level config key**, not a key inside `provider.<name>`. The value format is `"<provider-id>/<model-id>"`.

2. **`options` belongs inside `provider.<name>`**, not at the root level of config.

3. **For custom OpenAI-compatible endpoints you must define a custom provider** with `"npm": "@ai-sdk/openai-compatible"` — you cannot re-use the built-in `openai` provider for a different endpoint and a non-OpenAI model name without it fighting the provider's model registry.

4. **The `$schema` key references the JSON schema** at `https://opencode.ai/config.json` — this is the highest-precedence config layer and all standard schema keys are valid.

5. **Custom provider requires a models registry** inside the provider definition — define available models as `"models": { "model-id": { "name": "Display Name" } }`.

## Actions Taken

1. Updated Chart.yaml to v0.3.7
2. Deployed v0.3.7 to cluster
3. Recreated `llm-credentials-opencode` secret multiple times with incorrect formats
4. Re-enabled LLM readiness check (`LLM_PROVIDER=openai`)
5. Documented all attempts and errors in worklog 0076

## Status

**Watcher deployment**: ✅ Running (v0.3.7)
**LLM readiness check**: ✅ Enabled and working (not blocking jobs)
**RemediationJobs**: ✅ Being created for findings
**Agent jobs**: ✅ Starting with v0.3.7 agent image
**Agent execution**: ❌ BLOCKED — All agents fail with config errors
**LLM migration**: ⚠️ Partial — Secret created but config format incorrect

## Follow-up Items

1. **Apply correct config** — Update secret with custom provider definition as shown in "The Fix" section above.

2. **Test agent execution** — Verify agents can successfully connect to GLM-4.7 model on thekao.cloud and begin analysis/remediation.

3. **Monitor GitHub integration** — Once agents succeed, verify they can push PRs to the gitops repo.

## Git Commands Used

```bash
# Patched deployment to enable LLM readiness check
kubectl patch deployment mechanic -n default -p '{"spec":{"template":{"spec":{"containers":[{"name":"watcher","env":[{"name":"LLM_PROVIDER","value":"openai"}]}]}}}}'

# Recreated secret multiple times (incorrect formats)
kubectl delete secret llm-credentials-opencode -n default
kubectl create secret generic llm-credentials-opencode -n default \
  --from-literal=provider-config='...'

# Cleaned up stuck remediationjobs
kubectl delete remediationjobs --all -n default
```

## Deployment Info

**Image**: `ghcr.io/lenaxia/mechanic-watcher:v0.3.7`
**LLM_PROVIDER**: `openai`
**Secret**: `llm-credentials-opencode` in namespace `default`
**Agent Image**: `ghcr.io/lenaxia/mechanic-agent:v0.3.7`
