---
id: runbook
title: Operations runbook
sidebar_label: Runbook
sidebar_position: 2
description: "Deploying, probing, rolling back and routinely operating the Storefront BFF."
---

# Storefront BFF — Operations Runbook

Storefront BFF is the shopper-facing storefront edge (`/api/storefront`): a
stateless backend-for-frontend that composes six upstream services over REST
and carries cart/session state in the shopper's cookie. One deployable:
`cmd/api`. Runtime: the six upstreams only — no database, no Kafka, no
in-memory or permissive mode, so a pod missing an upstream URL refuses to boot
and never pretends to be healthy.

What this service does **not** have, so the corresponding runbook chapters are
short: no migrations, no Kafka topics or consumer groups, no outbox relay, no
DLQ, no sweepers or schedulers. Every variable is in
[Configuration](/docs/operations/configuration); metrics, logs and traces are in
[Observability](/docs/operations/observability); symptoms are in
[Troubleshooting](/docs/operations/troubleshooting).

## Deploy

The chart is `charts/storefront-bff` (packaged by the `release` job as
`oci://ghcr.io/iqvo/storefront-bff:<version>`; the image is
`ghcr.io/iqvo/storefront-bff:v<version>`). The kind cluster deploys it through
the warehouse-infra ArgoCD app with the local image `warehouse/storefront-bff`.
One Deployment (`app.kubernetes.io/component: api`), one ClusterIP Service
(port 80 to `8080`), optional Ingress and Gateway API route, optional HPA, all
off by default.

Mandatory values — the chart refuses to render without them:

| Value | Env var | Upstream dependency |
| --- | --- | --- |
| `config.productDiscoveryUrl` | `PRODUCT_DISCOVERY_URL` | product-discovery (catalogue reads) |
| `config.deliveryPromiseUrl` | `DELIVERY_PROMISE_URL` | delivery-promise (delivery estimates) |
| `config.shoppingCartUrl` | `SHOPPING_CART_URL` | shopping-cart (cart commands/reads) |
| `config.checkoutUrl` | `CHECKOUT_URL` | checkout (order placement) |
| `config.customerOrderingUrl` | `CUSTOMER_ORDERING_URL` | customer-ordering (order history) |
| `config.pricingUrl` | `PRICING_URL` | pricing (prices and quotes) |

Useful optionals: `config.port` (`PORT`, default `8080`; must equal
`service.targetPort`), `config.cookieSecure` (`COOKIE_SECURE`; binary default
`true`, chart default `false` because the kind cluster serves plain HTTP — set
`true` behind TLS), `config.budgetQueryMs` / `config.budgetCommandMs` /
`config.budgetConfirmMs` (`BUDGET_QUERY_MS` / `BUDGET_COMMAND_MS` /
`BUDGET_CONFIRM_MS`, binary defaults 2000 / 5000 / 15000 ms), `config.logLevel`
(`LOG_LEVEL`), the `otel.*` block and `environment`,
`config.shutdownDrainDelay` (NOT an env var — endpoint-removal headroom that
only sizes the pod grace period). `terminationGracePeriodSeconds` (default 30)
must exceed drain + 20s (the `srv.Shutdown` timeout in `cmd/api/main.go`); the
chart refuses anything shorter. `extraEnv` may not set a `*_MODE` variable or any
variable the chart renders itself.

The pod runs as uid 1000 with a read-only root filesystem, no privilege
escalation and all capabilities dropped; `/tmp` is a 16 Mi `emptyDir`. Default
resources: requests 100m / 128Mi, limits 500m / 256Mi.

Deploy / upgrade:

```sh
helm upgrade --install storefront-bff charts/storefront-bff \
  --namespace storefront-bff --create-namespace \
  --set config.productDiscoveryUrl=http://product-discovery.product-discovery.svc.cluster.local \
  --set config.deliveryPromiseUrl=http://delivery-promise.delivery-promise.svc.cluster.local \
  --set config.shoppingCartUrl=http://shopping-cart.shopping-cart.svc.cluster.local \
  --set config.checkoutUrl=http://checkout.checkout.svc.cluster.local \
  --set config.customerOrderingUrl=http://customer-ordering.customer-ordering.svc.cluster.local \
  --set config.pricingUrl=http://pricing.pricing.svc.cluster.local
kubectl -n storefront-bff rollout status deployment/storefront-bff
```

The pod template carries a `checksum/config` annotation of the ConfigMap, so a
changed `config.*` value rolls the pods.

Release pipeline (push to `main`): `docker-publish` builds and pushes
`ghcr.io/iqvo/storefront-bff:latest` and `:<short sha>` (cosign-signed, SPDX SBOM
attested), then `release` tags `v<X.Y.Z>` (patch auto-increment from the latest
tag, `v0.1.0` first), packages the chart at that version and publishes a GitHub
release. See [Before the first release](/docs/operations/before-first-release).

## Rollback

```sh
helm history storefront-bff -n storefront-bff
helm rollback storefront-bff <revision> -n storefront-bff
kubectl -n storefront-bff rollout status deployment/storefront-bff
```

The service is stateless (no schema, no offsets, no local files), so a
rollback is always safe; in-flight shopper requests drain inside the grace
period. Shoppers keep their `sf_state` cookie across any rollout.

## Health, probes and what readiness waits for

- `GET /healthz` — liveness probe (initial delay 5 s, period 10 s).
- `GET /readyz` — readiness probe (initial delay 3 s, period 5 s). Both are
  static 200s from the router (`internal/adapters/inbound/http/server.go`);
  **readiness waits for nothing**: no upstream probe, no database, no broker.
  A slow upstream surfaces as request latency (`503`/`504`), not as a pod
  restart or a removal from the Service.

```sh
kubectl -n storefront-bff port-forward svc/storefront-bff 8080:80
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

Neither probe path is written to the access log (`requestlog` skips them).

A pod that exits at boot naming `... is required` is missing an upstream URL
value; the chart is meant to refuse that render first — if the pod still sees
it, the release bypassed the chart's values.

## Upstream dependencies

| Upstream | Used for | Budget (default) |
| --- | --- | --- |
| product-discovery | catalogue/search reads | `BUDGET_QUERY_MS` (2000) |
| delivery-promise | delivery estimates | `BUDGET_QUERY_MS` (2000) |
| pricing | delivery charge schedule | `BUDGET_QUERY_MS` (2000) |
| shopping-cart | cart reads and commands | query/command budget |
| checkout | order placement (idempotency-keyed) | `BUDGET_CONFIRM_MS` (15000) |
| customer-ordering | shoppers, orders | `BUDGET_QUERY_MS` (2000) / `BUDGET_COMMAND_MS` (5000) |

The budget belongs to the **route**, not the upstream: the whole composition
(for example order + tracking + checkout totals) shares one clock and an expired
one answers `504` and is never retried. Horizontal scaling is safe: state rides
in the cookie and every write is idempotency-keyed by the edge. The HPA
(`autoscaling.api`, 1–4 replicas at 70 % CPU) is off by default and has not been
load-tested.

## Migrations, Kafka, outbox, DLQ, sweepers

None. There is no database to migrate, no topic or consumer group to create or
reset, no outbox relay to watch, no dead-letter queue to drain and no periodic
sweeper. The Kafka writer and CloudEvents packages in the tree are unused by
`cmd/api` ([Architecture](/docs/overview/architecture)).

## Routine procedures

**Point the BFF at a new upstream address.** Change the `config.*Url` value and
`helm upgrade`; the checksum annotation rolls the pods. No data to move.

**Change a time budget.** Set `config.budgetQueryMs`, `budgetCommandMs` or
`budgetConfirmMs` (whole milliseconds, > 0). A bad value is a boot error
(`BUDGET_QUERY_MS must be a positive number of milliseconds`), so a typo shows
as a failing rollout, not as silent defaults.

**Turn on debug logs temporarily.** `helm upgrade --reuse-values --set config.logLevel=debug`; revert after. Logs never carry request or response
bodies, so debug level is safe for PII, but it is noisy.

**Serve behind TLS.** Set `config.cookieSecure=true`. Otherwise browsers refuse
to send back a `Secure` cookie over plain HTTP and every shopper looks
anonymous ([Troubleshooting](/docs/operations/troubleshooting)).

**Expose it.** `ingress.enabled` or `gatewayApi.enabled`. The binary strips an
optional `/api/storefront` prefix, so a route that rewrites the path or one that
does not both work. Through the platform Kong gateway the public path is
`/api/storefront`.

**Drain a node / restart.** A normal `kubectl rollout restart` is safe: on SIGTERM
the listener closes, in-flight requests get 20 s and telemetry flushes for up to
5 s.

## Dashboards and telemetry

Logs are stdout JSON collected by the cluster log pipeline. Dashboards and
alerts for the fleet live in the warehouse-infra repository (Grafana
dashboards under its observability stack, backed by Prometheus); look for the
storefront-bff service panels there. What the service emits is listed in
[Observability](/docs/operations/observability).
