# Story 04: Helm Chart Update

**Epic:** [epic08-pluggable-agent](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 2 hours

---

## User Story

As a **mendabot operator**, I want the Helm chart to render agent-type-aware ConfigMaps
and inject `AGENT_TYPE` into the watcher Deployment, so that a single `values.yaml`
field controls which agent runner is used end-to-end.

---

## Background

The chart currently renders one ConfigMap (`opencode-prompt`) from one file
(`default.txt`). This story replaces it with two ConfigMaps per-deployment and splits
the prompt into a shared core and an agent-specific supplement.

---

## Acceptance Criteria

- [ ] `values.yaml` gains `agentType: opencode` (default) with a comment listing
      accepted values
- [ ] `values.yaml` replaces `prompt.name` / `prompt.override` with:
  - `prompt.coreOverride` — full override for the core prompt
  - `prompt.agentOverride` — full override for the agent supplement
- [ ] `charts/mendabot/templates/configmap-prompt.yaml` renders **two** ConfigMaps:
  - `agent-prompt-core` — contains key `core.txt`
  - `agent-prompt-{{ .Values.agentType }}` — contains key `agent.txt`
- [ ] `charts/mendabot/files/prompts/core.txt` — current `default.txt` content
      (minus OpenCode-specific preamble)
- [ ] `charts/mendabot/files/prompts/opencode.txt` — short agent-specific preamble
      (available tools, opencode-specific notes)
- [ ] `charts/mendabot/files/prompts/claude.txt` — empty/stub
- [ ] `charts/mendabot/files/prompts/default.txt` — deleted (breaking change)
- [ ] `charts/mendabot/templates/deployment-watcher.yaml` gains:
  ```yaml
  - name: AGENT_TYPE
    value: {{ .Values.agentType | quote }}
  ```
- [ ] `charts/mendabot/templates/NOTES.txt` documents:
  - Secret renamed from `llm-credentials` to `llm-credentials-opencode`
  - New secret key structure (`provider-config`, `model`, `kube-api-server`)
  - Migration command
- [ ] `helm lint charts/mendabot` passes with required values set

---

## Technical Implementation

### configmap-prompt.yaml

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: agent-prompt-core
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "mendabot.labels" . | nindent 4 }}
data:
  core.txt: |
    {{- if .Values.prompt.coreOverride }}
    {{- .Values.prompt.coreOverride | nindent 4 }}
    {{- else }}
    {{- $content := .Files.Get "files/prompts/core.txt" }}
    {{- if not $content }}
    {{- fail "prompt file files/prompts/core.txt not found in chart" }}
    {{- end }}
    {{- $content | nindent 4 }}
    {{- end }}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: agent-prompt-{{ .Values.agentType }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "mendabot.labels" . | nindent 4 }}
data:
  agent.txt: |
    {{- if .Values.prompt.agentOverride }}
    {{- .Values.prompt.agentOverride | nindent 4 }}
    {{- else }}
    {{- $content := .Files.Get (printf "files/prompts/%s.txt" .Values.agentType) }}
    {{- $content | nindent 4 }}
    {{- end }}
```

Note: the agent ConfigMap is mounted at `/prompt/` alongside `core.txt`. The job
builder (STORY_02) mounts `agent-prompt-<agentType>` at `/prompt/`. The Helm chart
must ensure that ConfigMap has **both** keys: `core.txt` and `agent.txt`.

Wait — the job builder mounts a **single** ConfigMap at `/prompt/`. To have both
`core.txt` and `agent.txt` accessible at that path, the agent-type ConfigMap must
contain both keys. The simplest approach: render the agent-type ConfigMap with two
keys — `core.txt` (from `core.txt`) and `agent.txt` (from `<agentType>.txt`).

### Revised configmap-prompt.yaml (single ConfigMap per agent type, two keys)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agent-prompt-{{ .Values.agentType }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "mendabot.labels" . | nindent 4 }}
data:
  core.txt: |
    {{- if .Values.prompt.coreOverride }}
    {{- .Values.prompt.coreOverride | nindent 4 }}
    {{- else }}
    {{- $content := .Files.Get "files/prompts/core.txt" }}
    {{- if not $content }}
    {{- fail "prompt file files/prompts/core.txt not found in chart" }}
    {{- end }}
    {{- $content | nindent 4 }}
    {{- end }}
  agent.txt: |
    {{- if .Values.prompt.agentOverride }}
    {{- .Values.prompt.agentOverride | nindent 4 }}
    {{- else }}
    {{- $content := .Files.Get (printf "files/prompts/%s.txt" .Values.agentType) }}
    {{- $content | nindent 4 }}
    {{- end }}
```

This keeps the job builder's single-ConfigMap mount model while exposing both files.
The ConfigMap name is `agent-prompt-<agentType>` — matching what the job builder
derives from `AgentType`.

---

## Dependencies

Depends on STORY_02 and STORY_03.

## Definition of Done

- [ ] Two prompt files exist (`core.txt`, `opencode.txt`)
- [ ] `default.txt` deleted
- [ ] ConfigMap template renders correctly for all agent types
- [ ] Deployment injects `AGENT_TYPE`
- [ ] NOTES.txt updated with migration instructions
- [ ] `helm lint charts/mendabot` passes
