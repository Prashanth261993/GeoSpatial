import { LayerExtension } from "@deck.gl/core";

// PulseGlow — a custom deck.gl LayerExtension that injects GLSL into a
// ScatterplotLayer's fragment shader to turn each flat disc into a soft radial
// neon glow (bright core → transparent edge), instead of a hard-edged circle.
//
// It uses the disc's built-in local coordinate (geometry.uv, -1..1 across the
// sprite) so no custom uniforms are needed — robust across deck.gl builds while
// still being hand-written GLSL. The time-based pulsing of size/intensity is
// driven from JS per-frame; this shader owns the spatial falloff.
export class PulseGlow extends LayerExtension {
  getShaders() {
    return {
      inject: {
        // radial falloff: alpha fades from center to rim with an eased curve,
        // plus a subtle hot core boost — the essence of a bloom/neon look.
        "fs:DECKGL_FILTER_COLOR": `
          float d = length(geometry.uv);
          float glow = 1.0 - smoothstep(0.0, 1.0, d);
          glow = pow(glow, 1.8);
          color.a *= glow;
          color.rgb += glow * 0.35; // hot core bleed toward white
        `,
      },
    };
  }
}
