// Desktop (Tauri) clients have no native F5 refresh and tend to accumulate
// stale state when the user returns to the window after long absences. On
// visibility regaining, opportunistically re-fetch the data the user is most
// likely looking at right now. Browser users get a no-op; they already have
// F5 and tend to keep tabs fresh through normal navigation.

import { collectionStore } from '../stores/collectionContext.js';
import { itemDetailStore } from '../stores/itemDetailStore.svelte.js';
import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
import { isTauri } from './isTauri.js';

let installed = false;

export function initDesktopFocusRefresh() {
  if (installed || !isTauri()) return;
  installed = true;

  let inFlight = false;
  document.addEventListener('visibilitychange', async () => {
    if (document.hidden || inFlight) return;
    inFlight = true;
    try {
      const tasks = [];
      if (workspaceDataStore.workspaceId) {
        tasks.push(workspaceDataStore.refresh());
      }
      if (itemDetailStore.itemId && itemDetailStore.workspaceId) {
        tasks.push(itemDetailStore.loadItem(itemDetailStore.workspaceId, itemDetailStore.itemId));
      }
      tasks.push(Promise.resolve(collectionStore.reload()));
      await Promise.allSettled(tasks);
    } finally {
      inFlight = false;
    }
  });
}
