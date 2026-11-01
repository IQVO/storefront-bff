---
id: ubiquitous-language
title: Ubiquitous language
sidebar_label: Ubiquitous language
sidebar_position: 20
description: "Every term of the Storefront BFF with the code identifier that implements it, and the words not to use."
---

# Ubiquitous language

Every term of the context with the code identifier that implements it. Use these
exact names in code, tests, ADRs and conversation. Terms marked *(contract)* are
fixed by `apis/openapi.yaml` or an ADR rather than by a Go identifier.

| Term | Meaning | Code or contract |
| --- | --- | --- |
| **Edge** | The one HTTP surface of the storefront, `/api/storefront`. | `http.BasePath`, `inbound.Server` |
| **Upstream** | One of the six services the BFF calls. | `composition.Service` (`ProductDiscovery`, `DeliveryPromise`, `ShoppingCart`, `Checkout`, `CustomerOrdering`, `Pricing`) |
| **Call** | One outbound request to an upstream: service, method, path, query, body, key. | `composition.Call` |
| **Reply** | An upstream answer kept raw (status, content type, body), so problem+json passes through. | `composition.Reply`, `Reply.OK` |
| **Allowlist** | The closed list of `METHOD /path` templates the storefront may call; anything else is refused before a request is built. | `composition.Allowed`, `AllowedRoutes`, `allowed` |
| **Restricted route** | A network route the storefront never calls (ship-to, promise create/confirm/cancel, inventory feeds, policy). | `TestNoInOrgNetworkRoute` |
| **Gateway** | The set of composition use cases over the upstream port. | `usecases.Gateway`, `NewGateway` |
| **Use case** | One method of `Gateway`, one per public route. | [Use cases](/docs/ddd/use-cases) |
| **Relay** | Forward one upstream answer untouched. | `Gateway.relay`, `Server.respond` |
| **Composition** | A route that joins more than one upstream call, with a required leg and optional legs. | `DeliveryDates`, `GetOrder` |
| **Outcome** | A use case result: the reply to relay and the cookie state to store, if it changed. | `usecases.Outcome` |
| **Actor** | Who a request acts for: the cookie state plus the page's action id. | `usecases.Actor` |
| **Port** | The one outbound interface of the application layer. | `ports.Upstream` |
| **Budget** | The whole-request time limit of a route class; expiry is a 504, never retried. | `composition.Budgets`, `DefaultBudgets` |
| **Class (of a route)** | Query (2 s), Command (5 s) or Confirm (15 s). | `composition.Class`, `Query`, `Command`, `Confirm` |
| **Idempotency-Key** | The header the BFF mints for every non-GET upstream call. | `composition.MintKey`, `Call.IdempotencyKey` |
| **Key window** | The 10-minute retry window inside which an identical request mints the same key. | `composition.KeyWindow` |
| **Action id** | The optional `X-Action-Id` header a page sets per click; it pins the key and ignores body and window. | `Actor.ActionID`, `X-Action-Id` *(contract)* |
| **Subject** | The stable owner part of a key: the cookie's shopper, else cart, else `anonymous`. | `Actor.subject` |
| **State (cookie)** | The shopper's browsing state: open cart, shopper and last checkout session ids. State, not authentication. | `composition.State`, `CookieName` (`sf_state`) |
| **Valid id** | 1 to 128 characters of `[A-Za-z0-9._-]` starting with an alphanumeric; it cannot alter an upstream path. | `composition.ValidID` |
| **Shopper** | The customer-ordering record the cookie points to; created by registering, found by e-mail. | `State.ShopperID`, `RegisterShopper`, `Login` |
| **Session (route)** | `POST /sessions`: an e-mail lookup that stores the shopper id. There is no password and no authentication. | `usecases.Login`, `createSession` *(contract)* |
| **Cart** | The shopping-cart resource; its id rides in the cookie as `c`. | `State.CartID` |
| **Checkout session** | The checkout resource; its id rides as `k` and lets order detail join totals. | `State.CheckoutID` |
| **Delivery dates** | The product-page answer: one tier per quoted offer with its window, charge and time zone. | `usecases.DeliveryDates`, `composeDates`, `dateTier` |
| **Tier** | A delivery speed tier of the network (for example `NEXT_DAY`). | `quotedOffer.Tier`, `tierCharge` |
| **Postal prefix** | 1 to 10 letters, digits, spaces or dashes: a prefix, never an address. | `validPostalPrefix` |
| **Payment method token** | The PSP token the page sends on confirm; the only payment datum the BFF accepts. | `confirmRequest.PaymentMethodToken` |
| **Card guard** | Refusal of card-shaped JSON keys and Luhn-valid tokens. | `IsCardFieldName`, `LooksLikeCardNumber` |
| **Problem** | An RFC 9457 `application/problem+json` answer the edge itself produces, `type` under `/problems/`. | `problems.go`, `problemKind` |
| **Pass-through** | A body the BFF forwards byte for byte without storing or logging it (cart, shopper and address bodies). | ADR 0002 A10 |
| **Degrade** | An optional leg fails and is left out of the reply: tracking, order totals, the delivery charge. | `GetOrder`, `chargesByTier` |
| **Prefix strip** | Routes answer with or without `/api/storefront`. | `stripBase` |

## Words not to use

| Do not say | Say | Why |
| --- | --- | --- |
| log in, authenticate, auth token | look up a shopper by e-mail, state cookie | Nothing is authenticated fleet-wide; `sf_state` grants no access. |
| session (for the cookie) | state cookie | "session" names the `POST /sessions` lookup and the checkout session; the cookie holds ids only. |
| cache | relay | The BFF caches nothing (ADR 0004 defers caching to a fleet-wide decision). |
| order service, cart service (as the BFF's own code) | upstream | The BFF owns no cart, order or price. |
| retry (a budget) | re-send with the same action id | A budget expiry is never retried by the BFF; a client retry is safe because the key repeats. |
| card number, PAN, CVC | payment method token | The BFF refuses card data by design. |
