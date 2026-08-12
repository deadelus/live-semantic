# LiveSemantic — web (H2)

First vertical slice of the GUI (TODO.md § H2, decided 2026-08-12): one
screen — filter input, start/stop, live annotated video — wired against
the H1 backend's existing REST + WebSocket API. Deliberately not the full
design from `docs/gui/mockups/` yet (no sources list/mosaic, no reference
gallery, no theme system) — this validates the transport against a real
UI first.

Camera source today: the backend's own local camera (`CameraInput`, same
device the Go process runs on) — not the browser's camera. Browser-side
capture (`getUserMedia`, JPEG-over-WS first, WebRTC later) is next on the
roadmap, needed for the GUI to work as a real web app from any device
rather than just the machine running the backend.

## Run

Two processes, both required:

```bash
# 1. Backend (from the repo root)
go run ./cmd/livesemantic --web

# 2. This frontend
cd web
npm install
npm run dev
```

Then open the Vite dev server URL (printed on start, typically
`http://localhost:5173`). `vite.config.ts` proxies `/api` and `/ws` to the
backend on `localhost:8080` — no CORS setup needed in dev.

## A note on dependency versions

`package.json` is pinned to conservative, known-real versions (React 18,
Vite 5, TypeScript 5.6) rather than whatever `npm create vite@latest`
resolves at scaffold time — the very first scaffold pulled nonexistent/
malformed versions (`vite@^8.2.0`, pulling in a broken `rolldown`
dependency) that crashed `npm install` with `Invalid Version:` on a real
machine. `npm audit fix` will try to "fix" a moderate dev-only esbuild
advisory by upgrading to that same broken `vite@8.2.1` — don't run it
without checking first.
