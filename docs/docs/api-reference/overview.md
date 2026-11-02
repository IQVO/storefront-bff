---
id: overview
title: API overview
slug: /api-reference
sidebar_position: 1
description: "Interfaces of the Storefront BFF: the REST edge under /api/storefront, endpoint table with upstream mapping, and the contract conventions (problem+json, cookie state, Idempotency-Key, budgets)."
---

# API overview

:::info Implemented by cmd/api
`cmd/api` serves every route below (`internal/adapters/inbound/http`) on `PORT`
(default `:8080`). The contract is `apis/openapi.yaml` (OpenAPI 3.0.3, version
0.1.0, server `/api/storefront`), and `TestRoutesMatchOpenAPI` pins the router
to it. The router registers exactly the 23 API routes of the spec plus
`/healthz` and `/readyz`, which the spec does not list. See the
[Quickstart](/docs/overview/quickstart) for the first calls.
:::

| Interface | Source of truth | Notes |
| --- | --- | --- |
| REST | `apis/openapi.yaml` (OpenAPI 3.0.3, version 0.1.0) | The pages under *REST* are generated from the spec with `npm run gen-api-docs`; do not edit them by hand. Server in the spec: `/api/storefront` (Kong strips the prefix; the binary answers with or without it). |
| Events | none | The BFF publishes and consumes nothing: no AsyncAPI document, no event catalogue ([Domain events](/docs/ddd/domain-events)). |
| MCP | none | No `cmd/mcp`. |
| A2A | none | No `cmd/a2a`. |

## Endpoints

Public paths are REST-style; the BFF maps them to the upstream custom verbs
([ADR 0002](/docs/adr/0002-storefront-bff-assumptions), A1).

| Tag | Endpoint | Operation id | Upstream call | Success |
| --- | --- | --- | --- | --- |
| Catalog | `GET /products` | `searchProducts` | product-discovery `GET /products` | upstream body, filters `q`, `categoryId`, `inStock`, `priceMin`, `priceMax`, `sort`, `after`, `limit` |
| Catalog | `GET /categories` | `listCategories` | product-discovery `GET /categories` | the category tree, relayed unchanged |
| Catalog | `GET /products/{id}` | `getProduct` | product-discovery `GET /products/{id}` | upstream body |
| Catalog | `POST /products/{id}:delivery-dates` | `quoteProductDeliveryDates` | delivery-promise quote + pricing charge schedule | `200 {postalPrefix, tiers[], unavailable[]}` |
| Carts | `POST /carts` | `createCart` | shopping-cart `POST /carts` | upstream body; sets `sf_state` |
| Carts | `GET /carts` | `listCarts` | shopping-cart `GET /carts` | upstream body |
| Carts | `GET /carts/{id}` | `getCart` | shopping-cart `GET /carts/{id}` | upstream body |
| Carts | `POST /carts/{id}/lines` | `addCartLine` | shopping-cart `POST /carts/{id}/lines` | upstream body |
| Carts | `PATCH /carts/{id}/lines/{item}` | `setCartLineQuantity` | shopping-cart `POST .../lines/{item}:set-quantity` | upstream body |
| Carts | `DELETE /carts/{id}/lines/{item}` | `removeCartLine` | shopping-cart `POST .../lines/{item}:remove` | upstream body |
| Carts | `POST /carts/{id}/destination` | `setCartDestination` | shopping-cart `POST /carts/{id}:set-destination` | upstream body |
| Carts | `POST /carts/{id}/delivery:quote` | `quoteCartDelivery` | shopping-cart `POST /carts/{id}:quote-delivery` | upstream body |
| Carts | `POST /carts/{id}/delivery:select` | `selectCartDelivery` | shopping-cart `POST /carts/{id}:select-delivery` | upstream body |
| Checkout | `POST /checkout` | `openCheckout` | checkout `POST /checkout-sessions` | upstream body; sets `sf_state` |
| Checkout | `GET /checkout/{id}` | `getCheckout` | checkout `GET /checkout-sessions/{id}` | upstream body |
| Checkout | `POST /checkout/{id}:confirm` | `confirmCheckout` | checkout `POST /checkout-sessions/{id}:confirm` | upstream body (a `202` means the placement is in doubt: poll) |
| Orders | `GET /orders` | `listOrders` | customer-ordering `GET /orders` | upstream body |
| Orders | `GET /orders/{id}` | `getOrder` | customer-ordering order + tracking, joined with checkout totals | `200`, the order with `tracking` and `totals` when available |
| Orders | `POST /orders/{id}:cancel` | `cancelOrder` | customer-ordering `POST /orders/{id}:cancel` | upstream body |
| Account | `POST /shoppers` | `registerShopper` | customer-ordering `POST /shoppers` | upstream body; sets `sf_state` |
| Account | `POST /sessions` | `createSession` | customer-ordering `GET /shoppers?email=` | the shopper; sets `sf_state` |
| Account | `GET /me` | `getMe` | customer-ordering `GET /shoppers/{id}` | the shopper |
| Account | `POST /me/addresses` | `addMyAddress` | customer-ordering `POST /shoppers/{id}/addresses` | upstream body |
| Health | `GET /healthz`, `GET /readyz` | none (not in the spec) | none | `200 {"status":"ok"}`, static; readiness does not probe the upstreams |

## Conventions in the contract

- **Relay, not reshape.** Except the two composed routes (`delivery-dates`,
  `GET /orders/{id}`), the upstream status, `Content-Type` and body are returned
  untouched, so a 409 or a 422 from checkout reaches the page as checkout sent
  it. The generated reference therefore documents the edge's own fields; the
  body schemas of relayed routes belong to the upstream's OpenAPI.
- **Errors** are RFC 9457 `application/problem+json` with
  `type = /problems/<slug>`. The edge's own slugs: `malformed-request` (400),
  `resource-not-found` (404), `method-not-allowed` (405), `payload-too-large`
  (413), `validation-failed` (422), `session-required` (422),
  `client-closed-request` (499), `internal-error` (500), `route-not-allowed`
  (500), `upstream-unavailable`
  (503) and `gateway-timeout` (504). `detail` is a fixed sentence and never
  carries request or upstream data. The causes are listed on
  [Use cases](/docs/ddd/use-cases#errors-and-the-problem-types-they-become) and,
  with remedies, on [Troubleshooting](/docs/operations/troubleshooting).
- **State cookie.** `sf_state` carries `c` (cart), `s` (shopper) and `k` (last
  checkout session) as an unsigned query string: `HttpOnly`, `SameSite=Lax`,
  `Path=/api/storefront`, 30 days, `Secure` unless `COOKIE_SECURE=false`. It is
  state, not authentication. A malformed cookie decodes to an empty state.
- **`Idempotency-Key`.** The client never sends it. The BFF mints
  `sf-` + 40 hex characters for each command and forwards it. A page pins the
  identity of one click with the optional `X-Action-Id` request header (a valid
  id of at most 64 characters), after which body and time window are ignored;
  without it the key is deterministic for route, subject and body inside a
  10-minute window.
- **Time budgets.** 2 s for queries, 5 s for commands, 15 s for checkout
  confirm, each for the whole composition; expiry is `504 gateway-timeout` and
  is never retried.
- **Limits.** Request bodies above 1 MiB are `413`; a NUL character in the path,
  query or body is refused; upstream replies above 4 MiB are `503`.
- **Prefix.** Routes answer under `/api/storefront` and also without it.
- **Auth**: none. The fleet reverted REST auth on 2026-09-11, and
  `TestNoAuthMiddlewareReintroduced` fails CI if auth middleware returns under
  `internal/adapters/inbound`. The cookie is not a credential.
- **No card data.** Confirm accepts only a PSP `paymentMethodToken`; card-shaped
  keys and card-number tokens are refused with `422` before anything leaves the
  process.
