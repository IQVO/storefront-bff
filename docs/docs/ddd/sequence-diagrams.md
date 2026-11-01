---
id: sequence-diagrams
title: Sequence diagrams
sidebar_label: Sequence diagrams
sidebar_position: 21
description: "Sequence diagrams of the Storefront BFF's key flows: add to cart with a retry, delivery dates with a degraded leg, confirm checkout and order detail with joined tracking and totals, each with error branches."
---

# Sequence diagrams

Four flows, with their error branches, drawn from the use cases and the HTTP
adapter. Participants are the browser app, the BFF (`cmd/api`) and the upstream
services. Problem names are the `/problems/` slugs of the edge.

## Create a cart and add a line (with a retried click)

```mermaid
sequenceDiagram
    autonumber
    participant W as storefront-web
    participant B as storefront-bff
    participant C as shopping-cart
    W->>B: POST /carts (cookie s=shopper)
    B->>B: empty body adopts the cookie shopper, budget 5 s
    B->>C: POST /carts with Idempotency-Key sf-...
    C-->>B: 201 cartId
    B-->>W: 201 relayed, Set-Cookie sf_state c=cartId, s=shopper
    W->>B: POST /carts/c1/lines with X-Action-Id a1
    B->>C: POST /carts/c1/lines with key from route, subject, a1
    alt upstream answers
        C-->>B: 200 cart
        B-->>W: 200 relayed
    else connection reset or budget expired
        B-->>W: 503 upstream-unavailable or 504 gateway-timeout
        W->>B: same POST with the same X-Action-Id a1
        B->>C: same Idempotency-Key, so the cart replays its stored answer
        C-->>B: 200 cart
        B-->>W: 200 relayed
    end
```

Source: `internal/application/usecases/carts.go` (`CreateCart`, `AddLine`,
`cartCommand`), `gateway.go` (`call` mints the key, `classify`),
`internal/domain/composition/idempotency.go` (`MintKey`),
`internal/adapters/inbound/http/server.go` (`respond`, `cookie`).

## Delivery dates on a product page (one leg optional)

```mermaid
sequenceDiagram
    autonumber
    participant W as storefront-web
    participant B as storefront-bff
    participant D as delivery-promise
    participant P as pricing
    W->>B: POST /products/p1:delivery-dates with postalPrefix 787
    alt body not strict JSON or prefix invalid
        B-->>W: 422 validation-failed
    else valid
        B->>D: POST /delivery-offers:quote one unit of p1
        alt quote fails
            D-->>B: non-2xx
            B-->>W: the upstream answer relayed untouched
        else quote ok
            D-->>B: 200 offers and unavailable
            B->>P: GET /delivery-charge-schedules/current
            alt schedule ok
                P-->>B: 200 tiers with charge and freeOver
            else schedule fails
                P-->>B: error or non-2xx
                Note over B: the charge is simply left out
            end
            B-->>W: 200 postalPrefix, tiers with window, timeZone, charge, and unavailable
        end
    end
```

Source: `internal/application/usecases/catalog.go` (`DeliveryDates`),
`dates.go` (`chargesByTier`, `composeDates`: only `QUOTED` offers become tiers),
[ADR 0005](/docs/adr/0005-delivery-dates-carry-the-window-time-zone) (time zone
relayed as quoted). The two calls share one 2 s query budget.

## Confirm checkout (the purchase step)

```mermaid
sequenceDiagram
    autonumber
    participant W as storefront-web
    participant B as storefront-bff
    participant K as checkout
    W->>B: POST /checkout/k1:confirm with paymentMethodToken
    alt card-shaped key, unknown field, or token that passes Luhn
        B-->>W: 422 validation-failed, nothing leaves the process
    else token accepted
        B->>K: POST /checkout-sessions/k1:confirm with Idempotency-Key, budget 15 s
        alt placed
            K-->>B: 200 session with orderId and totals
            B-->>W: 200 relayed
        else placement in doubt
            K-->>B: 202
            B-->>W: 202 relayed
            loop until it settles
                W->>B: GET /checkout/k1
                B->>K: GET /checkout-sessions/k1
                K-->>B: session
                B-->>W: session relayed
            end
        else checkout refuses, for example 409 or 422
            K-->>B: problem+json
            B-->>W: the same status and problem relayed
        else budget expired
            B-->>W: 504 gateway-timeout, not retried by the BFF
        end
    end
```

Source: `internal/application/usecases/checkout.go` (`ConfirmCheckout`: card-key
probe, strict decode, token length and Luhn check), `gateway.go` (`relay`, the
Confirm budget), `internal/domain/composition/card.go`. The 202 behaviour is the
`GetCheckout` comment in `checkout.go`; its body is the upstream's.

## Order detail with tracking and totals

```mermaid
sequenceDiagram
    autonumber
    participant W as storefront-web
    participant B as storefront-bff
    participant O as customer-ordering
    participant K as checkout
    W->>B: GET /orders/o1 (cookie k=k1)
    B->>O: GET /orders/o1
    alt order missing or not 2xx
        O-->>B: 404 or other
        B-->>W: relayed untouched
    else order ok
        O-->>B: 200 order
        B->>O: GET /orders/o1/tracking
        Note over B,O: failure or non-JSON is ignored, tracking is left out
        B->>K: GET /checkout-sessions/k1
        Note over B,K: totals join only if the session orderId equals o1
        B-->>W: 200 order plus tracking and totals when available
    end
```

Source: `internal/application/usecases/orders.go` (`GetOrder`, `checkoutTotals`).
All three calls share one 2 s query budget. If the order body is not a JSON
object the answer is `503 upstream-unavailable`.
