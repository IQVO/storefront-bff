{{/*
Expand the name of the chart.
*/}}
{{- define "storefront-bff.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "storefront-bff.fullname" -}}
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
Chart name and version as used by the chart label.
*/}}
{{- define "storefront-bff.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "storefront-bff.labels" -}}
helm.sh/chart: {{ include "storefront-bff.chart" . }}
{{ include "storefront-bff.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels. The Deployment and the Service also pin
app.kubernetes.io/component (api): a Service selecting on these two alone would
select every component a future change adds.
*/}}
{{- define "storefront-bff.selectorLabels" -}}
app.kubernetes.io/name: {{ include "storefront-bff.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "storefront-bff.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "storefront-bff.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image reference. The release job publishes the image as v<version> (and packages
the chart with appVersion <version>), so an empty image.tag means "v" + appVersion.
*/}}
{{- define "storefront-bff.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default (printf "v%s" .Chart.AppVersion) }}
{{- end }}

{{/*
MANDATORY: cmd/api exits at boot naming every missing upstream base URL
(internal/config/config.go: UpstreamEnv). The chart fails the render naming the
values instead of crash-looping. There is no default and no way to switch an
upstream off: composing all six is what this edge is for.
*/}}
{{- define "storefront-bff.requireUpstreams" -}}
{{- range $u := list
  (dict "key" "productDiscoveryUrl" "env" "PRODUCT_DISCOVERY_URL")
  (dict "key" "deliveryPromiseUrl" "env" "DELIVERY_PROMISE_URL")
  (dict "key" "shoppingCartUrl" "env" "SHOPPING_CART_URL")
  (dict "key" "checkoutUrl" "env" "CHECKOUT_URL")
  (dict "key" "customerOrderingUrl" "env" "CUSTOMER_ORDERING_URL")
  (dict "key" "pricingUrl" "env" "PRICING_URL") -}}
{{- if not (index $.Values.config $u.key) -}}
{{- fail (printf "storefront-bff requires config.%s to be set (%s): cmd/api exits at boot naming every missing upstream base URL." $u.key $u.env) -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
extraEnv may not bring back what the chart owns: no *_MODE variable (this
service has no modes at all: the six upstreams over REST are the only runtime)
and none of the variables the chart renders itself (the same variable is never
set twice).
*/}}
{{- define "storefront-bff.requireCleanExtraEnv" -}}
{{- $owned := list "PORT" "COOKIE_SECURE" "PRODUCT_DISCOVERY_URL" "DELIVERY_PROMISE_URL" "SHOPPING_CART_URL" "CHECKOUT_URL" "CUSTOMER_ORDERING_URL" "PRICING_URL" "BUDGET_QUERY_MS" "BUDGET_COMMAND_MS" "BUDGET_CONFIRM_MS" "OTEL_EXPORTER_OTLP_ENDPOINT" "OTEL_SERVICE_NAME" "SERVICE_VERSION" "ENVIRONMENT" "LOG_LEVEL" -}}
{{- range .Values.extraEnv -}}
{{- $name := toString .name -}}
{{- if hasSuffix "_MODE" $name -}}
{{- fail (printf "extraEnv sets %s: *_MODE variables do not exist here - storefront-bff composes the six upstreams over REST only, with no permissive, off or in-memory mode." $name) -}}
{{- end -}}
{{- if has $name $owned -}}
{{- fail (printf "extraEnv sets %s, which the chart renders from its own values: use that value instead." $name) -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
config.shutdownDrainDelay in whole seconds. NOT an env var: the binary reads no
SHUTDOWN_DRAIN_DELAY; this value only sizes the grace period below (endpoint
removal headroom before the listener closes). The chart accepts "Ns", "Nm" and
"0" because it needs the number; unset means 0 (no headroom).
*/}}
{{- define "storefront-bff.drainSeconds" -}}
{{- $raw := toString .Values.config.shutdownDrainDelay -}}
{{- if eq $raw "" -}}0
{{- else if eq $raw "0" -}}0
{{- else if regexMatch "^[0-9]+s$" $raw -}}{{ trimSuffix "s" $raw }}
{{- else if regexMatch "^[0-9]+m$" $raw -}}{{ mul 60 (int (trimSuffix "m" $raw)) }}
{{- else -}}
{{- fail (printf "config.shutdownDrainDelay is %q - want whole seconds (\"5s\"), minutes (\"1m\") or \"0\"." $raw) -}}
{{- end -}}
{{- end -}}

{{/*
The graceful shutdown is one step (cmd/api/main.go): on SIGTERM the listener
closes and srv.Shutdown gives in-flight requests up to 20s. Add
config.shutdownDrainDelay of endpoint-removal headroom before that. A grace
period that does not exceed drain + 20s SIGKILLs a request mid-flight, and the
shopper's action is lost.
*/}}
{{- define "storefront-bff.requireGracePeriod" -}}
{{- $need := add (int (include "storefront-bff.drainSeconds" .)) 20 -}}
{{- if le (int .Values.terminationGracePeriodSeconds) $need -}}
{{- fail (printf "terminationGracePeriodSeconds is %v - it must exceed %v (config.shutdownDrainDelay %vs + srv.Shutdown 20s): a shorter period SIGKILLs a request mid-flight." .Values.terminationGracePeriodSeconds $need (include "storefront-bff.drainSeconds" .)) -}}
{{- end -}}
{{- end -}}

{{/*
PORT is the container port the probes and the Service target.
*/}}
{{- define "storefront-bff.requireHttpPort" -}}
{{- if ne (toString .Values.config.port) (toString .Values.service.targetPort) -}}
{{- fail (printf "config.port is %q but service.targetPort is %v: the container port, the probes and the Service must agree." .Values.config.port .Values.service.targetPort) -}}
{{- end -}}
{{- end -}}

{{/*
The upstream call budgets are whole milliseconds; the binary rejects a
non-positive or non-numeric one at boot, so the chart rejects it at render.
*/}}
{{- define "storefront-bff.requireBudgets" -}}
{{- range $b := list "budgetQueryMs" "budgetCommandMs" "budgetConfirmMs" -}}
{{- $raw := toString (index $.Values.config $b) -}}
{{- if ne $raw "" -}}
{{- if not (regexMatch "^[0-9]+$" $raw) -}}
{{- fail (printf "config.%s is %q - want a positive whole number of milliseconds." $b $raw) -}}
{{- end -}}
{{- if le (int $raw) 0 -}}
{{- fail (printf "config.%s is %q - want a positive whole number of milliseconds." $b $raw) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Every render-time requirement of the real runtime, in one include. The api
Deployment (always rendered) includes it, so a mis-set release fails
`helm template` / `helm install`.
*/}}
{{- define "storefront-bff.requireRealRuntime" -}}
{{- include "storefront-bff.requireUpstreams" . -}}
{{- include "storefront-bff.requireCleanExtraEnv" . -}}
{{- include "storefront-bff.requireBudgets" . -}}
{{- include "storefront-bff.requireGracePeriod" . -}}
{{- include "storefront-bff.requireHttpPort" . -}}
{{- end -}}
