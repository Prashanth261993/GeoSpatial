# ADR 0008: Surge analytics — sliding windows, H3 rollups, Timescale persistence

- Status: Accepted
- Date: 2026-07-06

## Context
We need a real-time demand/supply signal per area to model surge — the aggregation
layer pricing and driver incentives build on. It must be smooth over time, spatially
meaningful, capture unmet demand, and persist for history.

## Decision
A `surge` service consumes a dedicated **requests topic** (every rider request, incl.
unmatched) as its own consumer group. Per H3 **res-7 zone** it maintains a **sliding
window** implemented as a ring of 30×10s demand sub-buckets (5 minutes). Supply is the
count of available drivers in the zone, summed from the indexer's res-9 Redis Sets via
H3 parent→child rollup. Multiplier = `clamp(1 + 0.6·(demand/supply − 1), 1, 3)`. The
current value is written to Redis (`surge:<cell>`) for the live map; per-zone snapshots
are persisted to a **TimescaleDB hypertable** (`surge_windows`) for history. The map adds
a 3D extruded H3 heatmap behind an ops/surge layer toggle.

## Alternatives
- Tumbling windows: simpler, but surge flickers at 5-min boundaries.
- Reuse the trips topic for demand: misses unmatched requests — the strongest signal.
- Redis-only (no history): loses time-series queries; Timescale was provisioned for this.
- Match-resolution (res 9) zones: too fine — you price neighborhoods, not blocks.

## Consequences
First real use of TimescaleDB (hypertable, time-chunked). Requests topic added
(12 partitions, H3-keyed) — surge is parallel by geography like matching. Surge caps at
3.0x; a 240-request spike drove a zone from 1.0x to the cap, verified in Timescale.
3D extrusion needs camera pitch to read; toggle keeps ops and analytics views separate.
