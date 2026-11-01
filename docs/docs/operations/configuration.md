---
id: configuration
title: Configuration
sidebar_label: Configuration
sidebar_position: 1
---

# Configuration

`storefront-bff` has **one binary**, `cmd/api`, and it is configured only through
environment variables. There is no config file, no flag and no secret-manager
integration. REST is unauthenticated fleet-wide, so there is no token or key to
configure; the one cookie the service sets is state, not authentication.

The service has no database, no Kafka and no in-memory mode
([ADR 0002](/docs/adr/0002-storefront-bff-assumptions)): its whole runtime is
the six upstream REST services it composes. The six base URLs are therefore
mandatory, and a missing or malformed one is a boot error that names it, raised
before the listener binds.

Two packages read the environment: `internal/config` (listen address, upstream
URLs, budgets, cookie) and `cmd/api` (log level and telemetry; the telemetry
adapter reads `ENVIRONMENT` itself). Test-only variables do not exist.

## `cmd/api`

| Variable | Default | Required? | Meaning | Source |
| --- | --- | --- | --- | --- |
| `PORT` | `8080` | no | TCP port of the HTTP listener (`":" + PORT`, all interfaces). Serves the storefront edge, `/healthz` and `/readyz`. The Helm chart keeps it equal to `service.targetPort`. | `internal/config/config.go` (`Load`) |
| `PRODUCT_DISCOVERY_URL` | none | **yes** | Base URL of the product-discovery service (catalogue and search reads). Must be an absolute `http(s)` URL; a path prefix is kept and the call path is appended. | `internal/config/config.go` (`UpstreamEnv`), `internal/adapters/outbound/rest/client.go` (`New`) |
| `DELIVERY_PROMISE_URL` | none | **yes** | Base URL of delivery-promise (delivery-date quotes). | same |
| `SHOPPING_CART_URL` | none | **yes** | Base URL of shopping-cart (cart reads and commands). | same |
| `CHECKOUT_URL` | none | **yes** | Base URL of checkout (checkout sessions, confirm, payment). | same |
| `CUSTOMER_ORDERING_URL` | none | **yes** | Base URL of customer-ordering (shoppers, addresses, orders). | same |
| `PRICING_URL` | none | **yes** | Base URL of pricing (price reads). | same |
| `BUDGET_QUERY_MS` | `2000` | no | Whole-request time budget of a *query* route, in whole milliseconds. Non-integer or `<= 0` is a boot error `BUDGET_QUERY_MS must be a positive number of milliseconds`. Empty = default. | `internal/config/config.go`, `internal/domain/composition/budget.go` |
| `BUDGET_COMMAND_MS` | `5000` | no | Budget of a *command* route (one write through one upstream). Same validation. | same |
| `BUDGET_CONFIRM_MS` | `15000` | no | Budget of the checkout *confirm* step (authorize payment and place the order). Same validation. | same |
| `COOKIE_SECURE` | `true` | no | `Secure` attribute of the state cookie. Parsed with `strconv.ParseBool` (`true`, `false`, `1`, `0`, ...); anything else is a boot error `COOKIE_SECURE must be a boolean`. Set `false` only for plain-HTTP local runs; the Helm chart renders `false` by default because the kind cluster serves plain HTTP. | `internal/config/config.go`, `internal/adapters/inbound/http/server.go` (`cookie`) |
| `LOG_LEVEL` | `info` | no | `debug`, `info`, `warn` (or `warning`), `error`, case-insensitive. Any other value silently falls back to `info`. Logs are JSON on stdout. | `cmd/api/main.go` (`newLogger`) |
| `OTEL_SERVICE_NAME` | `storefront-bff` | no | `service.name` of every span and metric, also the server span and request-duration metric name prefix. | `cmd/api/telemetry.go` (`loadTelemetryEnv`) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | no | OTLP/gRPC Collector endpoint, bare `host:port` or a full URL (a bare form becomes `http://host:port`; the variable is rewritten in place so the SDK does not log a parse error). Export is non-blocking: an absent Collector drops telemetry and never blocks boot or requests. | `cmd/api/telemetry.go`, `internal/adapters/outbound/telemetry/telemetry.go` (`normalizeEndpoint`) |
| `SERVICE_VERSION` | `dev` | no | `service.version` resource attribute. The chart sets it to the image tag. | `cmd/api/telemetry.go` |
| `ENVIRONMENT` | `local` | no | `deployment.environment.name` resource attribute. The chart default is `production`. | `internal/adapters/outbound/telemetry/telemetry.go` (`environment`) |

`OTEL_RESOURCE_ATTRIBUTES` is honoured by the OpenTelemetry SDK's default
resource (`resource.Default()`), but the code does not read it.

All configuration problems from `config.Load` are reported together, for example
with an empty environment:

```text
invalid configuration: PRODUCT_DISCOVERY_URL is required; DELIVERY_PROMISE_URL is required; SHOPPING_CART_URL is required; CHECKOUT_URL is required; CUSTOMER_ORDERING_URL is required; PRICING_URL is required
```

This line is assembled from the format string in `config.Load`
(`invalid configuration: %s`, problems joined with `; `), not captured from a
run. A URL that is set but not an absolute `http(s)` URL fails one step later,
in `rest.New`: `base URL of <service> is missing or not an absolute http(s) URL`.

## Readiness and health

`GET /healthz` and `GET /readyz` are static `200 {"status":"ok"}` handlers
(`internal/adapters/inbound/http/server.go`). Readiness does **not** probe the
upstreams: a slow or dead upstream shows up as request errors (`503`/`504`), not
as a pod restart. The configuration check is the boot itself: a pod with a
missing URL never becomes ready because the process exits.

## Compiled-in settings (not configurable)

These are constants in the source; changing them needs a code change.

| Setting | Value | Source |
| --- | --- | --- |
| HTTP server timeouts | `ReadHeaderTimeout` 5 s, `ReadTimeout` 15 s, `IdleTimeout` 60 s | `cmd/api/main.go` (`run`) |
| Graceful shutdown | SIGINT/SIGTERM closes the listener; in-flight requests get 20 s | `cmd/api/main.go` (`run`) |
| Telemetry flush on shutdown | 5 s, a failed flush is a WARN, never a non-zero exit | `cmd/api/telemetry.go` (`telemetryFlushTimeout`) |
| Metric export interval | 15 s | `internal/adapters/outbound/telemetry/telemetry.go` (`metricInterval`) |
| Request body limit | 1 MiB (`413` above it) | `internal/adapters/inbound/http/middleware.go` (`maxBodyBytes`) |
| Upstream reply cap | 4 MiB per reply | `internal/adapters/outbound/rest/client.go` (`maxReplyBytes`) |
| Upstream HTTP client | no redirects followed, no client-level timeout (the route budget is the only clock), 32 idle connections per host, idle timeout 90 s, `HTTP_PROXY` family honoured | `internal/adapters/outbound/rest/client.go` (`New`) |
| State cookie | name `composition.CookieName`, `Path=/api/storefront`, 30 days, `HttpOnly`, `SameSite=Lax` | `internal/adapters/inbound/http/server.go` (`cookie`) |
| Route prefix | `/api/storefront` is optional on the wire and stripped before routing | `internal/adapters/inbound/http/server.go` (`stripBase`) |

## What is deliberately absent

- **No database or Kafka variables.** The binary reads no `DATABASE_URL` and no
  `KAFKA_BROKERS`; the Kafka writer and CloudEvents packages in the tree are
  scaffold leftovers that `cmd/api` never wires (see
  [Architecture](/docs/overview/architecture)).
- **No auth variables.** REST is unauthenticated fleet-wide.
- **No `*_MODE` variable.** There are no modes; the chart refuses any `*_MODE`
  entry in `extraEnv`.
- **`SHUTDOWN_DRAIN_DELAY` is not read by the binary.** The chart's
  `config.shutdownDrainDelay` only sizes the pod's grace period.
- **Chart values.** `charts/storefront-bff` maps these variables onto the pod
  (`config.*`, `otel.*`, `environment`); see the [Runbook](/docs/operations/runbook).
