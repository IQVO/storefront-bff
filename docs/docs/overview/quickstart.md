---
id: quickstart
title: Quickstart
sidebar_label: Quickstart
sidebar_position: 3
description: "Build, test and run the Storefront BFF locally, then make the first calls against the storefront edge."
---

# Quickstart

`storefront-bff` is a single stateless binary, `cmd/api`. It needs **no
Postgres and no Kafka**: its whole runtime is the six upstream REST services it
composes ([Architecture](/docs/overview/architecture)). Everything below is
derived from the code and the Makefile; the curl calls were **not captured from
a live run** (see [What is not captured](#what-is-not-captured)).

## Prerequisites

| Tool | Why |
| --- | --- |
| Go `1.27.3` (`go.mod`) | build and test |
| `make` | the local quality gate |
| `golangci-lint` `v2.14.0` | `make lint` (the version CI pins) |
| Node 20+ | only to build this documentation site (`docs/`) |
| `helm` 3.16+ | only to lint or render the chart |
| Docker | only to build the image; no test needs it |

## Make targets

| Target | What it runs |
| --- | --- |
| `make build` | `go build ./...` |
| `make test` | `go test ./... -race` (unit, handler and fitness tests; no containers) |
| `make check` | `fmt-check vet build lint test`, the fast pre-commit bundle that lefthook runs on push |
| `make check-all` | `check` plus `coverage` (90 % gate), `arch-test`, `bdd` |
| `make coverage` | race tests with the coverage profile and the 90 % gate over the domain, application, inbound, outbound REST and config packages |
| `make arch-test` | `go test ./internal/architecture/... -v` (hexagonal and BFF fitness tests) |
| `make integration` | `go test -tags=integration ./... -race -count=1` (the four `cmd/api/integration_*_test.go` files; they use in-process `httptest` upstreams, not containers) |
| `make mutation` | gremlins on `./internal/domain/composition` (needs `gremlins` v0.6.0) |
| `make vuln` | `govulncheck ./...` |

## Run it

All six upstream base URLs are mandatory; a missing one stops the process before
the listener binds ([Configuration](/docs/operations/configuration)). Point them
at the kind cluster's services (for example through `kubectl port-forward`) or at
any HTTP stand-in:

```sh
export PRODUCT_DISCOVERY_URL=http://localhost:9101
export DELIVERY_PROMISE_URL=http://localhost:9102
export SHOPPING_CART_URL=http://localhost:9103
export CHECKOUT_URL=http://localhost:9104
export CUSTOMER_ORDERING_URL=http://localhost:9105
export PRICING_URL=http://localhost:9106
export COOKIE_SECURE=false      # plain HTTP locally, otherwise curl drops the cookie
go run ./cmd/api
```

The log line `storefront-bff listening` with `addr` `[::]:8080` means the listener
is up. There is no Collector requirement: `OTEL_EXPORTER_OTLP_ENDPOINT` defaults
to `localhost:4317` and an absent Collector only drops telemetry.

Kafka note: the platform has one broker (`localhost:9092` from the kind cluster),
but this service never connects to it.

## First calls

The edge answers with or without the `/api/storefront` prefix. REST is
unauthenticated; the `sf_state` cookie is state, not authentication.

```sh
# liveness and readiness: static 200 {"status":"ok"}
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz

# catalogue read, relayed from product-discovery
curl -s 'http://localhost:8080/api/storefront/products?q=shirt&limit=5'
curl -s  http://localhost:8080/api/storefront/categories

# register a shopper; the reply sets the sf_state cookie (s=<shopperId>)
curl -si -c jar.txt -X POST http://localhost:8080/api/storefront/shoppers \
  -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' -d '{"email":"ana@example.com"}'

# create a cart: with the cookie the cart is owned by the shopper, the cookie gains c=<cartId>
curl -si -b jar.txt -c jar.txt -X POST http://localhost:8080/api/storefront/carts \
  -H "Idempotency-Key: $(uuidgen)"

# every POST and PATCH needs an Idempotency-Key (one per user submit; re-send
# the same key to retry: the answer then carries Idempotency-Replayed: true)
curl -si -b jar.txt -X POST http://localhost:8080/api/storefront/carts/CART_ID/lines \
  -H 'Idempotency-Key: submit-1' -H 'Content-Type: application/json' \
  -d '{"catalogItemId":"ITEM_ID","quantity":1}'
```

Replace `CART_ID` and `ITEM_ID` with real ids (`[A-Za-z0-9._-]`, at most 128
characters, starting alphanumeric). The request bodies the upstream expects are in
the [API reference](/docs/api-reference); the BFF forwards most bodies
untouched.

Two edge failures worth recognising right away:

```sh
# unknown route: problem+json 404 /problems/resource-not-found
curl -si http://localhost:8080/nope

# no shopper cookie: problem+json 422 /problems/session-required
curl -si http://localhost:8080/api/storefront/me
```

## Build the documentation site

```sh
cd docs
npm ci
npm run build      # fails on any broken link; `npm start` serves it with reload
```

## What is not captured

The calls above are written from the handlers and use cases
(`internal/adapters/inbound/http/`, `internal/application/usecases/`), not
recorded from a running service, and the upstream response bodies depend on the
upstream you point at. Treat status codes and cookie behaviour as the contract
and the bodies as the upstream's.
