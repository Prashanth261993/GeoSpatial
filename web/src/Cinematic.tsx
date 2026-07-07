import { useEffect, useRef, useState } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { Deck } from "@deck.gl/core";
import { ScatterplotLayer } from "@deck.gl/layers";
import { TripsLayer, H3HexagonLayer } from "@deck.gl/geo-layers";
import { PulseGlow } from "./pulseGlow";

const KEY = import.meta.env.VITE_MAPTILER_KEY as string | undefined;
const STYLE = KEY
  ? `https://api.maptiler.com/maps/streets-dark/style.json?key=${KEY}`
  : "https://demotiles.maplibre.org/style.json";
const WS = (import.meta.env.VITE_WS_URL as string) || "ws://localhost:8090/ws";
const SURGE = (import.meta.env.VITE_SURGE_URL as string) || "http://localhost:8120/surge";

const CENTER: [number, number] = [-122.3321, 47.6062];
const TRAIL_SEC = 9; // seconds of visible trail

type Trail = { path: [number, number][]; ts: number[]; type: string };
type Ripple = { lng: number; lat: number; t0: number };

export function Cinematic() {
  const ref = useRef<HTMLDivElement>(null);
  const [stats, setStats] = useState({ drivers: 0, aircraft: 0, trips: 0 });

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
    let surge: { cell: string; surge: number }[] = [];

    const push = (id: string, lng: number, lat: number, type: string) => {
      let tr = trails.get(id);
      if (!tr) { tr = { path: [], ts: [], type }; trails.set(id, tr); }
      const now = clock();
      tr.path.push([lng, lat]); tr.ts.push(now);
      // trim points older than the trail window
      while (tr.ts.length > 2 && now - tr.ts[0] > TRAIL_SEC) { tr.path.shift(); tr.ts.shift(); }
    };

    const ws = new WebSocket(WS);
    ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.kind === "trip") {
        if (m.event === "assigned") ripples.push({ lng: m.riderLng, lat: m.riderLat, t0: clock() });
        return;
      }
      push(m.id, m.lng, m.lat, m.type || "driver");
    };

    const surgePoll = setInterval(async () => {
      try { const j = await (await fetch(SURGE)).json(); surge = j.zones ?? []; } catch {}
    }, 2000);

    let raf = 0;
    const tick = () => {
      const now = clock();
      // slow auto-orbit
      view.bearing = (view.bearing + 0.06) % 360;
      deck.setProps({ viewState: { ...view } });
      map.jumpTo({ center: [view.longitude, view.latitude], zoom: view.zoom, bearing: view.bearing, pitch: view.pitch });

      // drop entities with no recent update
      for (const [id, tr] of trails) { if (now - tr.ts[tr.ts.length - 1] > 8) trails.delete(id); }
      const drivers: Trail[] = [], aircraft: Trail[] = [];
      const glow: { position: [number, number]; type: string }[] = [];
      for (const tr of trails.values()) {
        (tr.type === "aircraft" ? aircraft : drivers).push(tr);
        const last = tr.path[tr.path.length - 1];
        if (last) glow.push({ position: last, type: tr.type });
      }
      setStats({ drivers: drivers.length, aircraft: aircraft.length, trips: ripples.length });

      // expire ripples (~2.5s life)
      for (let i = ripples.length - 1; i >= 0; i--) if (now - ripples[i].t0 > 2.5) ripples.splice(i, 1);

      const layers: any[] = [];

      // breathing neon surge columns — only render zones that are actually
      // surging so calm areas don't wash out the trails.
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

      // glowing motion trails (batched: one layer for all drivers, one for aircraft)
      layers.push(new TripsLayer({
        id: "driver-trails", data: drivers, getPath: (t: Trail) => t.path, getTimestamps: (t: Trail) => t.ts,
        getColor: [90, 220, 255], currentTime: now, trailLength: TRAIL_SEC, fadeTrail: true,
        widthMinPixels: 2.2, jointRounded: true, capRounded: true, opacity: 0.85,
      }));
      layers.push(new TripsLayer({
        id: "aircraft-trails", data: aircraft, getPath: (t: Trail) => t.path, getTimestamps: (t: Trail) => t.ts,
        getColor: [180, 190, 254], currentTime: now, trailLength: TRAIL_SEC, fadeTrail: true,
        widthMinPixels: 2, jointRounded: true, capRounded: true, opacity: 0.8,
      }));

      // soft radial glow halo (custom GLSL falloff) with a JS breathing pulse
      const pulse = 0.8 + 0.2 * Math.sin(now * 2.2);
      layers.push(new ScatterplotLayer({
        id: "glow", data: glow, getPosition: (d: any) => d.position,
        getRadius: (d: any) => (d.type === "aircraft" ? 700 : 380) * pulse, radiusUnits: "meters", radiusMinPixels: 6,
        getFillColor: (d: any) => (d.type === "aircraft" ? [180, 190, 254, 70] : [90, 220, 255, 70]),
        extensions: [new PulseGlow()], updateTriggers: { getRadius: now },
      }));
      // bright cores
      layers.push(new ScatterplotLayer({
        id: "cores", data: glow, getPosition: (d: any) => d.position,
        getRadius: 30, radiusUnits: "meters", radiusMinPixels: 2,
        getFillColor: (d: any) => (d.type === "aircraft" ? [220, 225, 255] : [200, 245, 255]),
      }));

      // match ripples — expanding fading rings at pickups
      layers.push(new ScatterplotLayer({
        id: "ripples", data: ripples.map((r) => ({ ...r, age: now - r.t0 })),
        getPosition: (d: any) => [d.lng, d.lat],
        getRadius: (d: any) => 120 + d.age * 900, radiusUnits: "meters", radiusMinPixels: 2,
        stroked: true, filled: false, getLineWidth: 8, lineWidthUnits: "meters", lineWidthMinPixels: 1.5,
        getLineColor: (d: any) => [245, 194, 231, Math.max(0, 220 * (1 - d.age / 2.5))],
        updateTriggers: { getRadius: now, getLineColor: now },
      }));

      deck.setProps({ layers });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    return () => {
      cancelAnimationFrame(raf); clearInterval(surgePoll); ws.close(); deck.finalize(); map.remove();
      document.body.classList.remove("cinematic");
    };
  }, []);

  return (
    <>
      <div ref={ref} style={{ position: "absolute", inset: 0, background: "#04060b" }} />
      <div className="cine-vignette" />
      <div className="cine-hud">
        <div className="cine-title">SEATTLE <span>· LIVE</span></div>
        <div className="cine-sub">{stats.drivers} drivers · {stats.aircraft} aircraft · real-time</div>
      </div>
      <a className="cine-back" href="#/">← ops view</a>
    </>
  );
}
