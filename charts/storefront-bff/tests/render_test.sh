#!/usr/bin/env bash
# Chart render assertions for storefront-bff: the six upstream base URLs are
# mandatory boot config with no silent default, no *_MODE is ever rendered, the
# pod is hardened and has room to shut down, and the env cmd/api reads
# (internal/config/config.go) is the env the chart renders.
# Run: bash charts/storefront-bff/tests/render_test.sh   (needs helm)
set -euo pipefail

chart_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_dir="$(cd "$chart_dir/../.." && pwd)"
release=storefront-bff
# Dummy upstreams, never dialled: the chart refuses to render without all six.
upstreams=(productDiscoveryUrl:http://product-discovery.example.invalid
  deliveryPromiseUrl:http://delivery-promise.example.invalid
  shoppingCartUrl:http://shopping-cart.example.invalid
  checkoutUrl:http://checkout.example.invalid
  customerOrderingUrl:http://customer-ordering.example.invalid
  pricingUrl:http://pricing.example.invalid)
base=()
for u in "${upstreams[@]}"; do base+=(--set "config.${u%%:*}=${u#*:}"); done
# Every optional value set, so every env var cmd/api can read is rendered.
everything=(--set config.cookieSecure=true --set config.budgetQueryMs=3000
  --set config.budgetCommandMs=6000 --set config.budgetConfirmMs=20000
  --set config.shutdownDrainDelay=5s --set config.logLevel=debug
  --set autoscaling.api.enabled=true --set ingress.enabled=true --set gatewayApi.enabled=true)

fail() { echo "FAIL: $*" >&2; exit 1; }
command -v helm >/dev/null || fail "helm is not installed"

configmap="$(helm template "$release" "$chart_dir" "${base[@]}" --show-only templates/configmap.yaml)"
api_deployment="$(helm template "$release" "$chart_dir" "${base[@]}" --show-only templates/deployment.yaml)"
configmap_name="$(awk '/^  name:/ {print $2; exit}' <<<"$configmap")"
[[ -n "$configmap_name" ]] || fail "could not read the ConfigMap name"

# Every config value is rendered into the chart ConfigMap, and the api
# Deployment -- the only workload -- must mount it with envFrom, or a value
# that is only in the ConfigMap silently does nothing.
grep -A3 'envFrom:' <<<"$api_deployment" | grep -q 'configMapRef:' \
  || fail "api Deployment does not mount the ConfigMap via envFrom"
grep -A3 'envFrom:' <<<"$api_deployment" | grep -q "name: $configmap_name" \
  || fail "api Deployment envFrom does not reference ConfigMap $configmap_name"
grep -q 'checksum/config:' <<<"$api_deployment" || fail "the pods must roll when the ConfigMap changes (checksum/config)"

# A tuned config.* value reaches the ConfigMap the api mounts and the container env.
tuned="$(helm template "$release" "$chart_dir" "${base[@]}" --set config.budgetQueryMs=3000 \
  --set config.budgetCommandMs=6000 --set config.budgetConfirmMs=20000 --set config.cookieSecure=true \
  --show-only templates/configmap.yaml)"
grep -q 'BUDGET_QUERY_MS: "3000"' <<<"$tuned" || fail "config.budgetQueryMs is not rendered into the ConfigMap"
grep -q 'BUDGET_COMMAND_MS: "6000"' <<<"$tuned" || fail "config.budgetCommandMs is not rendered into the ConfigMap"
grep -q 'BUDGET_CONFIRM_MS: "20000"' <<<"$tuned" || fail "config.budgetConfirmMs is not rendered into the ConfigMap"
grep -q 'COOKIE_SECURE: "true"' <<<"$tuned" || fail "config.cookieSecure is not rendered into the ConfigMap"

# The budgets default to empty (not rendered; the binary defaults 2000/5000/15000).
if grep -q 'BUDGET_' <<<"$configmap"; then fail "the budgets must be unset with default values"; fi
# COOKIE_SECURE defaults to false in the chart (the kind cluster serves plain
# HTTP) even though the binary defaults to true.
grep -q 'COOKIE_SECURE: "false"' <<<"$configmap" || fail "COOKIE_SECURE must default to false in the chart"

# Chart vs binary: every env var cmd/api reads must be rendered into the
# Deployment when every optional value is set. The names come from the binary's
# own getenv("...") literals and from the UpstreamEnv map's *_URL literals
# (read through a variable), so a new variable in cmd/api fails this test until
# the chart renders it.
binary_env() {
  {
    grep -ohE '(Getenv|getenv)\("[A-Z][A-Z0-9_]*"' "$repo_dir"/cmd/api/*.go "$repo_dir"/internal/config/*.go --exclude='*_test.go' | sed -E 's/.*\("//; s/"$//'
    grep -ohE '"[A-Z][A-Z0-9_]*_(URL|MS)"' "$repo_dir"/internal/config/config.go | tr -d '"'
  } | sort -u
}
# Word splitting is safe: the names are [A-Z0-9_] only (bash 3.2 has no mapfile).
# shellcheck disable=SC2207
api_vars=($(binary_env))
(( ${#api_vars[@]} >= 10 )) || fail "found only ${#api_vars[@]} env names in cmd/api + internal/config; the scan is looking in the wrong place"
full_deployment="$(helm template "$release" "$chart_dir" "${base[@]}" "${everything[@]}" --show-only templates/deployment.yaml)"
for var in "${api_vars[@]}" PORT COOKIE_SECURE PRODUCT_DISCOVERY_URL DELIVERY_PROMISE_URL \
  SHOPPING_CART_URL CHECKOUT_URL CUSTOMER_ORDERING_URL PRICING_URL \
  BUDGET_QUERY_MS BUDGET_COMMAND_MS BUDGET_CONFIRM_MS; do
  # (OTEL_* / SERVICE_VERSION / ENVIRONMENT render from the otel block below.)
  grep -q "name: $var\$" <<<"$full_deployment" || fail "the api Deployment does not render $var, which cmd/api reads"
done

# Every key the ConfigMap renders is a variable cmd/api reads (no dead keys).
full_configmap="$(helm template "$release" "$chart_dir" "${base[@]}" "${everything[@]}" --show-only templates/configmap.yaml)"
api_known=" ${api_vars[*]} "
while read -r key; do
  [[ "$api_known" == *" $key "* ]] || fail "ConfigMap key $key is read by no binary"
done < <(awk '/^data:/ {d=1; next} d && /^  [A-Z]/ {sub(/:.*/, ""); sub(/^  /, ""); print}' <<<"$full_configmap")

# The mandatory configuration is rendered from values.
env_value() { # env_value <rendered> <NAME>
  grep -A1 "name: $2\$" <<<"$1" | grep -m1 'value:' | sed -E 's/.*value: "?//; s/"$//'
}
# Each upstream chart value lands on its env var:
for kv in 'PRODUCT_DISCOVERY_URL=http://product-discovery.example.invalid' \
  'DELIVERY_PROMISE_URL=http://delivery-promise.example.invalid' \
  'SHOPPING_CART_URL=http://shopping-cart.example.invalid' \
  'CHECKOUT_URL=http://checkout.example.invalid' \
  'CUSTOMER_ORDERING_URL=http://customer-ordering.example.invalid' \
  'PRICING_URL=http://pricing.example.invalid'; do
  [[ "$(env_value "$api_deployment" "${kv%%=*}")" == "${kv#*=}" ]] || fail "the api Deployment does not render $kv"
  grep -q "^  ${kv%%=*}: \"${kv#*=}\"" <<<"$configmap" || fail "the ConfigMap does not carry $kv"
done
[[ "$(env_value "$api_deployment" PORT)" == "8080" ]] || fail "PORT is not rendered from config.port"
# The observability contract renders from the otel block.
[[ "$(env_value "$full_deployment" "OTEL_EXPORTER_OTLP_ENDPOINT")" == "otel-collector.observability.svc.cluster.local:4317" ]] \
  || fail "OTEL_EXPORTER_OTLP_ENDPOINT is not rendered from otel.endpoint"
[[ "$(env_value "$full_deployment" "OTEL_SERVICE_NAME")" == "storefront-bff" ]] \
  || fail "OTEL_SERVICE_NAME is not rendered from otel.serviceName"
[[ -n "$(env_value "$full_deployment" "SERVICE_VERSION")" ]] || fail "SERVICE_VERSION must render (image tag or appVersion)"
[[ -n "$(env_value "$full_deployment" "ENVIRONMENT")" ]] || fail "ENVIRONMENT must render from .Values.environment"
[[ "$(env_value "$full_deployment" "LOG_LEVEL")" == "debug" ]] || fail "LOG_LEVEL is not rendered from config.logLevel"
[[ "$(env_value "$api_deployment" COOKIE_SECURE)" == "false" ]] || fail "COOKIE_SECURE must default to false in the chart"

# Pod hardening, probes, the image tag and shutdown room.
for want in 'runAsNonRoot: true' 'readOnlyRootFilesystem: true' 'allowPrivilegeEscalation: false' 'type: RuntimeDefault' 'path: /healthz' 'path: /readyz'; do
  grep -q -- "$want" <<<"$api_deployment" || fail "the api Deployment lacks '$want'"
done
grep -A2 'drop:' <<<"$api_deployment" | grep -q 'ALL' || fail "the api container must drop ALL capabilities"
grep -A3 'livenessProbe:' <<<"$api_deployment" | grep -q '/healthz' || fail "livenessProbe must be /healthz"
grep -A3 'readinessProbe:' <<<"$api_deployment" | grep -q '/readyz' || fail "readinessProbe must be /readyz"
grep -q 'image: "warehouse/storefront-bff:v0.1.0"' <<<"$api_deployment" \
  || fail "an empty image.tag must resolve to v<appVersion>, the tag the release job publishes"
grace="$(awk '/terminationGracePeriodSeconds:/ {print $2; exit}' <<<"$api_deployment")"
# config.shutdownDrainDelay 0 (unset) + srv.Shutdown 20s = 20s.
(( grace > 20 )) || fail "terminationGracePeriodSeconds is $grace; it must exceed the binary's shutdown bound (20s)"

# No *_MODE or in-memory/permissive wording anywhere in the render.
render_all="$(helm template "$release" "$chart_dir" "${base[@]}" "${everything[@]}")"
if grep -qE '(^|[^A-Z_])[A-Z]+_MODE\b' <<<"$render_all"; then fail "the render carries a *_MODE variable, which this service does not have"; fi
if grep -qiE 'in-memory|silently runs|permissive' <<<"$render_all"$'\n'"$(cat "$chart_dir/templates/NOTES.txt")"; then
  fail "the render or NOTES.txt mentions an in-memory / permissive mode"
fi
# No database and no Kafka: neither a Secret nor a broker variable ever renders.
if grep -q 'kind: Secret' <<<"$render_all"; then fail "this service has no database: no Secret may render"; fi
if grep -qE 'KAFKA_BROKERS|DATABASE_URL' <<<"$render_all"; then fail "this service reads no KAFKA_BROKERS / DATABASE_URL"; fi

# Only the api component exists: one Deployment, one Service, no MCP or frontend.
for stray in mcp frontend projector reports analytics; do
  if grep -qE "app.kubernetes.io/component: .*$stray" <<<"$render_all"; then fail "the chart must not render a $stray component"; fi
done
[[ "$(grep -c '^kind: Deployment' <<<"$render_all")" == 1 ]] || fail "exactly one Deployment (the api) must render"
[[ "$(grep -c '^kind: Service$' <<<"$render_all")" == 1 ]] || fail "exactly one Service (the api) must render"
for off in HorizontalPodAutoscaler Ingress HTTPRoute; do
  if grep -q "^kind: $off" <<<"$(helm template "$release" "$chart_dir" "${base[@]}")"; then fail "$off must be off by default"; fi
  grep -q "^kind: $off" <<<"$render_all" || fail "$off must render when enabled"
done

# --- refusals: the mandatory values name themselves ---------------------------------
expect_refusal() { # expect_refusal <why> <needle> <helm args...>
  local why="$1" needle="$2" err; shift 2
  if err="$(helm template "$release" "$chart_dir" "$@" 2>&1)"; then
    fail "the chart must refuse to render: $why"
  fi
  grep -qF -- "$needle" <<<"$err" || fail "the refusal for '$why' does not name '$needle':
$err"
}
# args that satisfy everything except the one upstream a case removes.
mandatory_except() { # mandatory_except <upstream key>
  local skip="$1" out=()
  for u in "${upstreams[@]}"; do
    [[ "${u%%:*}" == "$skip" ]] || out+=(--set "config.${u%%:*}=${u#*:}")
  done
  printf '%s\n' "${out[@]}"
}
refusal_without() { # refusal_without <upstream key> <needle>
  local args=()
  while IFS= read -r line; do args+=("$line"); done < <(mandatory_except "$1")
  expect_refusal "no $1" "$2" "${args[@]}"
}
for u in "${upstreams[@]}"; do refusal_without "${u%%:*}" "config.${u%%:*}"; done
expect_refusal "a *_MODE through extraEnv" "_MODE" "${base[@]}" --set 'extraEnv[0].name=CACHE_MODE' --set 'extraEnv[0].value=permissive'
expect_refusal "a *_MODE through extraEnv (off)" "_MODE" "${base[@]}" --set 'extraEnv[0].name=UPSTREAM_MODE' --set 'extraEnv[0].value=off'
expect_refusal "a chart-owned variable through extraEnv" "PORT" "${base[@]}" --set 'extraEnv[0].name=PORT' --set 'extraEnv[0].value=9090'
expect_refusal "a chart-owned upstream through extraEnv" "PRICING_URL" "${base[@]}" --set 'extraEnv[0].name=PRICING_URL' --set 'extraEnv[0].value=x'
expect_refusal "a grace period under the shutdown bound" "terminationGracePeriodSeconds" "${base[@]}" --set terminationGracePeriodSeconds=19
expect_refusal "a grace period equal to the shutdown bound" "must exceed 20" "${base[@]}" --set terminationGracePeriodSeconds=20
expect_refusal "no grace period" "terminationGracePeriodSeconds" "${base[@]}" --set terminationGracePeriodSeconds=0
helm template "$release" "$chart_dir" "${base[@]}" --set terminationGracePeriodSeconds=21 >/dev/null \
  || fail "a 21s grace period (just above the 20s bound) must still render"
# A drain delay raises the bar by the same amount.
expect_refusal "a 30s drain delay with the default grace period" "must exceed 50" "${base[@]}" --set config.shutdownDrainDelay=30s
helm template "$release" "$chart_dir" "${base[@]}" --set config.shutdownDrainDelay=30s --set terminationGracePeriodSeconds=51 >/dev/null \
  || fail "a 30s drain delay with a 51s grace period must render"
expect_refusal "a drain delay the chart cannot size" "config.shutdownDrainDelay" "${base[@]}" --set config.shutdownDrainDelay=soon
expect_refusal "a port off the container port" "config.port" "${base[@]}" --set config.port=9090
expect_refusal "a non-numeric budget" "config.budgetQueryMs" "${base[@]}" --set config.budgetQueryMs=soon
expect_refusal "a zero budget" "config.budgetCommandMs" "${base[@]}" --set config.budgetCommandMs=0

# The full render (ct lint's all-components values file) carries every optional
# template together, still exactly one Deployment and one Service.
together="$(helm template "$release" "$chart_dir" -f "$chart_dir/ci/all-components-values.yaml")"
grep -q 'app.kubernetes.io/component: api$' <<<"$together" || fail "the full render lacks the api component"
[[ "$(grep -c '^kind: Deployment' <<<"$together")" == 1 ]] || fail "the full render must carry exactly the api Deployment"
[[ "$(grep -c '^kind: Service$' <<<"$together")" == 1 ]] || fail "the full render must carry exactly the api Service"

echo "PASS: storefront-bff chart render assertions"
