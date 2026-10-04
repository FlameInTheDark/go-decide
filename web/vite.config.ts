import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig } from "vite"

// The playground is served by the Go binary from an embedded fs, so the build
// has to land in web/dist with root-relative asset URLs and no dev server
// assumptions. `vite dev` proxies /api to a locally running decide-playground
// so the frontend can be worked on without rebuilding the backend.
const apiTarget = process.env.DECIDE_PLAYGROUND ?? "http://127.0.0.1:842"

export default defineConfig({
  base: "/",
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // The frontend is a handful of views; the warning threshold in Vite's
    // default is sized for much larger apps and only adds noise here. The JSON
    // editor is the one chunk over it, and it is lazy, so it costs nothing
    // until the JSON pane is opened.
    chunkSizeWarningLimit: 2800,
  },
  server: {
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: false,
      },
    },
  },
})
