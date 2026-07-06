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
Open http://localhost:5174 — 200 simulated Seattle drivers move in real time.
Add `VITE_MAPTILER_KEY` to `.env` for dark vector tiles (else a plain fallback map).

## Lifecycle (start / stop / status)
```bash
docker compose ps                 # status of all services
docker compose logs -f ws-fanout  # tail a service's logs

docker compose stop               # stop all (keeps data + containers)
docker compose stop simulator     # stop just one service
docker compose start              # resume after a stop

docker compose down               # stop + remove containers/network (keeps volumes)
docker compose down -v            # full reset: also wipes the TimescaleDB volume
```
`stop` is the gentle pause; `down` tears down; `down -v` wipes persisted DB data.
The OSRM routing graph in `data/osrm/` is on disk and unaffected by any of these.

See [SPEC.md](SPEC.md) for architecture and [docs/adr](docs/adr) for decisions.
Roadmap and phase status: [ROADMAP.md](ROADMAP.md).

## Build journey (landing site)
A cinematic Astro site narrates how this was built, phase by phase — diagrams, key
code, screenshots, and GIFs of the live UI. Source in [`docs-site/`](docs-site).
Live: **https://prashanth261993.github.io/GeoSpatial/**

## MapTiler key (dark vector basemap)
Without a key the map uses a plain MapLibre demotiles fallback. For the dark
Seattle look:
1. Sign up free at https://www.maptiler.com/ → Account → API keys.
2. Copy your key into `.env`: `VITE_MAPTILER_KEY=your_key_here`.
3. Rebuild the web container: `docker compose up -d --build web`, reload http://localhost:5174.
The key is read at build time via Vite; `.env` is gitignored so it never lands in git.
