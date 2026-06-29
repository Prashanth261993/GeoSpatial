# ADR 0006: Matching engine — greedy + batched-optimal, Redis SET NX EX locking

- Status: Accepted
- Date: 2026-06-29

## Context
We must assign riders to nearby drivers in real time without ever double-booking a
driver, under concurrency and (eventually) horizontal scaling.

## Decision
A `matcher` service. Default **greedy** (online nearest-free) for low-latency live
assignment; an optional **batched-optimal** mode buffers a ~1s window and solves it
with the Hungarian algorithm. Double-assignment is prevented with an atomic Redis lock
`SET lock:driver:<id> <reqId> NX EX 30`: NX makes check-and-claim atomic; the 30s TTL
self-heals a crashed matcher. Assignments are idempotent by `reqId`. Trips run a small
lifecycle (assigned → pickup → completed) and publish to a `trips` channel relayed by
ws-fanout. A reproducible benchmark measures the optimality gain.

## Alternatives
- In-process mutex: only correct for a single instance; fails under scale-out.
- Optimistic CAS with versions: viable, but a Redis lock is simpler and canonical.
- Full multi-node Redlock: more fault-tolerant but unnecessary complexity at our scale.
- Greedy-only: simplest, but misses the optimality story and measurement.

## Consequences
Single-Redis lock isn't failover-safe (a primary failover could drop a lock); accepted
for scope, with Redlock noted as the production upgrade. Greedy stays the live path
(where locking matters most); optimal proves and measures the gain (13–19% lower avg
pickup distance, growing with density). Synchronous HTTP `/request` becomes a Kafka
consumer in Phase 4, where idempotency hardens to exactly-once.
