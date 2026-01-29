<script>
  import { onMount, onDestroy, untrack } from 'svelte';
  import { useEventListener } from 'runed';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { workspacePermissions, itemDetailStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { toHotkeyString, getShortcut, matchesShortcut, isTypingInField } from '../../utils/keyboardShortcuts.js';
  import { Trash2, FileText, AlertCircle, X, Maximize2, Minimize2, Copy } from 'lucide-svelte';
  import { scale, fly } from 'svelte/transition';
  import { quintOut } from 'svelte/easing';
  import { Bookmark, BookmarkCheck, ExternalLink } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { addToast, successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { timerStore } from '../../stores/timerStore.svelte.js';
  import { useItemAttachments } from '../../composables/useItemAttachments.svelte.js';
  import { createEventDispatcher } from 'svelte';
import {
    registerContextCommands,
    unregisterContextCommands,
    createContextCommand,
    COMMAND_PRIORITIES
  } from '../../utils/contextCommands.js';
import Modal from '../../dialogs/Modal.svelte';
import DeleteItemDialog from '../../dialogs/DeleteItemDialog.svelte';
import LinkItemModal from '../../dialogs/LinkItemModal.svelte';

  const dispatch = createEventDispatcher();

  // Import the shared content component
  import ItemDetailContent from '../items/ItemDetailContent.svelte';
import TimeLogModal from '../../dialogs/TimeLogModal.svelte';
import TestCaseViewModal from '../../dialogs/TestCaseViewModal.svelte';

  // Use centralized icon map for work item types
  const iconMap = itemTypeIconMap;

  let {
    workspaceId,
    itemId,
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
  let parentHierarchy = $derived(itemDetailStore.parentHierarchy);
  let milestones = $derived(itemDetailStore.milestones);
  let iterations = $derived(itemDetailStore.iterations);
  let priorities = $derived(itemDetailStore.priorities);
  let customFieldDefinitions = $derived(itemDetailStore.customFieldDefinitions);
  let workspaceScreenFields = $derived(itemDetailStore.workspaceScreenFields);
  let workspaceScreenSystemFields = $derived(itemDetailStore.workspaceScreenSystemFields);
  let loading = $derived(itemDetailStore.loading);
  let error = $derived(itemDetailStore.error);
  let saving = $derived(itemDetailStore.saving);

  // Modal state
  let isFullscreen = $derived(itemDetailStore.isFullscreen);
  let modalElement = $state(null);

  // Editing state - derive from store's unified editing object
  let editingTitle = $derived(itemDetailStore.editing.title.active);
  let editTitle = $derived(itemDetailStore.editing.title.value);
  let dropdownItems = $derived(itemDetailStore.dropdownItems);
  let editingDescription = $derived(itemDetailStore.editing.description.active);
  let editDescription = $derived(itemDetailStore.editing.description.value);
  let editingStatus = $derived(itemDetailStore.editing.status.active);
  let editStatus = $state('');
  let editingPriority = $derived(itemDetailStore.editing.priority.active);
  let editingDueDate = $derived(itemDetailStore.editing.dueDate.active);
  let editPriority = $state('');
  let editingMilestone = $derived(itemDetailStore.editing.milestone.active);
  let editMilestone = $derived(itemDetailStore.editing.milestone.value);
  let editingIteration = $derived(itemDetailStore.editing.iteration.active);
  let editIteration = $derived(itemDetailStore.editing.iteration.value);
  let editingProject = $derived(itemDetailStore.editing.project.active);
  let editProject = $derived(itemDetailStore.editing.project.value);
  let editingAssignee = $derived(itemDetailStore.editing.assignee.active);
  let editAssignee = $derived(itemDetailStore.editing.assignee.value);
  let editingCustomFields = $derived(itemDetailStore.editing.customFields.active);
  let editCustomFieldValues = $derived(itemDetailStore.editing.customFields.values);
  let itemLinks = $derived(itemDetailStore.itemLinks);
  let linkTypes = $derived(itemDetailStore.linkTypes);
  let loadingLinks = $derived(itemDetailStore.loadingLinks);
  let showLinkModal = $derived(itemDetailStore.showLinkModal);
  const TEST_LINK_TYPE_ID = 1;
  let showTestCaseModal = $derived(itemDetailStore.showTestCaseModal);
  let selectedTestCaseId = $derived(itemDetailStore.selectedTestCaseId);

  // Delete dialog state
  let showDeleteDialog = $derived(itemDetailStore.showDeleteDialog);

  // Filter link types for item → item linking
  // The "Tests" link type (ID=1) can only link between items and test cases
  let filteredLinkTypes = $derived(itemDetailStore.filteredLinkTypes);

  let currentItemType = $derived(itemDetailStore.currentItemType);
  let currentHierarchyLevel = $derived(itemDetailStore.currentHierarchyLevel);
  let availableSubIssueTypes = $derived(itemDetailStore.availableSubIssueTypes);
  let isWatching = $derived(itemDetailStore.isWatching);
  let loadingWatchStatus = $derived(itemDetailStore.loadingWatchStatus);
  let childItems = $derived(itemDetailStore.childItems);
  let loadingChildItems = $derived(itemDetailStore.loadingChildItems);
  let itemTypes = $derived(itemDetailStore.itemTypes);
  let timeProjects = $derived(itemDetailStore.timeProjects);
  let timeWorklogs = $derived(itemDetailStore.timeWorklogs);
  let showTimeLogModal = $derived(itemDetailStore.showTimeLogModal);
  let editingWorklog = $derived(itemDetailStore.editingWorklog);
  let workItems = $derived(itemDetailStore.workItems);
  let customers = $derived(itemDetailStore.customers);
  let workspaces = $derived(itemDetailStore.workspaces);

  // Diagrams
  let diagrams = $derived(itemDetailStore.diagrams);
  let loadingDiagrams = $derived(itemDetailStore.loadingDiagrams);

  // Manual actions
  let manualActions = $derived(itemDetailStore.manualActions);

  // Status transition lazy loading
  let availableStatusTransitions = $derived(itemDetailStore.availableStatusTransitions);
  let loadingStatusTransitions = $derived(itemDetailStore.loadingStatusTransitions);

  // Track if any changes were made
  let hasChanges = $derived(itemDetailStore.hasChanges);

  // Track itemId changes for reactivity
  let previousItemId = $state(itemId);

  // Timer guard flag to prevent duplicate timer starts
  let isStartingTimer = $state(false);

  // Animation state for smooth transitions
  let transitioning = $derived(itemDetailStore.transitioning);
  
  // Status options are loaded dynamically from the API
  // No hardcoded defaults - backend returns all statuses with category colors
  // Priority options are now loaded dynamically via PriorityPicker component

  // Status options derived from store
  let statusOptions = $derived(itemDetailStore.statusOptions);

  // Modal control functions
  function closeModal() {
    if (isModal && onclose) {
      onclose({ hasChanges: itemDetailStore.hasChanges });
    } else if (!isModal) {
      navigate(`/workspaces/${workspaceId}`);
    }
  }

  function toggleFullscreen() {
    itemDetailStore.toggleFullscreen();
  }

  // Handle Escape key manually (needs complex modal/editing state checks)
  useEventListener(() => document, 'keydown', (event) => {
    if (event.key !== 'Escape') return;
    if (!isModal) return;

    const isEditing = editingTitle || editingDescription || editingStatus ||
                     editingPriority || editingMilestone || editingIteration || editingProject || editingAssignee ||
                     Object.keys(editingCustomFields).length > 0;
    const createModalOpen = document.querySelector('.create-work-item-modal, [role="dialog"]');

    if (!isEditing && !createModalOpen) {
      closeModal();
    }
  });

  // Handle global keyboard shortcuts for item detail
  useEventListener(() => document, 'keydown', (e) => {
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

    // Shift+F - Fullscreen toggle / open full details
    if (matchesShortcut(e, getShortcut('itemDetail', 'fullscreen'))) {
      e.preventDefault();
      handleHotkeyFullscreen();
      return;
    }

    // Shift+W - Create child work item
    if (matchesShortcut(e, getShortcut('itemDetail', 'createChild'))) {
      e.preventDefault();
      if (availableSubIssueTypes?.length) handleHotkeyW();
      return;
    }
  });

  function handleHotkeyFullscreen() {
    if (isModal) {
      openFullDetails();
    } else {
      toggleFullscreen();
    }
  }

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
    navigate(`/workspaces/${workspaceId}/items/${itemId}`);
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
  function handleNavigate(event) {
    const path = event.detail?.path;
    if (!path) return;

    if (tryHandleModalItemNavigation(path)) {
      return;
    }

    navigate(path);
  }
  
  function handleGoBack() {
    navigate(`/workspaces/${workspaceId}/collections/default/list`);
  }
  
  function showError(title, message) {
    errorToast(message, title);
  }

  async function handleCopyKey() {
    console.log('[handleCopyKey] Copy key button clicked');
    try {
      const key = `${workspace?.key || 'WORK'}-${item.workspace_item_number}`;
      console.log('[handleCopyKey] Copying key:', key);
      await navigator.clipboard.writeText(key);
      console.log('[handleCopyKey] Copy successful, calling showCopySuccess');
      showCopySuccess(key);
    } catch (error) {
      console.error('[handleCopyKey] Failed to copy key to clipboard:', error);
      // Fallback for browsers that don't support clipboard API
      try {
        const textArea = document.createElement('textarea');
        const key = `${workspace?.key || 'WORK'}-${item.workspace_item_number}`;
        textArea.value = key;
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand('copy');
        document.body.removeChild(textArea);
        console.log('[handleCopyKey] Fallback copy successful');
        showCopySuccess(key);
      } catch (fallbackError) {
        console.error('[handleCopyKey] Fallback copy also failed:', fallbackError);
        showCopyError();
      }
    }
  }

  function showCopySuccess(key) {
    console.log('[showCopySuccess] Showing copy toast for key:', key);
    successToast(`${workspace?.key || 'WORK'}-${item?.workspace_item_number}`, t('toast.copied'));
  }

  function showCopyError() {
    errorToast(t('items.failedToCopyToClipboard'), t('items.copyError'));
  }
  
  async function handleSaveField(event) {
    const { field, value, assigneeName, iterationName } = event.detail;
    await saveField(field, value, assigneeName, iterationName);
  }
  
  function handleCancelEdit(event) {
    const { field } = event.detail;
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

    itemDetailStore.cancelEditing(storeField);
  }
  
  function handleStartEditingCustomField(event) {
    const fieldId = event.detail.fieldId;
    // Cancel assignee editing when starting to edit a custom field
    itemDetailStore.cancelEditing('assignee');
    itemDetailStore.startEditing(`custom_field_${fieldId}`);
  }
  
  function handleSwitchTab(event) {
    tab = event.detail.tab;
    const url = `/workspaces/${workspaceId}/items/${itemId}${tab !== 'comments' ? `?tab=${tab}` : ''}`;
    navigate(url);
  }
  
  function handleCreateSubIssue() {
    startCreateSubIssue();
  }

  function handleShowLinkModal() {
    itemDetailStore.openLinkModal();
  }

  function handleLinkModalCancel() {
    itemDetailStore.closeLinkModal();
  }

  async function handleLinkCreated(event) {
    const { link_type_id, target_id, target_type } = event.detail;

    try {
      await itemDetailStore.createLink(link_type_id, target_id, target_type);
    } catch (error) {
      console.error('Error creating link:', error);
      showError('Failed to create link', error.message || 'Unknown error');
    }
  }

  function handleViewTestCase(event) {
    const { testCaseId } = event.detail || {};
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

  async function handleRemoveLink(event) {
    const { linkId } = event.detail;

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
      console.log('Timer start already in progress, skipping duplicate request');
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

  function handleLogTime() {
    itemDetailStore.openTimeLogModal();
  }

  function handleEditWorklog(event) {
    itemDetailStore.openTimeLogModal(event.detail);
  }

  async function handleDeleteWorklog(event) {
    const worklog = event.detail;
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

  async function handleCopyItem() {
    try {
      const copiedItem = await itemDetailStore.copyItem();

      // Show clickable success toast that navigates to the copied item
      const itemKey = workspace?.key ? `${workspace.key}-${copiedItem.workspace_item_number}` : `ITEM-${copiedItem.workspace_item_number}`;
      addToast({
        title: t('items.itemCopiedAs', { key: itemKey }),
        message: t('items.clickToViewCopied'),
        variant: 'success',
        duration: 15000,
        clickable: true,
        onClick: () => {
          navigate(`/workspaces/${workspaceId}/items/${copiedItem.id}`);
        }
      });

    } catch (err) {
      console.error('Failed to copy item:', err);
      showError(t('items.failedToCopy'), err.message || String(err));
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

  function handleDeleteItem() {
    itemDetailStore.openDeleteDialog();
  }

  function handleDeleteComplete(result) {
    // Navigate based on deletion result
    if (result?.mode === 'reparent' && result?.newParentId) {
      // If reparenting, navigate to the new parent item
      navigate(`/workspaces/${workspaceId}/items/${result.newParentId}`);
    } else {
      // Otherwise, navigate to workspace list
      navigate(`/workspaces/${workspaceId}/list`);
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
          title: t('items.deleteWorkItem'),
          color: '#dc2626',
          hoverClass: 'hover:bg-red-50 hover:text-red-700',
          onClick: handleDeleteItem
        }
      );
    }

    itemDetailStore.dropdownItems = items;
  }

  // Reactive statement to handle itemId changes for navigation between items
  $effect(() => {
    if (itemId !== previousItemId && !itemDetailStore.loading) {
      previousItemId = itemId;
      itemDetailStore.transitioning = true;


      // Small delay to allow fade out animation
      setTimeout(() => {
        itemDetailStore.loading = true;

        // Clear all state before loading new data to prevent stale data during navigation
        itemDetailStore.clearForNavigation();

        loadData().then(() => {
          populateDropdownItems();
          itemDetailStore.loading = false;
          itemDetailStore.transitioning = false;
        }).catch((error) => {
          console.error('Failed to load item data after navigation:', error);
          itemDetailStore.loading = false;
          itemDetailStore.transitioning = false;
        });
      }, 150);
    }
  });

  // Handler for executing a manual action
  async function handleExecuteAction(event) {
    const action = event.detail;
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
    await itemDetailStore.loadDiagrams();
  }

  onMount(async () => {
    // Initialize timer state from server
    await timerStore.initialize();

    await loadData();

    // Register context-sensitive commands for this item
    // Pass current timer status to avoid creating reactive dependency
    registerItemContextCommands(!!timerStore.getCurrent());
  });
  
  // Register context commands for this work item
  // Pass hasActiveTimer as a parameter to avoid reactive dependency on $activeTimer
  function registerItemContextCommands(hasActiveTimer = false) {
    if (!item) return;

    const itemKey = workspace?.key ? `${workspace.key}-${item.workspace_item_number}` : `ITEM-${item.workspace_item_number}`;
    const commands = [];

    // Add Link command
    commands.push(createContextCommand({
      id: 'add-link-to-item',
      label: `Add Link to ${itemKey}`,
      description: 'Link this work item to another item',
      keywords: ['link', 'connect', 'relate', 'add', 'reference'],
      action: () => {
        showLinkModal = true;
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
        const url = `${window.location.origin}/workspaces/${workspaceId}/items/${itemId}`;
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
    // Cleanup timer intervals
    timerStore.cleanup();
  });

  // Load data using the store
  async function loadData() {
    await itemDetailStore.loadItem(workspaceId, itemId);

    // Load attachment settings and attachments (still using composable)
    await attachmentManager.loadSettings();
    if (attachmentManager.isEnabled()) {
      await attachmentManager.load();
    }
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
    {iconMap}
    {workspaceId}
    editingTitle={itemDetailStore.editing.title.active}
    editTitle={itemDetailStore.editing.title.value}
    saving={itemDetailStore.saving}
    dropdownItems={itemDetailStore.dropdownItems}
    statusOptions={itemDetailStore.statusOptions}
    editingDescription={itemDetailStore.editing.description.active}
    editDescription={itemDetailStore.editing.description.value}
    itemLinks={itemDetailStore.itemLinks}
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
    editingProject={itemDetailStore.editing.project.active}
    editingAssignee={itemDetailStore.editing.assignee.active}
    editingMilestone={itemDetailStore.editing.milestone.active}
    editingIteration={itemDetailStore.editing.iteration.active}
    editingCustomFields={itemDetailStore.editing.customFields.active}
    editCustomFieldValues={itemDetailStore.editing.customFields.values}
    workspaceScreenFields={itemDetailStore.workspaceScreenFields}
    workspaceScreenSystemFields={itemDetailStore.workspaceScreenSystemFields}
    customFieldDefinitions={itemDetailStore.customFieldDefinitions}
    milestones={itemDetailStore.milestones}
    iterations={itemDetailStore.iterations}
    priorities={itemDetailStore.priorities}
    attachments={attachmentManager.attachments || []}
    attachmentPagination={attachmentManager.pagination}
    diagrams={itemDetailStore.diagrams}
    loadingDiagrams={itemDetailStore.loadingDiagrams}
    manualActions={itemDetailStore.manualActions}
    on:navigate={handleNavigate}
    on:go-back={handleGoBack}
    on:copy-key={handleCopyKey}
    on:save-field={handleSaveField}
    on:cancel-edit={handleCancelEdit}
    on:switch-tab={handleSwitchTab}
    on:create-sub-issue={handleCreateSubIssue}
    on:remove-link={handleRemoveLink}
    on:view-test-case={handleViewTestCase}
    on:show-link-modal={handleShowLinkModal}
    on:start-editing-assignee={handleStartEditingAssignee}
    on:start-editing-milestone={handleStartEditingMilestone}
    on:start-editing-iteration={handleStartEditingIteration}
    on:start-editing-priority={handleStartEditingPriority}
    on:start-editing-due-date={handleStartEditingDueDate}
    on:start-editing-status={handleStartEditingStatus}
    on:start-editing-project={handleStartEditingProject}
    on:start-editing-description={handleStartEditingDescription}
    on:start-editing-custom-field={handleStartEditingCustomField}
    on:start-timer={handleStartTimer}
    on:log-time={handleLogTime}
    on:edit-worklog={handleEditWorklog}
    on:delete-worklog={handleDeleteWorklog}
    on:parent-changed={handleParentChanged}
    on:attachment-upload={attachmentManager.handleUpload}
    on:attachment-upload-files={attachmentManager.uploadFiles}
    on:attachment-delete={attachmentManager.handleDelete}
    on:attachment-page-change={attachmentManager.handlePageChange}
    on:attachment-page-size-change={attachmentManager.handlePageSizeChange}
    on:diagram-saved={handleDiagramSaved}
    on:execute-action={handleExecuteAction}
    on:close={closeModal}
  />
{/snippet}

{#if isModal}
  <Modal
    isOpen={true}
    maxWidth={itemDetailStore.isFullscreen ? 'max-w-[95vw]' : 'max-w-[80vw]'}
    onclose={closeModal}
  >
    <div
      bind:this={modalElement}
      class="flex flex-col relative w-full {itemDetailStore.isFullscreen ? 'h-[95vh]' : 'max-h-[90vh]'}"
    >
      {#if itemDetailStore.showTestCaseModal}
        <TestCaseViewModal
          embedded={true}
          isOpen={itemDetailStore.showTestCaseModal}
          testCaseId={itemDetailStore.selectedTestCaseId}
          on:close={handleCloseTestCaseModal}
        />
      {:else}
        {#if itemDetailStore.item && itemDetailStore.workspace}
          <!-- Modal Header -->
          <div class="flex items-center justify-between p-4 border-b" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
            <div class="flex items-center gap-3">
              <h1 class="text-lg font-semibold" style="color: var(--ds-text);">{t('items.workItemDetails')}</h1>
              <span class="px-2 py-1 text-sm font-mono rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {itemDetailStore.workspace.key}-{itemDetailStore.item.workspace_item_number}
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
                onclick={toggleFullscreen}
                class="p-2 rounded transition-colors"
                style="color: var(--ds-text-subtle);"
                onmouseenter={(e) => { e.currentTarget.style.color = 'var(--ds-text)'; e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; }}
                onmouseleave={(e) => { e.currentTarget.style.color = 'var(--ds-text-subtle)'; e.currentTarget.style.backgroundColor = ''; }}
                title={itemDetailStore.isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
              >
                {#if itemDetailStore.isFullscreen}
                  <Minimize2 class="w-5 h-5" />
                {:else}
                  <Maximize2 class="w-5 h-5" />
                {/if}
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
        {#key itemId}
          <div
            class="transition-opacity duration-300 ease-in-out overflow-y-auto flex-1"
            class:opacity-30={itemDetailStore.transitioning}
            class:opacity-100={!itemDetailStore.transitioning}
          >
            {@render contentSnippet()}
          </div>
        {/key}
      {/if}
    </div>
  </Modal>
{:else}
<!-- Full Page Container -->
<div
  bind:this={modalElement}
  class="flex flex-col w-full h-full relative"
  style="background-color: var(--ds-surface-raised);"
>
  <!-- Shared Content Component for Full Page -->
  {#key itemId}
    <div
      class="transition-opacity duration-300 ease-in-out"
      class:opacity-30={itemDetailStore.transitioning}
      class:opacity-100={!itemDetailStore.transitioning}
    >
      {@render contentSnippet()}
    </div>
  {/key}
</div>

{#if !isModal}
  <TestCaseViewModal
    isOpen={itemDetailStore.showTestCaseModal}
    testCaseId={itemDetailStore.selectedTestCaseId}
    on:close={handleCloseTestCaseModal}
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

<!-- Delete Item Dialog -->
<DeleteItemDialog
  show={itemDetailStore.showDeleteDialog}
  item={itemDetailStore.item}
  ondeleted={handleDeleteComplete}
  onerror={handleDeleteError}
/>

<!-- Link Item Modal -->
<LinkItemModal
  isOpen={itemDetailStore.showLinkModal}
  linkTypes={itemDetailStore.filteredLinkTypes}
  currentItemId={parseInt(itemId)}
  on:submit={handleLinkCreated}
  on:cancel={handleLinkModalCancel}
/>
