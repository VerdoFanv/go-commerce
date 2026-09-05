{{/* Chart helper templates */}}
{{- define "golang-be.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "golang-be.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "golang-be.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "golang-be.labels" -}}
app.kubernetes.io/name: {{ include "golang-be.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "golang-be.image" -}}
{{- $repo := printf "%s/%s" .Values.image.registry .Values.image.repository -}}
{{- printf "%s-%s:%s" $repo .component .Values.image.tag -}}
{{- end -}}
