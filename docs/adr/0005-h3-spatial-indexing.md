# ADR 0005: H3 hex grid for spatial indexing (resolution as a parameter)

- Status: Accepted
- Date: 2026-06-29

## Context
We need a proximity index ("entities within radius R") that powers matching, surge,
and UI. It must be fast (not O(N)), restart-safe, shareable across services, and
provide a stable key we can later use to partition the event stream.

## Decision
Use Uber's H3 hexagonal grid via `github.com/uber/h3-go/v4`. A dedicated `indexer`
service consumes the positions stream and maintains a Redis Set per H3 cell
(`h3:<cell>`), plus `geo:pos` (latest position) and `geo:cell` (current cell, for
transition detection). Index resolution is a parameter (`H3_RES`, default 9 for city
cars; aircraft will use ~6). Queries use broad phase (gridDisk over the radius) +
narrow phase (exact great-circle filter). Membership lives in Redis (restart-safe,
shared with the matcher). We keep the Phase-1 Redis GEO as a reference oracle.

## Alternatives
- Geohash/Redis GEO only: simple but rectangular unequal cells, ambiguous neighbors,
  and a key not ideal for partitioning.
- Quadtree/R-tree: adaptive to skew but pointer-heavy, less cache-friendly, no stable
  partition key for streaming.
- In-process index: faster but not restart-safe or shareable.

## Consequences
H3 brings cgo: the indexer needs a C toolchain and a glibc runtime (distroless base,
not static) via `services/Dockerfile.cgo`; Go bumped to 1.24. The H3 cell becomes the
partition key in Phase 4. Resolution stays configurable so OpenSky aircraft (Phase 6)
is a config change, not a rewrite.
