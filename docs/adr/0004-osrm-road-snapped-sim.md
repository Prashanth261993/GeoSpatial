# ADR 0004: OSRM road-snapped simulator with straight-line fallback

- Status: Accepted
- Date: 2026-06-28

## Context
Straight-line dots look fake. Real road movement is a big visual upgrade, but
OSRM data is large and preprocessing is slow.

## Decision
Preprocess a WA OSM extract (MLD: extract→partition→customize), serve osrm-routed.
Simulator requests /route geojson, resamples ~80m/tick, walks it. If OSRM is
down or returns no route, fall back to straight-line so the system never stalls.

## Alternatives
- Keep straight-line: simplest, unconvincing.
- Real GTFS-RT only: real but not controllable for matching demo (kept for P7).

## Consequences
data/osrm gitignored (357MB pbf). Reproducible via data/README.md. Fallback keeps
resilience; OSRM optional via OSRM_URL.
