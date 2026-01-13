import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { preprocessMeltUI } from '@melt-ui/pp'
import { visualizer } from 'rollup-plugin-visualizer'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    svelte({
      preprocess: [preprocessMeltUI()]
    }),
    react(),
    tailwindcss(),
    visualizer({
      filename: 'dist/bundle-analyzer.html',
      open: false,
      gzipSize: true,
      brotliSize: true,
      template: 'treemap'
    })
  ],
  server: {
    port: 5555,
    proxy: {
      '/api': {
        target: 'http://localhost:7777',
        changeOrigin: true,
      }
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          'milkdown': [
            '@milkdown/kit/core',
            '@milkdown/kit/preset/commonmark',
            '@milkdown/kit/preset/gfm',
            '@milkdown/theme-nord'
          ],
          'mermaid': ['mermaid'],
          'd3': ['d3-scale', 'd3-shape', 'd3-time-format'],
          'excalidraw': ['@excalidraw/excalidraw'],
          'svelteflow': ['@xyflow/svelte'],
          'dnd': ['@atlaskit/pragmatic-drag-and-drop']
        }
      }
    }
  }
})
