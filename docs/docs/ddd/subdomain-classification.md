---
id: subdomain-classification
title: Subdomain classification
sidebar_label: Subdomain classification
sidebar_position: 2
description: "Why the Storefront BFF is a Generic subdomain in the Storefront area, with neighbour classifications copied from the warehouse-docs strategic design."
---

# Subdomain classification

`storefront-bff` is a **Generic** subdomain in the **Storefront** area
(application portfolio, 2026-10-31). It is the storefront's experience-layer
edge: it holds no domain model, no aggregates and no data. The neighbour rows
below are copied from the aggregator's
[subdomain classification](https://iqvo.github.io/warehouse-docs/storefront/strategic-design/subdomain-classification)
and [context map](https://iqvo.github.io/warehouse-docs/storefront/strategic-design/context-map)
(`origin/main`), not recalled.

## Why Generic

- **Not Core.** Nothing here differentiates the business. The rules it enforces
  (a closed upstream allowlist, per-route time budgets, deterministic
  `Idempotency-Key`s, a state cookie, a card-data guard, problem+json) are the
  hygiene any competent backend-for-frontend has. The competitive logic lives
  upstream: the price of speed per tier in `pricing` (Core) and the promise in
  `delivery-promise` (Core).
- **Not Supporting.** It owns no business concept the storefront must get
  uniquely right: no cart, no checkout, no catalogue. It has no invariant
  beyond "compose faithfully and refuse what must not leave the process".
- **Generic.** A gateway or BFF framework could do the same job; the code is
  custom only because the six upstream contracts are. It is cheap to replace
  and should stay thin. Evolution: custom-built plumbing, business model:
  conversion enabler (every shopper request passes through it).

The complexity is low: one pure package (`internal/domain/composition`, 69
mutants), 23 use cases that each translate one route into one to three
allowlisted calls, and no state. See the
[Core domain chart](/docs/ddd/core-domain-chart).

What would move it: an edge-owned capability with its own data (a recommendation
or personalisation layer, a server-side cart cache, a price comparison)
would give it a model of its own and a reason to be classified again.

## Bounded context or not

[ADR 0001](/docs/adr/0001-storefront-bff-bounded-context) records the BFF as "a
bounded context of the storefront domain" (it needs a name, a repo and a
CloudEvents namespace like every service). The aggregator's strategic design
treats the experience layer (`storefront-bff`, `storefront-web`) as **not** one
of the six storefront bounded contexts, because it carries no domain model. Both
statements are compatible with this page: the BFF is a deployable with a
boundary, and its subdomain class is Generic. The pack pages therefore describe
composition and edge rules and say "none" for aggregates, entities and events.

## Neighbours (from the strategic design)

| Bounded context | Classification | Tier | Relation to this service |
| --- | --- | --- | --- |
| `product-discovery` | Supporting | storefront | upstream REST: search, product, categories |
| `shopping-cart` | Supporting | storefront | upstream REST: cart reads and commands |
| `checkout` | Supporting | storefront | upstream REST: open, poll, confirm |
| `pricing` | Core | storefront | upstream REST: the current delivery charge schedule only |
| `product-content` | Generic | storefront | no edge (reached through product-discovery) |
| `customer-payments` | Generic | storefront | no edge (reached through checkout) |
| `delivery-promise` | Core | network | upstream REST: the delivery quote |
| `customer-ordering` | Generic | network | upstream REST: orders, tracking, shoppers, addresses |
| `storefront-web` | experience layer, not a bounded context | storefront | the only caller |

Count in the storefront: 3 Supporting, 2 Generic, 1 Core context, plus the
experience layer without a class of its own in the aggregator; this page
classifies the BFF Generic.

## Tier is not classification

`storefront` and `network` group contexts by area, not by Core, Supporting or
Generic. The BFF is storefront-tier and Generic; `pricing` is storefront-tier and
Core.
