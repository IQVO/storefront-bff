# storefront-bff

Storefront (e-commerce) domain of the retail-network organisation, bounded context **Storefront BFF** (Generic, storefront tier).

Go backend-for-frontend: the only storefront edge (/api/storefront). Composes the six storefront contexts and the network OHS routes, mints Idempotency-Keys per user action, keeps cart/shopper ids in a cookie (state, not authentication), passes saved addresses through without storing or logging them. No database. Never logs request/response bodies.

Status: under construction. The design lives in [warehouse-docs](https://iqvo.github.io/warehouse-docs/storefront/) `storefront/model/model.yaml` (single source of truth).
This repository is part of the storefront domain: it consumes the retail network's Open Host Service routes and its
published `retailnetwork.*` events; it never calls an in-org or restricted network route.

Runtime: Go, hexagonal, stateless. No database and no Kafka: the BFF composes six upstream REST APIs
(`PRODUCT_DISCOVERY_URL`, `DELIVERY_PROMISE_URL`, `PRICING_URL`, `SHOPPING_CART_URL`, `CHECKOUT_URL`,
`CUSTOMER_ORDERING_URL` are mandatory boot config). Optional: `PORT` (8080), `COOKIE_SECURE` (true),
`BUDGET_QUERY_MS` / `BUDGET_COMMAND_MS` / `BUDGET_CONFIRM_MS` (2000 / 5000 / 15000). The surface is documented in
`apis/openapi.yaml`; the design assumptions are in `docs/adr/0002-storefront-bff-assumptions.md`.

## Disclaimer

This project is a domain-driven design study. It is not associated with, endorsed by or affiliated with any company, and it uses no company's code, data, logos or trademarks. All names, figures and examples are illustrative or synthetic.

See `docs/adr/` for decisions and `docs/before-first-release.md` for the packaging checklist.
