import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Dev-only proxy to the Go backend (H1, docs/gui/spec.md § 2) — avoids
// hardcoding http://localhost:8080 in fetch calls / juggling CORS during
// local dev. `ws: true` is required for the /ws proxy entry: Vite doesn't
// upgrade WebSocket connections through the proxy by default.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
})
