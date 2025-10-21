{{- define "storage-service.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "storage-service.labels" -}}
helm.sh/chart: {{ include "storage-service.fullname" . }}-{{ .Chart.Version | replace "+" "_" }}
{{- include "storage-service.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "storage-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "storage-service.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
