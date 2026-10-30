# ADR 0004: The BFF relays the category tree

## Status

Accepted (2026-10-29).

## Context

storefront-web is being redesigned as a full retail storefront with a
department navigation bar and a department facet on the search page. Both show
category NAMES. The BFF exposed only `GET /products` (whose `categoryId`
filter takes an id) and `GET /products/{id}`, so the page could only render raw
category ids. product-discovery already serves the category tree with
purchasable counts as `GET /categories` (the CategoryView:
`{version, categories: [{categoryId, parentId, name, path, purchasableCount}]}`),
but it was not on the BFF's closed upstream allowlist, and the BFF is the only
storefront edge — the browser never calls a context directly.

Alternatives weighed:

- **Derive categories from search results.** The page would collect
  `categoryId`s from `GET /products` items. It still has no names, sees only
  the categories on the current page of results, and cannot build a tree.
- **A composed or reshaped view in the BFF** (for example a pre-nested tree,
  or the tree joined with search facets). The BFF has no domain model by
  design (ADR 0001); reshaping would make it own a second category contract
  and drift from product-discovery's.
- **Let the browser call product-discovery directly.** Breaks the
  single-edge rule (ADR 0001) and the allowlist that guards it.

## Decision

We add `GET /categories` to the BFF (under `/api/storefront`, and unprefixed per
ADR 0002 A2), served by the `ListCategories` use case in
`internal/application/usecases/catalog.go`, exactly like the products relay:

- `GET /categories` is added to product-discovery's entry in the closed
  allowlist (`internal/domain/composition/allowlist.go`); only that exact path
  and method are allowed (`GET /categories/{id}` and `POST /categories` stay
  refused, pinned by `allowlist_test.go`).
- It is a query: the 2 s query budget applies, no Idempotency-Key is minted,
  nothing is retried, expiry answers 504 `gateway-timeout`, an unreachable
  upstream 503 `upstream-unavailable`, and upstream problems pass through.
- The upstream takes no parameters, so nothing from the incoming query string
  is forwarded.
- The upstream body is relayed unchanged (OpenAPI `Json` passthrough response,
  the same as `GET /products`). No cache headers are set, because the BFF sets
  none on any catalog read; caching stays a later, fleet-wide decision.
- Telemetry is the generic per-route one (ADR 0003): access log and spans by
  route pattern, the upstream call traced like every other.

## Consequences

- The web can render department names and a navigation tree from one
  call, and keep using `categoryId` as the `GET /products` filter.
- The BFF's surface grows by one read-only route and one allowlist entry; the
  model doc (warehouse-docs `storefront/model/model.yaml`) lists the new route
  and sync call so the design source stays in step.
- Because the body is relayed unchanged, any change to product-discovery's
  CategoryView reaches the page directly; the BFF does not shield the web from
  it, as with products.
- Each page load that shows navigation costs one upstream call; without a
  cache this is load on product-discovery, revisit if it shows up in the
  route's latency or upstream request rate.
