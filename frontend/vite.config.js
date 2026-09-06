import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import { visualizer } from 'rollup-plugin-visualizer';
import { defineConfig } from 'vite';
import { excalidrawAssetsPlugin } from './scripts/excalidraw-assets.js';

// When PLUGIN_DEV_PORTS is set (e.g. "ldap-config=5561,saml-config=5562,..."),
// add proxy rules that route plugin asset requests to individual Vite dev servers
// for HMR support. These rules are more specific than /api and take priority.
const enableBundleAnalyzer = process.env.ANALYZE === 'true';
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
  // Emit relative asset URLs. The Go server injects a <base> tag into
  // index.html so both root deployments and context-path deployments resolve
  // chunks/assets from the externally visible app root.
  base: './',
  plugins: [
    svelte(), // Uses svelte.config.js for preprocessors
    svelteTesting(),
    tailwindcss(),
    excalidrawAssetsPlugin(),
    ...(enableBundleAnalyzer
      ? [
          visualizer({
            filename: 'dist/bundle-analyzer.html',
            open: false,
            gzipSize: true,
            brotliSize: true,
            template: 'treemap',
          }),
        ]
      : []),
  ],
  optimizeDeps: {
    // React + jsx-runtime are force-included so the dev optimizer emits all
    // exports (Fragment, jsx, jsxs) — needed by Excalidraw's auto-JSX output.
    // Excalidraw and its mermaid bridge are pre-bundled rather than excluded:
    // their dep tree contains CJS packages (es6-promise-pool, sanitize-url,
    // lodash.throttle, …) and Vite's on-demand pre-bundle for an *excluded*
    // package's CJS dependencies produces interop-broken chunks that fail at
    // runtime ("does not provide an export named 'default' / 'Fragment'").
    // The startup cost is bounded since both are only loaded behind a dynamic
    // import — the optimizer just makes sure the pre-bundle is well-formed.
    include: [
      '@milkdown/core',
      '@milkdown/kit',
      '@milkdown/theme-nord',
      'react',
      'react-dom',
      'react/jsx-runtime',
      'react/jsx-dev-runtime',
      '@excalidraw/excalidraw',
      '@excalidraw/mermaid-to-excalidraw',
    ],
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
