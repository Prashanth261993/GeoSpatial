# ADR 0009: Real-data integrations via adapters, typed events, on-demand ingestion

- Status: Accepted
- Date: 2026-07-06

## Context
The system ran on simulated drivers only. To prove the pipeline scales to reality, we
integrate live public feeds (OpenSky aircraft first, GTFS-RT transit later) without
letting foreign schemas corrupt the core, without making a Boeing matchable, and without
burning external rate limits.

## Decision
A `feeds` service hosts per-source adapters. The OpenSky adapter polls the REST
`/states/all` over a Pacific-Northwest bounding box every 12s and normalizes state
vectors into internal `Position{type:"aircraft",hdg}` events (anti-corruption layer),
producing to the existing `positions` topic (H3-keyed). `Position` gains a `Type` field
(driver|aircraft|bus); the indexer skips non-drivers so aircraft reach the map via fanout
but are never proximity candidates; the matcher pairs drivers only. Pollers are OFF by
default and controlled on-demand via `POST /feeds {feed,enabled}` — the UI toggle starts/
stops polling, conserving OpenSky's rate budget (backpressure toward the source). The map
renders type-aware deck.gl `IconLayer` glyphs; planes rotate by `true_track`. Resilience:
per-request timeout, explicit 429 handling, skip-bad-record, keep-last-good.

## Alternatives
- Let foreign schema flow through: couples the whole system to OpenSky's array format.
- Topic per entity type: more isolation, more plumbing; typed events + one topic suffice.
- Always-on polling: wastes rate budget; on-demand is both thriftier and better UX.
- Index aircraft too: would surface them as match candidates (wrong).

## Consequences
Adding a feed is additive — indexer/matcher/surge/fanout/map handled real aircraft with
zero code changes, validating the clean event contract. First proof the pipeline ingests
real entities at scale (~130 live aircraft). GTFS-RT (protobuf) slots in as another
adapter. On-demand ingestion means the stack runs fully offline by default. SVG icons
require explicit width/height for deck rasterization.
