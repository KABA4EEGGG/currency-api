{{/*
Шаблоны имён и лейблов чарта currency.
*/}}

{{- define "currency.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "currency.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s" $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "currency.labels" -}}
app.kubernetes.io/name: {{ include "currency.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "currency.selectorLabels" -}}
app.kubernetes.io/name: {{ include "currency.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "currency.fullnameLabels" -}}
{{ include "currency.labels" . }}
app: {{ include "currency.fullname" . }}
{{- end -}}
