# CLAUDE.md — storefront-bff

`storefront-bff` is the **Storefront BFF** bounded context (Generic subdomain,
`storefront` tier) of the retail-network storefront domain (GitHub org `IQVO`,
Go module `github.com/IQVO/storefront-bff`). It is the only storefront edge
(`/api/storefront`, the prefix is optional on the wire): a Go
backend-for-frontend that composes six upstream services over REST
(product-discovery, delivery-promise, pricing, shopping-cart, checkout,
customer-ordering), mints the upstream Idempotency-Keys per user action and
keeps cart/shopper ids in a cookie. It has NO domain model, NO database and NO
Kafka runtime path; what it owns is composition and edge rules
(`internal/domain/composition`, pure, standard library only). One binary:
`cmd/api`.

## Hard rules (each one is fitness-tested or decided in an ADR)

1. **Hexagonal dependency rule.** `internal/domain` imports nothing outside the
   domain; `internal/application` only domain and application; inbound and
   outbound adapters never import each other; only `cmd/` wires layers
   (`TestHexagonalArchitecture`, `make arch-test`).
2. **Closed upstream allowlist**: the BFF calls only the routes of the
   composition allowlist (the network's Open Host Service routes plus the
   storefront contexts' own REST), never an in-org or restricted network route
   (`TestNoInOrgNetworkRoute`, `TestAllowlistHoldsNoRestrictedRoute`; ADR 0001).
3. **Upstream answers are relayed, not reinterpreted**: an upstream
   problem+json body passes through unchanged; only edge failures become edge
   problems (`upstream-unavailable` 503, `gateway-timeout` 504,
   `internal-error` 500 and the validation slugs in `.claude/rules/rest-api.md`).
4. **Time budgets, never retries**: query 2 s, command 5 s, confirm 15 s
   (`BUDGET_*_MS`); expiry is a 504 and nothing is retried. The outbound REST
   client keeps a circuit breaker per upstream host (open = immediate 503).
5. **Idempotency**: every POST and PATCH requires the page's `Idempotency-Key`
   (400 `idempotency-key-required` / `idempotency-key-invalid`, refused before
   any upstream call). The BFF stores nothing: it derives the upstream key with
   `composition.MintKey(route, subject, pageKey)` and relays the provider's
   `Idempotency-Replayed` and 409 `idempotency-key-reused` unchanged.
6. **Cookie `sf_state` is state, not authentication**: cart id, shopper id and
   last checkout id; HttpOnly, SameSite=Lax, path `/api/storefront`, Secure per
   `COOKIE_SECURE`.
7. **No auth layer** (fleet-wide revert 2026-09-11): never add bearer/JWT/API-key
   middleware (`TestNoAuthMiddlewareReintroduced`).
8. **Never log bodies and never touch card data**: the access log carries
   method, route pattern, status and duration only (`TestBFFNeverLogsBodies`);
   saved addresses pass through unstored and unlogged; only a PSP
   `paymentMethodToken` is accepted (`TestNoCardDataAnywhere`). Request bodies
   are capped at 1 MiB (413 `payload-too-large`), NUL bytes are refused.
9. **No events**: none published, none consumed. Adding a Kafka path needs an
   ADR (the CloudEvents rules in `.claude/rules/integration-events.md` would
   then apply, `TestCloudEventsOnly`).
10. **Real wiring only**: `cmd/*` never imports test doubles
    (`TestRuntimeBinaryHasNoInMemoryAdapters`); no Go file at the repo root
    (`TestNoGoFilesAtRepoRoot`).

## Commands

```bash
make check-fast   # fmt-check + vet + arch-test + tests of changed packages: run before saying "done"
make check        # fmt-check vet build lint test
make check-all    # check + coverage (90% gate) + arch-test + bdd
make bdd          # godog scenarios in features/ (runner: internal/bdd, scripted upstream)
make integration  # in-process run() wiring against httptest upstream fakes (no Docker)
make mutation     # gremlins on internal/domain/composition (CI job mutation-fast, blocking)
make guide-lint   # these guides: skills load, references resolve, context budget
```

Run locally: set the six upstream base URLs (`PRODUCT_DISCOVERY_URL`,
`DELIVERY_PROMISE_URL`, `PRICING_URL`, `SHOPPING_CART_URL`, `CHECKOUT_URL`,
`CUSTOMER_ORDERING_URL`, all required) and `go run ./cmd/api` (`PORT` 8080).

## Where things are written down (read before editing the matching area)

- `.claude/rules/domain-model.md` (always loaded): the composition vocabulary
  and the use cases. `.claude/rules/rest-api.md`: every edge route, the edge
  slugs, budgets and cookie. `.claude/rules/integration-events.md`: why there
  is no Kafka path.
- `.claude/rules/fleet/*.md`: fleet-wide rules copied from the harness
  template; never edit them here.
- `docs/adr/`: decisions (index: `docs/adr/index.md`). `apis/openapi.yaml`:
  the edge contract. Operating docs: `docs/docs/operations/` (configuration,
  runbook, observability, troubleshooting), `docs/docs/development/testing.md`,
  `docs/docs/overview/quickstart.md`. `HARNESS.md`: what each sensor runs and
  why. `INCIDENTS.md`: the fleet incident log behind the fitness tests.

harness-template: v4
(hooks, guide-lint).

<!-- harness:scoped-rules:start (generated by tools/migrate_v3.py in warehouse-harness-template; do not hand-edit) -->
## Scoped rules and harness

Claude Code loads each rule below automatically when you touch the matching paths. OpenCode and Codex do NOT: read the rule BEFORE editing matching files.

| When touching | Read |
|---|---|
| `internal/adapters/**/kafka/**`, `internal/adapters/outbound/events/**`, `apis/asyncapi*` | `.claude/rules/integration-events.md` |
| `internal/adapters/inbound/http/**`, `apis/openapi*.yaml`, `apis/openapi/**` | `.claude/rules/rest-api.md` |

Hooks (`scripts/harness/hook.py`, wired for Claude Code, Codex and OpenCode) block pushes to develop/main, `--no-verify`, bare `rm -rf`, and edits to generated files, and feed gofmt/vet findings back after each edit. Before saying "done" run `make check-fast`; the full gate is `make check-all`. `HARNESS_OFF=1` disables the hooks when debugging the harness itself.
<!-- harness:scoped-rules:end -->
