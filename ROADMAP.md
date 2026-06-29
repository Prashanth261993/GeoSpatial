# Roadmap

1 phase = 1 vertical slice = 1 PR. Branch per phase → verify → merge to main → push.

| Phase | Slice | Status |
|---|---|---|
| 0 | Skeleton: compose, SPEC, ADRs, .env.example | ✅ Done |
| 1 | Ingest + ws-fanout + simulator + deck.gl map (+OSRM road-snap) | ✅ Done |
| 2 | H3 indexing + "who's near me" (radius/nearest) | ✅ Done |
| 3 | Matching engine: greedy, locking, no double-assign | ⬜ Next |
| 4 | Redpanda backbone: partition by H3, idempotency, backpressure | ⬜ |
| 5 | Surge analytics: windowed spatial aggregation + heatmap | ⬜ |
| 6 | Real-feed toggle (OpenSky / GTFS-RT) → MVP demoable | ⬜ |
| 7 | Observability: Prometheus/Grafana, traces | ⬜ |
| 8 | Rust hot path (matcher/indexer), measured | ⬜ |
| 9 | Cinematic frontend: custom shaders, trails, surge ripple | ⬜ |
| 10 | Free-cloud deploy + CDN (final system only) | ⬜ |

Architecture: Go microservices · Redpanda · Redis · TimescaleDB · OSRM · React/deck.gl (Rust later).
Domain: Seattle rideshare. See SPEC.md and docs/adr.

Build journey (public): a cinematic Astro landing site in `docs-site/` narrates each
phase with diagrams, code, screenshots and GIFs. Deployed to GitHub Pages:
https://prashanth261993.github.io/GeoSpatial/
