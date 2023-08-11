{{- define "system_default_registry" -}}
{{- if .Values.global.cattle.systemDefaultRegistry -}}
{{- printf "%s/" .Values.global.cattle.systemDefaultRegistry -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "registry_url" -}}
{{- if .Values.global.cattle.systemDefaultRegistry -}}
{{ include "system_default_registry" . }}
{{- else if .Values.registry_url -}}
{{- printf "%s/" .Values.registry_url -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}
