# Story 03: Namespace Template

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Low
**Status:** Not Started
**Estimated Effort:** 10 minutes

---

## User Story

As a **cluster operator**, I want the chart to optionally create the target namespace
so I can do a one-shot install without pre-creating it manually.

---

## Acceptance Criteria

- [ ] `charts/mechanic/templates/namespace.yaml` exists
- [ ] The namespace resource is only rendered when `createNamespace: true` in values
- [ ] The namespace name is `{{ .Release.Namespace }}`
- [ ] Standard chart labels applied via `include "mechanic.labels"`
- [ ] When `createNamespace: false` (the default), the template renders nothing —
  no empty YAML document

---

## Tasks

- [ ] Write `templates/namespace.yaml` with `{{- if .Values.createNamespace }}` guard
- [ ] Verify `helm template` with `createNamespace: false` emits nothing for this template
- [ ] Verify `helm template` with `createNamespace: true` emits a valid Namespace resource

---

## Notes

- Default is `false` because operators typically pre-create namespaces with their
  own labels, annotations, or quota policies. Forcing namespace creation would
  overwrite those.
- No `--create-namespace` Helm flag interaction needed: if the operator passes
  `--create-namespace` to `helm install` and also sets `createNamespace: true`,
  the namespace is applied twice, which is idempotent (`kubectl apply` semantics).

---

## Dependencies

**Depends on:** STORY_02 (_helpers.tpl must exist)
**Blocks:** nothing (independent template)

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] Template guard verified manually with `helm template --set createNamespace=true`
