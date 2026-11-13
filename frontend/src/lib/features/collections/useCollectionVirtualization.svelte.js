import { collectionPreferences } from '../../stores/collectionPreferences.svelte.js';

export function useCollectionVirtualization(scope) {
  return {
    get enabled() {
      const { workspaceId, collectionId, view } = scope();
      return (
        collectionPreferences.viewStateValue(workspaceId, collectionId, view, 'virtualize_rows') !==
        false
      );
    },
    save(enabled) {
      const { workspaceId, collectionId, view } = scope();
      collectionPreferences.patchViewState(workspaceId, collectionId, view, {
        virtualize_rows: enabled,
      });
    },
  };
}
