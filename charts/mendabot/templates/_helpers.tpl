{{/*
Expand the name of the chart.
*/}}
{{- define "mendabot.name" -}}
{{- "mendabot" }}
{{- end }}

{{/*
Create a default fully qualified app name.
If the release name already contains "mendabot", use it as-is.
Otherwise append "-mendabot". Truncate to 63 characters.
*/}}
{{- define "mendabot.fullname" -}}
{{- if contains "mendabot" .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-mendabot" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "mendabot.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "mendabot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels (stable subset used in matchLabels and Service selectors).
*/}}
{{- define "mendabot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mendabot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Watcher ServiceAccount name.
*/}}
{{- define "mendabot.watcherSAName" -}}
{{- printf "%s-watcher" (include "mendabot.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Agent ServiceAccount name.
*/}}
{{- define "mendabot.agentSAName" -}}
{{- printf "%s-agent" (include "mendabot.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Namespace-scoped agent ServiceAccount name (used when watcher.agentRBACScope=namespace).
*/}}
{{- define "mendabot.agentNSSAName" -}}
{{- printf "%s-agent-ns" (include "mendabot.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Watcher container image (repository:tag).
Falls back to Chart.AppVersion when image.tag is empty.
*/}}
{{- define "mendabot.watcherImage" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Agent container image (repository:tag).
Falls back to Chart.AppVersion when agent.image.tag is empty.
*/}}
{{- define "mendabot.agentImage" -}}
{{- $tag := .Values.agent.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.agent.image.repository $tag }}
{{- end }}
