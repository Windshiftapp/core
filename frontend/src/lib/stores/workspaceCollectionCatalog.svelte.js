import { collections } from '../api/collections.js';
import { onCollectionsChanged } from '../utils/collectionChanges.js';

// This catalog belongs to the shell's navigation lifetime, not the session.
let revision = $state(0);
let controller = new AbortController();
const entries = new Map();

function reset() {
  controller.abort();
  controller = new AbortController();
  entries.clear();
  revision += 1;
}

onCollectionsChanged(reset);

export const workspaceCollectionCatalog = {
  get revision() {
    return revision;
  },
  reset,
  load(workspaceId) {
    const id = Number(workspaceId);
    if (!Number.isInteger(id) || id <= 0) return Promise.resolve([]);
    if (entries.has(id)) return entries.get(id);
    const signal = controller.signal;
    const pending = collections
      .getAll({ workspace_id: id }, { signal })
      .then((rows) => {
        signal.throwIfAborted();
        return rows ?? [];
      })
      .catch((error) => {
        if (entries.get(id) === pending) entries.delete(id);
        throw error;
      });
    entries.set(id, pending);
    return pending;
  },
};
