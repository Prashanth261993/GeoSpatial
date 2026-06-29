# Real-Time Geospatial Event Platform

Tracks many moving entities, indexes them spatially (H3), and matches
riders↔drivers in real time with a live MapLibre + deck.gl map. Seattle.

## Stack
Go microservices · Redpanda · Redis · TimescaleDB · OSRM · React/TS · (Rust later)

## Quickstart
```bash
cp .env.example .env
docker compose up -d
```
Brings up Redpanda, Redis, TimescaleDB. Services and map land in later phases.

See [SPEC.md](SPEC.md) for architecture and [docs/adr](docs/adr) for decisions.
