{{/*
Expand the name of the chart.
*/}}
{{- define "mlflow.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "mlflow.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "mlflow.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "mlflow.labels" -}}
helm.sh/chart: {{ include "mlflow.chart" . }}
{{ include "mlflow.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "mlflow.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mlflow.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "mlflow.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "mlflow.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}


{{- define "mlflow.autoGenCert" -}}
  {{- if and .Values.ingress.tls.enabled (eq .Values.ingress.tls.certSource "auto") -}}
    {{- printf "true" -}}
  {{- else -}}
    {{- printf "false" -}}
  {{- end -}}
{{- end -}}

{{- define "mlflow.autoGenCertForIngress" -}}
  {{- if and (eq (include "mlflow.autoGenCert" .) "true") .Values.ingress.enabled -}}
    {{- printf "true" -}}
  {{- else -}}
    {{- printf "false" -}}
  {{- end -}}
{{- end -}}

{{- define "mlflow.tlsSecretForIngress" -}}
  {{- if eq .Values.ingress.tls.certSource "none" -}}
    {{- printf "" -}}
  {{- else if eq .Values.ingress.tls.certSource "secret" -}}
    {{- .Values.ingress.tls.secret.secretName -}}
  {{- else -}}
    {{- printf "%s-ca" (include "mlflow.fullname" .) -}}
  {{- end -}}
{{- end -}}

{{- define "mysql.connectURL" -}}
mysql+pymysql://
{{- if .Values.mysql.auth.username -}}
    {{ urlquery .Values.mysql.auth.username -}}
    {{- if .Values.mysql.auth.password -}}
        :{{ urlquery .Values.mysql.auth.password }}
    {{- end -}}
@{{ printf "%s-mysql" (include "mlflow.fullname" .) }}
{{- end -}}
{{- if .Values.mysql.auth.database -}}
/{{- .Values.mysql.auth.database -}}
{{- end -}}
{{- end -}}