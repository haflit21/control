import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';

// The build lands in the Go server, which embeds it (go:embed ui/dist).
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: fileURLToPath(new URL('../../server/internal/server/ui/dist', import.meta.url)),
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false }, '/login': 'http://127.0.0.1:8080', '/media': 'http://127.0.0.1:8080' },
    fs: { allow: ['../..'] },
  },
});
