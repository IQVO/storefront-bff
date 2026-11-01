# Domain model: ubiquitous language, aggregates, events, use cases

storefront-bff has NO domain model and NO database. What it owns is
composition and edge rules; every business concept belongs to an upstream
context.

## Ubiquitous Language (use these exact names)

- **Composition** — the pure edge rules of the BFF (`internal/domain/composition`):
  the upstream allowlist, the cookie state, Idempotency-Key minting, route
  budgets and card-data refusal. No I/O, standard library only.
- **Upstream** — one of six services the BFF calls over REST: product-discovery,
  delivery-promise, pricing, shopping-cart, checkout, customer-ordering.
- **Call / Reply** — one outbound request and its raw answer. Upstream problem
  bodies travel in a Reply and are relayed unchanged; only edge failures are errors.
- **Allowlist** — the closed set of upstream routes the storefront may call
  (network OHS routes plus the storefront contexts' own REST). Anything else is refused.
- **State** — cart id, shopper id and last checkout id carried in the `sf_state`
  cookie. State, NOT authentication: there is no auth anywhere.
- **Action** — one user click. Its upstream Idempotency-Key is minted
  deterministically (route, subject, payload, ten minute window) or pinned by `X-Action-Id`.
- **Budget** — the time budget of a route class (query 2 s, command 5 s,
  confirm 15 s). Expiry is a 504, never a retry.

## Aggregates

None. `internal/domain/composition` holds value objects and pure functions only.

## Domain events

None published, none consumed. The BFF has no Kafka runtime path (the harness
kafka/cloudevents packages stay unused; see docs/adr/0002).

## Key use cases (`internal/application/usecases`)

- `SearchProducts`, `GetProduct` — relay product-discovery.
- `DeliveryDates` — one-line delivery-promise quote joined with the pricing charge schedule.
- `CreateCart`, `GetCart`, `ListCarts`, `AddLine`, `SetLineQuantity`, `RemoveLine`,
  `SetDestination`, `QuoteDelivery`, `SelectDelivery` — relay shopping-cart.
- `OpenCheckout`, `GetCheckout`, `ConfirmCheckout` — relay checkout; confirm refuses card data.
- `ListOrders`, `GetOrder`, `CancelOrder` — customer-ordering; `GetOrder` joins tracking and checkout totals.
- `RegisterShopper`, `Login`, `Me`, `AddAddress` — customer-ordering shopper routes (NG1-NG3).
