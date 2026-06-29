# Docs / Landing Site

A cinematic Astro site that narrates the build, one phase at a time — the public
"how we built this" companion to the repo. Deployed free to GitHub Pages.

## Local dev
```bash
cd docs-site
npm install
npm run dev      # http://localhost:4321/GeoSpatial
npm run build    # static output in dist/
```

## Deploy
Pushed automatically by `.github/workflows/pages.yml` on changes under `docs-site/`.
Enable once in repo Settings → Pages → Source: GitHub Actions.
Live at: https://prashanth261993.github.io/GeoSpatial/

## Adding a phase (SOP)
1. Capture visuals into `public/img/phaseN/` (screenshots + a GIF of the live UI).
2. Create `src/components/PhaseNN.astro` (use Phase01 as the template: problem,
   architecture mermaid, key code, result figure, tradeoffs, interview points, next).
3. Import it in `src/pages/index.astro` and flip the roadmap card status.
4. `npm run build` to verify, commit, push — Pages redeploys.
