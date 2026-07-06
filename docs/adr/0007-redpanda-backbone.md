# ADR 0007: Redpanda event backbone, partitioned by H3 cell

- Status: Accepted
- Date: 2026-07-06

## Context
Redis pub/sub (Phases 1–3) is fire-and-forget: no replay, no ordering across consumers,
no partitioning, no real backpressure. To become a genuine distributed system we need a
durable, partitioned, replayable event log — and a partitioning scheme that gives
spatial locality for parallel matching.

## Decision
Adopt Redpanda (Kafka API) as the backbone via the franz-go client. `ingest-gateway`
produces to `positions` (12 partitions) keyed by a coarse **H3 cell (res 7)**; `matcher`
produces trip events to `trips` (6 partitions). `indexer` consumes `positions` as a
shared consumer group with **manual offset commits** (at-least-once); its handlers are
idempotent (SADD/HSET set-ops), and the matcher dedupes by `reqId`. `ws-fanout` consumes
both topics with a **unique group per instance** starting at the latest offset (full
fan-out). Keying resolution (7) is decoupled from the indexer's indexing resolution (9).
Redis remains only for the spatial index and the matcher's distributed lock.

## Alternatives
- Keep Redis pub/sub: simplest, but no replay/partitioning/backpressure.
- Exactly-once (Kafka transactions): strongest, but coordinator overhead and complexity
  we don't need — at-least-once + idempotent consumers is what most systems run.
- Key by driver id or random: loses spatial locality (the whole point).
- segmentio/kafka-go: simpler client, fewer features (no built-in idempotent producer).

## Consequences
Keying by H3 pushes ingest and matcher into cgo (they compute H3); go floor rose to 1.25;
both Dockerfiles bumped. Spatial keying creates **hot partitions** (downtown p7≈17k vs
water p0≈0) — accepted for the locality benefit; mitigations (salt hot cells, more
partitions, per-region topics) noted for scale. Backbone now supports replay (17.2k
records reprocessable), lag-based backpressure (measured 0→2600→0 on consumer restart),
and geography-parallel consumption.
