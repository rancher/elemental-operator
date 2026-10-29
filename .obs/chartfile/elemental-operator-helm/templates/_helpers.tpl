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
{{- else if .Values.registryUrl -}}
{{- printf "%s/" .Values.registryUrl -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{/*
List the unique imagePullSecret names, accepting either strings or object references.
Templates can only return strings, so the list is rendered as a JSON array.
Usage: {{ range include "elemental-operator.imagePullSecretNames" . | fromJsonArray }}
*/}}
{{- define "elemental-operator.imagePullSecretNames" -}}
{{- $pullSecrets := list -}}
{{- range .Values.global.cattle.imagePullSecrets -}}
  {{- if kindIs "map" . -}}
    {{- if .name -}}
      {{- $pullSecrets = append $pullSecrets .name -}}
    {{- end -}}
  {{- else if not (empty .) -}}
    {{- $pullSecrets = append $pullSecrets . -}}
  {{- end -}}
{{- end -}}
{{- $pullSecrets | uniq | toJson -}}
{{- end -}}

{{/*
Render imagePullSecrets
*/}}
{{- define "elemental-operator.imagePullSecrets" -}}
{{- $pullSecrets := include "elemental-operator.imagePullSecretNames" . | fromJsonArray -}}
{{- if $pullSecrets -}}
imagePullSecrets:
  {{- range $pullSecrets }}
  - name: {{ . | quote }}
  {{- end }}
{{- end -}}
{{- end }}

{{/*
Render image-pull-secret flags
*/}}
{{- define "elemental-operator.imagePullSecretFlags" -}}
{{- $pullSecrets := include "elemental-operator.imagePullSecretNames" . | fromJsonArray -}}
{{- if $pullSecrets -}}
  {{- range $pullSecrets }}
- --image-pull-secret
- {{ . | quote }}
  {{- end }}
{{- end -}}
{{- end }}