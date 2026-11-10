import { collectionEditorOptions } from '../../stores/collectionEditorOptions.svelte.js';
import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';

/**
 * Drop cached iteration directories after an iteration mutation so boards,
 * backlogs, and pickers stop serving the pre-mutation list.
 *
 * A global iteration is part of every workspace's list, so it invalidates every
 * scoped editor cache; a workspace iteration only affects its own workspace.
 * The workspace store only holds one scope at a time, so it is refreshed only
 * when the mutated iteration belongs to (or is visible in) the active scope.
 *
 * @param {{ is_global?: boolean, workspace_id?: number|null }} iteration
 */
export async function invalidateIterationReferences(iteration) {
  const isGlobal = iteration?.is_global === true;
  const workspaceId = iteration?.workspace_id ?? null;

  if (isGlobal) {
    collectionEditorOptions.invalidateFamily('iterations');
  } else if (workspaceId) {
    collectionEditorOptions.invalidate(workspaceId, 'iterations');
  }

  const activeId = workspaceDataStore.workspaceId;
  if (activeId == null || activeId === 'global') return;
  if (isGlobal || Number(activeId) === Number(workspaceId)) {
    await workspaceDataStore.invalidate('iterations');
  }
}
