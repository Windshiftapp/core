import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { visualizer } from 'rollup-plugin-visualizer';
import { defineConfig } from 'vite';

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    svelte(), // Uses svelte.config.js for preprocessors
    react(),
    tailwindcss(),
    visualizer({
      filename: 'dist/bundle-analyzer.html',
      open: false,
      gzipSize: true,
      brotliSize: true,
      template: 'treemap',
    }),
  ],
  optimizeDeps: {
    include: ['@milkdown/core', '@milkdown/kit', '@milkdown/theme-nord'],
  },
  server: {
    port: 5555,
    proxy: {
      '/api': {
        target: 'http://localhost:7777',
        changeOrigin: true,
      },
    },
  },
  build: {
    sourcemap: true, // Generate .map files for debugging
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          milkdown: [
            '@milkdown/core',
            '@milkdown/kit/core',
            '@milkdown/kit/preset/commonmark',
            '@milkdown/kit/preset/gfm',
            '@milkdown/kit/plugin/listener',
            '@milkdown/kit/plugin/upload',
            '@milkdown/kit/utils',
            '@milkdown/utils',
            '@milkdown/kit/component/image-block',
            '@milkdown/theme-nord',
          ],
          d3: ['d3-scale', 'd3-shape', 'd3-time-format'],
          excalidraw: ['@excalidraw/excalidraw'],
          svelteflow: ['@xyflow/svelte'],
          dnd: ['@atlaskit/pragmatic-drag-and-drop'],
        },
      },
    },
  },
});
