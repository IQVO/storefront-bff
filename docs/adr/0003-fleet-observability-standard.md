# ADR 0003: Adopt the fleet observability standard (logs, metrics, traces)

## Status

Accepted (2026-10-29).

## Context

The BFF logged JSON with `log/slog` and nothing else: no trace or span id on a
log line, no request id, no request log of the fleet's shape, no metrics, no
traces, and no `traceparent` carried into the six upstream services it
composes — so a shopper's action stopped being traceable at the edge. The
storefront services (checkout et al.) and the WMS/WES fleet already share one
pipeline and one convention (inventory-storage ADR 0016, wes-work-planning
ADR 0013; checkout's ADR 0006 is the storefront instance of it).

## Decision

Adopt the standard, in the same shape as checkout's:

- `internal/adapters/outbound/telemetry` (Setup: OTLP traces + metrics over
  gRPC, W3C TraceContext + Baggage propagator, Go runtime metrics,
  non-blocking with no Collector), `WithTraceContext` on the JSON handler,
  `LOG_LEVEL`.
- The chi router runs the standard order: RequestID, otelchi (a server span
  named after the route pattern), the request-duration histogram,
  `requestlog.RequestLogger`, then the panic recovery. `recoverPanic` stays
  instead of chi's `Recoverer`: it preserves the problem+json 500 contract
  the contract tests pin. `rejectNUL` stays after these.
- The REST client's transport is wrapped in `otelhttp`, so every upstream call
  starts a client span parented to the edge request's server span and carries
  its `traceparent`: one trace across the BFF hop. Bodies are still never
  read or logged (`TestBFFNeverLogsBodies` unchanged).
- Chart: `otel.{enabled,endpoint,serviceName}`, `environment`,
  `config.logLevel`, rendered as `OTEL_EXPORTER_OTLP_ENDPOINT`,
  `OTEL_SERVICE_NAME`, `SERVICE_VERSION`, `ENVIRONMENT`, `LOG_LEVEL`;
  `extraEnv` may not set these names; render tests assert each.
- `normalizeEndpoint` builds the plaintext prefix through `net/url` instead of
  an `http://` string literal: this repo's no-in-org-network-route sensor
  flags hard-coded URL literals, and the only URL this package may name is
  the one the environment handed it. This is the single deliberate
  difference from the reference telemetry package.

No Tier-2 business counter: the BFF is an edge with no domain outcomes (the
standard's Tier 2 applies to bounded contexts).

## Consequences

- `/healthz` and `/readyz` are unchanged; no auth middleware is ever added.
- A Collector that is down degrades to dropped telemetry, never to a refused
  boot or a hung request.
