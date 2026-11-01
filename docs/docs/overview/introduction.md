---
id: introduction
title: Introduction
sidebar_label: Introduction
sidebar_position: 1
description: "What the Storefront BFF owns and does not own, its Generic subdomain class in the Storefront area, its capabilities and where to read next."
---

# Storefront BFF

`storefront-bff` is the **only storefront edge** of the warehouse-systems fleet.
The browser (`storefront-web`) talks to this one service under `/api/storefront`;
the BFF turns each screen or click into calls to the six REST services behind it
(product-discovery, delivery-promise, pricing, shopping-cart, checkout,
customer-ordering), relays their answers, and keeps the one thing a stateless
edge needs: a small state cookie. It has **no database, no Kafka and no domain
model**. It is classified **Generic** (Storefront area): replaceable plumbing
that must be correct and boring, not a source of competitive advantage
([Subdomain classification](/docs/ddd/subdomain-classification)).

:::info What runs on develop
One binary, `cmd/api` (Go, chi): the REST adapter with problem+json errors, the
closed upstream allowlist, per-route time budgets, minted `Idempotency-Key`s,
the `sf_state` cookie and the body-free request log; an outbound HTTP client with
trace propagation; OpenTelemetry traces, one request-duration histogram and
Go runtime metrics; a Helm chart `charts/storefront-bff` and a Dockerfile.

Not built, by design: authentication (REST is unauthenticated fleet-wide), an MCP
or A2A server, any Kafka producer or consumer (the `kafka` and `cloudevents`
packages are scaffold leftovers `cmd/api` never wires), any persistence.
:::

## What the context owns

| Concept | Where it lives | Rule in one line |
| --- | --- | --- |
| **Upstream allowlist** | `internal/domain/composition/allowlist.go` | A closed list of 25 `METHOD /path` templates over the six services. Anything else (ship-to, `POST /promises`, promise confirm and cancel, inventory feeds, policy routes) is refused before a request is built. |
| **Time budgets** | `composition/budget.go` | 2 s for queries, 5 s for commands, 15 s for checkout confirm, applied to the whole composition; expiry is a 504 and nothing is retried. |
| **Idempotency-Key minting** | `composition/idempotency.go` | `sf-` plus 40 hex chars, deterministic for route, subject and the page's required `Idempotency-Key` (one per user submit). |
| **Shopper state** | `composition/state.go` | The `sf_state` cookie carries opaque ids: cart `c`, shopper `s`, last checkout `k`. State, never authentication. |
| **Card-data guard** | `composition/card.go` | Raw card fields and card-shaped tokens never leave the process; only a PSP payment-method token is accepted on confirm. |
| **Composition use cases** | `internal/application/usecases` | One method per route: validate ids, build the allowlisted calls, join optional legs, classify failures. |
| **Problem mapping** | `internal/adapters/inbound/http/problems.go` | RFC 9457 `application/problem+json` under `/problems/`; upstream answers pass through untouched. |

## What it does not own

- **No business data.** Carts, checkout sessions, orders, shoppers, prices and
  catalogue belong to the six upstream services; the BFF forwards bodies byte for
  byte and does not store or log them (saved addresses included).
- **No authentication.** "Login" is a lookup of a shopper by e-mail
  (`POST /sessions`); the cookie is state, nothing trusts it for access.
- **No events.** It publishes and consumes nothing; there is no AsyncAPI
  document ([Integration](/docs/ecosystem/integration)).
- **No restricted network routes.** It reaches only the network's Open Host
  Service routes that customer-ordering and delivery-promise expose to
  shoppers, and an architecture test fails the build otherwise.
- **No presentation logic.** Turning instants into local dates is the page's job
  ([ADR 0005](/docs/adr/0005-delivery-dates-carry-the-window-time-zone)).

## Classification and area

- **Generic subdomain**, Storefront area. The BFF carries no domain model: the
  rules it enforces (allowlist, budgets, keys, cookie, card guard) are edge
  hygiene that any competent BFF has.
  [ADR 0001](/docs/adr/0001-storefront-bff-bounded-context) records it as a
  bounded context of the storefront domain; the aggregator's strategic design
  treats the experience layer as outside the six bounded contexts. Either way it
  has no aggregates ([Aggregate design canvas](/docs/ddd/aggregate-design-canvas)).
- No CloudEvents namespace is used because nothing is emitted
  (ADR 0001 reserves `com.retailnetwork.storefront.storefront-bff.<entity>.<Event>`).

## Main capabilities

1. **Browse**: `GET /products`, `GET /products/{id}` and `GET /categories`,
   relayed from product-discovery with only the documented search filters
   forwarded.
2. **Delivery dates on a product page**: `POST /products/{id}/delivery-date-quotes`
   quotes one unit for a postal prefix and joins each tier's delivery charge
   from pricing; a failing charge schedule only drops the charge.
3. **Cart**: create, read, list, add a line, change a quantity, remove a line,
   set a destination, quote and select delivery, each mapped to the cart's
   custom verbs ([ADR 0002](/docs/adr/0002-storefront-bff-assumptions)).
4. **Checkout**: open a session from the cookie's cart and shopper, poll it, and
   confirm with a PSP token under the 15 s budget.
5. **Orders**: list, cancel, and read an order joined with its tracking and, when
   the cookie's last checkout placed it, the checkout totals.
6. **Shopper**: register, "log in" by e-mail, read `/me`, save an address.
7. **Operate**: `/healthz` and `/readyz` (static 200), JSON logs correlated with
   traces, and boot-time validation of every upstream URL.

## Where to go next

| You want to | Read |
| --- | --- |
| See the layers, the binary and the rules that police them | [Architecture](/docs/overview/architecture) |
| Run it and make the first calls | [Quickstart](/docs/overview/quickstart) |
| Know every setting | [Configuration](/docs/operations/configuration) |
| Deploy and operate it | [Runbook](/docs/operations/runbook), [Observability](/docs/operations/observability), [Before first release](/docs/operations/before-first-release) |
| Fix something that broke | [Troubleshooting](/docs/operations/troubleshooting) |
| Know what tests and CI gates exist | [Testing](/docs/development/testing) |
| See who talks to whom | [Integration](/docs/ecosystem/integration) |
| See every use case and its invariants | [Use cases](/docs/ddd/use-cases) |
| Read the design in ddd-crew artifacts | [DDD artifacts](/docs/ddd/ddd-artifacts), [Ubiquitous language](/docs/ddd/ubiquitous-language) |
| Read the contract | [API overview](/docs/api-reference) |
| Read the decisions | [ADR index](/docs/adr) |

Published at [https://iqvo.github.io/storefront-bff/](https://iqvo.github.io/storefront-bff/).
