import { preprocessMeltUI } from '@melt-ui/pp';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [
    svelte({
      preprocess: [preprocessMeltUI()],
      hot: false, // Disable HMR in tests
    }),
  ],
  test: {
    globals: true,
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{js,ts}'],
    setupFiles: ['./src/setupTests.js'],
    // Prevent CSS import errors in tests
    css: false,
    // Reporter configuration
    reporters: ['verbose'],
    // Coverage configuration
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html'],
      exclude: ['node_modules/', 'src/setupTests.js', '**/*.test.{js,ts}', '**/*.spec.{js,ts}'],
    },
    // Svelte 5 requires browser conditions for component tests
    alias: {
      // Ensure Svelte uses client-side code in tests
      svelte: 'svelte',
    },
  },
  resolve: {
    // Ensure browser conditions are used for Svelte 5
    conditions: ['browser', 'development'],
  },
});
