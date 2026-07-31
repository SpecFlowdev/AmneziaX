import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The bundle is embedded in the Go binary, so it is emitted straight into the
// package that serves it.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 900,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/sub': 'http://127.0.0.1:8080',
    },
  },
})
