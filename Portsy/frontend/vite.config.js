import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      // Shim macOS-only native dep so any accidental import becomes a no-op
      'fsevents': fileURLToPath(new URL('./src/shims/empty.js', import.meta.url)),
    },
  },
  optimizeDeps: {
    // Don’t even try to prebundle it
    exclude: ['fsevents'],
  },
  build: {
    rollupOptions: {
      // And don’t try to bundle it either
      external: ['fsevents'],
    },
  },
  ssr: {
    external: ['fsevents'],
  },
  server: { port: 5173, strictPort: true },
});
