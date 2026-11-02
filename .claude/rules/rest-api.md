---
paths:
  - "internal/adapters/inbound/http/**"
  - "apis/openapi*.yaml"
  - "apis/openapi/**"
---

# REST API (inbound adapter)

The edge is served under `/api/storefront` (the prefix is optional on the
wire). `apis/openapi.yaml` documents it; `TestRoutesMatchOpenAPI` fails when
router and spec differ. Route -> use case -> upstream call:

- `GET /products` -> `SearchProducts` -> product-discovery `GET /products`
- `GET /categories` -> `ListCategories` -> product-discovery `GET /categories` (tree + purchasable counts, relayed unchanged)
- `GET /products/{id}` -> `GetProduct` -> `GET /products/{id}`
- `POST /products/{id}/delivery-date-quotes` -> `DeliveryDates` -> delivery-promise `POST /delivery-quotes` + pricing `GET /delivery-charge-schedules/current` (each tier relays the quoted window's optional `timeZone`, ADR 0005)
- `POST /carts` -> `CreateCart`; `GET /carts` -> `ListCarts`; `GET /carts/{id}` -> `GetCart`
- `POST /carts/{id}/lines` -> `AddLine`; `PATCH|DELETE /carts/{id}/lines/{item}` -> `SetLineQuantity` / `RemoveLine` (`:set-quantity` / `:remove`)
- `PUT /carts/{id}/destination` -> `SetDestination`; `POST /carts/{id}/delivery-quotes|select` -> `QuoteDelivery` / `SelectDelivery`
- `POST /checkout` -> `OpenCheckout`; `GET /checkout/{id}` -> `GetCheckout`; `POST /checkout/{id}/confirmations` -> `ConfirmCheckout`
- `GET /orders` -> `ListOrders`; `GET /orders/{id}` -> `GetOrder` (order + tracking + checkout totals); `POST /orders/{id}/cancellations` -> `CancelOrder`
- `POST /shoppers` -> `RegisterShopper`; `POST /sessions` -> `Login` (e-mail lookup); `GET /me` -> `Me`; `POST /me/addresses` -> `AddAddress`

## Conventions

- Error shape: RFC 9457 `application/problem+json`, `type` = `/problems/<slug>`.
  Upstream problems pass through unchanged. Edge slugs: `validation-failed` (422),
  `session-required` (422), `malformed-request` (400), `resource-not-found` (404),
  `payload-too-large` (413), `upstream-unavailable` (503), `gateway-timeout` (504), `internal-error` (500).
- Idempotency: every POST/PATCH requires the page's `Idempotency-Key` (400 `idempotency-key-required` / `idempotency-key-invalid`); the BFF derives
  the upstream key with `composition.MintKey(route, subject, pageKey)` and relays `Idempotency-Replayed`. Queries, PUT and DELETE carry no key. `X-Action-Id` is gone.
- Budgets: 2 s query, 5 s command, 15 s confirm (env `BUDGET_*_MS`); expiry is 504; nothing is retried.
- Bodies are never logged (`TestBFFNeverLogsBodies`); the access log has method, route pattern, status, duration.
- Cookie `sf_state`: HttpOnly, SameSite=Lax, path `/api/storefront`; state, not authentication.
- Auth: none. Fleet-wide auth was reverted 2026-09-11; `TestNoAuthMiddlewareReintroduced` guards it.
- Card data: only a PSP `paymentMethodToken` is accepted (`TestNoCardDataAnywhere`).
