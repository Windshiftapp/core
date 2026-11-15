import { onDestroy, tick, untrack } from 'svelte';
import { hydrateCurrentWorkspaceFromSharedData } from '../services/authenticatedShellUI.js';
import { collectionStore } from '../stores/collectionContext.svelte.js';
import { resetScopedNavigation } from '../stores/scopedNavigation.js';
import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
import { loadWorkspaceGradient } from '../stores/workspaceGradient.svelte.js';
import { workspacesStore } from '../stores/workspaces.svelte.js';
import {
  mainAppNavigationIdentity,
  resolveMainAppWorkspaceContext,
} from './mainAppWorkspaceContext.js';

const collectionViews = new Set(['board', 'backlog', 'list', 'tree', 'map', 'roadmap']);

export function useMainAppWorkspaceContext(getRoute, getPersonalWorkspaceId) {
  const identity = $derived(mainAppNavigationIdentity(getRoute(), getPersonalWorkspaceId()));
  let activeIdentity = $state(null);
  let ready = $state(false);
  let generation = $state(0);
  let request = 0;

  // Remove outgoing content first so save-on-leave handlers still see its state.
  $effect.pre(() => {
    const key = identity;
    const route = untrack(getRoute);
    const context = untrack(() => resolveMainAppWorkspaceContext(route, getPersonalWorkspaceId()));
    const token = ++request;
    untrack(() => {
      ready = false;
      generation += 1;
      void activate();
    });

    async function activate() {
      await tick();
      if (token !== request) return;
      resetScopedNavigation();
      if (context.kind === 'workspace') {
        await hydrateCurrentWorkspaceFromSharedData(context.workspaceId);
        if (token !== request) return;
        await loadWorkspaceGradient(context.workspaceId);
      } else if (context.kind === 'personal-pending') {
        void workspacesStore.loadPersonalWorkspace();
      } else if (context.kind === 'global-collection') {
        await workspaceDataStore.initializeGlobal();
      }
      if (token !== request) return;
      const view = route.view;
      if (collectionViews.has(view?.split('-').at(-1))) {
        if (context.kind === 'workspace') {
          void collectionStore.load(context.workspaceId, route.params?.collectionId ?? null, view);
        } else if (context.kind === 'global-collection') {
          void collectionStore.load(null, route.params.id, view);
        }
      }
      activeIdentity = key;
      ready = true;
    }
  });

  onDestroy(() => {
    request += 1;
    resetScopedNavigation();
  });

  return {
    get ready() {
      return ready && activeIdentity === identity;
    },
    get generation() {
      return generation;
    },
  };
}
