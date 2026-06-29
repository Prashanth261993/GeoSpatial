import { useEffect, useRef, useState } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { Deck } from "@deck.gl/core";
import { ScatterplotLayer } from "@deck.gl/layers";
import { H3HexagonLayer } from "@deck.gl/geo-layers";

type Pos = { id: string; lat: number; lng: number; ts: number };
type Track = { fromLng: number; fromLat: number; toLng: number; toLat: number; t0: number; t1: number };
type Query = { cells: string[]; matched: Set<string>; center: [number, number] | null; radius: number };

const KEY = import.meta.env.VITE_MAPTILER_KEY as string | undefined;
const STYLE = KEY
  ? `https://api.maptiler.com/maps/streets-dark/style.json?key=${KEY}`
  : "https://demotiles.maplibre.org/style.json";
const WS = (import.meta.env.VITE_WS_URL as string) || "ws://localhost:8090/ws";
const NEARBY = (import.meta.env.VITE_NEARBY_URL as string) || "http://localhost:8100/nearby";
const RADIUS = 1200;

export function App() {
  const ref = useRef<HTMLDivElement>(null);
  const [count, setCount] = useState(0);
  const [hud, setHud] = useState<string>("");

  useEffect(() => {
    const map = new maplibregl.Map({
      container: ref.current!,
      style: STYLE,
      center: [-122.3321, 47.6062],
      zoom: 11.5,
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
    const query: Query = { cells: [], matched: new Set(), center: null, radius: RADIUS };

    // Click the map -> ask the H3 index "who's near me".
    const runQuery = async (lng: number, lat: number) => {
      query.center = [lng, lat];
      try {
        const r = await fetch(`${NEARBY}?lat=${lat}&lng=${lng}&radius=${RADIUS}`);
        const j = await r.json();
        query.cells = j.cells ?? [];
        query.matched = new Set((j.drivers ?? []).map((d: { id: string }) => d.id));
        setHud(`${j.matchCount} drivers within ${RADIUS}m · ${j.cells.length} H3 cells scanned · ${j.candidateCount} candidates`);
      } catch {
        setHud("query failed — is the indexer up?");
      }
    };
    deck.setProps({
      onClick: (info: any) => { if (info?.coordinate) runQuery(info.coordinate[0], info.coordinate[1]); },
    });

    const ws = new WebSocket(WS);
    ws.onmessage = (e) => {
      const p: Pos = JSON.parse(e.data);
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
      const drivers = [...tracks.entries()].map(([id, t]) => {
        const a = Math.min(1, (now - t.t0) / (t.t1 - t.t0));
        return {
          id,
          position: [t.fromLng + (t.toLng - t.fromLng) * a, t.fromLat + (t.toLat - t.fromLat) * a],
          matched: query.matched.has(id),
        };
      });
      setCount(drivers.length);

      const layers: any[] = [];
      if (query.cells.length) {
        layers.push(new H3HexagonLayer({
          id: "disk",
          data: query.cells,
          getHexagon: (d: string) => d,
          extruded: false,
          filled: true,
          stroked: true,
          getFillColor: [137, 220, 235, 22],
          getLineColor: [137, 220, 235, 120],
          lineWidthMinPixels: 1,
          pickable: false,
        }));
      }
      layers.push(new ScatterplotLayer({
        id: "drivers",
        data: drivers,
        getPosition: (d: any) => d.position,
        getRadius: (d: any) => (d.matched ? 70 : 40),
        radiusMinPixels: 2.5,
        getFillColor: (d: any) => (d.matched ? [245, 194, 231] : [137, 220, 235]),
        opacity: 0.95,
        updateTriggers: { getFillColor: query.matched, getRadius: query.matched },
      }));
      if (query.center) {
        layers.push(new ScatterplotLayer({
          id: "marker",
          data: [query.center],
          getPosition: (d: number[]) => d,
          getRadius: 90,
          radiusMinPixels: 5,
          getFillColor: [243, 139, 168],
          stroked: true,
          getLineColor: [255, 255, 255],
          lineWidthMinPixels: 2,
        }));
      }
      deck.setProps({ layers });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    return () => { cancelAnimationFrame(raf); ws.close(); deck.finalize(); map.remove(); };
  }, []);

  useEffect(() => { document.getElementById("count")!.textContent = String(count); }, [count]);
  useEffect(() => { const el = document.getElementById("near"); if (el) el.textContent = hud || "click the map to find nearby drivers"; }, [hud]);

  return <div ref={ref} style={{ position: "absolute", inset: 0 }} />;
}
