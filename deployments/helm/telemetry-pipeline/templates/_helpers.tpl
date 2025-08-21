{{/*
Expand the name of the chart.
*/}}
{{- define "telemetry-pipeline.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "telemetry-pipeline.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "telemetry-pipeline.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "telemetry-pipeline.labels" -}}
helm.sh/chart: {{ include "telemetry-pipeline.chart" . }}
{{ include "telemetry-pipeline.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "telemetry-pipeline.selectorLabels" -}}
app.kubernetes.io/name: {{ include "telemetry-pipeline.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "telemetry-pipeline.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "telemetry-pipeline.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the database connection string
*/}}
{{- define "telemetry-pipeline.databaseHost" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "telemetry-pipeline.fullname" .) }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Create the database port
*/}}
{{- define "telemetry-pipeline.databasePort" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "5432" }}
{{- else }}
{{- .Values.externalDatabase.port | toString }}
{{- end }}
{{- end }}

{{/*
Create the database name
*/}}
{{- define "telemetry-pipeline.databaseName" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.externalDatabase.database }}
{{- end }}
{{- end }}

{{/*
Create the database username
*/}}
{{- define "telemetry-pipeline.databaseUsername" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.username }}
{{- else }}
{{- .Values.externalDatabase.username }}
{{- end }}
{{- end }}

{{/*
Create the database password secret name
*/}}
{{- define "telemetry-pipeline.databaseSecretName" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "telemetry-pipeline.fullname" .) }}
{{- else if .Values.externalDatabase.existingSecret }}
{{- .Values.externalDatabase.existingSecret }}
{{- else }}
{{- printf "%s-database" (include "telemetry-pipeline.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Create the database password secret key
*/}}
{{- define "telemetry-pipeline.databaseSecretPasswordKey" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "password" }}
{{- else if .Values.externalDatabase.existingSecret }}
{{- .Values.externalDatabase.existingSecretPasswordKey }}
{{- else }}
{{- printf "password" }}
{{- end }}
{{- end }}

{{/*
Create image name
*/}}
{{- define "telemetry-pipeline.image" -}}
{{- $registry := .Values.global.imageRegistry | default .Values.image.registry -}}
{{- $repository := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}

{{/*
Create streamer image name
*/}}
{{- define "telemetry-pipeline.streamer.image" -}}
{{- $registry := .Values.global.imageRegistry | default .Values.image.registry -}}
{{- $repository := .Values.streamer.image.repository | default (printf "%s-streamer" .Values.image.repository) -}}
{{- $tag := .Values.streamer.image.tag | default .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}

{{/*
Create collector image name
*/}}
{{- define "telemetry-pipeline.collector.image" -}}
{{- $registry := .Values.global.imageRegistry | default .Values.image.registry -}}
{{- $repository := .Values.collector.image.repository | default (printf "%s-collector" .Values.image.repository) -}}
{{- $tag := .Values.collector.image.tag | default .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}

{{/*
Create api-gateway image name
*/}}
{{- define "telemetry-pipeline.apiGateway.image" -}}
{{- $registry := .Values.global.imageRegistry | default .Values.image.registry -}}
{{- $repository := .Values.apiGateway.image.repository | default (printf "%s-api-gateway" .Values.image.repository) -}}
{{- $tag := .Values.apiGateway.image.tag | default .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}
