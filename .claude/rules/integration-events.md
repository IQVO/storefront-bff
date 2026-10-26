---
paths:
  - "internal/adapters/**/kafka/**"
  - "internal/adapters/outbound/events/**"
  - "apis/asyncapi*"
---

# Cross-service integration events (Kafka)

storefront-bff PUBLISHES nothing and CONSUMES nothing: it is a read/compose
edge with no database and no outbox. The harness packages
`internal/adapters/kafka/cloudevents` and `internal/adapters/outbound/kafka`
ship for the fitness tests and stay UNUSED at runtime (docs/adr/0002).

If an event is ever added, the fleet rule applies unchanged:

## Events: CloudEvents 1.0 is MANDATORY

Every Kafka message is a CloudEvents 1.0 event in structured content mode,
built and decoded only through `internal/adapters/kafka/cloudevents`:
`type` = `com.retailnetwork.storefront.storefront-bff.<entity>.<EventName>`,
`source` = `/retailnetwork/storefront-bff`, `dataschema` =
`urn:retailnetwork:storefront-bff:events:<EventName>:v1`, topics
`retailnetwork.storefront-bff.events` / `.analytics`. No flat envelope, no
envelope toggle (`TestNoEventEnvelopeToggleOrFlatEnvelope`). Consumers dispatch
on the full `type`, dedupe on `id`, and take the consumer group id from
configuration, never an inline literal. Never produce or consume a `warehouse.*` topic.

### Published types

None.

### Consumed types

None.
