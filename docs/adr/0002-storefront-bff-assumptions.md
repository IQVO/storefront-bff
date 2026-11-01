# 2. Storefront BFF design assumptions

Status: Accepted. Design source: warehouse-docs `storefront/model/model.yaml` (context `storefront-bff`), the
campaign plan section 4.7 and the upstream OpenAPI contracts.

## Context

The model gives the BFF a route list but no wire-level detail. Where it is silent the least-surprising reading is
chosen and recorded here (A1, A2, ...).

## Decision

- **A1 Public paths vs upstream verbs.** The BFF exposes REST-style paths (`PATCH /carts/{id}/lines/{item}`,
  `DELETE` the same, `POST /carts/{id}/destination`, `POST /carts/{id}/delivery:quote|select`, `POST /checkout`) and
  maps them to the upstream custom verbs (`:set-quantity`, `:remove`, `:set-destination`, `:quote-delivery`,
  `:select-delivery`, `POST /checkout-sessions`). The mapping lives in `internal/application/usecases`.
- **A2 Prefix.** Routes answer under `/api/storefront` and also without the prefix, so the BFF works behind a gateway
  that strips it or one that does not.
- **A3 Idempotency-Key.** Minted as `sf-` + SHA-256 prefix of (route with ids, subject, payload digest, ten minute
  window); the subject is the cookie's shopper, else cart, else `anonymous`. A page may pin the key per click with
  `X-Action-Id` (then payload and window are ignored). A retry straddling a window boundary mints a new key; the
  upstream still dedupes through its own business rules. Queries carry no key.
- **A4 Budgets.** 2 s queries, 5 s commands (including cart `:quote-delivery`, which calls delivery-promise
  synchronously), 15 s checkout confirm (authorize plus place order). Overridable with `BUDGET_QUERY_MS`,
  `BUDGET_COMMAND_MS`, `BUDGET_CONFIRM_MS`. Expiry answers 504 `gateway-timeout`; nothing is retried. A composed
  route (`GetOrder`, `DeliveryDates`) shares one budget across its calls.
- **A5 Delivery dates need pricing.** The page shows a charge per tier, which only pricing's
  `GET /delivery-charge-schedules/current` knows, so `PRICING_URL` is a sixth mandatory upstream. Item prices are
  NOT read from pricing (they come in the ProductView). A failing charge schedule leaves the charge out; a failing
  quote is the error.
- **A6 Order totals join.** customer-ordering has no checkout reference on an order and checkout has no lookup by
  order, so `GET /orders/{id}` joins the totals only when the cookie's last checkout session (`k`) placed that order.
  Tracking and totals are best effort; the order is required.
- **A7 Cookie.** `sf_state` carries `c` (cart), `s` (shopper), `k` (last checkout) as an unsigned query string,
  HttpOnly, SameSite=Lax, path `/api/storefront`, 30 days, `Secure` unless `COOKIE_SECURE=false`. Ids are validated
  (`[A-Za-z0-9._-]`, at most 128) before they touch an upstream path.
- **A8 No Kafka, no database.** The harness `kafka` and `cloudevents` packages are kept but unused; no gate objected,
  so no exception was needed. `/healthz` and `/readyz` are static 200: readiness does not ping the six upstreams, so
  one slow upstream cannot take the whole edge out of rotation.
- **A9 Harness adaptation.** `make integration` needs no containers: the integration tests boot the real binary
  wiring against `httptest` fakes of the upstreams (`-tags=integration`, `cmd/api`). The coverage gate keeps its
  domain and application scope and adds the BFF's own adapters and config; mutation covers the pure
  `internal/domain/composition` package. Tracing (otelhttp) is not wired yet.
- **A10 Passthrough of bodies.** Cart, shopper and address bodies are forwarded byte for byte; the upstream owns
  validation. The edge validates only what it composes or must refuse: ids, postal prefix, cancel reason, e-mail
  presence, and card data (card-shaped keys, unknown fields, a PAN-shaped token) on confirm.
- **A11 Edge problems.** `session-required` is 422 (no auth, so no 401): `GET /me`, address save, checkout open and
  order listing without a shopper in the cookie or query.

## Consequences

The BFF stays a thin, stateless edge. Upstream contract changes surface in the integration tests and in
`internal/domain/composition/allowlist.go`; restricted routes cannot be reached because the allowlist is closed
(`TestNoInOrgNetworkRoute`).
