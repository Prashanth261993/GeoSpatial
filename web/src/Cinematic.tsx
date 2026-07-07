import { useEffect, useRef, useState } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { Deck } from "@deck.gl/core";
import { ScatterplotLayer, IconLayer } from "@deck.gl/layers";
import { TripsLayer, H3HexagonLayer } from "@deck.gl/geo-layers";
import { PulseGlow } from "./pulseGlow";
import { GLYPH } from "./glyphs";

const KEY = import.meta.env.VITE_MAPTILER_KEY as string | undefined;
const STYLE = KEY
  ? `https://api.maptiler.com/maps/streets-dark/style.json?key=${KEY}`
  : "https://demotiles.maplibre.org/style.json";
const WS = (import.meta.env.VITE_WS_URL as string) || "ws://localhost:8090/ws";
const SURGE = (import.meta.env.VITE_SURGE_URL as string) || "http://localhost:8120/surge";
const FEEDS = (import.meta.env.VITE_FEEDS_URL as string) || "http://localhost:8130/feeds";

const CENTER: [number, number] = [-122.3321, 47.6062];
const TRAIL_SEC = 9;

// neon palette
const CYAN: [number, number, number] = [90, 220, 255];
const LAV: [number, number, number] = [180, 190, 254];
const PINK: [number, number, number] = [245, 150, 220];

type Trail = { path: [number, number][]; ts: number[]; type: string; hdg: number };
type Ripple = { lng: number; lat: number; t0: number };

export function Cinematic() {
  const ref = useRef<HTMLDivElement>(null);
  const [stats, setStats] = useState({ drivers: 0, aircraft: 0, onTrip: 0 });
  const [show, setShow] = useState({ air: false, surge: true, glyphs: true });
  const showRef = useRef(show);
  showRef.current = show;

  useEffect(() => {
    document.body.classList.add("cinematic");
    const t0 = performance.now();
    const clock = () => (performance.now() - t0) / 1000;

    const map = new maplibregl.Map({
      container: ref.current!, style: STYLE,
      center: CENTER, zoom: 12, pitch: 55, bearing: 0, interactive: false,
    });

    const view = { longitude: CENTER[0], latitude: CENTER[1], zoom: 12, pitch: 55, bearing: 0 };
    const deck = new Deck({
      parent: ref.current!, viewState: view, controller: true,
      onViewStateChange: ({ viewState: v }: any) => {
        Object.assign(view, v);
        map.jumpTo({ center: [v.longitude, v.latitude], zoom: v.zoom, bearing: v.bearing, pitch: v.pitch });
      },
      layers: [],
    });

    const trails = new Map<string, Trail>();
    const ripples: Ripple[] = [];
    const onTrip = new Map<string, number>(); // driverId -> assigned clock time
    let surge: { cell: string; surge: number }[] = [];

    const bearing = (a: [number, number], b: [number, number]) => {
      // screen-space heading for a north-up glyph: 0=up, clockwise
      return (Math.atan2(b[0] - a[0], b[1] - a[1]) * 180) / Math.PI;
    };

    const push = (id: string, lng: number, lat: number, type: string, hdg?: number) => {
      let tr = trails.get(id);
      if (!tr) { tr = { path: [], ts: [], type, hdg: hdg ?? 0 }; trails.set(id, tr); }
      const now = clock();
      const prev = tr.path[tr.path.length - 1];
      tr.path.push([lng, lat]); tr.ts.push(now);
      if (type === "aircraft" && hdg != null) tr.hdg = hdg;
      else if (prev) tr.hdg = bearing(prev, [lng, lat]); // derive heading from motion
      while (tr.ts.length > 2 && now - tr.ts[0] > TRAIL_SEC) { tr.path.shift(); tr.ts.shift(); }
    };

    const ws = new WebSocket(WS);
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.kind === "trip") {
        if (m.event === "assigned") { ripples.push({ lng: m.riderLng, lat: m.riderLat, t0: clock() }); onTrip.set(m.driver, clock()); }
        else if (m.event === "completed") onTrip.delete(m.driver);
        return;
      }
      push(m.id, m.lng, m.lat, m.type || "driver", m.hdg);
    };

    const surgePoll = setInterval(async () => {
      try { const j = await (await fetch(SURGE)).json(); surge = j.zones ?? []; } catch {}
    }, 2000);

    let raf = 0;
    const tick = () => {
      const now = clock();
      const s = showRef.current;
      view.bearing = (view.bearing + 0.06) % 360;
      map.jumpTo({ center: [view.longitude, view.latitude], zoom: view.zoom, bearing: view.bearing, pitch: view.pitch });

      for (const [id, tr] of trails) { if (now - tr.ts[tr.ts.length - 1] > 8) trails.delete(id); }
      // expire stale on-trip flags (safety)
      for (const [id, t] of onTrip) { if (now - t > 20) onTrip.delete(id); }

      const driverTrails: Trail[] = [], tripTrails: Trail[] = [], aircraftTrails: Trail[] = [];
      const heads: { id: string; position: [number, number]; type: string; hdg: number; onTrip: boolean }[] = [];
      for (const [id, tr] of trails) {
        const isTrip = onTrip.has(id);
        if (tr.type === "aircraft") aircraftTrails.push(tr);
        else (isTrip ? tripTrails : driverTrails).push(tr);
        const last = tr.path[tr.path.length - 1];
        if (last) heads.push({ id, position: last, type: tr.type, hdg: tr.hdg, onTrip: isTrip });
      }
      setStats({ drivers: driverTrails.length + tripTrails.length, aircraft: aircraftTrails.length, onTrip: tripTrails.length });

      for (let i = ripples.length - 1; i >= 0; i--) if (now - ripples[i].t0 > 2.5) ripples.splice(i, 1);

      const layers: any[] = [];

      if (s.surge) {
        const hot = surge.filter((z) => z.surge > 1.05);
        if (hot.length) {
          const breathe = 0.85 + 0.15 * Math.sin(now * 1.5);
          layers.push(new H3HexagonLayer({
            id: "surge", data: hot, getHexagon: (d: any) => d.cell, extruded: true, filled: true,
            getElevation: (d: any) => (d.surge - 1) * 1400 * breathe,
            getFillColor: (d: any) => { const t = Math.min(1, (d.surge - 1) / 2); return [120 + 135 * t, 90, 190 - 80 * t, 150]; },
            opacity: 0.6, updateTriggers: { getElevation: now, getFillColor: hot },
          }));
        }
      }

      const trip = (id: string, data: Trail[], color: number[], w: number, op: number) => new TripsLayer({
        id, data, getPath: (t: Trail) => t.path, getTimestamps: (t: Trail) => t.ts,
        getColor: color as any, currentTime: now, trailLength: TRAIL_SEC, fadeTrail: true,
        widthMinPixels: w, jointRounded: true, capRounded: true, opacity: op,
      });
      layers.push(trip("driver-trails", driverTrails, CYAN, 2.2, 0.85));
      layers.push(trip("trip-trails", tripTrails, PINK, 3, 0.95)); // matched drivers glow pink
      if (s.air) layers.push(trip("aircraft-trails", aircraftTrails, LAV, 2, 0.8));

      // soft radial glow (custom GLSL falloff) with breathing pulse
      const pulse = 0.8 + 0.2 * Math.sin(now * 2.2);
      const headColor = (h: any) => h.onTrip ? PINK : (h.type === "aircraft" ? LAV : CYAN);
      const glowHeads = heads.filter((h) => s.air || h.type !== "aircraft");
      layers.push(new ScatterplotLayer({
        id: "glow", data: glowHeads, getPosition: (d: any) => d.position,
        getRadius: (d: any) => (d.type === "aircraft" ? 700 : d.onTrip ? 460 : 360) * pulse, radiusUnits: "meters", radiusMinPixels: 6,
        getFillColor: (d: any) => [...headColor(d), 70] as any,
        extensions: [new PulseGlow()], updateTriggers: { getRadius: now, getFillColor: [...onTrip.keys()].join() },
      }));

      // custom cinematic glyphs at the trail head, oriented by heading
      if (s.glyphs) {
        layers.push(new IconLayer({
          id: "glyphs", data: glowHeads,
          getIcon: (d: any) => (d.type === "aircraft" ? GLYPH.plane : GLYPH.car),
          getPosition: (d: any) => d.position, getAngle: (d: any) => -d.hdg,
          getSize: (d: any) => (d.type === "aircraft" ? 26 : d.onTrip ? 24 : 18), sizeUnits: "pixels",
          getColor: (d: any) => headColor(d) as any,
          updateTriggers: { getAngle: now, getColor: [...onTrip.keys()].join(), getSize: [...onTrip.keys()].join() },
        }));
      }

      // match ripples
      layers.push(new ScatterplotLayer({
        id: "ripples", data: ripples.map((r) => ({ ...r, age: now - r.t0 })),
        getPosition: (d: any) => [d.lng, d.lat],
        getRadius: (d: any) => 120 + d.age * 900, radiusUnits: "meters", radiusMinPixels: 2,
        stroked: true, filled: false, getLineWidth: 8, lineWidthUnits: "meters", lineWidthMinPixels: 1.5,
        getLineColor: (d: any) => [245, 150, 220, Math.max(0, 220 * (1 - d.age / 2.5))],
        updateTriggers: { getRadius: now, getLineColor: now },
      }));

      deck.setProps({ viewState: { ...view }, layers });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    return () => {
      cancelAnimationFrame(raf); clearInterval(surgePoll); ws.close(); deck.finalize(); map.remove();
      document.body.classList.remove("cinematic");
    };
  }, []);

  const toggleAir = async () => {
    const next = !show.air;
    setShow((p) => ({ ...p, air: next }));
    try { await fetch(FEEDS, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ feed: "opensky", enabled: next }) }); } catch {}
  };

  return (
    <>
      <div ref={ref} style={{ position: "absolute", inset: 0, background: "#04060b" }} />
      <div className="cine-vignette" />
      <div className="cine-hud">
        <div className="cine-title">SEATTLE <span>· LIVE</span></div>
        <div className="cine-sub">{stats.drivers} drivers · {stats.onTrip} on trip · {stats.aircraft} aircraft</div>
      </div>
      <div className="cine-ctrl">
        <button className={show.air ? "on" : ""} onClick={toggleAir}>✈ aircraft</button>
        <button className={show.surge ? "on" : ""} onClick={() => setShow((p) => ({ ...p, surge: !p.surge }))}>surge</button>
        <button className={show.glyphs ? "on" : ""} onClick={() => setShow((p) => ({ ...p, glyphs: !p.glyphs }))}>glyphs</button>
      </div>
      <a className="cine-back" href="#/">← ops view</a>
    </>
  );
}
