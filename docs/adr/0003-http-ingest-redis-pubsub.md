# ADR 0003: HTTP ingest + Redis pub/sub in Phase 1; Kafka deferred

- Status: Accepted
- Date: 2026-06-28

## Context
Phase 1 needs a working vertical slice fast. Producers (drivers) are many and
cheap; the wow factor is the fanout to browsers.

## Decision
Drivers POST over HTTP to ingest-gateway. Ingest writes Redis GEO and PUBLISHes
to a Redis channel. ws-fanout subscribes and pushes to browsers over WebSocket.
Redpanda is intentionally not in the path yet.

## Alternatives
- Kafka ingest now: more robust, but bigger slice; replay/partitioning not needed yet.
- WS ingest: stateful producers, harder to scale; HTTP is stateless + LB-friendly.

## Consequences
Tiny, demoable slice. Redpanda replaces the bus in Phase 4 for partitioning/
idempotency/backpressure; HTTP boundary stays.
