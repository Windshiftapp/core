import Eye from '@lucide/svelte/icons/eye';
import Trash2 from '@lucide/svelte/icons/trash-2';
import { items } from '../api/items.js';
import { confirm } from '../composables/useConfirm.js';
import { t } from '../stores/i18n.svelte.js';
import { errorToast } from '../stores/toasts.svelte.js';

/**
 * Decorate work items with the workspace display key and name for table
 * rendering. Used by the pages that list items across workspaces
 * (Collections, SearchPage).
 */
export function decorateWorkItems(workItems, workspaces) {
  return workItems.map((item) => {
    const workspace = workspaces.find((w) => w.id === item.workspace_id);
    return {
      ...item,
      display_key: `${workspace?.key || 'WORK'}-${item.workspace_item_number || item.id}`,
      workspace_name: workspace?.name || 'Unknown',
    };
  });
}

/**
 * Confirm-then-delete handler for a work item row.
 *
 * @param {{
 *   confirmMessage: (item: any) => string,
 *   onDeleted: (item: any) => any,
 * }} opts confirmMessage builds the dialog body; onDeleted refreshes the list.
 */
export function createDeleteItemHandler({ confirmMessage, onDeleted }) {
  return async function deleteItem(item) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: confirmMessage(item),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await items.delete(item.id);
      await onDeleted(item);
    } catch (error) {
      console.error('Failed to delete item:', error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  };
}

/**
 * Standard view/delete action menu for work-item tables.
 *
 * @param {{
 *   viewItem: (item: any) => void,
 *   deleteItem: (item: any) => any,
 *   viewTitleKey?: string,
 * }} opts
 */
export function createItemActionsBuilder({
  viewItem,
  deleteItem,
  viewTitleKey = 'items.viewItem',
}) {
  return (item) => [
    {
      id: 'view',
      type: 'regular',
      icon: Eye,
      title: t(viewTitleKey),
      onClick: () => viewItem(item),
    },
    { type: 'divider' },
    {
      id: 'delete',
      type: 'regular',
      icon: Trash2,
      title: t('common.delete'),
      color: 'var(--ds-text-danger)',
      hoverClass: 'hover-danger',
      onClick: () => deleteItem(item),
    },
  ];
}

/**
 * Pagination handlers for pages backed by the work-item search store. Page
 * and size events both re-execute the search; the setters let the caller
 * keep its local $state in sync.
 */
export function createSearchPaginationHandlers(store, { setPage, setItemsPerPage }) {
  async function handle(event) {
    const { page, itemsPerPage } = event.detail;
    setPage(page);
    setItemsPerPage(itemsPerPage);
    await store.executeSearch({ page, limit: itemsPerPage });
  }
  return { handlePageChange: handle, handlePageSizeChange: handle };
}
