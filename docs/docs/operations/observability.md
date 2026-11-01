---
id: observability
title: Observability
sidebar_label: Observability
sidebar_position: 3
description: "Metrics, traces, logs, exporter variables and the dashboards and alerts that exist for the Storefront BFF."
---

# Observability

`cmd/api` follows the fleet observability standard
([ADR 0003](/docs/adr/0003-fleet-observability-standard)): JSON logs on stdout
correlated with traces, OTLP/gRPC traces and metrics to a Collector. The
instruments are the ones the libraries register; the BFF defines **no custom
metric of its own** (no business counters, no queue gauges, there is nothing to
queue). Source: `cmd/api/telemetry.go`,
`internal/adapters/outbound/telemetry/telemetry.go`,
`internal/adapters/inbound/http/server.go`.

## Exporter configuration

| Variable | Default | Effect |
| --- | --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTLP/gRPC for traces and metrics; bare `host:port` or URL |
| `OTEL_SERVICE_NAME` | `storefront-bff` | `service.name`; also the server-span and metric scope name |
| `SERVICE_VERSION` | `dev` | `service.version` |
| `ENVIRONMENT` | `local` | `deployment.environment.name` |
| `LOG_LEVEL` | `info` | slog level (see [Configuration](/docs/operations/configuration)) |

Export is non-blocking and lazy: an absent Collector drops telemetry, never
blocks boot or a request. Metrics are pushed every 15 s (`PeriodicReader`);
traces go through the SDK batch processor. Exporter errors are logged at DEBUG
only (`opentelemetry error`), so a silent pipeline is diagnosed with
`LOG_LEVEL=debug`. The W3C `traceparent` and `baggage` propagators are installed.
The chart default endpoint is `otel-collector.observability.svc.cluster.local:4317`
(`otel.enabled: true`); when `otel.enabled` is false the four OTel variables are
not rendered and the binary uses its own defaults above.

## Metrics

| Instrument | Type | Unit | Attributes | Source |
| --- | --- | --- | --- | --- |
| `http.server.request.duration` | histogram (float64; buckets 5 ms … 10 s) | `s` | `http.method`, `http.scheme`, `http.route` (chi route pattern, absent for unmatched routes) | `otelchimetric.NewServerRequestDuration` in `server.go` |
| Go runtime metrics | gauges and counters | per instrument | none | `go.opentelemetry.io/contrib/instrumentation/runtime` (`runtime.Start`): goroutines, heap, GC |

Prometheus names the first one `http_server_request_duration_seconds_*` with the
resource label `service_name` (this is what the dashboard queries). The runtime
metric names depend on the contrib version (`go_goroutine_count` or
`runtime_go_goroutines`); the dashboard accepts both.

The edge keeps no other metric. In particular there is no per-upstream metric:
use the client spans below, or the upstream's own telemetry, to see which
dependency is slow.

## Traces

| Span | Created by | Name | Notes |
| --- | --- | --- | --- |
| Server | `otelchi.Middleware(serviceName, WithChiRoutes(r))` | route pattern (`/carts/{id}/lines`) | one per request, the access-log record carries its ids |
| Client | `otelhttp.NewTransport` in `internal/adapters/outbound/rest/client.go` | HTTP client span per upstream call | child of the server span; carries `traceparent` to the upstream so one trace crosses the BFF hop |

A composed route (`GET /orders/{id}`, `POST /products/{id}:delivery-dates`) shows
up as one server span with two or three client spans; a failed optional leg
(tracking, totals, charge schedule) is visible only as that child span's error,
not in the reply. Bodies and query strings are never put on spans or logs.

## Logs

JSON on stdout through `slog` (`time`, `level`, `msg` plus fields). The handler is
wrapped so a record written with a span context carries `trace_id` and
`span_id`.

| Message | Level | Fields | When |
| --- | --- | --- | --- |
| `http request` | INFO (2xx/3xx), WARN (4xx), ERROR (5xx) | `method`, `route`, `status`, `duration_ms`, `bytes`, `request_id` (+ `trace_id`, `span_id`) | once per request except `/healthz` and `/readyz`; `route` is the pattern, never the raw path (ids) |
| `storefront-bff listening` | INFO | `addr` | listener bound |
| `telemetry configured` | INFO | `service_name`, `otlp_endpoint` | after OTel setup |
| `handler panic` | ERROR | `method`, `path` | a handler panicked; the panic value is deliberately not logged; the client gets a 500 problem |
| `telemetry flush failed on shutdown` | WARN | `error` | flush exceeded its 5 s |
| `storefront-bff stopped` | ERROR | `error` | boot failure or serve error; exit code 1 |

Request and response bodies, cookies, e-mails and addresses are never logged
(fitness tests `bff_no_body_logging_fitness_test.go` and
`bff_no_card_data_fitness_test.go`). An unmatched request is logged with route
`unmatched`, so a scanner cannot inject its path into the log.

## Dashboards and alerts that exist

Seen in `warehouse-infra` (`origin/develop`):

- Dashboard `terraform/dashboards/contexts/storefront-bff.json`
  (`/d/warehouse-storefront-bff`): Kong request rate, 5xx ratio and p95 request
  versus upstream latency for the `/api/storefront` route; per-route request
  rate and p95 from `http_server_request_duration_seconds` with
  `service_name=~"storefront-bff.*"`; goroutines and memory; Loki panels over
  `{app="storefront-bff"}` (live stream, volume by level, ERROR and WARN only).
- Alert group `warehouse-storefront-bff`: `WarehouseKong5xxRatioHigh` (more than
  5 % 5xx over 5 minutes, warning) and `WarehousePodRestarting` (two or more
  container restarts in 15 minutes, warning).
- Alert group `warehouse-storefront-bff-slo`: `WarehouseApiAvailabilityBurnFast`
  and `...BurnSlow` (99 % one-day availability SLO on Kong 5xx),
  `WarehouseApiLatencyBurnFast` and `...BurnSlow` (95 % of requests under about
  400–500 ms upstream latency), each needing at least 30 requests.

These are Kong-side and Kubernetes-side signals; there is no alert on the
`http.server.request.duration` metric itself and none on an individual upstream.
The alerts' `runbook_url` points at the pre-move `docs/operations/runbook.md`
path on `develop` (a redirect target to fix in warehouse-infra, out of scope
here); the current runbook is [Runbook](/docs/operations/runbook).
