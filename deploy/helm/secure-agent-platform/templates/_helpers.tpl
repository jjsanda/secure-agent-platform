{{/* Common naming + label helpers. */}}

{{- define "sap.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sap.fullname" -}}
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

{{- define "sap.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sap.labels" -}}
helm.sh/chart: {{ include "sap.chart" . }}
{{ include "sap.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: secure-agent-platform
{{- end -}}

{{- define "sap.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sap.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "sap.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "sap.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Render "repository:tag" for a component image, defaulting the tag to the chart
appVersion. Call with: (dict "image" .Values.x.image "ctx" $)
*/}}
{{- define "sap.image" -}}
{{- $tag := .image.tag | default .ctx.Chart.AppVersion -}}
{{- printf "%s:%s" .image.repository $tag -}}
{{- end -}}

{{/* Postgres host — the in-chart service or an external managed DB. */}}
{{- define "sap.postgres.host" -}}
{{- if .Values.externalDatabase.enabled -}}
{{- .Values.externalDatabase.host -}}
{{- else -}}
{{- printf "%s-postgres" (include "sap.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/* Where the backend fetches OIDC discovery/JWKS (in-cluster service). */}}
{{- define "sap.oidc.discoveryUrl" -}}
{{- if .Values.keycloak.enabled -}}
{{- printf "http://%s-keycloak:8080/realms/secure-agent" (include "sap.fullname" .) -}}
{{- else -}}
{{- printf "http://%s-mock-oidc:9000" (include "sap.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/* Browser-facing issuer (token `iss`). */}}
{{- define "sap.oidc.issuerUrl" -}}
{{- if .Values.keycloak.enabled -}}
{{- .Values.keycloak.issuerUrl -}}
{{- else -}}
{{- .Values.mockOidc.issuer -}}
{{- end -}}
{{- end -}}

{{/* OTLP endpoint — empty (no-op) unless observability is enabled. */}}
{{- define "sap.otlpEndpoint" -}}
{{- if .Values.observability.enabled -}}
{{- printf "http://%s-otel-collector:%v" (include "sap.fullname" .) .Values.observability.otelCollector.grpcPort -}}
{{- end -}}
{{- end -}}

{{/* imagePullSecrets block (used verbatim under pod spec). */}}
{{- define "sap.imagePullSecrets" -}}
{{- with .Values.image.pullSecrets }}
imagePullSecrets:
{{- range . }}
  - name: {{ .name | default . }}
{{- end }}
{{- end }}
{{- end -}}
