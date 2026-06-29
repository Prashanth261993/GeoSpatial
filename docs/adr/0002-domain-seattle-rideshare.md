# ADR 0002: Domain = Seattle rideshare with real-data integrations

- Status: Accepted
- Date: 2026-06-28

## Context
Need believable, visually impressive data, free. NYC TLC replay is NYC-only.

## Decision
Rideshare in Seattle. Sim drivers move road-snapped via OSRM (Seattle extract).
Synthetic POI-seeded demand. Real-feed toggle: OpenSky (aircraft), King County
Metro GTFS-RT (buses).

## Alternatives
- NYC + TLC replay: real demand, but reviewer asked for Seattle.
- Aircraft/ships hero feed: impressive but weak rider↔driver matching story.

## Consequences
Slightly synthetic demand vs NYC; matching/surge still authentic. Real feeds
prove ingest scales to thousands of actual moving entities.
