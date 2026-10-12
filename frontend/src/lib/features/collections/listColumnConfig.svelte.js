import { untrack } from 'svelte';
import { api } from '../../api.js';
import { collectionStore } from '../../stores/collectionContext.js';
import { t } from '../../stores/i18n.svelte.js';
import { authStore } from '../../stores/index.js';
import { errorToast } from '../../stores/toasts.svelte.js';
import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
import {
  buildListColumnConfiguration,
  DEFAULT_LIST_COLUMNS,
  listColumnsFromConfig,
} from '../../utils/workItemListColumns.js';
import { canConfigureListColumns } from './collectionService.js';

/**
 * Shared list-column configuration for list-shaped collection views (list,
 * tree). Columns live on the board configuration of the viewed collection or
 * workspace, so every view of the same scope shows the same fields.
 *
 * @param {() => { workspaceId: any, collectionId: any }} getScope
 */
export function useListColumnConfig(getScope) {
  let boardConfig = $state(null);
  let columns = $state([]);
  let loadToken = 0;

  // Collection columns follow collection ownership; workspace defaults follow
  // workspace administration. Mirrors the backend board-write authorization.
  const canConfigure = $derived.by(() => {
    const { workspaceId, collectionId } = getScope();
    return canConfigureListColumns({
      collectionId,
      collectionOwnerId: collectionStore.boardCollection?.created_by ?? null,
      currentUserId: authStore.currentUser?.id ?? null,
      canAdminWorkspace: workspacePermissions.canAdminWorkspace(workspaceId),
    });
  });

  // Board config depends only on the viewed collection/workspace, not on the
  // item set or load state — fetch it when the view changes, not every time
  // the collection reloads (which toggles collectionStore.loading).
  $effect(() => {
    const { workspaceId, collectionId } = getScope();
    if (collectionId || workspaceId) {
      untrack(() => load(workspaceId, collectionId));
    }
  });

  async function load(workspaceId, collectionId) {
    const token = ++loadToken;
    try {
      const config = await collectionStore.getBoardConfiguration(workspaceId, collectionId);
      if (token !== loadToken) return;
      boardConfig = config;
      columns = listColumnsFromConfig(config);
    } catch {
      if (token !== loadToken) return;
      boardConfig = null;
      columns = [...DEFAULT_LIST_COLUMNS];
    }
  }

  async function save(newColumns) {
    const { workspaceId, collectionId } = getScope();
    try {
      const configData = buildListColumnConfiguration(boardConfig, newColumns);
      const saved = boardConfig?.id
        ? await api.collections.updateBoardConfiguration(
            collectionId,
            boardConfig.id,
            configData,
            workspaceId
          )
        : // Pass raw collectionId so the API can detect workspace-level config
          await api.collections.createBoardConfiguration(collectionId, workspaceId, configData);
      loadToken++;
      boardConfig = saved;
      columns = listColumnsFromConfig(saved);
      // Keep the store's cached config current so switching views does not
      // show the columns from before this save.
      collectionStore.boardConfiguration = saved;
    } catch (error) {
      console.error('Failed to save board configuration:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  return {
    get columns() {
      return columns;
    },
    get canConfigure() {
      return canConfigure;
    },
    save,
  };
}
