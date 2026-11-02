# ADR 0006: Adopt the fleet REST API guideline

## Status

Accepted (2026-11-01). Supersedes ADR 0002 A3 (Idempotency-Key minting) and the
custom-verb routes listed in ADR 0002 and ADR 0005.

## Context

The warehouse fleet adopted one REST API guideline (warehouse-docs, REST
guideline R1-R9): nouns, never `:verb` suffixes or verb segments (R1, R2); every
`POST` and `PATCH` takes an `Idempotency-Key` (R3), answers 400
`idempotency-key-required` without one and 409 `idempotency-key-reused` when the
same key carries a different method, path or body (R4, R8); a replay carries
`Idempotency-Replayed: true` (R5, R6); collections page with `limit` and an
opaque `cursor` and answer `{items, nextCursor}` (R7), rejecting undeclared query
parameters with 400 `unknown-query-parameter`. `PUT` and `DELETE` are idempotent
by method and take no key. The cutover is hard: no aliases, no coexistence.

The BFF is stateless (no database): it never stored keys. It minted a
deterministic upstream key from route, subject and an optional `X-Action-Id`, or
from the body digest inside a ten minute window.

## Decision

- **Routes.** The edge routes follow the rename map:

  | Old | New |
  | --- | --- |
  | `POST /products/{id}:delivery-dates` | `POST /products/{id}/delivery-date-quotes` |
  | `POST /carts/{id}/destination` | `PUT /carts/{id}/destination` |
  | `POST /carts/{id}/delivery:quote` | `POST /carts/{id}/delivery-quotes` |
  | `POST /carts/{id}/delivery:select` | `PUT /carts/{id}/delivery-selection` |
  | `POST /checkout/{id}:confirm` | `POST /checkout/{id}/confirmations` |
  | `POST /orders/{id}:cancel` | `POST /orders/{id}/cancellations` |

  `operationId`s are unchanged. `TestRetiredRoutesAreGone` pins that every old
  route answers 404 or 405. The upstream calls (and the allowlist) use the
  providers' new routes: delivery-promise `POST /delivery-quotes`; shopping-cart
  `PUT|DELETE /carts/{id}/lines/{item}`, `PUT /carts/{id}/destination`,
  `POST /carts/{id}/delivery-quotes`, `PUT /carts/{id}/delivery-selection`;
  checkout `POST /checkout-sessions/{id}/confirmations`; customer-ordering
  `POST /orders/{id}/cancellations`.
- **Idempotency.** `X-Action-Id` is removed. Every edge `POST` and `PATCH`
  requires the page's `Idempotency-Key` (a fresh `crypto.randomUUID()` per user
  submit, reused on retry): missing is 400 `idempotency-key-required`, longer
  than 128 characters, not visible ASCII or repeated is 400
  `idempotency-key-invalid`. The upstream key is `MintKey(route, subject,
  pageKey)`: no window and no body digest, the page owns the identity. The same
  page key on a retry reaches the provider with the same key and is replayed;
  the same page key with a different body reaches it with the same key and a
  different fingerprint, and the provider's 409 `idempotency-key-reused` is
  relayed unchanged. The BFF stores nothing.
- **Replay header.** When an upstream write hop answers with
  `Idempotency-Replayed: true` the edge answer carries it too (a composition is
  replayed when any write hop was). It is declared on every 2xx of every POST and
  PATCH in `apis/openapi.yaml`.
- **Pagination.** `GET /products`, `GET /carts` and `GET /orders` take `limit`
  (1-500) and `cursor`, relay them to the provider and relay its
  `{items, nextCursor}` page (product search also `facets` and
  `categorySuggestions`). `after` is retired; an undeclared parameter is 400
  `unknown-query-parameter`, a bad limit 400 `invalid-limit`; a bad cursor is the
  provider's 400 `invalid-cursor`. The `page` package is the fleet reference.
- **Exemption.** `GET /categories` is `x-pagination-exempt: view`: the category
  tree is one bounded navigation document, not a resource collection.
- **Lint.** `.spectral.yaml` runs the fleet `rest-guideline` rule; `spectral lint
  apis/openapi.yaml --ruleset .spectral.yaml --fail-severity=warn` is clean.

## Consequences

- The BFF must deploy together with the REST cutover set (shopping-cart,
  checkout, customer-ordering, delivery-promise, product-discovery) and with
  storefront-web, its only browser client, which sends the new routes and the
  `Idempotency-Key` header and follows `nextCursor`.
- A page that sends a fresh key per click instead of per submit loses dedup on
  retry; a page that reuses a key for a different action gets a 409.
- Consumers changed in this repository: `internal/adapters/outbound/rest`
  (reads `Idempotency-Replayed`), `internal/application/usecases` (upstream
  routes and methods, cursor forwarding), `internal/domain/composition`
  (allowlist, `MintKey`), the architecture fitness test, and the integration
  tests under `cmd/api`.
