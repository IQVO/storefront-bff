---
id: domain-message-flow
title: Domain message flow
sidebar_label: Domain message flow
sidebar_position: 15
description: "Domain message flow modelling of the Storefront BFF: browse and delivery dates, cart commands, checkout confirm and order detail as numbered commands and queries between the browser, the BFF and the upstreams."
---

# Domain message flow

[Domain Message Flow Modelling](https://github.com/ddd-crew/domain-message-flow-modelling)
of the key scenarios: commands and queries as numbered messages between
collaborators. Every collaborator is code on `develop`: the HTTP adapter, the
`Gateway` use cases, the domain rules and the REST client; on the other side the
upstream services expose the routes used. **There are no events** in any flow:
the BFF is a synchronous edge ([Domain events](/docs/ddd/domain-events)).
Branches that end in a problem are in
[Sequence diagrams](/docs/ddd/sequence-diagrams) and
[Troubleshooting](/docs/operations/troubleshooting).

## Flow 1: browse and ask for delivery dates

```mermaid
flowchart LR
    web(["storefront-web"])
    http["inbound/http"]
    gw["Gateway use cases"]
    dom["composition: Allowed, Budgets"]
    pd["product-discovery"]
    dp["delivery-promise"]
    pr["pricing"]
    web -->|"1. qry: GET /products?q=lamp"| http
    http -->|"2. SearchProducts, only the known filters"| gw
    gw -->|"3. Allowed? budget 2 s"| dom
    gw -->|"4. qry: GET /products"| pd
    pd -->|"5. reply relayed untouched"| web
    web -->|"6. qry: POST /products/p1/delivery-date-quotes"| http
    http -->|"7. DeliveryDates"| gw
    gw -->|"8. qry: POST /delivery-quotes"| dp
    gw -->|"9. qry: GET /delivery-charge-schedules/current"| pr
    gw -->|"10. tiers with window, timeZone, charge"| web
```

Source: `internal/application/usecases/catalog.go`, `dates.go`,
`internal/domain/composition/allowlist.go`. Step 9 is optional: a failure drops
only the charge.

## Flow 2: build a cart

```mermaid
flowchart LR
    web(["storefront-web"])
    gw["Gateway use cases"]
    key["composition.MintKey"]
    sc["shopping-cart"]
    ck["sf_state cookie"]
    web -->|"1. cmd: POST /carts"| gw
    gw -->|"2. mint Idempotency-Key"| key
    gw -->|"3. cmd: POST /carts"| sc
    sc -->|"4. cartId"| gw
    gw -->|"5. set c=cartId"| ck
    ck -->|"6. Set-Cookie with the reply"| web
    web -->|"7. cmd: POST /carts/c1/lines, Idempotency-Key"| gw
    gw -->|"8. cmd: POST /carts/c1/lines"| sc
    web -->|"9. cmd: POST /carts/c1/destination"| gw
    gw -->|"10. cmd: PUT /carts/c1/destination"| sc
    web -->|"11. cmd: POST /carts/c1/delivery-quotes"| gw
    gw -->|"12. cmd: POST /carts/c1/delivery-quotes"| sc
    web -->|"13. cmd: PUT /carts/c1/delivery-selection"| gw
    gw -->|"14. cmd: PUT /carts/c1/delivery-selection"| sc
```

Source: `internal/application/usecases/carts.go`, `gateway.go`,
`composition/idempotency.go`, `internal/adapters/inbound/http/routes_cart.go`.
Every command carries a minted key; the cart service calls delivery-promise
itself on step 12, hence the 5 s command budget.

## Flow 3: open checkout and confirm

```mermaid
flowchart LR
    web(["storefront-web"])
    gw["Gateway use cases"]
    guard["composition card guard"]
    ck["checkout"]
    ck0["sf_state cookie"]
    web -->|"1. cmd: POST /checkout shipToAddressId"| gw
    gw -->|"2. cart and shopper from the cookie"| ck0
    gw -->|"3. cmd: POST /checkout-sessions"| ck
    ck -->|"4. sessionId"| gw
    gw -->|"5. set k=sessionId"| ck0
    web -->|"6. cmd: POST /checkout/k1/confirmations paymentMethodToken"| gw
    gw -->|"7. refuse card-shaped data"| guard
    gw -->|"8. cmd: POST /checkout-sessions/k1/confirmations, budget 15 s"| ck
    ck -->|"9. 200 or 202, relayed"| web
    web -->|"10. qry: GET /checkout/k1 while 202"| gw
    gw -->|"11. qry: GET /checkout-sessions/k1"| ck
```

Source: `internal/application/usecases/checkout.go`, `composition/card.go`.
Step 2 fails with `session-required` when the cookie has no shopper.

## Flow 4: read and cancel an order

```mermaid
flowchart LR
    web(["storefront-web"])
    gw["Gateway use cases"]
    co["customer-ordering"]
    ck["checkout"]
    web -->|"1. qry: GET /orders/o1"| gw
    gw -->|"2. qry: GET /orders/o1"| co
    gw -->|"3. qry: GET /orders/o1/tracking"| co
    gw -->|"4. qry: GET /checkout-sessions/k1 from the cookie"| ck
    gw -->|"5. order plus tracking plus totals"| web
    web -->|"6. cmd: POST /orders/o1/cancellations reason"| gw
    gw -->|"7. cmd: POST /orders/o1/cancellations"| co
    co -->|"8. reply relayed"| web
```

Source: `internal/application/usecases/orders.go`. Steps 3 and 4 are optional and
share the 2 s query budget with step 2; the cancel reason is 1 to 200
characters.
