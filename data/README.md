# OSRM data (gitignored)

The routing graph is large and not committed. Rebuild it once:

```bash
mkdir -p data/osrm
curl -L -o data/osrm/seattle.osm.pbf https://download.geofabrik.de/north-america/us/washington-latest.osm.pbf
docker run --rm -v "$PWD/data/osrm:/data" osrm/osrm-backend:v5.25.0 osrm-extract   -p /opt/car.lua /data/seattle.osm.pbf
docker run --rm -v "$PWD/data/osrm:/data" osrm/osrm-backend:v5.25.0 osrm-partition  /data/seattle.osrm
docker run --rm -v "$PWD/data/osrm:/data" osrm/osrm-backend:v5.25.0 osrm-customize  /data/seattle.osrm
```

Then `docker compose up -d osrm`. The simulator road-snaps when `OSRM_URL` is set,
and falls back to straight-line motion if OSRM is unavailable.
