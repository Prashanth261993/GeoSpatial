import { useEffect, useRef, useState } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { Deck } from "@deck.gl/core";
import { ScatterplotLayer, ArcLayer } from "@deck.gl/layers";
import { H3HexagonLayer } from "@deck.gl/geo-layers";

type Pos = { id: string; lat: number; lng: number; ts: number };
type Track = { fromLng: number; fromLat: number; toLng: number; toLat: number; t0: number; t1: number };
type Trip = { reqId: string; driver: string; dLat: number; dLng: number; rLat: number; rLng: number; t0: number; durMs: number };
type Query = { cells: string[]; matched: Set<string>; center: [number, number] | null };

const KEY = import.meta.env.VITE_MAPTILER_KEY as string | undefined;
const STYLE = KEY
  ? `https://api.maptiler.com/maps/streets-dark/style.json?key=${KEY}`
  : "https://demotiles.maplibre.org/style.json";
const WS = (import.meta.env.VITE_WS_URL as string) || "ws://localhost:8090/ws";
const NEARBY = (import.meta.env.VITE_NEARBY_URL as string) || "http://localhost:8100/nearby";
const STATS = (import.meta.env.VITE_STATS_URL as string) || "http://localhost:8110/stats";
const SURGE = (import.meta.env.VITE_SURGE_URL as string) || "http://localhost:8120/surge";
const RADIUS = 1200;

// surge multiplier (1..3) -> RGB (calm cyan -> hot magenta/red)
function surgeColor(m: number): [number, number, number] {
  const t = Math.min(1, Math.max(0, (m - 1) / 2)); // 1x..3x -> 0..1
  return [Math.round(80 + 175 * t), Math.round(200 - 140 * t), Math.round(235 - 160 * t)];
}

export function App() {
  const ref = useRef<HTMLDivElement>(null);
  const [count, setCount] = useState(0);

  useEffect(() => {
    const map = new maplibregl.Map({
      container: ref.current!, style: STYLE,
      center: [-122.3321, 47.6062], zoom: 11.5,
    });
    const deck = new Deck({
      parent: ref.current!,
      initialViewState: { longitude: -122.3321, latitude: 47.6062, zoom: 11.5 },
      controller: true,
      getCursor: () => "crosshair",
      onViewStateChange: ({ viewState: v }) => {
        map.jumpTo({ center: [v.longitude, v.latitude], zoom: v.zoom, bearing: v.bearing, pitch: v.pitch });
      },
      layers: [],
    });

    const tracks = new Map<string, Track>();
    const trips = new Map<string, Trip>();
    const query: Query = { cells: [], matched: new Set(), center: null };
    let surgeZones: { cell: string; surge: number }[] = [];
    const show = { ops: true, surge: false };

    // layer toggles
    const opsEl = document.getElementById("tgl-ops") as HTMLInputElement;
    const surgeEl = document.getElementById("tgl-surge") as HTMLInputElement;
    opsEl?.addEventListener("change", () => { show.ops = opsEl.checked; });
    surgeEl?.addEventListener("change", () => { show.surge = surgeEl.checked; });

    const runQuery = async (lng: number, lat: number) => {
      query.center = [lng, lat];
      try {
        const r = await fetch(`${NEARBY}?lat=${lat}&lng=${lng}&radius=${RADIUS}`);
        const j = await r.json();
        query.cells = j.cells ?? [];
        query.matched = new Set((j.drivers ?? []).map((d: { id: string }) => d.id));
        setNear(`${j.matchCount} drivers within ${RADIUS}m · ${j.cells.length} cells · ${j.candidateCount} candidates`);
      } catch { setNear("query failed — is the indexer up?"); }
    };
    deck.setProps({ onClick: (info: any) => { if (info?.coordinate) runQuery(info.coordinate[0], info.coordinate[1]); } });

    const ws = new WebSocket(WS);
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.kind === "trip") {
        if (m.event === "assigned") {
          trips.set(m.reqId, { reqId: m.reqId, driver: m.driver,
            dLat: m.driverLat, dLng: m.driverLng, rLat: m.riderLat, rLng: m.riderLng,
            t0: performance.now(), durMs: m.durMs });
        } else if (m.event === "completed") {
          trips.delete(m.reqId);
        }
        return;
      }
      const p: Pos = m;
      const now = performance.now();
      const prev = tracks.get(p.id);
      tracks.set(p.id, {
        fromLng: prev?.toLng ?? p.lng, fromLat: prev?.toLat ?? p.lat,
        toLng: p.lng, toLat: p.lat, t0: now, t1: now + 1000,
      });
    };

    let raf = 0;
    const tick = () => {
      const now = performance.now();
      const onTrip = new Set([...trips.values()].map((t) => t.driver));

      const drivers = [...tracks.entries()]
        .filter(([id]) => !onTrip.has(id))
        .map(([id, t]) => {
          const a = Math.min(1, (now - t.t0) / (t.t1 - t.t0));
          return { id, position: [t.fromLng + (t.toLng - t.fromLng) * a, t.fromLat + (t.toLat - t.fromLat) * a], matched: query.matched.has(id) };
        });
      setCount(tracks.size);

      const tripList = [...trips.values()];
      const markers = tripList.map((t) => {
        const a = Math.min(1, (now - t.t0) / t.durMs); // driver -> rider progress
        return { position: [t.dLng + (t.rLng - t.dLng) * a, t.dLat + (t.rLat - t.dLat) * a] };
      });

      const layers: any[] = [];

      // Surge heatmap (toggle): 3D extruded H3 zones, color+height by multiplier.
      if (show.surge && surgeZones.length) {
        layers.push(new H3HexagonLayer({
          id: "surge", data: surgeZones, getHexagon: (d: any) => d.cell,
          extruded: true, filled: true, wireframe: false,
          elevationScale: 1,
          getElevation: (d: any) => (d.surge - 1) * 900, // 1x=flat, 3x=~1800m
          getFillColor: (d: any) => [...surgeColor(d.surge), 180] as any,
          opacity: 0.75, pickable: false,
          updateTriggers: { getElevation: surgeZones, getFillColor: surgeZones },
        }));
      }

      if (show.ops) {
      if (query.cells.length) {
        layers.push(new H3HexagonLayer({
          id: "disk", data: query.cells, getHexagon: (d: string) => d,
          filled: true, stroked: true, extruded: false,
          getFillColor: [137, 220, 235, 18], getLineColor: [137, 220, 235, 110], lineWidthMinPixels: 1,
        }));
      }
      // trip arcs: driver -> rider
      layers.push(new ArcLayer({
        id: "arcs", data: tripList,
        getSourcePosition: (t: Trip) => [t.dLng, t.dLat],
        getTargetPosition: (t: Trip) => [t.rLng, t.rLat],
        getSourceColor: [245, 194, 231], getTargetColor: [249, 226, 175],
        getWidth: 2, getHeight: 0.3,
      }));
      // riders (pickup targets)
      layers.push(new ScatterplotLayer({
        id: "riders", data: tripList,
        getPosition: (t: Trip) => [t.rLng, t.rLat],
        getRadius: 60, radiusMinPixels: 3, getFillColor: [249, 226, 175],
        stroked: true, getLineColor: [20, 20, 20], lineWidthMinPixels: 1,
      }));
      // free/queried drivers
      layers.push(new ScatterplotLayer({
        id: "drivers", data: drivers,
        getPosition: (d: any) => d.position,
        getRadius: (d: any) => (d.matched ? 70 : 40), radiusMinPixels: 2.5,
        getFillColor: (d: any) => (d.matched ? [180, 190, 254] : [137, 220, 235]),
        opacity: 0.95, updateTriggers: { getFillColor: query.matched, getRadius: query.matched },
      }));
      // on-trip driver markers, animating toward the rider
      layers.push(new ScatterplotLayer({
        id: "trip-markers", data: markers,
        getPosition: (d: any) => d.position,
        getRadius: 80, radiusMinPixels: 4, getFillColor: [245, 194, 231],
        stroked: true, getLineColor: [255, 255, 255], lineWidthMinPixels: 1.5,
      }));
      if (query.center) {
        layers.push(new ScatterplotLayer({
          id: "marker", data: [query.center], getPosition: (d: number[]) => d,
          getRadius: 90, radiusMinPixels: 5, getFillColor: [243, 139, 168],
          stroked: true, getLineColor: [255, 255, 255], lineWidthMinPixels: 2,
        }));
      }
      }
      deck.setProps({ layers });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    // stats panel
    const poll = setInterval(async () => {
      try {
        const s = await (await fetch(STATS)).json();
        setText("mode", s.mode);
        setText("active", s.activeTrips);
        setText("assigned", s.totalAssigned);
        setText("avg", s.avgPickupM ? `${Math.round(s.avgPickupM)} m` : "—");
      } catch {}
    }, 1000);

    // surge poll (only fetch when the layer is on)
    const surgePoll = setInterval(async () => {
      if (!show.surge) return;
      try {
        const j = await (await fetch(SURGE)).json();
        surgeZones = j.zones ?? [];
      } catch {}
    }, 2000);

    return () => { cancelAnimationFrame(raf); clearInterval(poll); clearInterval(surgePoll); ws.close(); deck.finalize(); map.remove(); };
  }, []);

  useEffect(() => { document.getElementById("count")!.textContent = String(count); }, [count]);
  return <div ref={ref} style={{ position: "absolute", inset: 0 }} />;
}

function setText(id: string, v: any) { const el = document.getElementById(id); if (el) el.textContent = String(v); }
function setNear(s: string) { const el = document.getElementById("near"); if (el) el.textContent = s; }
