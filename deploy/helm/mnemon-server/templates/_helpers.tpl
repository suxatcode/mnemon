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

{{- define "mnemon-server.useExternalDatabase" -}}
{{- if or .Values.database.url .Values.database.existingSecret -}}
true
{{- end -}}
{{- end -}}

{{- define "mnemon-server.useBundledPostgres" -}}
{{- if and .Values.postgresql.enabled (not (include "mnemon-server.useExternalDatabase" .)) -}}
true
{{- end -}}
{{- end -}}

{{- define "mnemon-server.usePostgres" -}}
{{- if or (include "mnemon-server.useExternalDatabase" .) (include "mnemon-server.useBundledPostgres" .) -}}
true
{{- end -}}
{{- end -}}

{{- define "mnemon-server.replicas" -}}
{{- if include "mnemon-server.usePostgres" . -}}
{{- .Values.replicaCount -}}
{{- else -}}
1
{{- end -}}
{{- end -}}

{{- define "mnemon-server.appSecretName" -}}
{{- if .Values.server.existingSecret -}}
{{- .Values.server.existingSecret -}}
{{- else -}}
{{- printf "%s-app" (include "mnemon-server.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "mnemon-server.tlsSecretName" -}}
{{- if .Values.server.tls.existingSecret -}}
{{- .Values.server.tls.existingSecret -}}
{{- else -}}
{{- include "mnemon-server.appSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "mnemon-server.postgresSecretName" -}}
{{- printf "%s-postgresql" (include "mnemon-server.fullname" .) -}}
{{- end -}}

{{- define "mnemon-server.postgresHost" -}}
{{- printf "%s-postgresql" (include "mnemon-server.fullname" .) -}}
{{- end -}}

{{- define "mnemon-server.databaseSecretName" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecret -}}
{{- else if .Values.database.url -}}
{{- include "mnemon-server.appSecretName" . -}}
{{- else -}}
{{- include "mnemon-server.postgresSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "mnemon-server.databaseSecretKey" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecretKey | default "url" -}}
{{- else if .Values.database.url -}}
url
{{- else -}}
database-url
{{- end -}}
{{- end -}}
