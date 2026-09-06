<script>
  import { onMount, onDestroy, untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { api } from '../../api.js';
  import { navigate, currentRoute } from '../../router.js';
  import { publicBaseURL } from '../../runtime/contextPath.js';
  import { workspacePermissions, itemDetailStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { getShortcut, matchesShortcut, isTypingInField } from '../../utils/keyboardShortcuts.js';
  import { Trash2, X, Copy, BookOpen, Search, GitBranch, Repeat, FolderInput } from '@lucide/svelte';
  import { Bookmark, BookmarkCheck, ExternalLink } from '@lucide/svelte';
  import { addToast, successToast, errorToast, infoToast } from '../../stores/toasts.svelte.js';
  import { timerStore } from '../../stores/timerStore.svelte.js';
  import { useItemAttachments } from '../../composables/useItemAttachments.svelte.js';
  import { useWorkItemPoller } from '../../composables/useWorkItemPoller.svelte.js';
  import { useItemEventStream } from '../../composables/useItemEventStream.svelte.js';
  import { itemLiveUpdates } from '../../stores/itemLiveUpdates.svelte.js';
  import { notificationActions } from '../../stores/notifications.js';
  import { agentRuns } from '../../stores/agentRuns.svelte.js';
  import {
    registerContextCommands,
    unregisterContextCommands,
    createContextCommand,
    COMMAND_PRIORITIES
  } from '../../utils/contextCommands.js';
import Modal from '../../dialogs/Modal.svelte';
import FormModal from '../../dialogs/FormModal.svelte';
import DeleteItemDialog from '../../dialogs/DeleteItemDialog.svelte';
import LinkItemModal from '../../dialogs/LinkItemModal.svelte';
import AIViewModal from '../../dialogs/AIViewModal.svelte';
import AIConfirmModal from '../../dialogs/AIConfirmModal.svelte';
import CatchMeUpBriefing from './CatchMeUpBriefing.svelte';
import FindSimilarResults from './FindSimilarResults.svelte';
import ItemMoveWorkspaceDialog from './ItemMoveWorkspaceDialog.svelte';

  // Import the shared content component
  import ItemDetailContent from '../items/ItemDetailContent.svelte';
import TimeLogModal from '../../dialogs/TimeLogModal.svelte';
import TestCaseViewModal from '../../dialogs/TestCaseViewModal.svelte';
import RecurrenceEditor from '../../editors/RecurrenceEditor.svelte';
import Button from '../../components/Button.svelte';
import NativeSelect from '../../components/NativeSelect.svelte';

  let {
    workspaceId = null,
    itemId,
    workspaceKey = null,
    itemNumber = null,
    canonicalizeKeyRoute = false,
    tab = 'comments',
    moduleSettings = {
      time_tracking_enabled: true,
      test_management_enabled: true
    },
    isModal = false,
    onclose = null
  } = $props();

  // Initialize attachment composable
  const attachmentManager = useItemAttachments(
    () => item?.id,
    (title, message) => errorToast(message, title)
  );

  // Bind to store values using $derived
  let item = $derived(itemDetailStore.item);
  let workspace = $derived(itemDetailStore.workspace);

  // Keep the open issue detail in sync with agent/background changes. While the
  // SSE stream is healthy the poller is demoted (it resumes automatically if the
  // stream drops or is unsupported).
  useWorkItemPoller(() => itemDetailStore.refreshCurrentItem(), {
    enabled: () => !itemLiveUpdates.isLive(itemId),
  });

  // Live updates (WI-484): push changes instead of waiting for the 30s poll.
  // Maps each event kind to a targeted reload. Recovery after a connection gap
  // and explicit server reload events run a full loadData() reconciliation; the
  // initial healthy connection does not duplicate the route's bootstrap load.
  const liveStream = useItemEventStream(() => itemId, {
    // Full reconcile (reconnect/server reload): reload the item AND comments.
    // Comments is a separate component, so loadData() alone would leave it stale.
    onReconcile: () => {
      if (!itemDetailStore.loading) {
        loadData().catch((err) => console.error('SSE reconcile failed:', err));
      }
      window.dispatchEvent(new CustomEvent('item-comments-changed', { detail: { itemId } }));
      window.dispatchEvent(new CustomEvent('item-scm-links-changed', { detail: { itemId } }));
      window.dispatchEvent(new CustomEvent('item-zammad-links-changed', { detail: { itemId } }));
    },
    onItem: () => itemDetailStore.refreshCurrentItem().catch((err) => console.error('SSE item refresh failed:', err)),
    onChildren: () => itemDetailStore.loadChildItems().catch((err) => console.error('SSE children refresh failed:', err)),
    onComment: () => window.dispatchEvent(new CustomEvent('item-comments-changed', { detail: { itemId } })),
    // Generic and SCM links have independent targeted refresh paths. A link
    // event must not restart the full item-detail bootstrap.
    onLinks: () => {
      itemDetailStore.loadLinks().catch((err) => console.error('SSE links refresh failed:', err));
      window.dispatchEvent(new CustomEvent('item-scm-links-changed', { detail: { itemId } }));
    },
    onZammad: () => window.dispatchEvent(new CustomEvent('item-zammad-links-changed', { detail: { itemId } })),
    // The viewed item was deleted (its own topic published `deleted`). This is
    // authoritative — mark it gone so the view closes instead of refetching
    // (which would 404) and showing stale data.
    onDeleted: () => itemDetailStore.markDeleted(),
  });

  // Abort requests during a real unload, but keep the loaded detail when the
  // browser suspends this page for back-forward navigation.
  useEventListener(() => window, 'pagehide', (event) => {
    if (!event.persisted) itemDetailStore.reset();
  });

  // Close the detail when the open item is deleted. Consume the shared flag
  // before closing so the next detail does not inherit the deletion state.
  $effect(() => {
    if (!itemDetailStore.notFound) return;
    itemDetailStore.notFound = false;
    infoToast('This item was deleted.');
    if (isModal && onclose) {
      onclose({ hasChanges: false });
    } else if (!isModal) {
      closeModal();
    }
  });

  // Instant refresh after the AI chat agent completes a run — don't make
  // the user wait up to 30s for the next poll tick to see the agent's effects.
  $effect(() => agentRuns.subscribe(() => {
    itemDetailStore.refreshCurrentItem().catch((err) => {
      console.error('Failed to refresh item after agent run:', err);
    });
  }));

  // Modal state
  let modalElement = $state(null);

  // Recurrence state
  let recurrenceRule = $state(null);
  let showRecurrenceModal = $state(false);
  let recurrenceEditorRef = $state(null);
  let recurrenceSaving = $state(false);

  // Cross-workspace move state
  let showWorkspaceMoveDialog = $state(false);

  // Item type change state
  let showTypeChangeModal = $state(false);
  let typeChangeAnalysis = $state(null);
  let typeChangeTarget = $state(null);
  let selectedTypeChangeStatusId = $state(null);
  let changingItemType = $state(false);
  let typeChangeError = $state(null);

  let availableSubIssueTypes = $derived(itemDetailStore.availableSubIssueTypes);

  // Track itemId changes for reactivity
  // svelte-ignore state_referenced_locally
  let previousItemId = $state(itemId);

  // Timer guard flag to prevent duplicate timer starts
  let isStartingTimer = $state(false);

  // Modal control functions
  function closeModal() {
    if (isModal && onclose) {
      onclose({ hasChanges: itemDetailStore.hasChanges });
    } else if (!isModal) {
      const collectionId = $currentRoute.params?.collectionId;
      const url = collectionId
        ? `/workspaces/${workspaceId}/collections/${collectionId}`
        : `/workspaces/${workspaceId}`;
      navigate(url);
    }
  }

  // Handle Escape key manually (needs complex modal/editing state checks)
  useEventListener(() => document, 'keydown', (event) => {
    if (event.key !== 'Escape') return;
    if (!isModal) return;
    closeModal();
   
  });

  // Handle global keyboard shortcuts for item detail
  function handleItemDetailShortcuts(e) {
    // Only handle if item is loaded
    if (!item) return;

    // Don't trigger when typing in input fields
    if (isTypingInField(e)) return;

    // F - Focus status field
    if (matchesShortcut(e, getShortcut('itemDetail', 'focusStatus'))) {
      e.preventDefault();
      handleFocusStatus();
      return;
    }

    // Shift+F - Open full details
    if (matchesShortcut(e, getShortcut('itemDetail', 'fullscreen'))) {
      e.preventDefault();
      openFullDetails();
      return;
    }

    // Shift+W - Create child work item
    if (matchesShortcut(e, getShortcut('itemDetail', 'createChild'))) {
      e.preventDefault();
      if (availableSubIssueTypes?.length) handleHotkeyW();
      return;
    }
  }

  useEventListener(() => document, 'keydown', handleItemDetailShortcuts);

  // Reload child items when a new child is created against the item currently open.
  useEventListener(() => window, 'refresh-work-items', (/** @type {CustomEvent<{parentId?: number}>} */ event) => {
    const parentId = event?.detail?.parentId;
    if (parentId == null) return;
    const currentId = itemDetailStore.item?.id;
    if (currentId == null) return;
    if (Number(parentId) !== Number(currentId)) return;
    itemDetailStore.loadChildItems().catch((err) => {
      console.error('Failed to reload child items after child creation:', err);
    });
  });

  function handleFocusStatus() {
    // Focus the status field by starting edit mode
    itemDetailStore.startEditing('status');
  }

  function handleHotkeyW() {
    if (availableSubIssueTypes && availableSubIssueTypes.length > 0) {
      handleCreateSubIssue();
    }
  }

  function openFullDetails() {
    const collectionId = $currentRoute.params?.collectionId;
    const url = collectionId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${itemId}`
      : `/workspaces/${workspaceId}/items/${itemId}`;
    navigate(url);
  }

  function tryHandleModalItemNavigation(path) {
    if (!isModal || !path) {
      return false;
    }

    let normalizedPath = path;

    // Support absolute URLs (e.g., when called with anchor href)
    if (normalizedPath.startsWith('http://') || normalizedPath.startsWith('https://')) {
      try {
        const url = new URL(normalizedPath);
        normalizedPath = url.pathname + url.search;
      } catch (error) {
        console.warn('Failed to parse navigation URL:', error);
        return false;
      }
    }

    // Strip query params/fragments and trailing slashes for consistent matching
    const pathname = normalizedPath.split(/[?#]/)[0] || '/';
    const sanitizedPath = pathname.replace(/\/+$/, '') || '/';

    // Match /workspaces/:workspaceId/items/:itemId and /workspaces/:workspaceId/collections/:collectionId/items/:itemId
    const match = sanitizedPath.match(/^\/workspaces\/([^/]+)(?:\/collections\/[^/]+)?\/items\/([^/]+)$/);
    if (!match) {
      return false;
    }

    const [, targetWorkspaceId, targetItemId] = match;
    const targetWorkspaceIdStr = String(targetWorkspaceId);
    const targetItemIdStr = String(targetItemId);

    if (
      targetWorkspaceIdStr === String(workspaceId) &&
      targetItemIdStr === String(itemId)
    ) {
      return true;
    }

    workspaceId = targetWorkspaceIdStr;
    itemId = targetItemIdStr;
    return true;
  }

  // Event handlers
  function handleNavigate(detail) {
    const path = detail?.path;
    if (!path) return;

    if (tryHandleModalItemNavigation(path)) {
      return;
    }

    navigate(path);
  }
  
  function handleGoBack() {
    const collectionId = $currentRoute.params?.collectionId;
    const url = collectionId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/list`
      : `/workspaces/${workspaceId}/list`;
    navigate(url);
  }
  
  function showError(title, message) {
    errorToast(message, title);
  }

  async function handleCopyKey() {
    try {
      const key = `${item.workspace_key || workspace?.key || 'WORK'}-${item.workspace_item_number}`;
      await navigator.clipboard.writeText(key);
      showCopySuccess(key);
    } catch (error) {
      console.error('[handleCopyKey] Failed to copy key to clipboard:', error);
    }
  }

  function showCopySuccess(key) {
    successToast(`${item?.workspace_key || workspace?.key || 'WORK'}-${item?.workspace_item_number}`, t('toast.copied'));
  }
  
  async function handleSaveField(detail) {
    const { field, value, assigneeName, iterationName } = detail;
    await saveField(field, value, assigneeName, iterationName);
  }

  function handleCancelEdit(detail) {
    const { field } = detail;
    cancelEdit(field);
  }

  async function saveField(field, directValue = null, assigneeName = null, iterationName = null) {
    try {
      await itemDetailStore.saveField(field, directValue, assigneeName, iterationName);
    } catch (err) {
      console.error('Failed to update item:', err);
      showError('Failed to update item', err.message || String(err));
    }
  }

  function cancelEdit(field) {
    // Map legacy field names to store field names
    let storeField = field;
    if (field === 'status_id') storeField = 'status';
    if (field === 'priority_id') storeField = 'priority';
    if (field === 'due_date') storeField = 'dueDate';
    if (field === 'start_date') storeField = 'startDate';
    if (field === 'end_date') storeField = 'endDate';

    itemDetailStore.cancelEditing(storeField);
  }
  
  function handleStartEditingCustomField(detail) {
    const fieldId = detail.fieldId;
    // Cancel assignee editing when starting to edit a custom field
    itemDetailStore.cancelEditing('assignee');
    itemDetailStore.startEditing(`custom_field_${fieldId}`);
  }
  
  function handleSwitchTab(detail) {
    tab = detail.tab;
    if (tab === 'time') {
      itemDetailStore.loadWorklogs().catch((error) => {
        console.error('Failed to load time entries:', error);
      });
    }
    const url = `/workspaces/${workspaceId}/items/${itemId}${tab !== 'comments' ? `?tab=${tab}` : ''}`;
    navigate(url);
  }
  
  function handleCreateSubIssue() {
    startCreateSubIssue();
  }

  function handleShowLinkModal(data) {
    itemDetailStore.openLinkModal(data?.preselectLinkTypeId ?? null);
  }

  function handleLinkModalCancel() {
    itemDetailStore.closeLinkModal();
  }

  async function handleLinkCreated({ link_type_id, target_id, target_type }) {

    try {
      await itemDetailStore.createLink(link_type_id, target_id, target_type);
    } catch (error) {
      console.error('Error creating link:', error);
      showError('Failed to create link', error.message || 'Unknown error');
    }
  }

  function handleViewTestCase(detail) {
    const { testCaseId } = detail || {};
    if (!testCaseId) return;
    const normalizedId = Number(testCaseId);
    if (!Number.isFinite(normalizedId)) {
      console.warn('Received invalid test case ID from link:', testCaseId);
      return;
    }
    itemDetailStore.openTestCaseModal(normalizedId);
  }

  function handleCloseTestCaseModal() {
    itemDetailStore.closeTestCaseModal();
  }

  async function handleRemoveLink(detail) {
    const { linkId } = detail;

    try {
      await itemDetailStore.removeLink(linkId);
    } catch (error) {
      console.error('Error removing link:', error);
    }
  }

  function handleStartEditingAssignee() {
    // Cancel all custom field editing when starting to edit assignee
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('assignee');
  }

  function handleStartEditingMilestone() {
    // Cancel all custom field editing when starting to edit milestone
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('milestone');
  }

  function handleStartEditingIteration() {
    // Cancel all custom field editing when starting to edit iteration
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('iteration');
  }

  function handleStartEditingPriority() {
    // Cancel all custom field editing when starting to edit priority
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('priority');
  }

  function handleStartEditingDueDate() {
    // Cancel all custom field editing when starting to edit due date
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('dueDate');
  }

  function handleStartEditingStartDate() {
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('startDate');
  }

  function handleStartEditingEndDate() {
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('endDate');
  }

  function handleStartEditingStatus() {
    // Cancel all custom field editing when starting to edit status
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('status');
  }

  function handleStartEditingProject() {
    // Cancel all custom field editing when starting to edit project
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('project');
  }

  function handleStartEditingDescription() {
    // Cancel all custom field editing when starting to edit description
    itemDetailStore.editing.customFields.active = {};
    itemDetailStore.startEditing('description');
  }

  async function handleStartTimer() {
    // Guard: Check if we're already starting a timer
    if (isStartingTimer) {
      return;
    }

    // Guard: Use reactive store values
    if (!timerStore.canStart) {
      if (timerStore.syncing) {
        showError(t('items.timerBusy'), t('items.timerSyncingMessage'));
      } else if (timerStore.activeTimer) {
        showError(t('items.timerAlreadyRunning'), t('items.stopTimerFirst'));
      }
      return;
    }

    try {
      // Set the guard flag to prevent duplicate requests
      isStartingTimer = true;

      // Get the default project for time logging
      // Priority order: time_project_id > effective_project_id > workspace.time_project_id
      let projectId = null;
      if (item?.time_project_id) {
        projectId = item.time_project_id;
      } else if (item?.effective_project_id) {
        projectId = item.effective_project_id;
      } else if (workspace?.time_project_id) {
        projectId = workspace.time_project_id;
      }

      if (!projectId) {
        showError(t('items.noProjectConfigured'), t('items.setDefaultProject'));
        return;
      }

      const timerData = {
        workspace_id: parseInt(workspaceId),
        item_id: parseInt(itemId),
        project_id: projectId,
        description: t('items.workingOn', { title: item.title })
      };

      await timerStore.start(timerData);
    } catch (error) {
      console.error('Failed to start timer:', error);
      // Only show error if it's not a 409 conflict (already running)
      if (!error.message?.includes('already running')) {
        showError(t('items.failedToStartTimer'), error.message || t('errors.UNKNOWN'));
      }
    } finally {
      // Always reset the guard flag
      isStartingTimer = false;
    }
  }

  async function handleLogTime() {
    await itemDetailStore.loadTimeModalData();
    itemDetailStore.openTimeLogModal();
  }

  async function handleEditWorklog(detail) {
    await itemDetailStore.loadTimeModalData();
    itemDetailStore.openTimeLogModal(detail);
  }

  async function handleDeleteWorklog(detail) {
    const worklog = detail;
    try {
      await api.time.worklogs.delete(worklog.id);
      // Reload worklogs
      await itemDetailStore.reloadWorklogs();
    } catch (error) {
      console.error('Failed to delete worklog:', error);
      showError(t('items.failedToDeleteTimeEntry'), error.message || t('errors.UNKNOWN'));
    }
  }

  async function handleModalSave(event) {
    try {
      const data = event.detail;
      if (itemDetailStore.editingWorklog) {
        await api.time.worklogs.update(itemDetailStore.editingWorklog.id, data);
      } else {
        await api.time.worklogs.create(data);
      }

      // Reload worklogs
      await itemDetailStore.reloadWorklogs();
      itemDetailStore.closeTimeLogModal();
    } catch (error) {
      console.error('Failed to save worklog:', error);
      showError(t('items.failedToSaveTimeEntry'), error.message || t('errors.UNKNOWN'));
    }
  }

  function handleModalCancel() {
    itemDetailStore.closeTimeLogModal();
  }

  // Get default project for time logging
  function getDefaultProjectForTimeLogging() {
    return itemDetailStore.getDefaultProjectForTimeLogging();
  }

  // Recurrence handlers
  async function loadRecurrence() {
    if (!itemId) return;
    try {
      recurrenceRule = await api.recurrence.get(itemId);
    } catch {
      recurrenceRule = null;
    }
  }

  async function handleSetupRecurrence() {
    showRecurrenceModal = true;
  }

  function handleEditRecurrence() {
    showRecurrenceModal = true;
  }

  async function handleRecurrenceSave(result) {
    recurrenceRule = result;
    showRecurrenceModal = false;
    populateDropdownItems();
  }

  async function handleRecurrenceFormSave() {
    recurrenceSaving = true;
    try {
      await recurrenceEditorRef.handleSave();
    } finally {
      recurrenceSaving = false;
    }
  }

  async function handleRecurrenceDelete() {
    try {
      await api.recurrence.delete(itemId);
      recurrenceRule = null;
      showRecurrenceModal = false;
      populateDropdownItems();
    } catch (err) {
      console.error('Failed to delete recurrence:', err);
      showError(t('errors.UNKNOWN'), err.message || String(err));
    }
  }

  async function handleCopyItem() {
    try {
      const copiedItem = await itemDetailStore.copyItem();

      // Show clickable success toast that navigates to the copied item
      const itemKey = (() => { const k = copiedItem.workspace_key || workspace?.key; return k ? `${k}-${copiedItem.workspace_item_number}` : `ITEM-${copiedItem.workspace_item_number}`; })();
      addToast({
        title: t('items.itemCopiedAs', { key: itemKey }),
        message: t('items.clickToViewCopied'),
        variant: 'success',
        duration: 15000,
        clickable: true,
        onClick: () => {
          const collectionId = $currentRoute.params?.collectionId;
          const url = collectionId
            ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${copiedItem.id}`
            : `/workspaces/${workspaceId}/items/${copiedItem.id}`;
          navigate(url);
        }
      });

    } catch (err) {
      console.error('Failed to copy item:', err);
      showError(t('items.failedToCopy'), err.message || String(err));
    }
  }

  async function handleReorderChildren() {
    try {
      await itemDetailStore.loadChildItems();
    } catch (error) {
      console.error('Failed to reload child items after reorder:', error);
    }
  }

  async function handleParentChanged() {
    // Reload the item data to get updated parent hierarchy
    try {
      await loadData();
    } catch (error) {
      console.error('Failed to reload data after parent change:', error);
    }
  }

  async function handleItemTypeChange(targetType) {
    if (!targetType || !item || targetType.id === item.item_type_id) return;
    typeChangeError = null;
    typeChangeTarget = targetType;

    try {
      changingItemType = true;
      const analysis = await api.items.analyzeTypeChange(item.id, targetType.id);
      if (analysis?.requires_migration) {
        typeChangeAnalysis = analysis;
        selectedTypeChangeStatusId = analysis.suggested_status_id || null;
        showTypeChangeModal = true;
        return;
      }
      await executeItemTypeChange(targetType.id, null);
    } catch (error) {
      console.error('Failed to analyze item type change:', error);
      errorToast(error.message || String(error));
    } finally {
      changingItemType = false;
    }
  }

  async function executeItemTypeChange(targetItemTypeId = null, targetStatusId = null) {
    if (!item) return;
    const itemTypeId = targetItemTypeId || typeChangeTarget?.id;
    if (!itemTypeId) return;

    try {
      changingItemType = true;
      typeChangeError = null;
      const updated = await api.items.changeType(item.id, {
        target_item_type_id: itemTypeId,
        ...(targetStatusId ? { target_status_id: targetStatusId } : {})
      });
      itemDetailStore.item = { ...itemDetailStore.item, ...updated };
      itemDetailStore.hasChanges = true;
      showTypeChangeModal = false;
      typeChangeAnalysis = null;
      typeChangeTarget = null;
      selectedTypeChangeStatusId = null;
      await itemDetailStore.loadItem(workspaceId, item.id);
      successToast('Item type changed');
    } catch (error) {
      console.error('Failed to change item type:', error);
      typeChangeError = error.message || String(error);
      if (!showTypeChangeModal) {
        errorToast(typeChangeError);
      }
    } finally {
      changingItemType = false;
    }
  }

  function closeTypeChangeModal() {
    if (changingItemType) return;
    showTypeChangeModal = false;
    typeChangeAnalysis = null;
    typeChangeTarget = null;
    selectedTypeChangeStatusId = null;
    typeChangeError = null;
  }

  function handleDeleteItem() {
    itemDetailStore.openDeleteDialog();
  }

  function handleWorkspaceMoved(result) {
    const moved = result?.item;
    if (!moved) return;
    successToast(t('items.moveWorkspaceSuccess', { key: result.new_key }));
    navigate(`/workspaces/${moved.workspace_id}/items/${moved.id}`);
  }

  function handleDeleteComplete(result) {
    const collectionId = $currentRoute.params?.collectionId;
    // Navigate based on deletion result
    if (result?.mode === 'reparent' && result?.newParentId) {
      // If reparenting, navigate to the new parent item
      const url = collectionId
        ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${result.newParentId}`
        : `/workspaces/${workspaceId}/items/${result.newParentId}`;
      navigate(url);
    } else {
      // Otherwise, navigate to workspace list
      const url = collectionId
        ? `/workspaces/${workspaceId}/collections/${collectionId}/list`
        : `/workspaces/${workspaceId}/list`;
      navigate(url);
    }
  }

  function handleDeleteError(err) {
    console.error('Failed to delete item:', err);
    showError(t('items.failedToDelete'), err.message || String(err));
  }

  async function toggleWatch() {
    try {
      await itemDetailStore.toggleWatch();
    } catch (err) {
      console.error('Failed to toggle watch:', err);
      showError(t('items.failedToUpdateWatchStatus'), err.message || String(err));
    }
  }

  function populateDropdownItems() {
    if (!itemDetailStore.item) return;

    /** @type {any[]} */
    const items = [
      {
        id: 'copy',
        type: 'regular',
        icon: Copy,
        title: t('items.copyWorkItem'),
        onClick: handleCopyItem
      },
      {
        id: 'watch',
        type: 'regular',
        icon: itemDetailStore.isWatching ? BookmarkCheck : Bookmark,
        title: itemDetailStore.isWatching ? t('items.unwatchWorkItem') : t('items.watchWorkItem'),
        onClick: toggleWatch
      }
    ];

    if (!recurrenceRule) {
      items.push({
        id: 'add-recurrence',
        type: 'regular',
        icon: Repeat,
        testid: 'item-recurrence-add',
        title: t('recurrence.addRecurrence'),
        onClick: handleSetupRecurrence
      });
    }

    const canEdit = untrack(() => workspacePermissions.canEdit(workspaceId));
    if (canEdit) {
      items.push({
        id: 'move-workspace',
        type: 'regular',
        icon: FolderInput,
        testid: 'item-move-workspace-open',
        title: t('items.moveWorkspaceMenu'),
        onClick: () => { showWorkspaceMoveDialog = true; }
      });
    }

    // Only show delete option if user has permission
    // Use untrack to prevent creating reactive dependency that could cause infinite loops
    const canDelete = untrack(() => workspacePermissions.canDelete(workspaceId));
    if (canDelete) {
      items.push(
        {
          id: 'divider-1',
          type: 'divider'
        },
        {
          id: 'delete',
          type: 'regular',
          icon: Trash2,
          testid: 'item-delete-open',
          title: t('items.deleteWorkItem'),
          color: 'var(--ds-text-danger)',
          hoverClass: 'hover-danger',
          onClick: handleDeleteItem
        }
      );
    }

    itemDetailStore.dropdownItems = items;
  }

  // Reactive statement to handle itemId changes for navigation between items.
  // Stale-while-revalidate: keep the previous item rendered while the new one
  // loads so the swap is atomic instead of skeleton-flashing.
  $effect(() => {
    if (itemId !== previousItemId && !itemDetailStore.loading) {
      previousItemId = itemId;
      itemDetailStore.transitioning = true;

      loadData()
        .then(() => populateDropdownItems())
        .catch((error) => console.error('Failed to load item data after navigation:', error))
        .finally(() => { itemDetailStore.transitioning = false; });
    }
  });

  // Reload the open item in place when a notification points at the item
  // already in view. The itemId prop doesn't change, so the navigation effect
  // above can't cover it; NotificationCard dispatches `reload-item-detail`
  // (mirroring the `refresh-work-items` listener above) and we self-filter on
  // the open item's id. Comments listens for the same event.
  useEventListener(() => window, 'reload-item-detail', (/** @type {CustomEvent<{itemId?: number|string}>} */ event) => {
    const id = event?.detail?.itemId;
    if (id == null) return;
    const currentId = itemDetailStore.item?.id;
    if (currentId == null || String(id) !== String(currentId)) return;
    itemDetailStore.transitioning = true;
    loadData()
      .then(() => populateDropdownItems())
      .catch((error) => console.error('Failed to reload open item detail:', error))
      .finally(() => { itemDetailStore.transitioning = false; });
  });

  // --- AI Actions ---
  let aiModalType = $state(null);
  let aiLoading = $state(false);
  let aiError = $state(null);
  let aiResult = $state(null);
  let aiCreating = $state(false);

  async function handleAIAction(detail) {
    const { action } = detail;
    aiModalType = action;
    aiLoading = true;
    aiError = null;
    aiResult = null;
    try {
      if (action === 'catch-me-up') {
        aiResult = await api.ai.catchMeUp(itemId);
      } else if (action === 'find-similar') {
        aiResult = await api.ai.findSimilar(itemId);
      } else if (action === 'decompose') {
        aiResult = await api.ai.decompose(itemId);
      }
    } catch (err) {
      aiError = err.message || 'AI analysis failed';
    } finally {
      aiLoading = false;
    }
  }

  function closeAIModal() {
    aiModalType = null;
    aiLoading = false;
    aiError = null;
    aiResult = null;
    aiCreating = false;
  }

  async function handleCreateSubTasks(selectedTasks) {
    if (!selectedTasks.length || !item) return;
    aiCreating = true;
    try {
      const childTypeId = availableSubIssueTypes[0]?.id;
      if (!childTypeId) {
        errorToast('No child item type available');
        return;
      }
      for (const task of selectedTasks) {
        await api.items.create({
          title: task.title,
          description: task.description || '',
          workspace_id: parseInt(workspaceId),
          parent_id: parseInt(itemId),
          item_type_id: childTypeId,
        });
      }
      successToast(`Created ${selectedTasks.length} sub-task${selectedTasks.length > 1 ? 's' : ''}`);
      closeAIModal();
      // Reload child items
      await itemDetailStore.loadChildItems();
    } catch (err) {
      console.error('Failed to create sub-tasks:', err);
      errorToast(err.message || 'Failed to create sub-tasks');
    } finally {
      aiCreating = false;
    }
  }

  // Handler for executing a manual action
  async function handleExecuteAction(detail) {
    const action = detail;
    try {
      await itemDetailStore.executeAction(action.id);
      successToast(t('actions.test.executionQueued'));
    } catch (err) {
      console.error('Failed to execute action:', err);
      errorToast(err.message || t('errors.UNKNOWN'), t('actions.test.executionFailed'));
    }
  }

  // Handle diagram saved event - reload diagrams
  async function handleDiagramSaved() {
    await itemDetailStore.loadDiagrams({ force: true });
  }

  onMount(async () => {
    await loadData();

    // Register context-sensitive commands for this item
    // Pass current timer status to avoid creating reactive dependency
    registerItemContextCommands(!!timerStore.getCurrent());
  });
  
  // Register context commands for this work item
  // Pass hasActiveTimer as a parameter to avoid reactive dependency on $activeTimer
  function registerItemContextCommands(hasActiveTimer = false) {
    if (!item) return;

    const itemKey = (() => { const k = item.workspace_key || workspace?.key; return k ? `${k}-${item.workspace_item_number}` : `ITEM-${item.workspace_item_number}`; })();
    const commands = [];

    // Add Link command
    commands.push(createContextCommand({
      id: 'add-link-to-item',
      label: `Add Link to ${itemKey}`,
      description: 'Link this work item to another item',
      keywords: ['link', 'connect', 'relate', 'add', 'reference'],
      action: () => {
        itemDetailStore.showLinkModal = true;
      },
      priority: COMMAND_PRIORITIES.HIGH,
      category: 'action'
    }));

    // Create Child Work Item command (only if sub-issue types are available)
    if (availableSubIssueTypes && availableSubIssueTypes.length > 0) {
      commands.push(createContextCommand({
        id: 'create-child-item',
        label: `Create Child Work Item for ${itemKey}`,
        description: 'Create a child work item under this item',
        keywords: ['create', 'child', 'sub', 'issue', 'subtask', 'add', 'new', 'work', 'item'],
        action: () => {
          handleCreateSubIssue();
        },
        priority: COMMAND_PRIORITIES.HIGH,
        category: 'action'
      }));
    }

    // Time tracking commands (only if time tracking is enabled)
    if (moduleSettings.time_tracking_enabled) {
      commands.push(createContextCommand({
        id: 'log-time-for-item',
        label: `Log Time for ${itemKey}`,
        description: 'Add a time entry for this work item',
        keywords: ['log', 'time', 'hours', 'work', 'track', 'entry'],
        action: () => {
          // Trigger the time entry form in the tabs component
          if (tab !== 'time') {
            tab = 'time';
          }
          // Use a small delay to ensure tab is switched
          setTimeout(() => {
            window.dispatchEvent(new CustomEvent('item-detail-show-time-entry', {
              detail: { itemId: item.id }
            }));
          }, 100);
        },
        priority: COMMAND_PRIORITIES.HIGH,
        category: 'time'
      }));

      // Start Timer command (only if no active timer)
      // Use the passed parameter instead of reactive $activeTimer to avoid creating a dependency
      if (!hasActiveTimer) {
        commands.push(createContextCommand({
          id: 'start-timer-for-item',
          label: `Start Timer for ${itemKey}`,
          description: 'Start tracking time for this work item',
          keywords: ['start', 'timer', 'track', 'time', 'begin'],
          action: async () => {
            await handleStartTimer();
          },
          priority: COMMAND_PRIORITIES.HIGH,
          category: 'time'
        }));
      }
    }

    // Copy Item Link command
    commands.push(createContextCommand({
      id: 'copy-item-link',
      label: `Copy Link to ${itemKey}`,
      description: 'Copy a shareable link to this work item',
      keywords: ['copy', 'link', 'share', 'url'],
      action: async () => {
        const url = `${publicBaseURL()}/workspaces/${workspaceId}/items/${itemId}`;
        try {
          await navigator.clipboard.writeText(url);
          successToast(t('items.itemLinkCopied'));
        } catch (error) {
          console.error('Failed to copy to clipboard:', error);
          errorToast(t('items.failedToCopyToClipboard'));
        }
      },
      priority: COMMAND_PRIORITIES.NORMAL,
      category: 'action'
    }));

    registerContextCommands('item-detail', commands);
  }
  
  // Re-register commands when item changes (for updated item key)
  // Track previous item ID to avoid re-registering on every item property change
  let previousCommandItemId = $state(null);
  $effect(() => {
    if (item && item.id !== previousCommandItemId) {
      previousCommandItemId = item.id;
      // Pass current timer status to avoid creating reactive dependency
      registerItemContextCommands(!!timerStore.getCurrent());
      populateDropdownItems();
    }
  });

  // Re-register commands when active timer status changes
  // This allows us to show/hide the "Start Timer" command based on timer status
  let previousTimerStatus; // Plain variable, not reactive - prevents self-invalidation
  $effect(() => {
    const currentTimerStatus = !!timerStore.activeTimer;
    if (item && previousTimerStatus !== undefined && previousTimerStatus !== currentTimerStatus) {
      // Timer status changed, re-register commands with new status
      registerItemContextCommands(currentTimerStatus);
    }
    previousTimerStatus = currentTimerStatus;
  });

  // Rebuild dropdown when watch status changes
  $effect(() => {
    itemDetailStore.isWatching;
    itemDetailStore.item && populateDropdownItems();
  });

  // Reload worklogs when timer stops (activeTimer becomes null from non-null)
  let previousActiveTimer; // Plain variable, not reactive - prevents self-invalidation
  $effect(() => {
    const currentTimer = timerStore.activeTimer;

    // If we had an active timer and now it's null, and it was for this item, reload worklogs
    if (previousActiveTimer && !currentTimer && previousActiveTimer.item_id === parseInt(itemId)) {
      // Timer was stopped, reload worklogs for this item
      itemDetailStore.reloadWorklogs().catch(err => {
        console.error('Failed to reload worklogs after timer stop:', err);
      });
    }

    // Update previous timer - using plain variable doesn't trigger effect re-run
    previousActiveTimer = currentTimer;
  });

  onDestroy(() => {
    // Unregister context commands when component is destroyed
    unregisterContextCommands('item-detail');
  });

  // Load data using the store
  async function loadData() {
    const lookupWorkspaceKey = workspaceKey || (workspaceId && !/^\d+$/.test(String(workspaceId)) ? workspaceId : null);
    const lookupItemNumber = itemNumber || (lookupWorkspaceKey ? itemId : null);

    await itemDetailStore.loadItem(workspaceId, itemId, {
      workspaceKey: lookupWorkspaceKey,
      itemNumber: lookupItemNumber,
    });

    // Diagrams are non-blocking detail data, but they should appear without a
    // separate "Show Diagram" control.
    void itemDetailStore.loadDiagrams();

    if (tab === 'time') {
      await itemDetailStore.loadWorklogs();
    }

    // Backfill route props from the resolved item when the URL used a stable
    // key form (/workspace/WI/item/123 or /workspaces/WI/items/123).
    if (itemDetailStore.item?.id) {
      workspaceId = itemDetailStore.workspaceId;
      itemId = itemDetailStore.item.id;
      previousItemId = itemId;

      // Clear notifications pointing at this item: viewing an item should
      // mark its notifications read regardless of how it was opened, not only
      // when launched from the notification tray/list.
      notificationActions.markItemAsRead(itemId);
    } else if (!workspaceId) {
      workspaceId = itemDetailStore.workspaceId;
    }

    if (canonicalizeKeyRoute && !isModal && itemDetailStore.item?.id && itemDetailStore.workspaceId) {
      const suffix = tab !== 'comments' ? `?tab=${tab}` : '';
      navigate(`/workspaces/${itemDetailStore.workspaceId}/items/${itemDetailStore.item.id}${suffix}`, { replace: true });
    }

    // Load attachment settings and attachments (still using composable)
    await attachmentManager.loadSettings();
    if (attachmentManager.isEnabled()) {
      await attachmentManager.load();
    }

    // Load recurrence rule
    loadRecurrence();
  }

  // Sub-issue creation function
  function startCreateSubIssue() {
    if (itemDetailStore.availableSubIssueTypes.length === 0) {
      showError(t('items.noSubIssueTypes'), t('items.cannotCreateChildItems'));
      return;
    }

    // Set up for sub-issue creation and open the global create modal

    // First, trigger loading the CreateModal component
    window.dispatchEvent(new CustomEvent('show-create-modal'));

    // Small delay to let the modal load, then configure it
    setTimeout(() => {
      // Set the type first
      window.dispatchEvent(new CustomEvent('set-create-type', {
        detail: { type: 'work-item' }
      }));

      // Set the parent
      window.dispatchEvent(new CustomEvent('set-create-parent', {
        detail: {
          parentId: itemDetailStore.item.id,
          parentTitle: itemDetailStore.item.title,
          availableItemTypes: itemDetailStore.availableSubIssueTypes
        }
      }));

      // Open the modal (this will load workspaces)
      window.dispatchEvent(new CustomEvent('open-create-modal'));

      // After modal is open and workspaces are loaded, set the workspace
      setTimeout(() => {
        window.dispatchEvent(new CustomEvent('set-create-workspace', {
          detail: {
            workspaceId: workspaceId,
            workspaceName: itemDetailStore.workspace?.name
          }
        }));
      }, 200);
    }, 150);
  }
</script>

{#snippet contentSnippet()}
  <ItemDetailContent
    loading={itemDetailStore.loading}
    error={itemDetailStore.error}
    item={itemDetailStore.item}
    workspace={itemDetailStore.workspace}
    {isModal}
    parentHierarchy={itemDetailStore.parentHierarchy}
    currentItemType={itemDetailStore.currentItemType}
    currentHierarchyLevel={itemDetailStore.currentHierarchyLevel}
    {workspaceId}
    editingTitle={itemDetailStore.editing.title.active}
    editTitle={itemDetailStore.editing.title.value}
    saving={itemDetailStore.saving}
    dropdownItems={itemDetailStore.dropdownItems}
    statusOptions={itemDetailStore.statusOptions}
    pendingApproval={itemDetailStore.pendingApproval}
    onapprovalsChanged={() => itemDetailStore.loadItem(workspaceId, itemDetailStore.itemId)}
    editingDescription={itemDetailStore.editing.description.active}
    editDescription={itemDetailStore.editing.description.value}
    itemLinks={itemDetailStore.itemLinks}
    linkTypes={itemDetailStore.filteredLinkTypes}
    loadingLinks={itemDetailStore.loadingLinks}
    availableSubIssueTypes={itemDetailStore.availableSubIssueTypes}
    childItems={itemDetailStore.childItems}
    loadingChildItems={itemDetailStore.loadingChildItems}
    itemTypes={itemDetailStore.itemTypes}
    {tab}
    {moduleSettings}
    timeWorklogs={itemDetailStore.timeWorklogs}
    timeProjects={itemDetailStore.timeProjects}
    activeTimer={timerStore.activeTimer}
    editingStatus={itemDetailStore.editing.status.active}
    editingPriority={itemDetailStore.editing.priority.active}
    editingDueDate={itemDetailStore.editing.dueDate.active}
    editingStartDate={itemDetailStore.editing.startDate.active}
    editingEndDate={itemDetailStore.editing.endDate.active}
    editingProject={itemDetailStore.editing.project.active}
    editingAssignee={itemDetailStore.editing.assignee.active}
    editingMilestone={itemDetailStore.editing.milestone.active}
    editingIteration={itemDetailStore.editing.iteration.active}
    editingCustomFields={itemDetailStore.editing.customFields.active}
    editCustomFieldValues={itemDetailStore.editing.customFields.values}
    workspaceScreenFields={itemDetailStore.workspaceScreenFields}
    workspaceScreenSystemFields={itemDetailStore.workspaceScreenSystemFields}
    editableScreenFieldIds={itemDetailStore.editableScreenFieldIds}
    editableScreenSystemFields={itemDetailStore.editableScreenSystemFields}
    customFieldDefinitions={itemDetailStore.customFieldDefinitions}
    requestTypeFields={itemDetailStore.requestTypeFields}
    milestones={itemDetailStore.milestones}
    iterations={itemDetailStore.iterations}
    priorities={itemDetailStore.priorities}
    attachments={attachmentManager.attachments || []}
    attachmentPagination={attachmentManager.pagination}
    diagrams={itemDetailStore.diagrams}
    manualActions={itemDetailStore.manualActions}
    canCreate={untrack(() => workspacePermissions.canCreate(workspaceId))}
    onaiAction={handleAIAction}
    onnavigate={handleNavigate}
    ongoBack={handleGoBack}
    oncopyKey={handleCopyKey}
    onsaveField={handleSaveField}
    oncancelEdit={handleCancelEdit}
    onswitchTab={handleSwitchTab}
    oncreateSubIssue={handleCreateSubIssue}
    onremoveLink={handleRemoveLink}
    onviewTestCase={handleViewTestCase}
    onshowLinkModal={handleShowLinkModal}
    onstartEditingAssignee={handleStartEditingAssignee}
    onstartEditingMilestone={handleStartEditingMilestone}
    onstartEditingIteration={handleStartEditingIteration}
    onstartEditingPriority={handleStartEditingPriority}
    onstartEditingDueDate={handleStartEditingDueDate}
    onstartEditingStartDate={handleStartEditingStartDate}
    onstartEditingEndDate={handleStartEditingEndDate}
    onstartEditingStatus={handleStartEditingStatus}
    onstartEditingProject={handleStartEditingProject}
    onstartEditingDescription={handleStartEditingDescription}
    onstartEditingCustomField={handleStartEditingCustomField}
    onstartTimer={handleStartTimer}
    onlogTime={handleLogTime}
    oneditWorklog={handleEditWorklog}
    ondeleteWorklog={handleDeleteWorklog}
    onparentChanged={handleParentChanged}
    onitemtypechange={handleItemTypeChange}
    onattachmentUpload={attachmentManager.handleUpload}
    onattachmentUploadFiles={attachmentManager.uploadFiles}
    onattachmentDelete={attachmentManager.handleDelete}
    onattachmentPageChange={attachmentManager.handlePageChange}
    onattachmentPageSizeChange={attachmentManager.handlePageSizeChange}
    ondiagramSaved={handleDiagramSaved}
    onexecuteAction={handleExecuteAction}
    onreorderChildren={handleReorderChildren}
    onclose={closeModal}
    {recurrenceRule}
    onsetupRecurrence={handleSetupRecurrence}
    oneditRecurrence={handleEditRecurrence}
  />
{/snippet}

{#if isModal}
  <Modal
    isOpen={true}
    maxWidth={'max-w-6xl'}
    autoFocus={false}
    onclose={closeModal}
    onKeydown={handleItemDetailShortcuts}
  >
    <div
      bind:this={modalElement}
      data-testid="item-detail"
      data-live-updates={liveStream.connected ? 'connected' : 'disconnected'}
      class="flex flex-col relative w-full h-[85vh]"
    >
      {#if itemDetailStore.showTestCaseModal}
        <TestCaseViewModal
          embedded={true}
          isOpen={itemDetailStore.showTestCaseModal}
          testCaseId={itemDetailStore.selectedTestCaseId}
          onclose={handleCloseTestCaseModal}
        />
      {:else}
        {#if itemDetailStore.item && itemDetailStore.workspace}
          <!-- Modal Header -->
          <div class="flex items-center justify-between p-4 border-b" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
            <div class="flex items-center gap-3">
              <h1 class="text-lg font-semibold" style="color: var(--ds-text);">{t('items.workItemDetails')}</h1>
              <span class="px-2 py-1 text-sm font-mono rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {itemDetailStore.item.workspace_key || itemDetailStore.workspace.key}-{itemDetailStore.item.workspace_item_number}
              </span>
            </div>
            <div class="flex items-center gap-2">
              <button
                onclick={openFullDetails}
                class="inline-flex items-center gap-2 px-3 py-1.5 bg-[var(--ds-interactive)] text-white rounded hover:bg-[var(--ds-interactive-hovered)] transition-colors text-sm font-medium"
                title="Open full details (⇧F)"
              >
                <ExternalLink class="w-4 h-4" />
                {t('items.fullDetails')}
                <span class="ml-1 px-1.5 py-0.5 bg-[var(--ds-interactive-hovered)] bg-opacity-50 rounded text-xs font-mono">⇧F</span>
              </button>
              <button
                onclick={closeModal}
                class="p-2 rounded transition-colors"
                style="color: var(--ds-text-subtle);"
                onmouseenter={(e) => { e.currentTarget.style.color = 'var(--ds-text)'; e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; }}
                onmouseleave={(e) => { e.currentTarget.style.color = 'var(--ds-text-subtle)'; e.currentTarget.style.backgroundColor = ''; }}
                title="Close"
              >
                <X class="w-5 h-5" />
              </button>
            </div>
          </div>
        {/if}

        <!-- Shared Content Component -->
        <!-- Re-key on the displayed item id (not the incoming prop) so the
             subtree only swaps once new data has landed, producing an atomic
             transition instead of a mid-load tear-down. -->
        {#if !itemDetailStore.notFound}
          {#key itemDetailStore.item?.id ?? itemId}
            <div
              class="transition-opacity duration-200 ease-in-out flex flex-col flex-1 min-h-0 overflow-hidden"
              class:opacity-90={itemDetailStore.transitioning}
              class:opacity-100={!itemDetailStore.transitioning}
            >
              {@render contentSnippet()}
            </div>
          {/key}
        {/if}
      {/if}
    </div>
  </Modal>
{:else}
<!-- Full Page Container -->
<div
  bind:this={modalElement}
  data-testid="item-detail"
  data-live-updates={liveStream.connected ? 'connected' : 'disconnected'}
  class="flex flex-col w-full h-full relative"
  style="background-color: var(--ds-surface-raised);"
>
  <!-- Shared Content Component for Full Page -->
  {#if !itemDetailStore.notFound}
    {#key itemDetailStore.item?.id ?? itemId}
      <div
        class="transition-opacity duration-200 ease-in-out"
        class:opacity-90={itemDetailStore.transitioning}
        class:opacity-100={!itemDetailStore.transitioning}
      >
        {@render contentSnippet()}
      </div>
    {/key}
  {/if}
</div>

{#if !isModal}
  <TestCaseViewModal
    isOpen={itemDetailStore.showTestCaseModal}
    testCaseId={itemDetailStore.selectedTestCaseId}
    onclose={handleCloseTestCaseModal}
  />
{/if}

<!-- Time Log Modal -->
{#if itemDetailStore.showTimeLogModal}
  <TimeLogModal
    defaultProjectId={getDefaultProjectForTimeLogging()}
    defaultItemId={parseInt(itemId)}
    projects={itemDetailStore.timeProjects}
    customers={itemDetailStore.customers}
    workItems={itemDetailStore.workItems}
    workspaces={itemDetailStore.workspaces}
    editingWorklog={itemDetailStore.editingWorklog}
    showProjectField={true}
    showWorkItemField={false}
    onsave={handleModalSave}
    oncancel={handleModalCancel}
  />
{/if}
{/if}

<ItemMoveWorkspaceDialog
  bind:isOpen={showWorkspaceMoveDialog}
  item={itemDetailStore.item}
  onMoved={handleWorkspaceMoved}
/>

{#if showTypeChangeModal && typeChangeAnalysis}
  <Modal isOpen={showTypeChangeModal} maxWidth="max-w-lg" onclose={closeTypeChangeModal}>
    <div class="p-6 space-y-5" style="background-color: var(--ds-surface); color: var(--ds-text);">
      <div>
        <h2 class="text-lg font-semibold">Map status for item type change</h2>
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          Changing from {typeChangeAnalysis.current_item_type_name || 'current type'} to {typeChangeAnalysis.target_item_type_name} requires choosing a status in the target workflow.
        </p>
      </div>

      {#if typeChangeError}
        <div class="rounded border px-3 py-2 text-sm" style="border-color: var(--ds-border-danger); color: var(--ds-text-danger); background-color: var(--ds-background-danger);">
          {typeChangeError}
        </div>
      {/if}

      <div class="rounded p-3" style="background-color: var(--ds-surface-raised);">
        <div class="text-sm mb-2" style="color: var(--ds-text-subtle);">
          Current status: <span class="font-medium" style="color: var(--ds-text);">{typeChangeAnalysis.current_status_name || 'None'}</span>
        </div>
        <label class="block text-sm font-medium mb-1" for="type-change-status">Target status</label>
        <NativeSelect
          id="type-change-status"
          bind:value={selectedTypeChangeStatusId}
          options={(typeChangeAnalysis.available_statuses || []).map((status) => ({ value: status.id, label: status.name }))}
          placeholder="Select target status…"
          size="small"
        />
        <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
          The change is blocked if the selected status would bypass a condition-gated transition or an approval-bound status.
        </p>
      </div>

      <div class="flex justify-end gap-2 pt-2">
        <Button variant="secondary" onclick={closeTypeChangeModal} disabled={changingItemType}>Cancel</Button>
        <Button
          variant="primary"
          onclick={() => executeItemTypeChange(typeChangeAnalysis.target_item_type_id, selectedTypeChangeStatusId ? Number(selectedTypeChangeStatusId) : null)}
          disabled={!selectedTypeChangeStatusId || changingItemType}
        >
          {changingItemType ? 'Changing…' : 'Change item type'}
        </Button>
      </div>
    </div>
  </Modal>
{/if}


<!-- Delete Item Dialog -->
<DeleteItemDialog
  bind:show={itemDetailStore.showDeleteDialog}
  item={itemDetailStore.item}
  ondeleted={handleDeleteComplete}
  onerror={handleDeleteError}
/>

<!-- Link Item Modal -->
<LinkItemModal
  isOpen={itemDetailStore.showLinkModal}
  linkTypes={itemDetailStore.filteredLinkTypes}
  currentItemId={parseInt(itemId)}
  workspaceId={parseInt(workspaceId)}
  preselectLinkTypeId={itemDetailStore.linkModalPreselectTypeId}
  onsubmit={handleLinkCreated}
  oncancel={handleLinkModalCancel}
/>

<!-- AI View Modal (Catch Me Up / Find Similar) -->
{#if aiModalType === 'catch-me-up' || aiModalType === 'find-similar'}
  <AIViewModal
    show={aiModalType === 'catch-me-up' || aiModalType === 'find-similar'}
    title={aiModalType === 'catch-me-up' ? 'Catch Me Up' : 'Find Similar Items'}
    icon={aiModalType === 'catch-me-up' ? BookOpen : Search}
    loading={aiLoading}
    error={aiError}
    onclose={closeAIModal}
  >
    {#if aiResult && aiModalType === 'catch-me-up'}
      <CatchMeUpBriefing briefing={aiResult.briefing} itemKey={aiResult.item_key} />
    {:else if aiResult && aiModalType === 'find-similar'}
      <FindSimilarResults
        similarItems={aiResult.similar_items || []}
        summary={aiResult.summary}
        onnavigate={handleNavigate}
      />
    {/if}
  </AIViewModal>
{/if}

<!-- AI Confirm Modal (Decompose) -->
{#if aiModalType === 'decompose'}
  <AIConfirmModal
    show={aiModalType === 'decompose'}
    title="Break Down Into Sub-Tasks"
    icon={GitBranch}
    loading={aiLoading}
    error={aiError}
    subTasks={aiResult?.sub_tasks || []}
    reasoning={aiResult?.reasoning || ''}
    creating={aiCreating}
    onclose={closeAIModal}
    oncreate={handleCreateSubTasks}
  />
{/if}

<!-- Recurrence Editor Modal -->
{#if showRecurrenceModal}
  <FormModal
    isOpen={true}
    title={t('recurrence.title')}
    saveLabel={t('common.save')}
    saving={recurrenceSaving}
    onSave={handleRecurrenceFormSave}
    onCancel={() => showRecurrenceModal = false}
    maxWidth="max-w-lg"
  >
    {#snippet footerExtra()}
      {#if recurrenceRule}
        <Button variant="danger" size="small" icon={Trash2} onclick={handleRecurrenceDelete}>
          {t('recurrence.deleteRule')}
        </Button>
      {/if}
    {/snippet}
    <RecurrenceEditor
      bind:this={recurrenceEditorRef}
      {itemId}
      existingRule={recurrenceRule}
      compact
      onsave={handleRecurrenceSave}
    />
  </FormModal>
{/if}
