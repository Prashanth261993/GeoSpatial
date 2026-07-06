import { useEffect, useRef, useState } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { Deck } from "@deck.gl/core";
import { ScatterplotLayer, ArcLayer, IconLayer } from "@deck.gl/layers";
import { H3HexagonLayer } from "@deck.gl/geo-layers";

type Pos = { id: string; lat: number; lng: number; ts: number; type?: string; hdg?: number; callsign?: string; country?: string; altM?: number; velMps?: number; vRateMps?: number };
type Track = { fromLng: number; fromLat: number; toLng: number; toLat: number; t0: number; t1: number };
type Air = Track & { hdg: number; callsign: string; country: string; altM: number; velMps: number; vRateMps: number };
type Trip = { reqId: string; driver: string; dLat: number; dLng: number; rLat: number; rLng: number; t0: number; durMs: number };
type Wait = { lng: number; lat: number; t0: number };
type Query = { cells: string[]; matched: Set<string>; center: [number, number] | null };

const KEY = import.meta.env.VITE_MAPTILER_KEY as string | undefined;
const STYLE = KEY
  ? `https://api.maptiler.com/maps/streets-dark/style.json?key=${KEY}`
  : "https://demotiles.maplibre.org/style.json";
const WS = (import.meta.env.VITE_WS_URL as string) || "ws://localhost:8090/ws";
const NEARBY = (import.meta.env.VITE_NEARBY_URL as string) || "http://localhost:8100/nearby";
const STATS = (import.meta.env.VITE_STATS_URL as string) || "http://localhost:8110/stats";
const SURGE = (import.meta.env.VITE_SURGE_URL as string) || "http://localhost:8120/surge";
const FEEDS = (import.meta.env.VITE_FEEDS_URL as string) || "http://localhost:8130/feeds";
const RADIUS = 1200;

function surgeColor(m: number): [number, number, number] {
  const t = Math.min(1, Math.max(0, (m - 1) / 2));
  return [Math.round(80 + 175 * t), Math.round(200 - 140 * t), Math.round(235 - 160 * t)];
}

// White mask SVGs (tinted per type/state via IconLayer getColor + mask:true).
function icon(path: string, w = 24, h = 24) {
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='48' height='48' viewBox='0 0 ${w} ${h}'><path d='${path}' fill='#fff'/></svg>`;
  return { url: "data:image/svg+xml;base64," + btoa(svg), width: 48, height: 48, mask: true, anchorX: 24, anchorY: 24 };
}
const ICON = {
  car: icon("M3 13l1.5-4.5A2 2 0 016.4 7h11.2a2 2 0 011.9 1.5L21 13v5a1 1 0 01-1 1h-1a1 1 0 01-1-1v-1H6v1a1 1 0 01-1 1H4a1 1 0 01-1-1zM6.5 16a1.5 1.5 0 100-3 1.5 1.5 0 000 3zm11 0a1.5 1.5 0 100-3 1.5 1.5 0 000 3z"),
  pin: icon("M12 2a7 7 0 00-7 7c0 5 7 13 7 13s7-8 7-13a7 7 0 00-7-7zm0 9.5A2.5 2.5 0 1112 6a2.5 2.5 0 010 5.5z"),
  plane: icon("M12 2c-.8 0-1.4.9-1.4 2v5.3L3 14v1.7l7.6-2.2V18l-2 1.4V21l3.4-1 3.4 1v-1.6l-2-1.4v-3.5l7.6 2.2V14l-7.6-4.7V4c0-1.1-.6-2-1.4-2z"),
};

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
      controller: true, getCursor: () => "crosshair", pickingRadius: 12,
      onViewStateChange: ({ viewState: v }) => {
        map.jumpTo({ center: [v.longitude, v.latitude], zoom: v.zoom, bearing: v.bearing, pitch: v.pitch });
      },
      layers: [],
    });

    const tracks = new Map<string, Track>();
    const aircraft = new Map<string, Air>();
    const trips = new Map<string, Trip>();
    const waiting = new Map<string, Wait>(); // reqId -> unassigned rider
    const query: Query = { cells: [], matched: new Set(), center: null };
    let surgeZones: { cell: string; surge: number }[] = [];
    const show = { ops: true, surge: false, air: false };

    const opsEl = document.getElementById("tgl-ops") as HTMLInputElement;
    const surgeEl = document.getElementById("tgl-surge") as HTMLInputElement;
    const airEl = document.getElementById("tgl-air") as HTMLInputElement;
    opsEl?.addEventListener("change", () => { show.ops = opsEl.checked; });
    surgeEl?.addEventListener("change", () => { show.surge = surgeEl.checked; });
    airEl?.addEventListener("change", async () => {
      show.air = airEl.checked;
      setAir(airEl.checked ? "starting live feed…" : "");
      try {
        await fetch(FEEDS, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ feed: "opensky", enabled: airEl.checked }) });
      } catch { setAir("feed control failed"); }
      if (!airEl.checked) aircraft.clear();
    });

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
    deck.setProps({
      onClick: (info: any) => { if (info?.coordinate && !info?.object) runQuery(info.coordinate[0], info.coordinate[1]); },
      getTooltip: (info: any) => {
        const o = info?.object;
        if (!o || o.kind !== "aircraft") return null;
        const alt = o.altM ? `${Math.round(o.altM * 3.281).toLocaleString()} ft` : "—";
        const spd = o.velMps ? `${Math.round(o.velMps * 1.944)} kts` : "—";
        const vr = o.vRateMps ? (o.vRateMps > 0 ? `▲ ${Math.round(o.vRateMps * 196.85)} ft/min` : `▼ ${Math.round(-o.vRateMps * 196.85)} ft/min`) : "level";
        return {
          html: `<div style="font:12px ui-monospace,monospace"><b style="color:#89dceb">${o.callsign || o.id}</b><br/>${o.country || ""}<br/>alt ${alt} · ${spd}<br/>${vr} · hdg ${Math.round(o.hdg)}°</div>`,
          style: { background: "#0b0e14f2", border: "1px solid #2a3346", borderRadius: "8px", color: "#cdd6f4", padding: "8px 10px" },
        };
      },
    });

    const ws = new WebSocket(WS);
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.kind === "trip") {
        if (m.event === "requested") {
          waiting.set(m.reqId, { lng: m.riderLng, lat: m.riderLat, t0: performance.now() });
        } else if (m.event === "assigned") {
          waiting.delete(m.reqId);
          trips.set(m.reqId, { reqId: m.reqId, driver: m.driver, dLat: m.driverLat, dLng: m.driverLng, rLat: m.riderLat, rLng: m.riderLng, t0: performance.now(), durMs: m.durMs });
        } else if (m.event === "completed") {
          const t = trips.get(m.reqId);
          trips.delete(m.reqId);
          // Snap the driver to the drop-off (rider) so it resumes from there
          // instead of teleporting to wherever the simulator drifted it.
          if (t) tracks.set(t.driver, { fromLng: t.rLng, fromLat: t.rLat, toLng: t.rLng, toLat: t.rLat, t0: performance.now(), t1: performance.now() + 1000 });
        } else if (m.event === "unmatched") {
          waiting.set(m.reqId, { lng: m.riderLng, lat: m.riderLat, t0: performance.now() });
        }
        return;
      }
      const p: Pos = m;
      const now = performance.now();
      if (p.type === "aircraft") {
        const prev = aircraft.get(p.id);
        aircraft.set(p.id, { fromLng: prev?.toLng ?? p.lng, fromLat: prev?.toLat ?? p.lat, toLng: p.lng, toLat: p.lat, t0: now, t1: now + 12000, hdg: p.hdg ?? 0, callsign: p.callsign || "", country: p.country || "", altM: p.altM || 0, velMps: p.velMps || 0, vRateMps: p.vRateMps || 0 });
        return;
      }
      const prev = tracks.get(p.id);
      tracks.set(p.id, { fromLng: prev?.toLng ?? p.lng, fromLat: prev?.toLat ?? p.lat, toLng: p.lng, toLat: p.lat, t0: now, t1: now + 1000 });
    };

    let raf = 0;
    const tick = () => {
      const now = performance.now();
      const onTrip = new Set([...trips.values()].map((t) => t.driver));
      const lerp = (t: Track) => { const a = Math.min(1, (now - t.t0) / (t.t1 - t.t0)); return [t.fromLng + (t.toLng - t.fromLng) * a, t.fromLat + (t.toLat - t.fromLat) * a]; };

      const drivers = [...tracks.entries()].filter(([id]) => !onTrip.has(id)).map(([id, t]) => ({ id, position: lerp(t), matched: query.matched.has(id) }));
      setCount(tracks.size);
      const tripList = [...trips.values()];
      const markers = tripList.map((t) => { const a = Math.min(1, (now - t.t0) / t.durMs); return { position: [t.dLng + (t.rLng - t.dLng) * a, t.dLat + (t.rLat - t.dLat) * a] }; });
      const airList = [...aircraft.entries()].map(([id, a]) => ({ kind: "aircraft", id, position: lerp(a), hdg: a.hdg, callsign: a.callsign, country: a.country, altM: a.altM, velMps: a.velMps, vRateMps: a.vRateMps }));

      // Expire stale waiting riders (unmatched linger, then fade after 15s).
      for (const [id, w] of waiting) { if (now - w.t0 > 15000) waiting.delete(id); }
      const waitList = [...waiting.values()].map((w) => ({ position: [w.lng, w.lat], age: now - w.t0 }));
      setText("waiting", String(waiting.size));

      const layers: any[] = [];
      if (show.surge && surgeZones.length) {
        layers.push(new H3HexagonLayer({
          id: "surge", data: surgeZones, getHexagon: (d: any) => d.cell, extruded: true, filled: true,
          getElevation: (d: any) => (d.surge - 1) * 900, getFillColor: (d: any) => [...surgeColor(d.surge), 180] as any,
          opacity: 0.75, updateTriggers: { getElevation: surgeZones, getFillColor: surgeZones },
        }));
      }
      if (show.ops) {
        if (query.cells.length) {
          layers.push(new H3HexagonLayer({ id: "disk", data: query.cells, getHexagon: (d: string) => d, filled: true, stroked: true, extruded: false, getFillColor: [137, 220, 235, 18], getLineColor: [137, 220, 235, 110], lineWidthMinPixels: 1 }));
        }
        layers.push(new ArcLayer({ id: "arcs", data: tripList, getSourcePosition: (t: Trip) => [t.dLng, t.dLat], getTargetPosition: (t: Trip) => [t.rLng, t.rLat], getSourceColor: [245, 194, 231], getTargetColor: [249, 226, 175], getWidth: 2, getHeight: 0.3 }));
        // waiting riders (unassigned): pulsing halo + pin
        layers.push(new ScatterplotLayer({
          id: "waiting-halo", data: waitList, getPosition: (d: any) => d.position,
          getRadius: (d: any) => 70 + 55 * Math.abs(Math.sin((now + d.age) / 450)),
          radiusUnits: "meters", radiusMinPixels: 6, getFillColor: [249, 226, 175, 40],
          updateTriggers: { getRadius: now },
        }));
        layers.push(new IconLayer({ id: "waiting", data: waitList, getIcon: () => ICON.pin, getPosition: (d: any) => d.position, getSize: 22, sizeUnits: "pixels", getColor: [249, 226, 175] }));
        // drivers as cars (matched = lavender, else cyan)
        layers.push(new IconLayer({ id: "drivers", data: drivers, getIcon: () => ICON.car, getPosition: (d: any) => d.position, getSize: (d: any) => (d.matched ? 26 : 20), sizeUnits: "pixels", getColor: (d: any) => (d.matched ? [180, 190, 254] : [137, 220, 235]), updateTriggers: { getColor: query.matched, getSize: query.matched } }));
        // on-trip cars in pink (heading to pickup)
        layers.push(new IconLayer({ id: "trip-cars", data: markers, getIcon: () => ICON.car, getPosition: (d: any) => d.position, getSize: 24, sizeUnits: "pixels", getColor: [245, 194, 231] }));
        if (query.center) {
          layers.push(new ScatterplotLayer({ id: "marker", data: [query.center], getPosition: (d: number[]) => d, getRadius: 90, radiusMinPixels: 5, getFillColor: [243, 139, 168], stroked: true, getLineColor: [255, 255, 255], lineWidthMinPixels: 2 }));
        }
      }
      // real aircraft (toggle): plane glyphs rotated by heading, clickable for details
      if (show.air && airList.length) {
        setAir(`${airList.length} live aircraft · click for details`);
        layers.push(new IconLayer({
          id: "aircraft", data: airList, getIcon: () => ICON.plane,
          getPosition: (d: any) => d.position, getSize: 30, sizeUnits: "pixels",
          getAngle: (d: any) => -d.hdg, getColor: [180, 190, 254],
          pickable: true, updateTriggers: { getAngle: now },
        }));
      }
      deck.setProps({ layers });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    const poll = setInterval(async () => {
      try { const s = await (await fetch(STATS)).json(); setText("mode", s.mode); setText("active", s.activeTrips); setText("assigned", s.totalAssigned); setText("avg", s.avgPickupM ? `${Math.round(s.avgPickupM)} m` : "—"); } catch {}
    }, 1000);
    const surgePoll = setInterval(async () => { if (!show.surge) return; try { const j = await (await fetch(SURGE)).json(); surgeZones = j.zones ?? []; } catch {} }, 2000);

    return () => { cancelAnimationFrame(raf); clearInterval(poll); clearInterval(surgePoll); ws.close(); deck.finalize(); map.remove(); };
  }, []);

  useEffect(() => { document.getElementById("count")!.textContent = String(count); }, [count]);
  return <div ref={ref} style={{ position: "absolute", inset: 0 }} />;
}

function setText(id: string, v: any) { const el = document.getElementById(id); if (el) el.textContent = String(v); }
function setNear(s: string) { const el = document.getElementById("near"); if (el) el.textContent = s; }
function setAir(s: string) { const el = document.getElementById("air"); if (el) el.textContent = s; }
