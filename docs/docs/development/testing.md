---
id: testing
title: Testing
sidebar_label: Testing
sidebar_position: 1
description: "The Storefront BFF test pyramid as it is: unit, httptest, in-process integration with upstream fakes, route/OpenAPI contract, mutation and architecture fitness tests, make targets and CI jobs."
---

# Testing

The test pyramid **as it is on `develop`**. The BFF is stateless and has no
database, so nothing here needs Postgres, Kafka or Docker: the integration suite
boots the real `run()` wiring in-process against `httptest` upstream fakes.
There are **no godog features** and no schemathesis job (see the table).

| Layer | Count and threshold | Command |
| --- | --- | --- |
| Unit, domain (`internal/domain/composition`) | 22 top-level tests: allowlist 3, budget 3, call 2, card 4, idempotency 6, state 4 | `make test` |
| Unit, use cases (`internal/application/usecases`) | 46: catalog 10, orders 8, checkout 7, carts 6, shoppers 6, boundaries 5, gateway 4, over a fake `ports.Upstream` | `make test` |
| Unit and httptest, adapters and config | `inbound/http` 17 (behaviour 12, routes 3, contract 1, log sanitising 1), `inbound/requestlog` 4, `outbound/rest` 8, `outbound/telemetry` 9, `outbound/kafka` 2 and `kafka/cloudevents` 6 (scaffold packages, see below), `internal/config` 4, `cmd/api` 3 | `make test` |
| Coverage gate | 90 % over `./internal/domain/...`, `./internal/application/...`, `./internal/adapters/inbound/...`, `./internal/adapters/outbound/rest/...`, `./internal/config/...` | `make coverage` |
| Integration, in-process | 7 top-level tests behind `//go:build integration`, all in `cmd/api`: shopper journey 1, composed contract and keys 1, never-calls-restricted-routes 1, card-data-never-reaches-checkout 1, budget expiry is 504 without retry 1, upstream problems pass through 1, unreachable upstream is 503 1 | `make integration` |
| Contract | `TestRoutesMatchOpenAPI` (`internal/adapters/inbound/http/contract_test.go`) checks the router against `apis/openapi.yaml`; Spectral lints the spec in CI | `make test`, CI `api-lint` |
| BDD (godog / Gherkin) | **none**: the repository has no `.feature` file and no godog dependency in the tree. `make bdd` (and the `bdd` CI job, which calls it) prints that there are no godog features and exits 0 without running any test | `make bdd` (notice only) |
| Architecture fitness | 35 top-level tests in `internal/architecture` | `make arch-test` |
| Mutation (gremlins) | efficacy 98, mutant coverage 97 (`.gremlins.yaml`); measured 100.00 % / 98.57 % over the 69 mutants of `./internal/domain` on 2026-10-26 | `make mutation`, `make mutation-full` |
| Contract fuzzing (schemathesis), evals | **none**: no configuration and no CI job | none |

The 163 top-level `Test` functions come from `grep '^func Test'` on this commit
(7 of them carry the `integration` build tag); many are table-driven, so the
case count is higher. Run `go test ./... -v` to see the cases.

`internal/adapters/outbound/kafka`, `internal/adapters/kafka/cloudevents` and the
MCP sensors in `internal/architecture` come from the fleet service template. The
BFF publishes and consumes nothing, so `cmd/api` wires none of the Kafka code
(see [Architecture](/docs/overview/architecture)); the tests are kept because the
template's fitness sensors expect them.

## Make targets

Every target mirrors a CI job; the `Makefile` is the only definition of the
coverage scope and the mutation package.

| Target | Runs | Notes |
| --- | --- | --- |
| `make help` | prints the target list | default target |
| `make build` | `go build ./...` | compiles `cmd/api` |
| `make vet` | `go vet ./...` | |
| `make fmt` / `make fmt-check` | `gofmt -w .` / fail if `gofmt -l .` is non-empty | |
| `make lint` | `golangci-lint run --allow-parallel-runners ./...` | pinned `v2.14.0`, config `.golangci.yml` |
| `make test` | `go test ./... -race` | unit and httptest; no Docker |
| `make coverage` | `go test ./... -race -coverprofile=coverage.out -coverpkg=<scope above>` then the 90 % gate | the CI `test` job runs exactly this |
| `make integration` | `go test -tags=integration ./... -race -count=1` | in-process, no Docker; not part of `check` |
| `make bdd` | prints `no godog features in this repo` | exits 0, runs nothing (no godog suite) |
| `make arch-test` | `go test ./internal/architecture/... -v` | |
| `make mutation` | `gremlins unleash ./internal/domain/composition` | the fast blocking subset (`MUTATION_FAST_PKG`) |
| `make mutation-full` | `gremlins unleash ./internal/domain --workers 1 --timeout-coefficient 30` | the exhaustive scheduled run |
| `make vuln` | `govulncheck ./...` | |
| `make check` | `fmt-check vet build lint test` | the lefthook pre-push hook runs this bundle (pre-commit runs `fmt-check`, `vet` and `lint`) |
| `make check-all` | `check coverage arch-test bdd` | run before pushing |
| `make check-fast` | `fmt-check vet arch-test` plus `go test` of the packages changed vs `HEAD` | the agent Stop hook |
| `make guide-lint` / `make harness-test` | `scripts/harness/guide_lint.py` / `scripts/harness/test_hook.py` | agent-guide checks, not product tests |

Run one package or one test:

```bash
go test ./internal/domain/composition -run TestMintKey -v
go test ./internal/application/usecases -v
go test ./cmd/api -tags=integration -run TestIntegrationShopperJourney -v
```

## Unit tests

- **Domain** (`internal/domain/composition`): pure, standard library only. They
  pin the route allowlist (including every rule that `ship-to`, promise
  confirm/cancel and policy routes stay refused), the time budgets (2 s, 5 s,
  15 s, non-positive rejected), the cookie state round trip and `ValidID`, the
  deterministic `Idempotency-Key` (same route, subject, action and window give
  the same key; a different body does not) and the card-number guard (Luhn,
  13 to 19 digits).
- **Use cases**: run over a fake upstream (`fake_test.go`) and assert the exact
  upstream calls, forwarded query parameters, minted keys, cookie state changes
  and the optional-leg degradation of order detail and delivery dates.
  `boundaries_test.go` pins input validation (ids, `postalPrefix`, reason length).
- **HTTP adapter**: `httptest` over the real router. `routes_test.go` checks that
  every route maps to its upstream call and that commands carry a minted key
  (and that `X-Action-Id` pins it); `behaviour_test.go` covers the state cookie,
  pass-through of upstream problems, the problem mapping, NUL and oversize body
  refusal, panic to 500, health endpoints and that the access log carries no
  body, query or cookie.
- **Upstream client** (`outbound/rest`): URL joining, headers, redirect refusal,
  the 4 MiB cap, and that bodies are never logged.
- **Telemetry and request log**: endpoint normalisation, the trace-aware log
  handler, log level by status and that probes are skipped.
- **`cmd/api`**: boot refuses missing upstream URLs, serves and shuts down, and
  fails when the port is taken (`main_test.go`).

## Integration (in-process)

`cmd/api/integration_*_test.go` carry `//go:build integration`. The `bff` helper
in `integration_helpers_test.go` calls the real `run()` with `PORT=0` and
`COOKIE_SECURE=false` against `httptest` servers that record every request
(method, path, query, `Idempotency-Key`, body) and answer from a
`METHOD path` table. Nothing else is started.

| Test | What it proves |
| --- | --- |
| `TestIntegrationShopperJourney` | the whole journey over the real wiring and cookie: categories, search, product, delivery dates, register, saved address, cart, line add and quantity change, destination, delivery quote and select, open checkout, confirm, then `GET /orders/o1` joined with tracking and the checkout total |
| `TestIntegrationComposedContractAndKeys` | a retried add-line command carries one minted `Idempotency-Key` to shopping-cart; the open-checkout body is composed from the cookie (`cartId`, `shopperId`, `shipToAddressId`); the delivery quote is a one-line quote for the postal prefix |
| `TestIntegrationNeverCallsInOrgRoutes` | no upstream receives a path containing `ship-to` or `/promises` during register, order read and cancel |
| `TestIntegrationCardDataNeverReachesCheckout` | a card number as token and a body with a `cvc` field both get 422; the checkout fake never sees the confirm |
| `TestIntegrationBudgetExpiryIs504WithoutRetry` | with `BUDGET_QUERY_MS=100` and a 2 s upstream the answer is 504 `gateway-timeout` in under a second and the upstream saw exactly one request |
| `TestIntegrationUpstreamProblemsPassThrough` | an upstream problem body and status reach the client untouched |
| `TestIntegrationUnreachableUpstreamIs503` | a dead upstream yields the 503 problem |

Architecture sensors (`TestKafkaIntegrationTestsUseTestcontainers`,
`TestPostgresIntegrationTestsUseTestcontainers`) keep any future integration test
off `DATABASE_URL`/`KAFKA_BROKERS`.

## Mutation testing

`.gremlins.yaml` sets `workers: 1`, `timeout-coefficient: 30` and the thresholds
`efficacy: 98` and `mutant-coverage: 97`. Gremlins fails when the measured value is
**less than or equal to** the threshold, so a threshold sits below what the code
achieves. The file records a measurement of 2026-10-26 over `./internal/domain`:
69 mutants, efficacy 100.00 %, mutator coverage 98.57 % (one constant expression,
`KeyWindow`, cannot be instrumented). Only the pure composition package is
mutated; use cases and adapters are not. The scheduled `mutation` job (Monday
06:00 UTC and manual dispatch) mutates `./internal/domain` as a whole; a failing
scheduled run opens or refreshes a `harness:red` issue
(`scripts/harness/red_issue.py`).

## Architecture fitness tests

`internal/architecture` holds the hexagonal dependency rules
(`TestHexagonalArchitecture`) and source-scanning sensors. The ones that define
this service:

| Test | Rule |
| --- | --- |
| `TestNoInOrgNetworkRoute`, `TestAllowlistHoldsNoRestrictedRoute` | the BFF names no restricted retail-network route and the allowlist holds none |
| `TestNoCardDataAnywhere` | no raw card field outside the guard |
| `TestBFFNeverLogsBodies` | no log call takes a request or response body |
| `TestRuntimeBinaryHasNoInMemoryAdapters` | `cmd/**` imports no memory adapter |
| `TestNoGoFilesAtRepoRoot` | the module root holds no Go file |
| `TestNoAuthMiddlewareReintroduced` | no auth middleware returns |
| `TestCloudEventsOnly`, `TestEventCatalogueMatchesContract` and the Kafka writer/outbox sensors | template guards that stay green because the service has no events |

Each sensor that scans source has a `...SensorBites` or `...SensorFailsOnBadFixtures`
companion proving it can fail.

```bash
make arch-test
```

## CI jobs

`.github/workflows/ci.yml` runs on push and pull request to `main` and `develop`,
weekly (Monday 06:00 UTC, mutation job only) and on manual dispatch.

| Job | Runs | When |
| --- | --- | --- |
| `lint` | `golangci-lint` v2.14.0, `--timeout=5m` | always |
| `guide-lint` | `scripts/harness/guide_lint.py` and `test_hook.py`; **advisory** (`continue-on-error`) | always |
| `complexity` | golangci-lint with only `gocyclo,gocognit,cyclop,funlen,nestif`, plus an informational gocyclo report | always |
| `file-length` | revive `file-length-limit` (300 lines of code per production Go file, config in `.golangci.yml`): blocking for the Go files a pull request into `develop` adds or touches, an informational backlog report otherwise | always |
| `test` | `go build`, `go vet`, then `make coverage` (90 % gate); uploads `coverage.out` | always |
| `bdd` | `make bdd` (prints that there are no godog features, runs no test) | always |
| `integration` | `go test -tags=integration ./... -race -count=1` | always |
| `mutation-fast` | gremlins v0.6.0 on `./internal/domain/composition` | always; blocking |
| `mutation` | `gremlins unleash ./internal/domain --workers 1 --timeout-coefficient 30`, `harness:red` issue on a scheduled failure | schedule and dispatch only |
| `api-lint` | Spectral on `apis/openapi.yaml` (`.spectral.yaml`), `--fail-severity=warn`; no AsyncAPI step | always |
| `vuln` | `govulncheck ./...` | always |
| `helm-lint` | `ct lint` (pull requests) and `helm lint` with both `ci/*-values.yaml` | always |
| `chart-tests` | every `charts/storefront-bff/tests/*.sh` and `test_*.py` (`render_test.sh`, `test_service_selectors.py`); a test that prints `SKIP` fails the job | always |
| `arch-test` | `go test ./internal/architecture/... -v` | always |
| `docker-publish` | build, push `ghcr.io/iqvo/storefront-bff`, cosign signature, SBOM; needs the jobs above | push to `main` only |
| `release` | tags the next patch version, retags the image, packages and pushes the Helm chart, creates the GitHub release | push to `main` only, after `docker-publish` |

`.github/workflows/docs.yml` builds this site (`npm ci`, `npm run build` in `docs/`)
on pull requests and deploys to GitHub Pages only on push to `main`;
`codeql.yml` runs CodeQL and `ai-review.yml` is an advisory review on pull
requests. Jobs that show *skipping* on a pull request into `develop`
(`mutation`, `docker-publish`, `release`) are gated by their `if:` conditions;
that is expected, not a failure.

## Reproducing the docs build

```bash
cd docs
npm ci
npx tsc --noEmit -p .
npm run build
```

The build fails on a broken link or anchor. Mermaid diagrams are checked with
`scripts/validate-mermaid.cjs` of the warehouse-docs repository, because
`docusaurus build` alone never catches a broken diagram.
