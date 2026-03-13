import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { visualizer } from 'rollup-plugin-visualizer';
import { defineConfig } from 'vite';

// When PLUGIN_DEV_PORTS is set (e.g. "ldap-config=5561,saml-config=5562,..."),
// add proxy rules that route plugin asset requests to individual Vite dev servers
// for HMR support. These rules are more specific than /api and take priority.
const pluginProxies = {};
if (process.env.PLUGIN_DEV_PORTS) {
  for (const entry of process.env.PLUGIN_DEV_PORTS.split(',')) {
    const [name, port] = entry.split('=');
    if (name && port) {
      pluginProxies[`/api/plugins/${name}/assets`] = {
        target: `http://localhost:${port}`,
        changeOrigin: true,
        ws: true,
      };
    }
  }
}

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
      ...pluginProxies,
      '/api': {
        target: 'http://localhost:7777',
        changeOrigin: true,
      },
    },
  },
  build: {
    sourcemap: false,
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: '_app',
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              name: 'milkdown',
              test: /@milkdown/,
            },
            {
              name: 'd3',
              test: /d3-(scale|shape|time-format)/,
            },
            {
              name: 'excalidraw',
              test: /@excalidraw\/excalidraw/,
            },
            {
              name: 'svelteflow',
              test: /@xyflow\/svelte/,
            },
            {
              name: 'dnd',
              test: /@atlaskit\/pragmatic-drag-and-drop/,
            },
          ],
        },
      },
    },
  },
});
