# Story 07: Prompt ConfigMap and files/prompts/default.txt

**Epic:** [epic10-helm-chart](README.md)
**Priority:** High (agent cannot function without the prompt)
**Status:** Not Started
**Estimated Effort:** 20 minutes

---

## User Story

As a **cluster operator**, I want the chart to install the agent prompt ConfigMap so
the agent Job can find its prompt, and I want to be able to override the prompt content
entirely via a single values field.

---

## Acceptance Criteria

- [ ] `charts/mechanic/files/prompts/default.txt` contains the full prompt text from
  `deploy/kustomize/configmap-prompt.yaml` (the `data.prompt.txt` value, extracted
  verbatim — not the YAML wrapper)
- [ ] `charts/mechanic/templates/configmap-prompt.yaml` renders a ConfigMap named
  `opencode-prompt` in `{{ .Release.Namespace }}`
- [ ] When `prompt.override` is empty (default): ConfigMap data is loaded via
  `.Files.Get (printf "files/prompts/%s.txt" .Values.prompt.name)`
- [ ] When `prompt.override` is non-empty: ConfigMap data uses `prompt.override`
  content instead; `prompt.name` is ignored
- [ ] ConfigMap carries standard chart labels
- [ ] `helm template` renders a ConfigMap with the full prompt content visible in
  the `data.prompt.txt` key

---

## Tasks

- [ ] Extract prompt content from `deploy/kustomize/configmap-prompt.yaml` into
  `charts/mechanic/files/prompts/default.txt`
- [ ] Write `templates/configmap-prompt.yaml`
- [ ] Verify `.Files.Get` path resolution — the path is relative to the chart root
- [ ] Verify `prompt.override` takes precedence when set
- [ ] Verify rendered ConfigMap key is `prompt.txt` (matching what the entrypoint reads)

---

## Notes

- The ConfigMap is named `opencode-prompt` because the agent entrypoint and JobBuilder
  currently hardcode this name. Changing the name would require a Go code change — out
  of scope for this epic.
- `.Files.Get` returns an empty string for a missing file (it does not error at render
  time). A `required` wrapper should be used:
  ```
  {{- $content := .Files.Get (printf "files/prompts/%s.txt" .Values.prompt.name) }}
  {{- if not $content }}
  {{- fail (printf "prompt file files/prompts/%s.txt not found in chart" .Values.prompt.name) }}
  {{- end }}
  ```
- The prompt file contains `${VARIABLE}` shell-substitution syntax. These must be
  preserved verbatim — do not use Helm `{{ }}` templating inside the prompt file.
  Use the `|` block scalar in YAML to avoid any escaping issues.

---

## Dependencies

**Depends on:** STORY_02 (helpers for labels)
**Blocks:** nothing

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] `helm template` output contains `data.prompt.txt` with non-empty content
- [ ] `prompt.override: "custom content"` renders `data.prompt.txt: custom content`
- [ ] `prompt.name: nonexistent` renders a helm fail error
