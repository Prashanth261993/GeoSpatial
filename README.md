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
Brings up Redpanda, Redis, TimescaleDB, the Go services, simulator, and the map.
Open http://localhost:5173 — 200 simulated Seattle drivers move in real time.
Add `VITE_MAPTILER_KEY` to `.env` for dark vector tiles (else a plain fallback map).

See [SPEC.md](SPEC.md) for architecture and [docs/adr](docs/adr) for decisions.
Roadmap and phase status: [ROADMAP.md](ROADMAP.md).

## MapTiler key (dark vector basemap)
Without a key the map uses a plain MapLibre demotiles fallback. For the dark
Seattle look:
1. Sign up free at https://www.maptiler.com/ → Account → API keys.
2. Copy your key into `.env`: `VITE_MAPTILER_KEY=your_key_here`.
3. Rebuild the web container: `docker compose up -d --build web`, reload http://localhost:5173.
The key is read at build time via Vite; `.env` is gitignored so it never lands in git.
