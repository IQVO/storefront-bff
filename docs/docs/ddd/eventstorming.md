---
id: eventstorming
title: EventStorming
sidebar_label: EventStorming
sidebar_position: 16
description: "Design-level EventStorming of the Storefront BFF: shopper commands, the policies the edge applies, external upstream systems, read models and hotspots, with no domain events of its own."
---

# EventStorming

Design-level EventStorming of a purchase journey, using the
[ddd-crew glossary](https://github.com/ddd-crew/eventstorming-glossary-cheat-sheet)
colours: blue commands, lilac policies, pink external systems, green read models,
red hotspots. There are **no orange domain events**: the BFF raises none
([Domain events](/docs/ddd/domain-events)). The facts of a purchase (cart
changed, order placed, payment captured) are events of the upstream contexts and
appear here as pink external systems answering a command. The model was derived
from the code and ADRs, not from a workshop.

```mermaid
flowchart LR
    classDef command fill:#74c0fc,stroke:#1971c2,color:#000
    classDef policy fill:#d0bfff,stroke:#6741d9,color:#000
    classDef ext fill:#faa2c1,stroke:#c2255c,color:#000
    classDef read fill:#8ce99a,stroke:#2b8a3e,color:#000
    classDef hot fill:#ff8787,stroke:#c92a2a,color:#000

    shopper["Shopper on storefront-web"]:::ext

    c1["Search products, list categories"]:::command
    c2["Ask delivery dates for a postal prefix"]:::command
    c3["Create cart, add line, set destination, quote and select delivery"]:::command
    c4["Open checkout"]:::command
    c5["Confirm checkout with a PSP token"]:::command
    c6["Read or cancel an order"]:::command
    c7["Register or look up a shopper, save an address"]:::command

    p1["Whenever a call is not on the allowlist, refuse it before building the request"]:::policy
    p2["Whenever a call is a command, mint a deterministic Idempotency-Key"]:::policy
    p3["Whenever the whole route exceeds its budget, answer 504 and do not retry"]:::policy
    p4["Whenever the confirm body looks like card data, refuse it with 422"]:::policy
    p5["Whenever an upstream replies with an id, keep it in the sf_state cookie"]:::policy
    p6["Whenever an optional leg fails, leave it out of the reply"]:::policy

    u1["product-discovery"]:::ext
    u2["delivery-promise and pricing"]:::ext
    u3["shopping-cart"]:::ext
    u4["checkout"]:::ext
    u5["customer-ordering"]:::ext

    r1["Product and category views, relayed"]:::read
    r2["Delivery dates view: tiers, windows, charges"]:::read
    r3["Order view: order plus tracking plus totals"]:::read

    h1{{"HOTSPOT: a 202 on confirm means placement in doubt, the page must poll"}}:::hot
    h2{{"HOTSPOT: readiness never probes the upstreams"}}:::hot
    h3{{"HOTSPOT: the page must reuse its Idempotency-Key on a retry, or the retry is a new action"}}:::hot

    shopper --> c1 --> p1 --> u1 --> r1
    shopper --> c2 --> p1
    c2 --> u2 --> r2
    p6 -.-> r2
    shopper --> c3 --> p2 --> u3
    u3 --> p5
    shopper --> c7 --> p2 --> u5
    u5 --> p5
    shopper --> c4 --> p2 --> u4
    u4 --> p5
    shopper --> c5 --> p4 --> u4
    shopper --> c6 --> u5 --> r3
    p6 -.-> r3
    p3 -.-> c3
    p3 -.-> c5
    h1 -.-> c5
    h2 -.-> p3
    h3 -.-> p2
```

Source: `internal/domain/composition/{allowlist,budget,idempotency,card,state}.go`,
`internal/application/usecases/*.go` (`call`, `relay`, `begin`, `adoptShopper`,
`GetOrder`, `DeliveryDates`), `internal/adapters/inbound/http/server.go` (`respond`),
`docs/adr/0001` to `0005`. Arrows show the order of a journey, not data
ownership.

## Reading it

1. **Browse.** Search, product and category reads are plain relays to
   product-discovery under the 2 s query budget.
2. **Delivery dates.** One composed view: the quote is required, the charge
   schedule is optional (policy 6).
3. **Build the cart and register.** Commands carry a minted key (policy 2); the
   ids that come back go into the cookie (policy 5).
4. **Open checkout and confirm.** Open needs a shopper and a cart from the cookie.
   Confirm refuses card data before anything leaves (policy 4) and has the 15 s
   budget (policy 3).
5. **After the purchase.** Order detail is a composed view with an optional
   tracking leg and totals leg (policy 6).

## Hotspots

| # | Hotspot | State |
| --- | --- | --- |
| 1 | **A `202` on confirm.** The payment outcome is in doubt; the page polls `GET /checkout/{id}` and may re-send the same confirm with the same `Idempotency-Key`. The BFF does not decide. | By design ([ADR 0002](/docs/adr/0002-storefront-bff-assumptions)) |
| 2 | **Readiness is a static 200.** A dead upstream shows up as 503/504 and on Kong's 5xx ratio, never as a pod restart (A8), so one slow upstream cannot take the whole edge out of rotation. | Accepted by ADR 0002 A8 |
| 3 | **Page-owned key.** The page sends one `Idempotency-Key` per user submit and reuses it on retry; a retry with a fresh key is a new action. The same key with a different body is answered 409 `idempotency-key-reused` by the provider. | By design ([ADR 0006](/docs/adr/0006-adopt-the-fleet-rest-api-guideline), supersedes ADR 0002 A3) |
| 4 | **Relays are unshielded.** An upstream field change reaches the page untouched, except in the two composed routes (ADR 0004, ADR 0005). | Accepted |
| 5 | **No event, no push.** The page cannot learn that an order changed without asking again. | By design; revisit only with a real need |
