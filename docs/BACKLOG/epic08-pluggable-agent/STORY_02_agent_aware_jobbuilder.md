# Story 02: Agent-Aware Job Builder

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mechanic developer**, I want the job builder to derive secret names and prompt
ConfigMap names from the agent type, so that adding a new agent runner requires no
changes to the job builder — only a new Secret and entrypoint script.

---

## Background

Today `job.go` hardcodes `"llm-credentials"`, `OPENAI_API_KEY`, `OPENAI_BASE_URL`,
`OPENAI_MODEL`, and `"opencode-prompt"`. These names must become agent-type-derived.

The opaque blob pattern means the job builder injects exactly three env vars from the
secret (regardless of agent type): `AGENT_PROVIDER_CONFIG`, `AGENT_MODEL`, and
`KUBE_API_SERVER`.

---

## Acceptance Criteria

- [ ] `jobbuilder.Config` gains an `AgentType config.AgentType` field
- [ ] Secret name injected by `Build()` is `"llm-credentials-" + string(cfg.AgentType)`
- [ ] Three env vars are injected from the secret:
  - `AGENT_PROVIDER_CONFIG` from key `"provider-config"`
  - `AGENT_MODEL` from key `"model"`
  - `KUBE_API_SERVER` from key `"kube-api-server"`
- [ ] The old `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_MODEL` env vars are removed
- [ ] Prompt volume uses ConfigMap `"agent-prompt-" + string(cfg.AgentType)`
      (e.g. `"agent-prompt-opencode"`)
- [ ] `TestBuild_SecretKeyRefs` updated to assert new names
- [ ] `TestBuild_EnvVars_AllPresent` updated — old OPENAI_* entries replaced by new names
- [ ] New table-driven test `TestBuild_SecretName_ByAgentType` asserts correct secret
      name for each agent type
- [ ] `go test -timeout 30s -race ./internal/jobbuilder/...` passes

---

## Technical Implementation

### `internal/jobbuilder/job.go`

Update `Config`:

```go
type Config struct {
    AgentNamespace string
    AgentType      config.AgentType
}
```

Update `New()` to default `AgentType` if empty:

```go
func New(cfg Config) (*Builder, error) {
    if cfg.AgentNamespace == "" {
        return nil, fmt.Errorf("jobbuilder: AgentNamespace must not be empty")
    }
    if cfg.AgentType == "" {
        cfg.AgentType = config.AgentTypeOpenCode
    }
    return &Builder{cfg: cfg}, nil
}
```

In `Build()`, replace the three OPENAI_ env vars and the hardcoded secret name with:

```go
secretName := "llm-credentials-" + string(b.cfg.AgentType)

// Three env vars from the per-agent secret (opaque blob pattern)
{
    Name: "AGENT_PROVIDER_CONFIG",
    ValueFrom: &corev1.EnvVarSource{
        SecretKeyRef: &corev1.SecretKeySelector{
            LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
            Key:                  "provider-config",
        },
    },
},
{
    Name: "AGENT_MODEL",
    ValueFrom: &corev1.EnvVarSource{
        SecretKeyRef: &corev1.SecretKeySelector{
            LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
            Key:                  "model",
        },
    },
},
{
    Name: "KUBE_API_SERVER",
    ValueFrom: &corev1.EnvVarSource{
        SecretKeyRef: &corev1.SecretKeySelector{
            LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
            Key:                  "kube-api-server",
        },
    },
},
```

Replace prompt ConfigMap reference:

```go
promptCMName := "agent-prompt-" + string(b.cfg.AgentType)

// In volumes:
{
    Name: "prompt-configmap",
    VolumeSource: corev1.VolumeSource{
        ConfigMap: &corev1.ConfigMapVolumeSource{
            LocalObjectReference: corev1.LocalObjectReference{Name: promptCMName},
        },
    },
},
```

---

## Dependencies

Depends on STORY_01 (`AgentType` type must exist in `internal/config`).

## Definition of Done

- [ ] Secret name, env vars, and ConfigMap name are agent-type-derived
- [ ] All old hardcoded names removed
- [ ] All tests updated and passing
