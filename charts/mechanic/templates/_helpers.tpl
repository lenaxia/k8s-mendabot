{{/*
Expand the name of the chart.
*/}}
{{- define "mechanic.name" -}}
{{- "mechanic" }}
{{- end }}

{{/*
Create a default fully qualified app name.
If the release name already contains "mechanic", use it as-is.
Otherwise append "-mechanic". Truncate to 63 characters.
*/}}
{{- define "mechanic.fullname" -}}
{{- if contains "mechanic" .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-mechanic" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "mechanic.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "mechanic.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels (stable subset used in matchLabels and Service selectors).
*/}}
{{- define "mechanic.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mechanic.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Watcher ServiceAccount name.
*/}}
{{- define "mechanic.watcherSAName" -}}
{{- printf "%s-watcher" (include "mechanic.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Agent ServiceAccount name.
*/}}
{{- define "mechanic.agentSAName" -}}
{{- printf "%s-agent" (include "mechanic.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Namespace-scoped agent ServiceAccount name (used when watcher.agentRBACScope=namespace).
*/}}
{{- define "mechanic.agentNSSAName" -}}
{{- printf "%s-agent-ns" (include "mechanic.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Watcher container image (repository:tag).
Falls back to Chart.AppVersion when image.tag is empty.
*/}}
{{- define "mechanic.watcherImage" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Agent container image (repository:tag).
Falls back to Chart.AppVersion when agent.image.tag is empty.
*/}}
{{- define "mechanic.agentImage" -}}
{{- $tag := .Values.agent.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.agent.image.repository $tag }}
{{- end }}
