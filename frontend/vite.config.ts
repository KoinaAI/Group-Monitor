import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// The Go backend serves the REST API + SSE on 127.0.0.1:8787. In dev we proxy
// /api there so the session cookie stays same-origin. This is a STANDALONE SPA:
// in prod `npm run build` emits dist/ for any static host, which must itself
// reverse-proxy /api → the backend to keep the SameSite=Lax nap_session cookie
// same-origin. Nothing is embedded into the Go binary.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    // Bind all interfaces so the dev server is reachable off-box (0.0.0.0 + ::).
    host: true,
    // Public exposure: accept any Host header (disables DNS-rebinding guard).
    allowedHosts: true,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8787',
        changeOrigin: true,
        // SSE (/api/events) must stream, not buffer.
        configure: (proxy) => {
          proxy.on('proxyRes', (proxyRes) => {
            if (proxyRes.headers['content-type']?.includes('text/event-stream')) {
              proxyRes.headers['cache-control'] = 'no-cache'
            }
          })
        },
      },
    },
  },
})
