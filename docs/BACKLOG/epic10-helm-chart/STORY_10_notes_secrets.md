# Story 10: NOTES.txt and Secret Guidance

**Epic:** [epic10-helm-chart](README.md)
**Priority:** Medium (operator experience)
**Status:** Not Started
**Estimated Effort:** 15 minutes

---

## User Story

As a **new operator**, I want `helm install` to print clear post-install instructions
so I know exactly what to do next without reading documentation separately.

---

## Acceptance Criteria

- [ ] `charts/mechanic/templates/NOTES.txt` is rendered after every `helm install`
  and `helm upgrade`
- [ ] NOTES.txt output covers:
  1. Confirmation the chart was installed with the release name and namespace
  2. Verification command: `kubectl get rjob -n {{ .Release.Namespace }}`
  3. Required Secrets — exact key names for each:
     - `secrets.githubApp.name` (default: `mechanic-github-app`): `app-id`,
       `installation-id`, `private-key`
     - `secrets.llm.name` (default: `mechanic-llm`): `api-key`, `base-url`, `model`
  4. Example `kubectl create secret` commands for both Secrets
  5. Warning if `gitops.repo` is empty (should not happen with `required`, but
     belt-and-suspenders)
  6. Conditional block: if `metrics.serviceMonitor.enabled` is true, note that
     Prometheus Operator CRDs must be installed

---

## Tasks

- [ ] Write `templates/NOTES.txt`
- [ ] Verify it renders cleanly after `helm template`
- [ ] Include concrete example Secret creation commands

---

## NOTES.txt content (reference)

```
mechanic has been installed in namespace {{ .Release.Namespace }}.

Verify the watcher is running:
  kubectl get deployment -n {{ .Release.Namespace }} {{ include "mechanic.fullname" . }}
  kubectl get rjob -n {{ .Release.Namespace }}

REQUIRED: Create the following Secrets before the watcher can function.
The key names below are hardcoded in the mechanic jobbuilder — do not change them.

1. GitHub App credentials ({{ .Values.secrets.githubApp.name }}):
   kubectl create secret generic {{ .Values.secrets.githubApp.name }} \
     --namespace {{ .Release.Namespace }} \
     --from-literal=app-id=<your-app-id> \
     --from-literal=installation-id=<your-installation-id> \
     --from-file=private-key=<path-to-private-key.pem>

2. LLM + cluster credentials ({{ .Values.secrets.llm.name }}):
   kubectl create secret generic {{ .Values.secrets.llm.name }} \
     --namespace {{ .Release.Namespace }} \
     --from-literal=api-key=<your-llm-api-key> \
     --from-literal=base-url=<your-llm-base-url> \
     --from-literal=model=<your-model-name> \
     --from-literal=kube-api-server=https://<your-cluster-api-server>:6443

{{- if .Values.metrics.serviceMonitor.enabled }}

NOTE: ServiceMonitor is enabled. Ensure Prometheus Operator is installed in your
cluster, otherwise the ServiceMonitor resource will be ignored.
{{- end }}

For documentation see https://github.com/lenaxia/k8s-mechanic
```
mechanic has been installed in namespace {{ .Release.Namespace }}.

Verify the watcher is running:
  kubectl get deployment -n {{ .Release.Namespace }} {{ include "mechanic.fullname" . }}
  kubectl get rjob -n {{ .Release.Namespace }}

REQUIRED: Create the following Secrets before the watcher can function.

1. GitHub App credentials ({{ .Values.secrets.githubApp.name }}):
   kubectl create secret generic {{ .Values.secrets.githubApp.name }} \
     --namespace {{ .Release.Namespace }} \
     --from-literal=app-id=<your-app-id> \
     --from-literal=installation-id=<your-installation-id> \
     --from-file=private-key=<path-to-private-key.pem>

2. LLM API credentials ({{ .Values.secrets.llm.name }}):
   kubectl create secret generic {{ .Values.secrets.llm.name }} \
     --namespace {{ .Release.Namespace }} \
     --from-literal=api-key=<your-api-key> \
     --from-literal=base-url=<your-llm-base-url> \
     --from-literal=model=<your-model-name>

{{- if .Values.metrics.serviceMonitor.enabled }}

NOTE: ServiceMonitor is enabled. Ensure Prometheus Operator is installed in your
cluster, otherwise the ServiceMonitor resource will be ignored.
{{- end }}

For documentation see https://github.com/lenaxia/k8s-mechanic
```

---

## Notes

- NOTES.txt is Go-templated like any other chart template.
- Helm strips leading/trailing whitespace from NOTES.txt output.
- The `kubectl create secret` examples use `\` line continuations which render
  correctly in most terminals.

---

## Dependencies

**Depends on:** STORY_02 (helpers)
**Blocks:** STORY_11 (CI story should verify NOTES renders)

---

## Definition of Done

- [ ] `helm template` renders NOTES.txt without errors
- [ ] Both Secret creation commands reference the correct key names
