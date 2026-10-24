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
    // Vite's default scan crawls every .html in the project, including
    // design-system.html and the stale dist-design-system build. That pulls the
    // entire design-system viewer and every icon library it compares into the
    // dep graph, roughly tripling the pre-bundle and making dev-server startup
    // crawl. This config owns only the app entry.
    entries: ['index.html'],
    // The dev dependency scanner does not walk into route-level lazy-loaded
    // Svelte components, so anything imported only from those must be listed
    // here. Otherwise the first visit registers a missing import, re-runs the
    // whole optimizer (~10 s), and reloads the page.
    //
    // `debug` and `extend` are CJS interop for the Milkdown tree. React +
    // jsx-runtime are force-included so the dev optimizer emits all exports
    // (Fragment, jsx, jsxs) needed by Excalidraw's auto-JSX output. The editor
    // stacks stay pre-bundled: excluding them serves a large CJS tree from
    // source and breaks the editors at runtime.
    include: [
      'react',
      'react-dom',
      'react-dom/client',
      'react/jsx-runtime',
      'react/jsx-dev-runtime',
      'debug',
      'extend',
      '@excalidraw/excalidraw',
      '@excalidraw/mermaid-to-excalidraw',
      '@milkdown/kit',
      '@milkdown/kit/component/image-block',
      '@milkdown/kit/core',
      '@milkdown/kit/plugin/clipboard',
      '@milkdown/kit/plugin/listener',
      '@milkdown/kit/plugin/upload',
      '@milkdown/kit/preset/commonmark',
      '@milkdown/kit/preset/gfm',
      '@milkdown/kit/prose/state',
      '@milkdown/kit/prose/view',
      '@milkdown/kit/utils',
      '@milkdown/utils',
      '@milkdown/theme-nord',
      '@xyflow/svelte',
      '@atlaskit/pragmatic-drag-and-drop/element/adapter',
      '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge',
      '@atlaskit/pragmatic-drag-and-drop-hitbox/tree-item',
      '@atlaskit/pragmatic-drag-and-drop-auto-scroll/element',
      'marked',
      'mermaid',
      'shiki/core',
      'shiki/engine/javascript',
      'shiki/langs/bash.mjs',
      'shiki/langs/c.mjs',
      'shiki/langs/cpp.mjs',
      'shiki/langs/css.mjs',
      'shiki/langs/diff.mjs',
      'shiki/langs/go.mjs',
      'shiki/langs/html.mjs',
      'shiki/langs/java.mjs',
      'shiki/langs/javascript.mjs',
      'shiki/langs/json.mjs',
      'shiki/langs/jsx.mjs',
      'shiki/langs/markdown.mjs',
      'shiki/langs/php.mjs',
      'shiki/langs/python.mjs',
      'shiki/langs/ruby.mjs',
      'shiki/langs/rust.mjs',
      'shiki/langs/scss.mjs',
      'shiki/langs/shellscript.mjs',
      'shiki/langs/sql.mjs',
      'shiki/langs/svelte.mjs',
      'shiki/langs/toml.mjs',
      'shiki/langs/tsx.mjs',
      'shiki/langs/typescript.mjs',
      'shiki/langs/xml.mjs',
      'shiki/langs/yaml.mjs',
      'shiki/themes/github-dark.mjs',
      'shiki/themes/github-light.mjs',
    ],
    // Exclude deps that are only loaded via dynamic import() behind a runtime
    // guard. Pre-bundling them wastes startup time and, when Vite's scanner
    // misses one, the dep is discovered on the first hit and triggers a
    // re-bundle plus full reload. Excluded deps load on demand.
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
    // Pre-transform the app shell and the heaviest route roots right after
    // "ready" so the first navigation does not pay the compile cost (WI-1760).
    // Warmup follows static imports recursively and runs in the background,
    // so startup itself is not delayed. Without it, the first item-detail
    // load after a server restart transforms ~400 modules (~1.8 s); once
    // warm the same walk is ~10 ms.
    warmup: {
      clientFiles: [
        'src/App.svelte',
        'src/lib/pages/MainApp.svelte',
        'src/lib/features/items/ItemDetail.svelte',
        'src/lib/workspaces/WorkspaceDetail.svelte',
        'src/lib/features/collections/CollectionBoard.svelte',
        'src/lib/features/collections/CollectionList.svelte',
        'src/lib/features/assets/AssetBrowser.svelte',
      ],
    },
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
