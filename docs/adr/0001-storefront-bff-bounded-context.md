# 1. Storefront BFF is a bounded context of the storefront domain

Status: Accepted (bootstrap). Design source: warehouse-docs `storefront/model/model.yaml`, context `storefront-bff`.

## Context

The storefront is the e-commerce domain of the retail-network organisation: six bounded contexts, each its own
service and database. Go backend-for-frontend: the only storefront edge (/api/storefront). Composes the six storefront contexts and the network OHS routes, mints Idempotency-Keys per user action, keeps cart/shopper ids in a cookie (state, not authentication), passes saved addresses through without storing or logging them. No database. Never logs request/response bodies.

## Decision

- This repo owns the aggregates Composition; the invariants and error names are those of the model.
- Business decisions of the context:
- No domain model and no database: composition and edge rules only.
- Never logs bodies (TestBFFNeverLogsBodies).
- Calls only OHS storefront routes of the network (TestNoInOrgNetworkRoute).
- No PAN/CVC-shaped data anywhere (TestNoCardDataAnywhere).
- Events use the storefront CloudEvents namespace `com.retailnetwork.storefront.storefront-bff.<entity>.<Event>` on `retailnetwork.storefront-bff.events`
  (analytics-only events on `.analytics`). Published: none (read side). Consumed: none. No `warehouse.*` topic is consumed or produced.
- The storefront calls only the network's Open Host Service routes (model inbound rows for `shopper / storefront`),
  never an in-org or restricted route; sibling data used in a decision is a local event-fed read model, except the
  calls the model lists under `sync_calls`.
- No authentication on REST, consistent with the fleet and the network. This context stores no PII and no card data.

## Consequences

Relationships and the full consumed-event list are in the model and in the AsyncAPI contract added with the first feature PR.
