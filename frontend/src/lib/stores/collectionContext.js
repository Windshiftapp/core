import { derived, writable } from 'svelte/store';
import { currentRoute } from '../router.js';
import { fetchCollectionItems, fetchCollectionBacklog } from '../features/collections/collectionService.js';

const COLLECTION_VIEWS = new Set([
  'workspace-board', 'workspace-board-config', 'workspace-backlog',
  'workspace-list', 'workspace-tree', 'workspace-map'
]);

// Reload trigger — increment to force a re-fetch without route change
const reloadTrigger = writable(0);

// Track current value for loading states (keep previous items visible while loading)
let _current = { items: [], backlogItems: [], collectionName: 'Default', loading: false };
let _loadId = 0;

/**
 * Main collection data store.
 * Derives from currentRoute — fires whenever route params change.
 * Loads items + backlog via collectionService, exposes via set().
 */
export const collectionData = derived(
  [currentRoute, reloadTrigger],
  ([$route, _trigger], set) => {
    const wsId = $route.params?.id;
    const colId = $route.params?.collectionId || null;
    const view = $route.view;

    // Only load for collection-aware views
    if (!wsId || !COLLECTION_VIEWS.has(view)) return;

    const loadId = ++_loadId;

    // Set loading immediately (keep previous items visible)
    _current = { ..._current, loading: true };
    set(_current);

    Promise.all([
      fetchCollectionItems(wsId, colId),
      fetchCollectionBacklog(wsId, colId),
    ]).then(([itemsResult, backlogResult]) => {
      if (loadId !== _loadId) return; // stale
      _current = {
        items: itemsResult.items,
        backlogItems: backlogResult.items,
        collectionName: itemsResult.collectionName,
        loading: false,
      };
      set(_current);
    }).catch(error => {
      if (loadId !== _loadId) return;
      console.error('[collectionContext] Load failed:', error);
      _current = { ..._current, loading: false };
      set(_current);
    });
  },
  _current // initial value
);

/** Trigger a re-fetch of items for the current route context */
export function reloadCollection() {
  reloadTrigger.update(n => n + 1);
}
