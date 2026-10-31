---
id: index
title: Architecture decision records
sidebar_label: ADR index
slug: /
sidebar_position: 0
---

# Architecture decision records

The decisions of `storefront-bff`, in the order they were taken. The ADR bodies are
immutable; a changed decision gets a new ADR that supersedes the old one. The
*Status* column is copied from each ADR's own status line.

| ADR | Title | Status |
| --- | --- | --- |
| [0001](/docs/adr/0001-storefront-bff-bounded-context) | Storefront BFF is a bounded context of the storefront domain | Accepted (bootstrap). Design source: warehouse-docs `storefront/model/model.yaml`, context `storefront-bff`. |
| [0002](/docs/adr/0002-storefront-bff-assumptions) | Storefront BFF design assumptions | Accepted. Design source: warehouse-docs `storefront/model/model.yaml` (context `storefront-bff`), the |
| [0003](/docs/adr/0003-fleet-observability-standard) | Adopt the fleet observability standard (logs, metrics, traces) | Accepted (2026-10-29). |
| [0004](/docs/adr/0004-bff-relays-the-category-tree) | The BFF relays the category tree | Accepted (2026-10-29). |
| [0005](/docs/adr/0005-delivery-dates-carry-the-window-time-zone) | Delivery dates carry the window's time zone | Accepted (2026-10-30). |
