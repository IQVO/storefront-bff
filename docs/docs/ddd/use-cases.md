---
id: use-cases
title: Use cases
sidebar_label: Use cases
sidebar_position: 1
description: "Every application use case of the Storefront BFF Gateway: trigger, inputs, invariants checked at the edge, upstream calls, budget class and events raised."
---

# Use cases

The application layer is `internal/application/usecases`. Every use case is a
method of `Gateway`, one per public route, over one OUT port
(`ports.Upstream`). There are **23 use cases**: 11 under the query budget (the
reads, the two composed reads and the e-mail lookup of `Login`), 11 under the
command budget and 1 under the confirm budget; none raises a domain event (the
BFF has no aggregates and publishes nothing).

Common rules, applied by `Gateway.call` and `Gateway.relay`
(`internal/application/usecases/gateway.go`):

- **Allowlist first.** A call off the closed list is refused before a request is
  built (`ErrRouteNotAllowed`, which surfaces as a 500 because it is a coding
  error, not a client error).
- **Budget per class.** The whole composition runs under one context deadline:
  *query* 2 s, *command* 5 s, *confirm* 15 s (`BUDGET_*_MS`). Expiry is
  `ErrUpstreamTimeout` (504); no retry.
- **Idempotency.** Every non-GET call gets `Idempotency-Key = MintKey(route,
  subject, X-Action-Id, body, now)`; queries carry none.
- **Pass-through.** The upstream reply (status, content type, body) is relayed
  untouched, so an upstream 4xx/409/422 reaches the page as is. Only "no answer"
  becomes an edge error: `ErrUpstreamUnavailable` (503), a canceled context (499).
- **Ids** (`ValidID`): 1 to 128 characters of `[A-Za-z0-9._-]` starting with an
  alphanumeric, so an id cannot alter the upstream path. A bad id is `ErrInvalid`
  (422 `validation-failed`).
- **Cookie.** A use case that learns an id (new cart, shopper, checkout session)
  returns the new `State` and the server sets `sf_state`; an upstream failure
  never changes it.

Triggers are the public routes of `apis/openapi.yaml`; the upstream column uses
the call the BFF actually makes (see [Integration](/docs/ecosystem/integration)).

## Catalogue

| # | Use case (file) | Trigger | Inputs | Invariants checked at the edge | Upstream and class | Events |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `SearchProducts` (`catalog.go`) | `GET /products` | query | only `q`, `categoryId`, `inStock`, `priceMin`, `priceMax`, `sort`, `after`, `limit` are forwarded, the rest is dropped | product-discovery `GET /products`, query | none |
| 2 | `ListCategories` (`catalog.go`) | `GET /categories` | none | nothing from the query is forwarded; the tree is relayed unchanged ([ADR 0004](/docs/adr/0004-bff-relays-the-category-tree)) | product-discovery `GET /categories`, query | none |
| 3 | `GetProduct` (`catalog.go`) | `GET /products/{id}` | product id | valid id | product-discovery `GET /products/{id}`, query | none |
| 4 | `DeliveryDates` (`catalog.go`, `dates.go`) | `POST /products/{id}:delivery-dates` | product id, `{postalPrefix}` | strict body (no unknown field, no trailing data); `postalPrefix` 1 to 10 letters, digits, spaces or dashes; valid id | delivery-promise `POST /delivery-offers:quote` for one unit (required), then pricing `GET /delivery-charge-schedules/current` (optional), one query budget for both | none |
| 5 | `CreateCart` (`carts.go`) | `POST /carts` | optional body | an empty body adopts the cookie's shopper (`{"shopperId":...}`) or `{}`; a body is forwarded as is | shopping-cart `POST /carts`, command; stores `cartId` in the cookie if valid | none |
| 6 | `GetCart` (`carts.go`) | `GET /carts/{id}` | cart id | valid id | shopping-cart `GET /carts/{id}`, query | none |
| 7 | `ListCarts` (`carts.go`) | `GET /carts` | optional `shopperId`, `state` | shopper defaults to the cookie's; a valid shopper is required (`validation-failed`); only `shopperId` and `state` forwarded | shopping-cart `GET /carts`, query | none |
| 8 | `AddLine` (`carts.go`) | `POST /carts/{id}/lines` | cart id, body | valid id | shopping-cart `POST /carts/{id}/lines`, command | none |
| 9 | `SetLineQuantity` (`carts.go`) | `PATCH /carts/{id}/lines/{item}` | cart id, item id, body | both ids valid | shopping-cart `POST /carts/{id}/lines/{item}:set-quantity`, command | none |
| 10 | `RemoveLine` (`carts.go`) | `DELETE /carts/{id}/lines/{item}` | cart id, item id | both ids valid | shopping-cart `POST /carts/{id}/lines/{item}:remove`, command | none |
| 11 | `SetDestination` (`carts.go`) | `POST /carts/{id}/destination` | cart id, body | valid id | shopping-cart `POST /carts/{id}:set-destination`, command | none |
| 12 | `QuoteDelivery` (`carts.go`) | `POST /carts/{id}/delivery:quote` | cart id | valid id; the cart service calls delivery-promise itself, hence the command budget | shopping-cart `POST /carts/{id}:quote-delivery`, command | none |
| 13 | `SelectDelivery` (`carts.go`) | `POST /carts/{id}/delivery:select` | cart id, body | valid id | shopping-cart `POST /carts/{id}:select-delivery`, command | none |
| 14 | `OpenCheckout` (`checkout.go`) | `POST /checkout` | optional `{cartId?, shipToAddressId}` | strict body; shopper required in the cookie (`session-required`, 422); `cartId` defaults to the cookie's; `cartId` and `shipToAddressId` valid ids | checkout `POST /checkout-sessions` with `{cartId, shopperId, shipToAddressId}`, command; stores `sessionId` as `k` in the cookie | none |
| 15 | `GetCheckout` (`checkout.go`) | `GET /checkout/{id}` | session id | valid id; the page polls it while a placement is in doubt (202) | checkout `GET /checkout-sessions/{id}`, query | none |
| 16 | `ConfirmCheckout` (`checkout.go`) | `POST /checkout/{id}:confirm` | session id, `{paymentMethodToken}` | valid id; no card-shaped key (`pan`, `cvc`, `expiry`, ...); strict body; token 1 to 256 characters and not a Luhn-valid 13 to 19 digit number | checkout `POST /checkout-sessions/{id}:confirm`, **confirm** class (15 s) | none |
| 17 | `ListOrders` (`orders.go`) | `GET /orders` | optional `shopperId`, `after`, `limit` | shopper defaults to the cookie's, else `session-required` (422) | customer-ordering `GET /orders`, query | none |
| 18 | `GetOrder` (`orders.go`) | `GET /orders/{id}` | order id | valid id | customer-ordering `GET /orders/{id}` (required), `/orders/{id}/tracking` and the cookie's checkout session (both optional), one query budget | none |
| 19 | `CancelOrder` (`orders.go`) | `POST /orders/{id}:cancel` | order id, `{reason}` | valid id; strict body; `reason` 1 to 200 characters | customer-ordering `POST /orders/{id}:cancel`, command | none |
| 20 | `RegisterShopper` (`shoppers.go`) | `POST /shoppers` | body (e-mail, optional addresses) | body passed through untouched, never logged | customer-ordering `POST /shoppers`, command; stores `shopperId` in the cookie | none |
| 21 | `Login` (`shoppers.go`) | `POST /sessions` | `{email}` | strict body; e-mail 1 to 254 characters; no password, no authentication | customer-ordering `GET /shoppers?email=`, query; stores `shopperId` in the cookie | none |
| 22 | `Me` (`shoppers.go`) | `GET /me` | none | shopper required in the cookie (`session-required`) | customer-ordering `GET /shoppers/{id}`, query | none |
| 23 | `AddAddress` (`shoppers.go`) | `POST /me/addresses` | address body | shopper required in the cookie; address passed through, not stored or logged | customer-ordering `POST /shoppers/{id}/addresses`, command | none |

The two composed reads (4 and 18) use the query budget although 4 is a `POST`;
use case 21 is a read that the page treats as a sign-in.

## Domain operations behind them

The invariants live in `internal/domain/composition`, not in the adapters.

| Operation | Rule |
| --- | --- |
| `Allowed(Call)` | A call is permitted only when `(service, method, path)` matches a template of the closed list; `{}` is one path segment that must itself be a valid id, and a literal tail after it (`:confirm`) is a custom-verb suffix. |
| `Budgets.For(Class)` | Query, Command or Confirm duration; an unknown class gets the command budget; non-positive budgets are rejected at config load. |
| `MintKey(route, subject, action, body, now)` | `sf-` plus the first 40 hex characters of SHA-256 over null-separated fields. With a valid `X-Action-Id` (at most 64 characters) the key ignores body and window; otherwise it hashes the body digest and `now` truncated to 10 minutes. |
| `State.Encode` / `DecodeState` | Query-string cookie value with `c`, `s`, `k`; malformed input and ids failing `ValidID` decode to an empty field, never an error. |
| `IsCardFieldName`, `LooksLikeCardNumber` | JSON keys only a raw card would carry; 13 to 19 digits passing the Luhn check, spaces and dashes ignored. |

## Errors and the problem types they become

Mapped in `internal/adapters/inbound/http/problems.go` (`writeError`); the
symptom-first view is [Troubleshooting](/docs/operations/troubleshooting).

| Error | Problem type (status) |
| --- | --- |
| `composition.ErrInvalid` (detail is a fixed sentence) | `/problems/validation-failed` (422) |
| `composition.ErrNoSession` | `/problems/session-required` (422) |
| `composition.ErrUpstreamTimeout` | `/problems/gateway-timeout` (504) |
| `composition.ErrUpstreamUnavailable` | `/problems/upstream-unavailable` (503) |
| `context.Canceled` | `/problems/client-closed-request` (499) |
| anything else, including `ErrRouteNotAllowed` | `/problems/internal-error` (500) |
| malformed route or NUL in input (middleware) | `/problems/malformed-request` (400), `/problems/resource-not-found` (404), `method-not-allowed` (405), `/problems/payload-too-large` (413) |

Details never carry request or upstream data.

## Where the code differs from the ADR text

- ADR 0002 A9 says tracing (`otelhttp`) "is not wired yet"; it is
  (`internal/adapters/outbound/rest/client.go`, and
  [ADR 0003](/docs/adr/0003-fleet-observability-standard) adopts the fleet
  observability standard).
- `ErrRouteNotAllowed` has no dedicated problem type and surfaces as a 500
  `internal-error`; it can only come from a code change that calls a route
  missing from the allowlist.
