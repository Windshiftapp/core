import { items as itemsApi } from '../api/items.js';
import { labels as labelsApi } from '../api/misc.js';
import { getAssignableUsers } from '../api/users.js';
import { t } from '../stores/i18n.svelte.js';
import { successToast, surfaceError, warningToast } from '../stores/toasts.svelte.js';
import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
import { runBulkTransitionWithScreenRetry } from '../utils/bulkActions.js';

/**
 * Multi-select state and bulk actions shared by the board and list views.
 *
 * Views own the selection chrome (checkboxes, bulk toolbar) and pass their
 * visible items; the composable owns the selection set, edit-permission
 * checks, and the bulk API calls so both views behave identically.
 *
 * @param {{
 *   items: () => any[],
 *   scopeWorkspaceId?: () => any,
 *   reload?: () => any,
 *   labelNameLookup?: () => any[],
 * }} options
 */
export function useItemBulkSelection({
  items,
  scopeWorkspaceId = () => null,
  reload = () => {},
  labelNameLookup = () => [],
}) {
  let selectedIds = $state([]);
  // Selection mode is explicit: checkboxes and bulk actions only appear once
  // the user opts in, so the view at rest stays free of selection chrome.
  let selectionMode = $state(false);
  let selectionAnchorId = $state(null);
  let bulkBusy = $state(false);
  let assignableUsers = $state([]);
  let assignableUsersLoaded = $state(false);
  let assignableUsersLoading = $state(false);

  let selectedIdSet = $derived(new Set(selectedIds));
  let selectedCount = $derived(selectedIds.length);
  let selectedItems = $derived(items().filter((item) => selectedIdSet.has(item.id)));
  let selectedItemsEditable = $derived(
    selectedItems.length > 0 &&
      selectedItems.every((item) => workspacePermissions.canEdit(item.workspace_id))
  );

  // Drop ids that leave the loaded set so the toolbar never acts on stale rows.
  $effect(() => {
    const present = new Set(items().map((item) => item.id));
    const pruned = selectedIds.filter((id) => present.has(id));
    if (pruned.length !== selectedIds.length) selectedIds = pruned;
  });

  function enterSelectionMode() {
    selectionMode = true;
  }

  function exitSelectionMode() {
    selectionMode = false;
    clearSelection();
  }

  function toggleSelectionMode() {
    if (selectionMode) exitSelectionMode();
    else enterSelectionMode();
  }

  function toggleSelected(itemId) {
    selectionMode = true;
    selectedIds = selectedIdSet.has(itemId)
      ? selectedIds.filter((id) => id !== itemId)
      : [...selectedIds, itemId];
    selectionAnchorId = itemId;
  }

  function clearSelection() {
    selectedIds = [];
    selectionAnchorId = null;
  }

  function itemLabelIds(item) {
    if (Array.isArray(item.label_ids)) return item.label_ids;
    return (item.labels || []).map((label) => label.id);
  }

  function bulkSelectionIds() {
    return selectedItems.map((item) => item.id);
  }

  async function bulkMoveToStatus(targetStatusId) {
    if (targetStatusId == null || bulkBusy) return;
    const ids = bulkSelectionIds();
    bulkBusy = true;
    try {
      const result = await runBulkTransitionWithScreenRetry(
        itemsApi.bulkTransition,
        ids,
        targetStatusId
      );
      // The user declined the retry; nothing moved and the selection stays.
      if (result === null) return;
      const failed = result?.failed_count ?? 0;
      if (failed > 0) {
        warningToast(
          t('collections.bulkStatusPartial', {
            count: result?.transitioned_count ?? 0,
            failed,
          })
        );
      } else {
        successToast(
          t('collections.bulkStatusSuccess', { count: result?.transitioned_count ?? ids.length })
        );
      }
      clearSelection();
      reload();
    } catch (error) {
      console.error('Bulk transition failed:', error);
      warningToast(error?.message || t('collections.bulkUpdateFailed'));
    } finally {
      bulkBusy = false;
    }
  }

  async function bulkUpdateFields(fields) {
    if (bulkBusy) return;
    const ids = bulkSelectionIds();
    if (ids.length === 0) return;
    bulkBusy = true;
    try {
      const result = await itemsApi.bulkUpdate(ids, fields);
      successToast(
        t('collections.bulkUpdateSuccess', { count: result?.updated_count ?? ids.length })
      );
      clearSelection();
      reload();
    } catch (error) {
      console.error('Bulk update failed:', error);
      warningToast(error?.message || t('collections.bulkUpdateFailed'));
    } finally {
      bulkBusy = false;
    }
  }

  let bulkLabelsModalOpen = $state(false);
  // Names drive the picker; ids are the resolved target set it reports back.
  let bulkLabelNames = $state([]);
  let bulkLabelIds = $state([]);

  let bulkLabelsWorkspaceId = $derived(
    scopeWorkspaceId() ?? selectedItems[0]?.workspace_id ?? null
  );

  // Labels present on every selected item are the modal's starting point; the
  // apply step only propagates the user's additions and removals so labels
  // that belong to just some items are left alone.
  function commonLabelIds() {
    const targets = selectedItems;
    if (targets.length === 0) return [];
    return itemLabelIds(targets[0]).filter((id) =>
      targets.every((item) => itemLabelIds(item).includes(id))
    );
  }

  function labelNameById(labelId) {
    for (const item of selectedItems) {
      const label = (item.labels || []).find((candidate) => candidate.id === labelId);
      if (label) return label.name;
    }
    return labelNameLookup().find((label) => label.id === labelId)?.name ?? null;
  }

  function openBulkLabelsModal() {
    if (bulkBusy || !selectedItemsEditable) return;
    const ids = commonLabelIds();
    bulkLabelIds = ids;
    bulkLabelNames = ids.map(labelNameById).filter(Boolean);
    bulkLabelsModalOpen = true;
  }

  // Labels are a join table, so the view sends the add/remove delta for the
  // whole selection in one request instead of one PUT per item.
  async function applyBulkLabels() {
    if (bulkBusy) return;
    const targets = selectedItems;
    if (targets.length === 0) return;
    const commonIds = commonLabelIds();
    const added = bulkLabelIds.filter((id) => !commonIds.includes(id));
    const removed = commonIds.filter((id) => !bulkLabelIds.includes(id));
    bulkLabelsModalOpen = false;
    if (added.length === 0 && removed.length === 0) return;
    bulkBusy = true;
    try {
      const result = await labelsApi.bulkSetLabels(
        targets.map((item) => item.id),
        { add: added, remove: removed }
      );
      const updated = result?.updated_count ?? 0;
      const failed = result?.failed_count ?? 0;
      if (failed > 0) {
        warningToast(t('collections.bulkLabelPartial', { count: updated, failed }));
      } else {
        successToast(t('collections.bulkLabelSuccess', { count: updated }));
      }
    } catch (error) {
      console.error('Bulk label update failed:', error);
      warningToast(error?.message || t('collections.bulkUpdateFailed'));
    } finally {
      bulkBusy = false;
      clearSelection();
      reload();
    }
  }

  async function ensureAssignableUsers() {
    if (assignableUsersLoaded || assignableUsersLoading) return;
    const scopeId = scopeWorkspaceId() ?? selectedItems[0]?.workspace_id;
    if (!scopeId) return;
    assignableUsersLoading = true;
    try {
      assignableUsers = (await getAssignableUsers(scopeId)) || [];
      assignableUsersLoaded = true;
    } catch (error) {
      surfaceError(error, { context: 'Load assignable users', silent: true });
    } finally {
      assignableUsersLoading = false;
    }
  }

  return {
    get selectedIds() {
      return selectedIds;
    },
    set selectedIds(value) {
      selectedIds = value;
    },
    get selectionMode() {
      return selectionMode;
    },
    set selectionMode(value) {
      selectionMode = value;
    },
    get selectionAnchorId() {
      return selectionAnchorId;
    },
    set selectionAnchorId(value) {
      selectionAnchorId = value;
    },
    get selectedIdSet() {
      return selectedIdSet;
    },
    get selectedCount() {
      return selectedCount;
    },
    get selectedItems() {
      return selectedItems;
    },
    get selectedItemsEditable() {
      return selectedItemsEditable;
    },
    get bulkBusy() {
      return bulkBusy;
    },
    get assignableUsers() {
      return assignableUsers;
    },
    get bulkLabelsModalOpen() {
      return bulkLabelsModalOpen;
    },
    set bulkLabelsModalOpen(value) {
      bulkLabelsModalOpen = value;
    },
    get bulkLabelNames() {
      return bulkLabelNames;
    },
    set bulkLabelNames(value) {
      bulkLabelNames = value;
    },
    get bulkLabelIds() {
      return bulkLabelIds;
    },
    set bulkLabelIds(value) {
      bulkLabelIds = value;
    },
    get bulkLabelsWorkspaceId() {
      return bulkLabelsWorkspaceId;
    },
    enterSelectionMode,
    exitSelectionMode,
    toggleSelectionMode,
    toggleSelected,
    clearSelection,
    bulkMoveToStatus,
    bulkUpdateFields,
    openBulkLabelsModal,
    applyBulkLabels,
    ensureAssignableUsers,
  };
}
