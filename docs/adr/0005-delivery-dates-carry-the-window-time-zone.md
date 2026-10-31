# ADR 0005: Delivery dates carry the window's time zone

## Status

Accepted (2026-10-30).

## Context

A delivery window is a span of instants, but a shopper reads it as local days
and hours of the DESTINATION. A next-day window in Chicago quoted as
`2026-10-31T14:00:00Z`..`2026-11-01T02:00:00Z` is all of Saturday Oct 31 for
the shopper (9 AM - 9 PM CDT); read in UTC it spans two days, and read in the
browser's or the server's zone it is whatever that machine happens to be in.

delivery-promise (its ADR 0011) added an optional IANA `timeZone` to the
`DeliveryWindow` of quoted offers (`POST /delivery-offers:quote`) and to the
promise terms; it is absent only for rows stored before, which are read as UTC.
customer-ordering is adding the same optional `timeZone` to its order groups
(`GET /orders`, `GET /orders/{id}`, `GET /orders/{id}/tracking`) and to the
requoted offers on the 409 offer-not-quotable answer of checkout.

The BFF touches promise windows in two ways:

- `POST /products/{id}:delivery-dates` is COMPOSED (`composeDates` in
  `internal/application/usecases/dates.go`): it decodes the quote into a typed
  struct and re-emits one tier per quoted offer with `deliverByEarliest`,
  `deliverByLatest`, `offerExpiresAt` and the charge. Any field it does not
  name is dropped, so the new zone would never reach the product page.
- Orders, tracking and checkout are RELAYS: `ListOrders`, `CancelOrder`,
  `OpenCheckout`, `GetCheckout` and `ConfirmCheckout` hand the upstream body
  through untouched; `GetOrder` decodes the order into a
  `map[string]json.RawMessage`, adds `tracking`/`totals`, and re-encodes it,
  which keeps every upstream field.

Alternatives weighed:

- **Resolve the zone in the BFF and send local dates instead of instants.**
  It would make the BFF own presentation (which days, which hours, which
  locale) and a tz database, against ADR 0001's "no domain model" edge, and
  the page would lose the instants it needs for hour ranges.
- **Derive the zone from the postal prefix in the BFF.** Duplicates the lookup
  delivery-promise already does, and two lookups can disagree.
- **Leave delivery-dates alone and let the page assume a zone.** That is the
  bug being fixed.

## Decision

- `composeDates` reads `window.timeZone` from each QUOTED offer and emits it as
  an optional `timeZone` on the tier, next to `deliverByEarliest`/
  `deliverByLatest`. It is omitted (`omitempty`) when the quote has none; the
  page then reads the window as UTC, matching delivery-promise's rule for old
  rows. The BFF relays the value as is: it does not validate the name, load a
  tz database, or turn instants into dates.
- `apis/openapi.yaml` documents `timeZone` on `DeliveryDates.tiers[]`
  (optional, not in `required`), so the change is additive and a page that
  ignores it keeps working.
- The order, tracking and checkout relays stay passthrough: no field
  allowlist is introduced, so customer-ordering's `timeZone` on groups,
  tracking groups and `requotedOffers` reaches the page unchanged. Tests pin
  this (`TestListOrdersRelaysGroupTimeZoneUnchanged`,
  `TestGetOrderKeepsGroupAndTrackingTimeZone`,
  `TestOpenCheckoutRelaysRequotedOfferTimeZone`), alongside
  `TestDeliveryDatesRelaysWindowTimeZone` for the composed tiers.

## Consequences

- storefront-web gets one zone per promise window on every surface it shows
  (buy box, orders list, order detail, tracking, requoted offers) and formats
  them with it; the BFF stays free of time-zone logic.
- An unknown or misspelled zone name reaches the page as sent; the page must
  fall back to UTC for a name its `Intl` implementation rejects.
- delivery-dates remains a composed view: the next field delivery-promise adds
  to a quoted offer still needs a change here before the page sees it. The
  relays, conversely, give the page every upstream change unshielded, as
  ADR 0004 already notes for catalog reads.
- warehouse-docs' copy of this repo's OpenAPI needs the same refresh as any
  other contract change.
