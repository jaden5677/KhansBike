import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Where `make run` serves the Go API in development (HTTP_ADDR). The dev
// server forwards API and media requests there, so the browser sees a single
// origin, exactly as in production, where api.exe serves both.
const apiServer = 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': apiServer,
      '/media': apiServer,
    },
  },
  build: {
    // web/web.go embeds this folder into the Go binary.
    outDir: '../web/dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['src/test/setup.ts'],
  },
})
