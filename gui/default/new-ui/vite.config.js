import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  base: '/new-ui/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/rest': 'http://localhost:8384',
      '/meta.js': 'http://localhost:8384',
      '/themes.json': 'http://localhost:8384',
      '/assets/lang': 'http://localhost:8384',
    }
  }
});
