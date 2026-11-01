---
id: domain-events
title: Domain events
sidebar_label: Domain events
sidebar_position: 22
description: "The Storefront BFF publishes and consumes no events: evidence, the reserved namespace, what it relays instead and what adding an event would require."
---

# Domain events

**The Storefront BFF publishes no event and consumes none.** There is no event
catalogue to list, no AsyncAPI document (`apis/` holds only `openapi.yaml`), no
Kafka topic and no consumer group. This page records the evidence, so nobody
searches for events that do not exist, and what the service does in their place.

## Evidence

| Check | Result |
| --- | --- |
| `apis/` | `openapi.yaml` only; no `asyncapi.yaml` |
| `cmd/api/main.go` wiring | `config.Load`, `startTelemetry`, `rest.New`, `usecases.NewGateway`, `inbound.NewServer`: no Kafka writer, reader, outbox or CloudEvents encoder |
| Environment | no `KAFKA_BROKERS`, consumer group or topic variable ([Configuration](/docs/operations/configuration)) |
| CI | `api-lint` lints the OpenAPI document only; `ci.yml` notes the edge "publishes/consumes no events by design" |
| ADR 0001 | "Published: none (read side). Consumed: none. No `warehouse.*` topic is consumed or produced." |
| Template leftovers | `internal/adapters/outbound/kafka` (writer) and `internal/adapters/kafka/cloudevents` (envelope) exist from the fleet scaffold but nothing in `cmd/api` imports them; their tests and the Kafka and CloudEvents fitness sensors stay green |

## What the BFF has instead

| Need | How the BFF meets it without an event |
| --- | --- |
| Tell the web app a cart or order changed | It does not: the page asks again. Every answer is the upstream's current state, relayed. |
| Know a payment is settled | The page polls `GET /checkout/{id}` while a placement is in doubt (a `202`); see [Sequence diagrams](/docs/ddd/sequence-diagrams). |
| Record what happened | One structured `http request` log line per request with route, status, duration and ids, never a body ([Observability](/docs/operations/observability)). |
| Keep a retry safe | A deterministic `Idempotency-Key` instead of an outbox: the upstream owns de-duplication ([Use cases](/docs/ddd/use-cases)). |

## Reserved namespace

[ADR 0001](/docs/adr/0001-storefront-bff-bounded-context) reserves, for the day an
event exists:

- CloudEvents `type`: `com.retailnetwork.storefront.storefront-bff.<entity>.<Event>`
- Topic: `retailnetwork.storefront-bff.events`, and `.analytics` for
  analytics-only events.

Nothing uses either. Facts about carts, checkouts, payments and orders are
published by their owners in the upstream services and are catalogued on the
aggregator's storefront site, not here.

## If an event is ever added

It would be the first stateful behaviour of the service, so it needs an ADR first.
At minimum: an `apis/asyncapi.yaml`, a transactional outbox (which needs a
database, which the service does not have), the `kafka` and `cloudevents`
packages wired in `cmd/api`, `KAFKA_BROKERS` in the configuration page and chart,
and an event catalogue page `api-reference/events` in this site's sidebar. The
architecture sensors (`TestEventCatalogueMatchesContract`, `TestCloudEventsOnly`,
the writer and outbox sensors) would then start guarding it.
