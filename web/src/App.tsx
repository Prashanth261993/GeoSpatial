import { useEffect, useRef, useState } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { Deck } from "@deck.gl/core";
import { ScatterplotLayer } from "@deck.gl/layers";

type Pos = { id: string; lat: number; lng: number; ts: number };
type Track = { fromLng: number; fromLat: number; toLng: number; toLat: number; t0: number; t1: number };

const KEY = import.meta.env.VITE_MAPTILER_KEY as string | undefined;
const STYLE = KEY
  ? `https://api.maptiler.com/maps/streets-dark/style.json?key=${KEY}`
  : "https://demotiles.maplibre.org/style.json";
const WS = (import.meta.env.VITE_WS_URL as string) || "ws://localhost:8090/ws";

export function App() {
  const ref = useRef<HTMLDivElement>(null);
  const [count, setCount] = useState(0);

  useEffect(() => {
    const map = new maplibregl.Map({
      container: ref.current!,
      style: STYLE,
      center: [-122.3321, 47.6062],
      zoom: 11.5,
      pitch: 0,
    });
    const deck = new Deck({
      parent: ref.current!,
      initialViewState: { longitude: -122.3321, latitude: 47.6062, zoom: 11.5 },
      controller: true,
      onViewStateChange: ({ viewState: v }) => {
        map.jumpTo({ center: [v.longitude, v.latitude], zoom: v.zoom, bearing: v.bearing, pitch: v.pitch });
      },
      layers: [],
    });

    const tracks = new Map<string, Track>();
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
      const data = [...tracks.values()].map((t) => {
        const a = Math.min(1, (now - t.t0) / (t.t1 - t.t0));
        return [t.fromLng + (t.toLng - t.fromLng) * a, t.fromLat + (t.toLat - t.fromLat) * a];
      });
      setCount(data.length);
      deck.setProps({ layers: [new ScatterplotLayer({ id: "drivers", data, getPosition: (d: number[]) => d, getRadius: 40, radiusMinPixels: 2.5, getFillColor: [137, 220, 235], opacity: 0.9 })] });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    return () => { cancelAnimationFrame(raf); ws.close(); deck.finalize(); map.remove(); };
  }, []);

  useEffect(() => { document.getElementById("count")!.textContent = String(count); }, [count]);
  return <div ref={ref} style={{ position: "absolute", inset: 0 }} />;
}
