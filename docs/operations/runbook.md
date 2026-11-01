# Storefront BFF — Operations Runbook

Storefront BFF is the shopper-facing storefront edge (`/api/storefront`): a
stateless backend-for-frontend that composes six upstream services over REST
and carries cart/session state in the shopper's cookie. One deployable:
`cmd/api`. Runtime: the six upstreams only — no database, no Kafka, no
in-memory or permissive mode, so a pod missing an upstream URL refuses to boot
and never pretends to be healthy.

## Deploy

The chart is `charts/storefront-bff` (packaged by the `release` job as
`oci://ghcr.io/iqvo/storefront-bff:<version>`; the image is
`ghcr.io/iqvo/storefront-bff:v<version>`). The kind cluster deploys it through
the warehouse-infra ArgoCD app with the local image `warehouse/storefront-bff`.

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
`BUDGET_CONFIRM_MS`, binary defaults 2000 / 5000 / 15000 ms),
`config.shutdownDrainDelay` (NOT an env var — endpoint-removal headroom that
only sizes the pod grace period). `terminationGracePeriodSeconds` must exceed
drain + 20s (the `srv.Shutdown` timeout in `cmd/api/main.go`); the chart
refuses anything shorter.

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

## Rollback

```sh
helm history storefront-bff -n storefront-bff
helm rollback storefront-bff <revision> -n storefront-bff
kubectl -n storefront-bff rollout status deployment/storefront-bff
```

The service is stateless (no schema, no offsets, no local files), so a
rollback is always safe; in-flight shopper requests drain inside the grace
period.

## Health

- `GET /healthz` — liveness (liveness probe).
- `GET /readyz` — readiness (readiness probe). Both are static 200s from the
  router (`internal/adapters/inbound/http/server.go`); upstream reachability is
  NOT probed — a slow upstream surfaces as request latency, not as a pod
  restart.

```sh
kubectl -n storefront-bff port-forward svc/storefront-bff 8080:80
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

A pod that exits at boot naming `... is required` is missing an upstream URL
value; the chart is meant to refuse that render first — if the pod still sees
it, the release bypassed the chart's values.

## Upstream dependencies

| Upstream | Used for | Budget (default) |
| --- | --- | --- |
| product-discovery | catalogue/search reads | `BUDGET_QUERY_MS` (2000) |
| delivery-promise | delivery estimates | `BUDGET_QUERY_MS` (2000) |
| pricing | prices and quotes | `BUDGET_QUERY_MS` (2000) |
| shopping-cart | cart reads and commands | query/command budget |
| checkout | order placement (idempotency-keyed) | `BUDGET_CONFIRM_MS` (15000) |
| customer-ordering | order history reads | `BUDGET_QUERY_MS` (2000) |

Commands use `BUDGET_COMMAND_MS` (default 5000). The confirm budget covers the
shopper's final commit. Horizontal scaling is safe: state rides in the cookie
and every write is idempotency-keyed by the edge.

## Dashboards and telemetry

Logs are stdout JSON collected by the cluster log pipeline. Dashboards and
alerts for the fleet live in the warehouse-infra repository (Grafana
dashboards under its observability stack, backed by Prometheus); look for the
storefront-bff service panels there.
