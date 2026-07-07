# ADR 0010: Cinematic frontend as a separate route (libraries + one custom shader)

- Status: Accepted
- Date: 2026-07-06

## Context
The goal explicitly prioritizes superior frontend. We want a jaw-dropping real-time
visualization without compromising the functional operations map, and without a
multi-day custom-shader rabbit-hole.

## Decision
Add react-router to the web SPA: `/` = existing operations map, `/cinematic` = showpiece.
Both share the same WebSocket data layer. The cinematic view renders: glowing motion
trails (deck.gl `TripsLayer` fed by a client-side rolling path buffer with timestamps),
breathing 3D neon surge columns (extruded `H3HexagonLayer`, filtered to multiplier > 1.05),
match-ripple rings (JS-animated), a tilted auto-orbiting camera, a vignette, and a soft
radial glow via a hand-written GLSL `LayerExtension` (`PulseGlow`) that injects a fragment
shader using the disc's local `geometry.uv` — no custom uniforms (portable across deck.gl
v9's UBO uniform model). Time-based pulsing is JS-driven.

## Alternatives
- Enhance the ops map in place: would compromise the functional tool; harder to go all-out.
- All-library (luma bloom post-process): looked good but version-fragile in deck v9 and
  leaves no genuine shader story.
- Heavy custom shaders: strongest flex but high risk/time; one signature effect is enough.
- Query-param view toggle: no dep, but less polished than clean /cinematic URLs.

## Consequences
New deps: react-router-dom only (trails/glow/orbit use deck.gl already present). The
showcase is robust across GPUs (no bloom pass) and deck builds (no custom uniforms).
Surge columns require scarcity to be visible — driven live via the ops sliders. Pure
frontend phase: zero backend changes, same data feed.
