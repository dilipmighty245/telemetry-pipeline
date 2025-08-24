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

{{/*
Determine Redis host based on configuration
*/}}
{{- define "telemetry-pipeline.redisHost" -}}
{{- if .Values.externalRedis.enabled }}
{{- .Values.externalRedis.host }}
{{- else }}
{{- printf "%s-redis-master" (include "telemetry-pipeline.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Determine Redis port based on configuration
*/}}
{{- define "telemetry-pipeline.redisPort" -}}
{{- if .Values.externalRedis.enabled }}
{{- .Values.externalRedis.port | toString }}
{{- else }}
{{- printf "6379" }}
{{- end }}
{{- end }}

{{/*
Create Redis connection URL
*/}}
{{- define "telemetry-pipeline.redisURL" -}}
{{- if .Values.externalRedis.enabled }}
{{- if .Values.externalRedis.tls.enabled }}
{{- printf "rediss://%s:%s" (include "telemetry-pipeline.redisHost" .) (include "telemetry-pipeline.redisPort" .) }}
{{- else }}
{{- printf "redis://%s:%s" (include "telemetry-pipeline.redisHost" .) (include "telemetry-pipeline.redisPort" .) }}
{{- end }}
{{- else }}
{{- printf "redis://%s:%s" (include "telemetry-pipeline.redisHost" .) (include "telemetry-pipeline.redisPort" .) }}
{{- end }}
{{- end }}

{{/*
Determine deployment mode
*/}}
{{- define "telemetry-pipeline.deploymentMode" -}}
{{- if and .Values.streamer.enabled .Values.collector.enabled .Values.apiGateway.enabled }}
{{- printf "same-cluster" }}
{{- else if and .Values.streamer.enabled (not .Values.collector.enabled) (not .Values.apiGateway.enabled) }}
{{- printf "edge-cluster" }}
{{- else if and (not .Values.streamer.enabled) .Values.collector.enabled .Values.apiGateway.enabled }}
{{- printf "central-cluster" }}
{{- else }}
{{- printf "custom" }}
{{- end }}
{{- end }}

{{/*
Create deployment mode labels
*/}}
{{- define "telemetry-pipeline.deploymentModeLabels" -}}
deployment-mode: {{ include "telemetry-pipeline.deploymentMode" . }}
{{- if .Values.externalRedis.enabled }}
redis-mode: "external"
{{- else }}
redis-mode: "embedded"
{{- end }}

{{- end }}

{{/*
Create environment variables for Redis connection
*/}}
{{- define "telemetry-pipeline.redisEnvVars" -}}
- name: REDIS_URL
  value: {{ include "telemetry-pipeline.redisURL" . | quote }}
{{- if .Values.externalRedis.enabled }}
{{- if .Values.externalRedis.auth.enabled }}
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.externalRedis.auth.existingSecret | default (printf "%s-redis-auth" (include "telemetry-pipeline.fullname" .)) }}
      key: {{ .Values.externalRedis.auth.existingSecretPasswordKey | default "password" }}
{{- end }}
{{- if .Values.externalRedis.tls.enabled }}
- name: REDIS_TLS_ENABLED
  value: "true"
{{- if .Values.externalRedis.tls.caCert }}
- name: REDIS_CA_CERT
  valueFrom:
    secretKeyRef:
      name: {{ printf "%s-redis-ca" (include "telemetry-pipeline.fullname" .) }}
      key: ca.crt
{{- end }}
{{- end }}
{{- else }}
{{- if .Values.redis.auth.enabled }}
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ printf "%s-redis" (include "telemetry-pipeline.fullname" .) }}
      key: redis-password
{{- end }}
{{- end }}
{{- end }}


