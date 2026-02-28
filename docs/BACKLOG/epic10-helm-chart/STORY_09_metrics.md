# Story 09: Metrics Service and ServiceMonitor

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Low (optional feature)
**Status:** Not Started
**Estimated Effort:** 20 minutes

---

## User Story

As a **platform engineer**, I want the chart to optionally create a Prometheus-scrape
Service and ServiceMonitor so I can monitor mechanic's internal metrics without manual
configuration.

---

## Acceptance Criteria

- [ ] `charts/mechanic/templates/service-metrics.yaml` is only rendered when
  `metrics.enabled: true`
- [ ] Service selects the watcher Pod via `mechanic.selectorLabels`
- [ ] Service exposes port `8080` with name `metrics`, protocol `TCP`
- [ ] Service type is `ClusterIP`
- [ ] `charts/mechanic/templates/servicemonitor.yaml` is only rendered when both
  `metrics.enabled: true` AND `metrics.serviceMonitor.enabled: true`
- [ ] ServiceMonitor `apiVersion: monitoring.coreos.com/v1`
- [ ] ServiceMonitor selects the metrics Service by its labels
- [ ] ServiceMonitor `endpoints[0].port: metrics`, `interval` and `scrapeTimeout`
  sourced from values
- [ ] Additional labels from `metrics.serviceMonitor.labels` are merged into the
  ServiceMonitor metadata labels (for Prometheus Operator label selectors)
- [ ] When `metrics.enabled: false`, both templates render nothing

---

## Tasks

- [ ] Write `templates/service-metrics.yaml` with `{{- if .Values.metrics.enabled }}` guard
- [ ] Write `templates/servicemonitor.yaml` with double guard
- [ ] Verify Service selector matches watcher Deployment pod template labels
- [ ] Verify labels merge from `metrics.serviceMonitor.labels`

---

## Notes

- The watcher controller already exposes metrics on `:8080` via controller-runtime's
  default metrics server. No code changes are needed.
- The ServiceMonitor requires Prometheus Operator CRDs to be installed. If they are
  absent, `helm lint` still passes (Helm does not validate CRD existence at lint time).
  Document this requirement in NOTES.txt (STORY_10).
- The `metrics.serviceMonitor.labels` merge pattern in Helm:
  ```yaml
  labels:
    {{- include "mechanic.labels" . | nindent 4 }}
    {{- with .Values.metrics.serviceMonitor.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  ```

---

## Dependencies

**Depends on:** STORY_02 (helpers)
**Blocks:** nothing

---

## Definition of Done

- [ ] `helm lint charts/mechanic/` exits 0
- [ ] `helm template --set metrics.enabled=true` renders Service only
- [ ] `helm template --set metrics.enabled=true --set metrics.serviceMonitor.enabled=true`
  renders both Service and ServiceMonitor
- [ ] `helm template` with defaults renders neither resource
