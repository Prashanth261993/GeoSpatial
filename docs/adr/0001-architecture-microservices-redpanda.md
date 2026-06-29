# ADR 0001: Event-driven Go microservices on Redpanda

- Status: Accepted
- Date: 2026-06-28

## Context
Need a real distributed-systems story: spatial indexing, concurrent matching,
real-time fanout, surge analytics — runnable free locally and on free cloud.

## Decision
Go microservices (ingest, indexer, matcher, surge, ws-fanout) with Redpanda as
the event backbone, Redis for live geo + pub/sub, TimescaleDB for history.
Rust hot path added later only if a measured bottleneck justifies it.

## Alternatives
- Go monolith + Redis-only: fastest MVP, but partitioning/backpressure simulated.
- Rust-first: best perf, slower MVP, two languages up front.
- Edge/serverless: cheap CDN, weak partitioning, poor local/cloud parity.

## Consequences
More moving parts, but every hard problem (partitioning by H3, idempotency,
backpressure) becomes real and demonstrable. Keeps cost free.
