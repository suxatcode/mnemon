{{- define "mnemon-server.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "mnemon-server.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "mnemon-server.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Public DNS name for Ingress, cert-manager, and client --server. */}}
{{- define "mnemon-server.hostname" -}}
{{- if .Values.hostname -}}
{{- .Values.hostname -}}
{{- else if and .Values.ingress.enabled .Values.ingress.hosts -}}
{{- (index .Values.ingress.hosts 0).host -}}
{{- end -}}
{{- end -}}

{{/* Postgres is always external: a DSN in values or an existing Secret. */}}
{{- define "mnemon-server.useExternalDatabase" -}}
{{- if or .Values.database.url .Values.database.existingSecret -}}
true
{{- end -}}
{{- end -}}

{{- define "mnemon-server.usePostgres" -}}
{{- include "mnemon-server.useExternalDatabase" . -}}
{{- end -}}

{{- define "mnemon-server.replicas" -}}
{{- if include "mnemon-server.usePostgres" . -}}
{{- .Values.replicaCount -}}
{{- else -}}
1
{{- end -}}
{{- end -}}

{{- define "mnemon-server.portName" -}}
{{- if .Values.server.tls.enabled -}}https{{- else -}}http{{- end -}}
{{- end -}}

{{- define "mnemon-server.appSecretName" -}}
{{- if .Values.server.existingSecret -}}
{{- .Values.server.existingSecret -}}
{{- else -}}
{{- printf "%s-app" (include "mnemon-server.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "mnemon-server.jwtSecretName" -}}
{{- include "mnemon-server.appSecretName" . -}}
{{- end -}}

{{- define "mnemon-server.tlsSecretName" -}}
{{- if .Values.server.tls.existingSecret -}}
{{- .Values.server.tls.existingSecret -}}
{{- else if .Values.certManager.enabled -}}
{{- default (printf "%s-tls" (include "mnemon-server.fullname" .)) .Values.certManager.secretName -}}
{{- else if and .Values.server.tls.enabled .Values.server.existingSecret -}}
{{- printf "%s-tls" (include "mnemon-server.fullname" .) -}}
{{- else -}}
{{- include "mnemon-server.jwtSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "mnemon-server.generateTLS" -}}
{{- if and .Values.server.tls.enabled (not .Values.server.tls.existingSecret) (not .Values.certManager.enabled) -}}
true
{{- end -}}
{{- end -}}

{{- define "mnemon-server.mountCA" -}}
{{- if and .Values.server.tls.enabled (or (include "mnemon-server.generateTLS" .) .Values.server.tls.caKey) -}}
true
{{- end -}}
{{- end -}}

{{- define "mnemon-server.databaseSecretName" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecret -}}
{{- else if .Values.database.url -}}
{{- if .Values.server.existingSecret -}}
{{- printf "%s-database" (include "mnemon-server.fullname" .) -}}
{{- else -}}
{{- include "mnemon-server.appSecretName" . -}}
{{- end -}}
{{- else -}}
{{- printf "%s-database" (include "mnemon-server.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "mnemon-server.databaseSecretKey" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecretKey | default "url" -}}
{{- else -}}
url
{{- end -}}
{{- end -}}

{{- define "mnemon-server.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "mnemon-server.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}
