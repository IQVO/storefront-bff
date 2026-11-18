# ADR 0007: Batch delivery estimates, the capacity band and the CONSOLIDATED tier

## Status

Accepted (2026-11-18). Refines ADR 0006 for one safe POST (the Idempotency-Key rule) and extends ADR 0005
(the delivery-dates composition keeps relaying what delivery-promise adds to an offer).

## Context

A read-only check of origin/develop (N0.7) found three gaps between delivery-promise and the storefront edge.

1. **Batch estimates (G60, delivery-promise ADR 0019).** `POST /delivery-estimates` answers up to 50 items of a
   listing page in one call and stores nothing. The BFF's closed allowlist
   (`internal/domain/composition/allowlist.go`) already lists `POST /delivery-estimates`, but no use case called it and
   no edge route exposed it, so a listing tile could only show a date by quoting once per product
   (`POST /products/{id}/delivery-date-quotes`: one offer row and one event per tile at the producer).
2. **CONSOLIDATED (delivery-promise ADR 0024).** The producer's `Tier` enum gained the opt-in `CONSOLIDATED` tier
   (a customer-chosen delivery day, requested with `tiers` or `consolidatedDeliveryDay`) and a
   `consolidationEligible` flag on its offers. The BFF's `DeliveryDates` answer declared four tiers only.
3. **Capacity band (delivery-promise ADR 0023).** Quoted offers and estimate options may carry
   `capacityBand` (`AMPLE`, `TIGHT`, `SCARCE`). `composeDates` decodes the quote into typed structs and drops every
   field it does not name (ADR 0005), so the band never reached the product page.

The hexagonal rules stay as they are: the BFF reaches delivery-promise only through the declared gateway and the
outbound allowlist, a domain context never calls a sibling, nothing here is authenticated (by decision, fleet-wide),
and no money and no personal data beyond what the BFF already relays are added.

Alternatives weighed:

- **Quote each listing tile.** Rejected by delivery-promise ADR 0019: it writes an offer and an event per tile per page
  view, none of which can back a promise.
- **Put the estimate under `/products/...`** (for example `POST /products/delivery-estimates`). `GET /products/{id}`
  already owns that segment, and a computation over a body that names many products is not a sub-resource of one.
- **Require an Idempotency-Key on the estimate** like every other edge POST. The key would mean nothing: see below.
- **Have the BFF request CONSOLIDATED now** (a `tiers` field on the estimate, or `consolidatedDeliveryDay` on the
  quote). Not yet: see the ordering below.

## Decision

### 1. `POST /delivery-estimates` (use case `DeliveryEstimates`)

- **Route.** `POST /delivery-estimates` at the edge (under `/api/storefront`, and unprefixed per ADR 0002 A2): a noun
  route for the result of a computation with no aggregate id, at collection level, as the fleet guideline models
  `POST /delivery-quotes` and as the producer names it. `operationId` `estimateDeliveries`, tag `Catalog`.
- **Use case.** `Gateway.DeliveryEstimates` in `internal/application/usecases/catalog.go`. It is a query (the 2 s query
  budget, one upstream call, nothing retried). The upstream call is
  `composition.Call{Service: DeliveryPromise, Method: POST, Path: /delivery-estimates, Body: {destinationPostalPrefix,
  items: [{catalogItemId, quantity}]}}`, allowlisted already. The producer's answer is relayed unchanged (status,
  content type and body), including its problems.
- **Request.** `{postalPrefix, items}` with the same `postalPrefix` rule as delivery-date-quotes (a prefix, never an
  address) and `items` of 1 to 50 entries, each `catalogItemId` once (a valid id of at most 64 characters), each
  `quantity` 1 to 999. A strict decode refuses any other field. A violation answers 422 `validation-failed` before any
  upstream call. There is no `tiers` field: the producer then estimates the four classic tiers and never
  `CONSOLIDATED`, which it refuses with 422 `validation-failed` on this route, so the BFF has no way to send it.
- **Response.** The producer's `EstimateResponse`, documented in `apis/openapi.yaml`:
  `{computedAt, destinationPostalPrefix, estimates: [{catalogItemId, quantity, options: [{tier, window: {start, end,
  timeZone}, basis, confidence?, orderByAt?, capacityBand?, carrierArea?}], constraints: [{tier?, code}]}]}`. The
  estimate window is `start`/`end`, which a quote calls `earliest`/`latest`; the BFF relays each as named and does not
  rename. The estimate is advisory: no offer id, it cannot back a promise, it reserves nothing.
- **Idempotency-Key.** ADR 0006 requires the page's key on every edge POST and PATCH because each of those writes
  upstream. This POST writes nothing, here or upstream: delivery-promise ADR 0019 makes it a safe POST that takes no key
  and ignores one that is sent, as the guideline treats `PUT` and `DELETE`. So the edge does not require the key, forwards
  none and mints none (a retry is idempotent by effect), never sets `Idempotency-Replayed`, and never answers 409. The
  OpenAPI operation still declares an optional `Idempotency-Key` header, a 409 and the 2xx `Idempotency-Replayed` header,
  each described as never produced, because the fleet Spectral rule R3-R5 requires them on every POST (the producer
  does the same). Every other edge POST and PATCH is unchanged.
- **Failure and budget.** A 400, 422 or 5xx from the producer passes through; a slow producer is a 504
  `gateway-timeout` and an unreachable one a 503 `upstream-unavailable`, as for every query. A listing page that gets
  neither simply shows no date on its tiles.

### 2. `capacityBand` is relayed

`composeDates` copies the optional `capacityBand` of each QUOTED offer onto its tier, and the estimates route relays it
on each option. The value is a signal (AMPLE below 0.60 utilisation, TIGHT from 0.60, SCARCE from 0.85): the BFF
neither interprets it, computes a fee from it, nor validates it. It is `omitempty`, and `apis/openapi.yaml` declares it
as an optional enum on `DeliveryDates.tiers[]` (`CapacityBand` schema).

### 3. CONSOLIDATED is accepted, not requested

- `DeliveryDates.tiers[].tier` gains `CONSOLIDATED` in `apis/openapi.yaml`, and `composeDates` relays an optional
  `consolidationEligible` boolean (a pointer, so a quoted `false` survives; absent when the quote has none). A QUOTED
  CONSOLIDATED offer that delivery-promise returns therefore reaches the product page as a tier like any other.
- The BFF does **not** request the tier: no `tiers` and no `consolidatedDeliveryDay` is sent by `catalog.go` or
  `carts.go` (tests pin that the quote and estimate request bodies contain neither). The producer returns CONSOLIDATED
  only when asked, so today no CONSOLIDATED tier appears.
- **Ordering.** A shopper who picks a CONSOLIDATED offer must be able to select it in the cart and confirm it at
  checkout. shopping-cart and checkout must accept the tier first (separate PRs in flight, in their own repositories).
  Only after both have landed on develop does a follow-up change make the BFF request it, with its own ADR and its own
  contract tests against refreshed pins. Until then, requesting it from here would offer a shopper a delivery option the
  rest of the checkout flow refuses.

## Consequences

- Listing pages can show a delivery date per tile with one call and no write load at delivery-promise. The call is
  advisory; a shopper who goes on to buy still gets a quote, which can differ.
- The change is additive and optional. `apis/openapi.yaml` gains a path, schemas, two optional tier properties and one
  response enum value, so the spec-breaking gate sees an addition (and, for the enum value, the fleet's warning, not a
  failure). A response with no `capacityBand`, no CONSOLIDATED offer and no `consolidationEligible` is byte-identical
  to the previous one, pinned by a golden fixture produced by the code before this change
  (`delivery-dates.existing-offers.golden.json`). REST clients must tolerate an unknown tier value, as ever.
- Contract tests R2 (fixtures satisfy the pinned EstimateRequest, EstimateResponse and QuoteResponse), R3 (the real
  client sends a request the producer's schema accepts, expects 200, relays the body byte for byte, mints no key) and R4
  (the fields the BFF sends and decodes are declared by the pinned schemas) cover the new surface against
  `testdata/delivery-promise.openapi.yaml`; a further test checks that the BFF's own schemas accept the fixtures it
  relays and composes.
- The estimate relay does not shield the page from a producer change (as for catalog reads, ADR 0004): a new field
  reaches the page; the composed delivery-dates view still needs a change here for each new field (ADR 0005).
- storefront-web is the only consumer. It sends `POST /api/storefront/delivery-estimates` with `postalPrefix` and the
  visible items, reads `options[].window.start`/`end` in `window.timeZone`, may show `capacityBand` as a scarcity hint,
  and must ignore a tier or property it does not know. Its adoption is a separate change.
- warehouse-docs' copy of this repository's OpenAPI needs the same refresh as any other contract change.
