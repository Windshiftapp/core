import { untrack } from 'svelte';
import { collectionEditorOptions } from '../stores/collectionEditorOptions.svelte.js';
import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';

/**
 * Prime the collection row-editor cache from the shared workspace store once
 * that workspace is loaded. Global collections deliberately skip this: their
 * rows can belong to different workspaces and must resolve independently.
 *
 * The option sets are read through a getter so callers can keep their own
 * `?? []` fallbacks.
 *
 * @param {() => number | string | null | undefined} getWorkspaceId
 * @param {() => {
 *   statuses: any, users: any, milestones: any, iterations: any, projects: any
 * }} getOptions
 */
export function useCollectionEditorOptionsPrime(getWorkspaceId, getOptions) {
  $effect(() => {
    const id = Number(getWorkspaceId());
    if (
      !Number.isInteger(id) ||
      id <= 0 ||
      !workspaceDataStore.initialized ||
      Number(workspaceDataStore.workspaceId) !== id
    ) {
      return;
    }

    untrack(() =>
      collectionEditorOptions.prime(id, getOptions(), workspaceDataStore.planningCompleteness)
    );
  });
}
