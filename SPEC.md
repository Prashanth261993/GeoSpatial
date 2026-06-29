# SPEC — Real-Time Geospatial Event Platform

## Purpose
Track many moving entities, index them spatially, and match supply↔demand
(riders↔drivers) in real time with a live map. Domain: rideshare, Seattle.

## Components
- **ingest-gateway** (Go): receives location updates, validates, publishes events.
- **indexer** (Go): maintains spatial index (Redis GEO + H3 cell keys).
- **matcher** (Go): assigns riders↔drivers; locking prevents double-assignment.
- **surge** (Go): windowed spatial aggregation → demand/surge metrics.
- **ws-fanout** (Go): pushes live updates to browser clients.
- **web** (TypeScript/React): MapLibre + deck.gl live map.

## Infrastructure
- **Redpanda**: event backbone (Kafka API). Partition key = H3 cell.
- **Redis**: live geo index + pub/sub fanout.
- **TimescaleDB**: historical positions, trips, surge windows.
- **OSRM**: road-snapped movement for simulated drivers.

## Data flow
drivers → ingest-gateway → Redpanda → {indexer→Redis, matcher, surge} → ws-fanout → web

## Spatial model
- H3 hex grid for index keys and partitioning (2D→1D).
- Redis GEO for radius "who's near me" queries.

## Non-functionals
- Runs free: docker-compose locally + free cloud tiers.
- Idempotent ingest, partitioned by H3, backpressure via consumer groups.
- Observability: Prometheus/Grafana, structured logs, traces.

## Region defaults
Seattle: center 47.6062, -122.3321.
