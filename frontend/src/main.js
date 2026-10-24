import { mount } from 'svelte';
import './app.css';
import {
  clearDevelopmentServiceWorkerState,
  registerMobileServiceWorker,
} from './lib/mobile/serviceWorkerClient.js';
import { installContextPathTranslation } from './lib/runtime/contextPath.js';

// Keep this entry free of store, API and router imports: Vite treats those as
// importer edges for HMR, and anything reachable only from here cannot accept
// a hot update, so editing it would reload the whole page.
installContextPathTranslation();
if (import.meta.env.DEV) {
  // A worker installed by an earlier production build on this origin would keep
  // serving stale modules and stale HTML, which breaks HMR and looks like
  // random regressions. Dev always starts from a clean worker state.
  void clearDevelopmentServiceWorkerState();
} else {
  // Register before the large application chunk loads. This lets installed PWAs
  // receive worker updates and navigation recovery even if startup later fails.
  void registerMobileServiceWorker();
}

const { default: App } = await import('./App.svelte');

const target = document.getElementById('app');
// Keep the no-JavaScript/module-failure recovery visible until the root chunk
// is ready, then remove it before Svelte mounts. The placeholder is ordinary
// document flow, so it never overlays or intercepts the application in E2E.
target.replaceChildren();
const app = mount(App, {
  target,
});

export default app;
