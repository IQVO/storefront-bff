---
id: ddd-artifacts
title: DDD artifacts (ddd-crew)
sidebar_label: DDD artifacts index
sidebar_position: 10
description: "Index of the ddd-crew artifacts of the Storefront BFF: strategic and tactical design drawn from the code, with the honest 'none' where the service has no aggregate, table or event."
---

# DDD artifacts (ddd-crew)

The strategic and tactical design of `storefront-bff`, drawn with the
[ddd-crew](https://github.com/ddd-crew) tools plus UML and ER diagrams. Every
diagram is Mermaid, derived from the code and ADRs on `develop`, and carries a
`Source:` line naming the files it came from.

This service is a **stateless composition edge**: it has no aggregate, no table
and no event. The pages say so plainly rather than inventing a model; the
interesting content is the composition rules, the cookie state, the allowlist and
the six upstream relationships.

| Artifact | ddd-crew tool / notation | What it shows |
| --- | --- | --- |
| [Core domain chart](/docs/ddd/core-domain-chart) | [Core Domain Charts](https://github.com/ddd-crew/core-domain-charts) | Where the BFF sits on differentiation vs complexity, and why it is Generic. |
| [Bounded context canvas](/docs/ddd/bounded-context-canvas) | [Bounded Context Canvas v5](https://github.com/ddd-crew/bounded-context-canvas) | Purpose, roles, every inbound and outbound message, business decisions. |
| [Context map](/docs/ddd/context-map) | [Context Mapping](https://github.com/ddd-crew/context-mapping) | The BFF's edges to storefront-web and the six upstreams with the pattern on each. |
| [Aggregate design canvas](/docs/ddd/aggregate-design-canvas) | [Aggregate Design Canvas v1.1](https://github.com/ddd-crew/aggregate-design-canvas) | No aggregate: the canvas for the one thing that is state, the `sf_state` cookie value object. |
| [Domain message flow](/docs/ddd/domain-message-flow) | [Domain Message Flow Modelling](https://github.com/ddd-crew/domain-message-flow-modelling) | The key scenarios as numbered command and query flows across the browser, the BFF and the upstreams. |
| [EventStorming](/docs/ddd/eventstorming) | [EventStorming glossary](https://github.com/ddd-crew/eventstorming-glossary-cheat-sheet) | A purchase journey with the commands the BFF relays, the policies it applies and the hotspots. |
| [Ubiquitous language](/docs/ddd/ubiquitous-language) | Glossary | Every term with the code identifier that implements it. |
| [Class diagrams](/docs/ddd/class-diagram) | UML class diagram | The composition types, the use-case `Gateway`, the one port and the adapters. |
| [Entity-relationship diagram](/docs/ddd/entity-relationship) | ER diagram | No tables; the logical identifiers the cookie holds and the upstream resources they point to. |
| [Sequence diagrams](/docs/ddd/sequence-diagrams) | UML sequence diagram | Add to cart, delivery dates, confirm checkout and order detail, with error branches. |
| [Domain events](/docs/ddd/domain-events) | Event catalogue | None published or consumed, and what the BFF relays instead. |

Related pages outside the pack: [Use cases](/docs/ddd/use-cases),
[Subdomain classification](/docs/ddd/subdomain-classification),
[Integration](/docs/ecosystem/integration) and the [API overview](/docs/api-reference).

:::note[Sources of truth]
The code wins. The composition rules come from `internal/domain/composition`,
behaviour from `internal/application/usecases`, the edge contract from
`apis/openapi.yaml`, and decisions from the ADRs in `docs/adr/`. Where a page and
the code disagree, the page is wrong.
:::

:::info Not in this pack
There is no business-context page: its content is folded into the
[Introduction](/docs/overview/introduction). There is no MCP, A2A or analytics
surface to document.
:::
