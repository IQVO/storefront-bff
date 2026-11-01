# storefront-bff

[![CI](https://github.com/IQVO/storefront-bff/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/IQVO/storefront-bff/actions/workflows/ci.yml?query=branch%3Adevelop) [![coverage gate](https://img.shields.io/badge/coverage%20gate-90%25-brightgreen)](https://github.com/IQVO/storefront-bff/blob/develop/Makefile) [![Go](https://img.shields.io/badge/go-1.27.3-00ADD8?logo=go&logoColor=white)](https://github.com/IQVO/storefront-bff/blob/develop/go.mod) [![release](https://img.shields.io/github/v/release/IQVO/storefront-bff?sort=semver)](https://github.com/IQVO/storefront-bff/releases) [![docs](https://img.shields.io/badge/docs-iqvo.github.io-blue)](https://iqvo.github.io/storefront-bff/) [![license](https://img.shields.io/badge/license-MIT-blue)](https://github.com/IQVO/storefront-bff/blob/develop/LICENSE)

Storefront (e-commerce) domain of the retail-network organisation, bounded
context **Storefront BFF** (Generic, storefront tier).

Go backend-for-frontend: the only storefront edge (`/api/storefront`).
Composes the six storefront contexts and the network OHS routes, mints
Idempotency-Keys per user action, keeps cart/shopper ids in a cookie (state,
not authentication), passes saved addresses through without storing or logging
them. No database. Never logs request/response bodies. This repository is part
of the storefront domain: it consumes the retail network's Open Host Service
routes and its published `retailnetwork.*` events; it never calls an in-org or
restricted network route (an executable architecture rule, not a convention).

## Disclaimer

This project is a domain-driven design study. It is not associated with, endorsed by or affiliated with any company, and it uses no company's code, data, logos or trademarks. All names, figures and examples are illustrative or synthetic.

---

## Documentation

This repo has no docs site of its own. The context's design lives in
[warehouse-docs](https://iqvo.github.io/warehouse-docs/storefront/) —
`storefront/model/model.yaml` there is the single source of truth.

Page map:

| Area | Pages |
|---|---|
| Storefront domain | [Storefront hub](https://iqvo.github.io/warehouse-docs/storefront/) |
| This context | [Storefront BFF API](https://iqvo.github.io/warehouse-docs/storefront/api-reference/rest/storefront-bff/storefront-bff-api) — the edge contract, copied from this repo's `apis/openapi.yaml` |
| Strategic design | [Storefront strategic design](https://iqvo.github.io/warehouse-docs/storefront/strategic-design/) — subdomain classification, context map, CloudEvents event standard, model gap register |
| Decisions | [Storefront ADR index](https://iqvo.github.io/warehouse-docs/storefront/adr/) |
| In this repo | [`docs/adr/`](docs/adr/) (the repo's own ADRs), [`docs/operations/runbook.md`](docs/operations/runbook.md), [`docs/before-first-release.md`](docs/before-first-release.md) |

## Architecture

Hexagonal / ports & adapters, one stateless deployable. Dependencies point
inward only:

```
cmd/api/                     the storefront edge: chi server on /api/storefront (prefix stripped), composition root
internal/
  domain/composition/        pure Go: the upstream allowlist, call budgets, idempotency-key minting,
                             the sf_state cookie encoding, card-data rejection, error classification
  application/               ports (OUT interfaces), usecases (one per composed screen/action)
  adapters/
    inbound/http/            routes, DTOs, RFC 9457 problem+json errors, the cookie
    inbound/requestlog/      the body-free request log
    outbound/rest/           the six upstream REST clients (trace-context propagating)
    outbound/telemetry/      OpenTelemetry wiring
  config/                    env parsing and validation (a bad value refuses the boot)
  architecture/              the fitness tests
```

The rules are executable tests in `internal/architecture/`, not conventions:
`architecture_test.go` (hexagonal dependencies),
`bff_no_body_logging_fitness_test.go` (no request/response body is ever
logged), `bff_no_card_data_fitness_test.go` (card data is rejected, never
stored or logged), `bff_no_inorg_route_fitness_test.go` (no in-org or
restricted network route is ever called), `no_go_files_at_root_fitness_test.go`
— run by `make arch-test`.

## Running it

Runtime: Go, hexagonal, stateless. No database and no Kafka: the BFF composes
six upstream REST APIs, and a pod missing an upstream URL refuses to boot and
never pretends to be healthy.

```sh
export PRODUCT_DISCOVERY_URL=http://localhost:<sibling-addr>   # the six sibling base URLs are mandatory
export DELIVERY_PROMISE_URL=http://localhost:<sibling-addr>    # (a boot error names the missing one)
export PRICING_URL=http://localhost:<sibling-addr>
export SHOPPING_CART_URL=http://localhost:<sibling-addr>
export CHECKOUT_URL=http://localhost:<sibling-addr>
export CUSTOMER_ORDERING_URL=http://localhost:<sibling-addr>
go run ./cmd/api                # listens on :8080 (PORT), serves /api/storefront
curl -i http://localhost:8080/healthz   # liveness; /readyz is served alongside
```

### Configuration

| Env var | Default | Meaning |
|---|---|---|
| `PRODUCT_DISCOVERY_URL` | **required** | product-discovery base URL (catalogue reads) |
| `DELIVERY_PROMISE_URL` | **required** | delivery-promise base URL (delivery estimates) |
| `PRICING_URL` | **required** | pricing base URL (prices and quotes) |
| `SHOPPING_CART_URL` | **required** | shopping-cart base URL (cart commands/reads) |
| `CHECKOUT_URL` | **required** | checkout base URL (order placement) |
| `CUSTOMER_ORDERING_URL` | **required** | customer-ordering base URL (order history) |
| `PORT` | `8080` | Listen address port |
| `COOKIE_SECURE` | `true` | `Secure` flag on the `sf_state` cookie (the chart defaults it to `false` for the plain-HTTP kind cluster) |
| `BUDGET_QUERY_MS` | `2000` | Time budget for composed queries |
| `BUDGET_COMMAND_MS` | `5000` | Time budget for composed commands |
| `BUDGET_CONFIRM_MS` | `15000` | Time budget for checkout confirm |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |
| `OTEL_SERVICE_NAME` | `storefront-bff` | `service.name` on every span and metric |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTel Collector OTLP/gRPC endpoint, `host:port` or URL |
| `SERVICE_VERSION` | `dev` | `service.version` |
| `ENVIRONMENT` | `local` | `deployment.environment.name` |

An unparsable budget or a non-boolean `COOKIE_SECURE` is a boot error.

## Observability

Observability follows the fleet standard ([ADR 0003](docs/adr/0003-fleet-observability-standard.md)): traces and the
request-duration histogram come from the chi middleware, `OTEL_SERVICE_NAME`
(default `storefront-bff`), `OTEL_EXPORTER_OTLP_ENDPOINT`, `SERVICE_VERSION`
and `ENVIRONMENT` configure the OTLP/gRPC export (which never blocks), and
every upstream call carries the edge request's `traceparent`, so a composed
response's trace spans all six services. Logs are structured and never contain
request or response bodies (enforced by a fitness test); saved addresses pass
through without being logged.

## API

Contract: [`apis/openapi.yaml`](apis/openapi.yaml) (OpenAPI; also mirrored on
the warehouse-docs storefront site). The edge answers with or without the
`/api/storefront` prefix (the binary strips its `BasePath`). Errors are
RFC 9457 `application/problem+json`. The BFF mints a fresh
`Idempotency-Key` per user action and replays it for
[10 minutes](docs/adr/0002-storefront-bff-assumptions.md) (the retry window);
cart and shopper ids ride in the `sf_state` cookie (state, not
authentication) — there is no server-side session store.

| Method and path | Composes |
| --- | --- |
| `GET /products`, `GET /products/{id}`, `GET /categories` | product-discovery (+ pricing) |
| `POST /products/{id}:delivery-dates` | delivery-promise |
| `GET /carts`, `POST /carts`, `GET /carts/{id}` | shopping-cart |
| `POST /carts/{id}/lines`, `PATCH`/`DELETE /carts/{id}/lines/{item}` | shopping-cart |
| `POST /carts/{id}/destination`, `POST /carts/{id}/delivery:quote`, `POST /carts/{id}/delivery:select` | shopping-cart + delivery-promise + pricing |
| `POST /checkout`, `GET /checkout/{id}`, `POST /checkout/{id}:confirm` | checkout |
| `GET /orders`, `GET /orders/{id}`, `POST /orders/{id}:cancel` | customer-ordering |
| `POST /shoppers`, `POST /sessions`, `GET /me`, `POST /me/addresses` | customer-ordering (addresses pass through unlogged) |

## Helm chart

The chart is `charts/storefront-bff` (one `api` Deployment and Service). It
refuses to render without the six `config.*Url` base URLs of the sibling
services it composes (the failure names the value):

```sh
helm upgrade --install storefront-bff charts/storefront-bff \
  --namespace storefront-bff --create-namespace \
  --set config.productDiscoveryUrl=http://product-discovery.product-discovery.svc.cluster.local \
  --set config.deliveryPromiseUrl=http://delivery-promise.delivery-promise.svc.cluster.local \
  --set config.shoppingCartUrl=http://shopping-cart.shopping-cart.svc.cluster.local \
  --set config.checkoutUrl=http://checkout.checkout.svc.cluster.local \
  --set config.customerOrderingUrl=http://customer-ordering.customer-ordering.svc.cluster.local \
  --set config.pricingUrl=http://pricing.pricing.svc.cluster.local
```

Values that matter: the six `config.*Url` values, `config.port` (must equal
`service.targetPort`), `config.cookieSecure`, the three `config.budget*Ms`
values and `config.shutdownDrainDelay` (endpoint-removal headstart; the chart
refuses a `terminationGracePeriodSeconds` shorter than drain + 20s); Ingress
and the Gateway API route are optional and off by default. The chart is
packaged by the CI `release` job as `oci://ghcr.io/iqvo/storefront-bff`.
Render invariants are asserted by `charts/storefront-bff/tests/` over the
`ci/` values files; linted in CI with `ct lint`.

## Local development / quality gate

Every CI sensor is a `make` target. `make help` lists them all.

| Command | What it runs |
| --- | --- |
| `make check` | format, vet, build, lint and unit tests (the pre-push gate) |
| `make check-fast` | fmt-check, vet, arch-test |
| `make check-all` | check + coverage + arch-test + bdd |
| `make coverage` | 90% gate over `internal/domain` and `internal/application` |
| `make mutation` | gremlins on `internal/domain` |
| `make arch-test` | hexagonal and BFF fitness tests |
| `make integration` | integration suites over fake upstreams (build-tagged) |
| `make vuln` | govulncheck |

Git hooks are managed by [lefthook](https://github.com/evilmartians/lefthook)
(`lefthook.yml`): pre-commit runs `fmt-check` + `vet` + `lint`; pre-push runs
`make check`. Activate once per clone: `lefthook install`.

## Tests

- **Unit** — `go test ./...` (composition domain: allowlist, budgets,
  idempotency, state, card-data rejection; handlers; `make test`).
- **Integration** — `cmd/api` integration suites: per-call budget behavior,
  upstream contract mapping and the shopper journey (`make integration`).
- **Coverage gate** — `make coverage`: ≥ 90% over `internal/domain` +
  `internal/application`.
- **Mutation** — gremlins over `internal/domain` (`make mutation`; the
  exhaustive `mutation-full`).
- **Architecture fitness** — `internal/architecture/`, including the BFF's
  own sensors (never log bodies, no card data, no in-org routes).

CI (`.github/workflows/ci.yml`) runs `lint`, `guide-lint`, `complexity`,
`test` (with the 90% coverage gate), `bdd`, `integration`, `mutation-fast`,
`api-lint`, `vuln`, `chart-tests` and `arch-test` on every push and PR
(`helm-lint` with `ct lint` on PRs); the exhaustive `mutation` job runs only
on the weekly schedule or manual dispatch; `docker-publish` pushes the image
to GHCR on `main` once the gate jobs are green, and `release` then tags the
next version and publishes the chart.

## Architecture Decision Records

The ADR index lives at [docs/adr/](docs/adr/) — 4 decisions: the bounded
context, the design assumptions, the fleet observability standard, and the
relaying of the category tree.

## License

MIT — see [LICENSE](LICENSE).
