// Custom cinematic glyphs — sleek directional marks that fit the neon theme
// (not clip-art icons). Rendered as white SVG masks so deck.gl IconLayer can
// tint them per state (cyan drivers / lavender aircraft / pink on-trip).
// Each points "up" (north) at angle 0; IconLayer rotates by heading.

function mask(svgInner: string, size = 24) {
  const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='${size}' height='${size}' viewBox='0 0 ${size} ${size}'>${svgInner}</svg>`;
  return { url: "data:image/svg+xml;base64," + btoa(svg), width: size, height: size, mask: true, anchorX: size / 2, anchorY: size / 2 };
}

export const GLYPH = {
  // rounded chevron/arrowhead — ground vehicle
  car: mask(`<path d='M12 3 L20 20 L12 15.5 L4 20 Z' fill='#fff'/>`),
  // slim delta — aircraft
  plane: mask(`<path d='M12 2 L21 21 L12 16 L3 21 Z' fill='#fff' opacity='0.95'/><rect x='11' y='9' width='2' height='9' rx='1' fill='#fff'/>`),
};
