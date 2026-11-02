---
id: troubleshooting
title: Troubleshooting
sidebar_label: Troubleshooting
sidebar_position: 4
description: "Symptom, cause, check and fix for the failure modes of the Storefront BFF edge: boot errors, 4xx problems, 503/504 from upstreams, cookie and idempotency surprises."
---

# Troubleshooting

The BFF is a stateless edge over six REST services: it has no database, no Kafka
and no queue, so every failure is either a boot error, a refusal at the edge
(4xx), or an upstream that is slow, down or unhappy (5xx). Every error the edge
itself produces is `application/problem+json` with a `type` under `/problems/`
(`internal/adapters/inbound/http/problems.go`). An upstream's own answer
(including its problem body and status) is relayed untouched, so first decide
whose problem it is: the `type` starts with `/problems/` and is in the table
below, or it is the upstream's.

Tools you will use: the JSON log (`http request` records with `route`,
`status`, `request_id`, `trace_id`), the trace (one server span plus one client
span per upstream call), and `LOG_LEVEL=debug`. See
[Observability](/docs/operations/observability).

## Boot and liveness

| Symptom | Cause | Check | Fix |
| --- | --- | --- | --- |
| Pod in `CrashLoopBackOff`, last log `storefront-bff stopped` with `invalid configuration: ... is required` | One or more of the six `*_URL` variables is unset or blank; all problems are listed together | `kubectl -n storefront-bff logs deploy/storefront-bff` | Set `config.productDiscoveryUrl`, `deliveryPromiseUrl`, `shoppingCartUrl`, `checkoutUrl`, `customerOrderingUrl`, `pricingUrl` ([Configuration](/docs/operations/configuration)) |
| `storefront-bff stopped` with `base URL of <service> is missing or not an absolute http(s) URL` | The variable is set but has no scheme or host (`product-discovery:80`, a bare host) | The message names the service | Use `http://host[:port]` or `https://...` |
| `invalid configuration: BUDGET_QUERY_MS must be a positive number of milliseconds` (or `_COMMAND_`, `_CONFIRM_`) | Non-numeric, zero or negative budget | The variable value in the Deployment | Fix the value or unset it for the default |
| `invalid configuration: COOKIE_SECURE must be a boolean` | Value `strconv.ParseBool` rejects (`yes`, `on`) | Same | Use `true`/`false`/`1`/`0` |
| `storefront-bff stopped` with `listen: ... address already in use` | `PORT` is taken, or two processes run locally | `lsof -i :8080` | Change `PORT` (the chart keeps it equal to `service.targetPort`) |
| Pod is `Ready` and `/readyz` is 200 but requests fail | Readiness is a static handler and does not probe the upstreams | `GET /products` and look for 503/504 | Fix the upstream; the pod does not need a restart |
| Pod killed at rollout with requests lost | In-flight requests get 20 s after SIGTERM; a confirm may need up to 15 s | Pod `terminationGracePeriodSeconds` vs `config.shutdownDrainDelay` | Keep the grace period above drain delay plus 20 s |

## Problems the edge produces

| Status and `type` | Cause | Check | Fix |
| --- | --- | --- | --- |
| `400 /problems/malformed-request` | A NUL character (`%00`, `\u0000`) in the query string or body, or the body could not be read | The `detail` says which | Strip the NUL on the client |
| `404 /problems/resource-not-found` | No route matches, or a NUL in the path. The edge routes only the paths in the [API reference](/docs/api-reference); `:verb` suffixes are part of the path (`/orders/{id}:cancel`) | Compare the path with the endpoint table; the log line has `route=unmatched` | Fix the URL. An upstream 404 for a real route (unknown product, cart, order) is the upstream's body, not this one |
| `405` `method-not-allowed` | Right path, wrong verb (for example `GET /checkout/{id}:confirm`) | Table in the API reference | Use the documented verb |
| `413 /problems/payload-too-large` | Body above 1 MiB | `Content-Length` | Send less; nothing in the storefront legitimately needs 1 MiB |
| `422 /problems/validation-failed` | The edge refused the request before calling anyone. `detail` is a fixed sentence, never request data: malformed id (`[A-Za-z0-9._-]`, 1 to 128 chars, alphanumeric first), unknown JSON field, missing `postalPrefix`, `reason` empty or over 200 chars, e-mail over 254 chars, `paymentMethodToken` empty, over 256 chars or card-shaped | Read `detail`; bodies are strict (`DisallowUnknownFields`) so a stray field fails | Send exactly the documented body |
| `422` with detail `card data is refused; send the payment method token only` or `paymentMethodToken must be a PSP token, never a card number` | The confirm body has a card-shaped key (`pan`, `cvc`, `expiry`, ...) or the token is a 13 to 19 digit number passing the Luhn check | The client sends raw card data | Tokenise at the PSP first and send only the token. This guard is intentional and must not be relaxed |
| `422 /problems/session-required` | The route needs the shopper (open checkout, `/me`, `/me/addresses`, `/orders` without `shopperId`) and the cookie carries none | Is the `sf_state` cookie sent? See the cookie row below | Register or log in first; send the cookie |
| `499 /problems/client-closed-request` | The caller went away before the answer; the edge does not retry | Usually a browser navigation or a short client timeout | None; if frequent, a client timeout is below the route budget |
| `503 /problems/upstream-unavailable` | The upstream refused or dropped the connection, DNS failed, the reply was over 4 MiB, the reply could not be read, or (order detail, delivery dates) a 2xx body was not the JSON the composition needs | The problem body does not name the service (it never carries upstream data). Open the trace: the failing client span is the culprit; with `LOG_LEVEL=debug` read the SDK errors | Fix the upstream or its URL. Retrying with the same `X-Action-Id` is safe (see below) |
| `504 /problems/gateway-timeout` | The whole-request budget expired: 2 s (query), 5 s (command), 15 s (confirm) by default. Budgets are never retried | Trace: which client span is long | Fix the slow upstream first; raise `BUDGET_*_MS` only when the upstream is legitimately slower |
| `500 /problems/internal-error` | A handler panic (`handler panic` ERROR log, value deliberately not logged) or a JSON marshal failure | The ERROR log line has `method` and `path` | Report with `request_id` |
| `500 /problems/route-not-allowed` | A call the allowlist refused (`ErrRouteNotAllowed`): a code change added an upstream route without allowlisting it. Not a client error, so it stays a 5xx | The ERROR log line has `method` and `path`; the upstream service is in the wrapped error, not in the response | Report with `request_id`; add the route to `internal/domain/composition/allowlist.go` and its test |

## Cookie, session and idempotency

| Symptom | Cause | Check | Fix |
| --- | --- | --- | --- |
| `session-required` right after a successful login or register, with curl | The cookie is `Secure` by default, and curl will not send a `Secure` cookie over plain `http://` | `curl -i` shows `Set-Cookie: sf_state=...; Secure` | For plain-HTTP runs set `COOKIE_SECURE=false` (the chart already does by default). Through Kong over HTTPS keep it `true` |
| The browser keeps no cookie, or sends none to some paths | The cookie is scoped to `Path=/api/storefront`; calls made to the bare paths (`/carts`, no prefix) are outside it | The `Path` in `Set-Cookie` | Call through the `/api/storefront` prefix; the BFF accepts it with or without |
| A corrupted or hand-edited cookie behaves like no cookie | `DecodeState` drops malformed values and ids failing `ValidID`; it never errors | Decode the value: `c=<cart>&s=<shopper>&k=<checkout>` | Log in again. Do not look for an error; there is none |
| The cart is gone after logging in as someone else | `POST /sessions` replaces the shopper in the cookie, not the cart | Cookie `c=` | Create a new cart; carts are listed with `GET /carts?shopperId=` |
| A double click creates two carts, lines or orders | Without an `X-Action-Id` the key is derived from route, subject, body **and a 10-minute window**: two identical bodies are one action, but a retry that crosses a window boundary, or a body that differs in a byte, is a new action | Compare the `Idempotency-Key` in the upstream's log, `sf-` plus 40 hex chars | Have the page set one `X-Action-Id` per click (valid id, up to 64 chars): it replaces body and window |
| Re-sending the same `X-Action-Id` for a different payload gets the first answer | The key then ignores the body: the page owns the identity of the action | By design | Mint a new `X-Action-Id` per distinct action |
| A `202` from checkout confirm | The payment outcome is in doubt; the upstream placed or will place the order | `GET /checkout/{id}` until it settles | Poll, do not re-post with a new `X-Action-Id`; re-posting the same one is safe |

## Composed routes that degrade

| Symptom | Cause | Check | Fix |
| --- | --- | --- | --- |
| `GET /orders/{id}` has no `tracking` | The tracking leg failed or was not 2xx; it is optional and only visible as a failed child span | Trace: `/orders/{id}/tracking` client span | Fix customer-ordering tracking; nothing to do at the edge |
| `GET /orders/{id}` has no `totals` | Totals are joined only when the cookie's last checkout session placed **this** order and that read succeeded | Cookie `k=`; `GET /checkout/{k}` returns the same `orderId` | Expected for an order placed on another device or after the cookie changed |
| `POST /products/{id}:delivery-dates` returns tiers without `charge` | The pricing schedule read failed; only the charge is dropped | Trace: `/delivery-charge-schedules/current` client span | Fix pricing. The quote itself is required: if delivery-promise fails the whole call is 503 or the upstream's own status |
| `delivery-dates` returns an empty `tiers` list | Only offers in state `QUOTED` are listed; the rest are in `unavailable` | The `unavailable` array | The network cannot serve that prefix and product |
| `delivery-dates` is `422` for a valid-looking prefix | `postalPrefix` is 1 to 10 letters, digits, spaces or dashes, never an address | `detail` | Send a prefix |
| Dates read one day off on the page | The window carries the destination's IANA `timeZone` as quoted ([ADR 0005](/docs/adr/0005-delivery-dates-carry-the-window-time-zone)); the BFF never converts. No `timeZone` means UTC | The `timeZone` field | Convert on the page using that zone |
| `GET /categories` is stale or flat | The BFF relays the category tree unchanged ([ADR 0004](/docs/adr/0004-bff-relays-the-category-tree)) | Call product-discovery directly | Fix at product-discovery |
| An unknown query parameter has no effect on `GET /products` | Only `q`, `categoryId`, `inStock`, `priceMin`, `priceMax`, `sort`, `after`, `limit` are forwarded | `searchParams` in `usecases/catalog.go` | Use the listed filters |

## No telemetry

| Symptom | Cause | Check | Fix |
| --- | --- | --- | --- |
| No traces or metrics for `storefront-bff` | Collector unreachable or `OTEL_EXPORTER_OTLP_ENDPOINT` wrong; export drops silently | Start with `LOG_LEVEL=debug`: exporter failures log as `opentelemetry error` | Point the variable at the Collector (`otel.endpoint` in the chart); the edge keeps serving meanwhile |
| Metrics under an unexpected `service_name` | `OTEL_SERVICE_NAME` overridden | `telemetry configured` log line shows `service_name` | Unset it |
| No access-log line for `/healthz` | The logger skips probes on purpose | None | Expected |

## When it is not the BFF

If the problem body has a `type` that is not under `/problems/` and a status the
table does not list, the upstream answered it. Take the `request_id` or
`trace_id`, find the matching client span, and continue in that service's
runbook. The BFF never rewrites an upstream status, so a `409` or `422` from
checkout reaches the browser exactly as checkout sent it.
