---
id: class-diagram
title: Class diagrams
sidebar_label: Class diagrams
sidebar_position: 18
description: "UML class diagrams of the Storefront BFF: the composition domain types, the Gateway use cases and the hexagonal port with the adapters that exist."
---

# Class diagrams

Two views. Names are the Go identifiers on `develop`. The domain has value types
and functions, no entity or aggregate
([Aggregate design canvas](/docs/ddd/aggregate-design-canvas)).

## Domain: `internal/domain/composition`

```mermaid
classDiagram
    class Service {
        <<string>>
        ProductDiscovery
        DeliveryPromise
        ShoppingCart
        Checkout
        CustomerOrdering
        Pricing
    }
    class Call {
        Service Service
        string Method
        string Path
        Values Query
        bytes Body
        string IdempotencyKey
    }
    class Reply {
        int Status
        string ContentType
        bytes Body
        OK() bool
    }
    class Budgets {
        Duration Query
        Duration Command
        Duration Confirm
        Validate() error
        For(Class) Duration
    }
    class Class {
        <<enum>>
        Query
        Command
        Confirm
    }
    class State {
        string CartID
        string ShopperID
        string CheckoutID
        Encode() string
    }
    class Allowlist {
        <<functions>>
        Allowed(Call) bool
        AllowedRoutes() map
    }
    class Keys {
        <<functions>>
        MintKey(route, subject, action, body, now) string
        KeyWindow 10 minutes
    }
    class CardGuard {
        <<functions>>
        IsCardFieldName(key) bool
        LooksLikeCardNumber(s) bool
    }
    class Errors {
        <<sentinels>>
        ErrInvalid
        ErrNoSession
        ErrUpstreamTimeout
        ErrUpstreamUnavailable
        ErrRouteNotAllowed
    }
    Call --> Service
    Budgets ..> Class : For
    Allowlist ..> Call : checks
    Keys ..> Call : mints IdempotencyKey
```

Source: `internal/domain/composition/call.go`, `budget.go`, `state.go`,
`allowlist.go`, `idempotency.go`, `card.go`, `errors.go`. `DecodeState` and
`ValidID` are package functions next to `State`.

## Application and adapters

```mermaid
classDiagram
    class Upstream {
        <<interface>>
        Do(ctx, Call) Reply, error
    }
    class Gateway {
        SearchProducts()
        ListCategories()
        GetProduct()
        DeliveryDates()
        CreateCart()
        GetCart()
        ListCarts()
        AddLine()
        SetLineQuantity()
        RemoveLine()
        SetDestination()
        QuoteDelivery()
        SelectDelivery()
        OpenCheckout()
        GetCheckout()
        ConfirmCheckout()
        ListOrders()
        GetOrder()
        CancelOrder()
        RegisterShopper()
        Login()
        Me()
        AddAddress()
    }
    class Actor {
        State State
        string ActionID
    }
    class Outcome {
        Reply Reply
        State State
        bool SetState
    }
    class Client {
        New(bases) Client
        Do(ctx, Call) Reply, error
    }
    class Server {
        NewServer(gateway, log, secure, serviceName)
        Handler() Handler
    }
    class Config {
        string Addr
        map Upstreams
        Budgets Budgets
        bool CookieSecure
        Load(getenv) Config
    }
    Gateway --> Upstream : uses
    Gateway ..> Actor : takes
    Gateway ..> Outcome : returns
    Client ..|> Upstream
    Server --> Gateway : routes to
    Config ..> Gateway : budgets
```

Source: `internal/application/ports/upstream.go`,
`internal/application/usecases/gateway.go` and the per-area files,
`internal/adapters/outbound/rest/client.go`,
`internal/adapters/inbound/http/server.go`, `internal/config/config.go`,
`cmd/api/main.go` (wiring). The request logger
(`internal/adapters/inbound/requestlog`) and the telemetry adapter
(`internal/adapters/outbound/telemetry`) are middleware and setup, not part of the
use-case model, and are left out. The fleet-template Kafka writer and CloudEvents
packages are not wired by `cmd/api` and are left out too.
