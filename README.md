# storefront-bff

Storefront (e-commerce) domain of the retail-network organisation, bounded context **Storefront BFF** (Generic, storefront tier).

Go backend-for-frontend: the only storefront edge (/api/storefront). Composes the six storefront contexts and the network OHS routes, mints Idempotency-Keys per user action, keeps cart/shopper ids in a cookie (state, not authentication), passes saved addresses through without storing or logging them. No database. Never logs request/response bodies.

Status: under construction. The design lives in [warehouse-docs](https://iqvo.github.io/warehouse-docs/storefront/) `storefront/model/model.yaml` (single source of truth).
This repository is part of the storefront domain: it consumes the retail network's Open Host Service routes and its
published `retailnetwork.*` events; it never calls an in-org or restricted network route.

Runtime: Go, hexagonal, real PostgreSQL and Kafka only (`DATABASE_URL`, `KAFKA_BROKERS` are mandatory), CloudEvents 1.0 on every message
(`com.retailnetwork.storefront.storefront-bff.<entity>.<Event>`), transactional outbox.

## Disclaimer

This project is a domain-driven design study. It is not associated with, endorsed by or affiliated with any company, and it uses no company's code, data, logos or trademarks. All names, figures and examples are illustrative or synthetic.

See `docs/adr/` for decisions and `docs/before-first-release.md` for the packaging checklist.
