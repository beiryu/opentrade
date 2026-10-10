import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

// Build output goes straight into the Go embed directory so `make build`
// produces one binary. In dev, /api is proxied to a running `opentrade serve`.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/api/ui/dist",
    emptyOutDir: false,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8080",
    },
  },
})
