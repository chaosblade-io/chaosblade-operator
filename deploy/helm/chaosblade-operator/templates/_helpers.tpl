{{/* vim: set filetype=mustache: */}}

{{- define "image.domain" -}}
{{- $domain := "" -}}
{{- if eq .Values.env.region "cn-public" }}
   {{- printf "%s" "registry" -}}
{{- else -}}
   {{- printf "%s" "registry-vpc" -}}
{{- end -}}
{{- end -}}

{{- define "image.region" -}}
{{- if eq .Values.env.region "cn-public" }}
    {{- printf "%s" "cn-hangzhou" -}}
{{- else -}}
    {{- printf "%s" .Values.env.region -}}
{{- end -}}
{{- end -}}


{{/*
Create the repository for the service image
*/}}
{{- define "operator.image" -}}
{{- printf "%s.%s.aliyuncs.com/chaosblade/chaosblade-operator" (include "image.domain" .) (include "image.region" .) -}}
{{- end -}}

{{/*
Create the repository for the service image
*/}}
{{- define "tool.image" -}}
{{- printf "%s.%s.aliyuncs.com/chaosblade/chaosblade-tool" (include "image.domain" .) (include "image.region" .) -}}
{{- end -}}