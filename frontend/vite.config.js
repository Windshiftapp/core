import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
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
    // exclude deps that are only loaded via dynamic import() behind a
    // runtime guard (isTauri, route-level lazy loads). Pre-bundling them
    // wastes startup time and — worse — when Vite's scanner misses one
    // (e.g. @tauri-apps/plugin-dialog) the dep gets discovered on the
    // first hit, triggering a re-bundle and full reload that costs
    // ~30 s in our codebase. Excluded deps are loaded on demand from
    // their original location.
    exclude: [
      '@tauri-apps/api',
      '@tauri-apps/api/path',
      '@tauri-apps/plugin-dialog',
      '@tauri-apps/plugin-fs',
      'tauri-pty',
      '@excalidraw/excalidraw',
      '@excalidraw/mermaid-to-excalidraw',
      '@xterm/xterm',
      '@xterm/addon-fit',
      '@xterm/addon-webgl',
    ],
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
              test: /react|react-dom|@excalidraw\/excalidraw/,
            },
            {
              name: 'svelteflow',
              test: /@xyflow\/svelte/,
            },
            {
              name: 'dnd',
              test: /@atlaskit\/pragmatic-drag-and-drop/,
            },
            {
              name: 'xterm',
              test: /@xterm/,
            },
          ],
        },
      },
    },
  },
});
