<script>
  import { onMount, onDestroy, untrack } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { workspacePermissions } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { Trash2, FileText, AlertCircle, X, Maximize2, Minimize2, Copy } from 'lucide-svelte';
  import { scale, fly } from 'svelte/transition';
  import { quintOut } from 'svelte/easing';
  import { Bookmark, BookmarkCheck, ExternalLink } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { addToast, successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { useTimer } from '../../composables/useTimer.svelte.js';
  import { useItemAttachments } from '../../composables/useItemAttachments.svelte.js';
  import { createEventDispatcher } from 'svelte';
import { 
    registerContextCommands, 
    unregisterContextCommands, 
    createContextCommand,
    COMMAND_PRIORITIES 
  } from '../../utils/contextCommands.js';
import Modal from '../../dialogs/Modal.svelte';
  
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

  // Initialize timer composable with reactive stores
  const timer = useTimer();
  const { activeTimer, canStartTimer, canStopTimer } = timer;

  // Initialize attachment composable
  const attachmentManager = useItemAttachments(
    () => item?.id,
    (title, message) => errorToast(message, title)
  );

  // All component state variables...
  let item = $state(null);
  let workspace = $state(null);
  let parentHierarchy = $state([]);
  let milestones = $state([]);
  let iterations = $state([]);
  let priorities = $state([]);
  let customFieldDefinitions = $state([]);
  let workspaceScreenFields = $state([]);
  let workspaceScreenSystemFields = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let saving = $state(false);

  // Modal state
  let isFullscreen = $state(false);
  let modalElement = $state(null);

  // All other state variables...
  let editingTitle = $state(false);
  let editTitle = $state('');
  let dropdownItems = $state([]);
  let editingDescription = $state(false);
  let editDescription = $state('');
  let editingStatus = $state(false);
  let editStatus = $state('');
  let editingPriority = $state(false);
  let editingDueDate = $state(false);
  let editPriority = $state('');
  let editingMilestone = $state(false);
  let editMilestone = $state(null);
  let editingIteration = $state(false);
  let editIteration = $state(null);
  let editingProject = $state(false);
  let editProject = $state(null);
  let editingAssignee = $state(false);
  let editAssignee = $state(null);
  let editingCustomFields = $state({});
  let editCustomFieldValues = $state({});
  let itemLinks = $state([]);
  let linkTypes = $state([]);
  let loadingLinks = $state(false);
  let showAddLinkForm = $state(false);
  const TEST_LINK_TYPE_ID = 1;
  let addLinkData = $state({
    link_type_id: null,
    target_id: null,
    target_title: '',
    target_type: 'item'
  });
  let showTestCaseModal = $state(false);
  let selectedTestCaseId = $state(null);
  let searchResults = $state([]);
  let searchQuery = $state('');
  let searching = $state(false);

  // Filter link types for item → item linking
  // The "Tests" link type (ID=1) can only link between items and test cases
  let filteredLinkTypes = $derived(linkTypes);

  let currentItemType = $state(null);
  let currentHierarchyLevel = $state(null);
  let availableSubIssueTypes = $state([]);
  let isWatching = $state(false);
  let loadingWatchStatus = $state(false);
  let childItems = $state([]);
  let loadingChildItems = $state(false);
  let itemTypes = $state([]);
  let timeProjects = $state([]);
  let timeWorklogs = $state([]);
  let showTimeLogModal = $state(false);
  let workItems = $state([]);
  let customers = $state([]);
  let workspaces = $state([]);

  // Diagrams
  let diagrams = $state([]);
  let loadingDiagrams = $state(false);

  // Status transition lazy loading
  let availableStatusTransitions = $state([]);
  let loadingStatusTransitions = $state(false);

  // Track if any changes were made
  let hasChanges = $state(false);

  // Track itemId changes for reactivity
  let previousItemId = $state(itemId);

  // Timer guard flag to prevent duplicate timer starts
  let isStartingTimer = $state(false);

  // Animation state for smooth transitions
  let transitioning = $state(false);
  
  // Status options are loaded dynamically from the API
  // No hardcoded defaults - backend returns all statuses with category colors
  // Priority options are now loaded dynamically via PriorityPicker component

  // All functions for data loading and management...
  // Lazy load status transitions for the current item
  async function loadAvailableStatusTransitions() {
    if (!item?.id || loadingStatusTransitions) {
      return;
    }

    try {
      loadingStatusTransitions = true;
      const result = await api.items.getAvailableStatusTransitions(item.id);
      availableStatusTransitions = result.available_transitions || [];
    } catch (error) {
      console.error('Failed to load status transitions:', error);
      availableStatusTransitions = [];
    } finally {
      loadingStatusTransitions = false;
    }
  }

  async function loadPriorities() {
    if (!workspace) return;

    try {
      if (workspace.configuration_set_id) {
        // Load priorities from configuration set
        const configSet = await api.configurationSets.get(workspace.configuration_set_id);
        priorities = configSet.priorities_detailed || [];
      } else {
        // No configuration set - load all priorities
        priorities = await api.priorities.getAll();
      }

      // Sort by sort_order
      priorities = priorities.sort((a, b) => a.sort_order - b.sort_order);
    } catch (err) {
      console.error('Failed to load priorities:', err);
      priorities = [];
    }
  }

  // Reactive status options based on loaded transitions
  let statusOptions = $derived.by(() => {
    // If transitions are loaded, use them
    if (availableStatusTransitions.length > 0) {
      return availableStatusTransitions.map(transition => ({
        id: transition.id,
        value: transition.value,
        label: transition.name,
        categoryColor: transition.category_color || null
      }));
    }

    // Return loading state or empty array
    return loadingStatusTransitions ? [{ value: '', label: t('common.loading') }] : [];
  });

  // Modal control functions
  function closeModal() {
    if (isModal && onclose) {
      onclose({ hasChanges });
    } else if (!isModal) {
      navigate(`/workspaces/${workspaceId}`);
    }
  }

  function toggleFullscreen() {
    isFullscreen = !isFullscreen;
  }

  function handleKeydown(event) {
    if (event.key === 'Escape') {
      // Only handle ESC in modal mode
      if (!isModal) return;

      // Don't close if we're editing any field
      const isEditing = editingTitle || editingDescription || editingStatus ||
                       editingPriority || editingMilestone || editingIteration || editingProject || editingAssignee ||
                       Object.keys(editingCustomFields).length > 0;

      // Don't close if the create modal is open
      const createModalOpen = document.querySelector('.create-work-item-modal, [role="dialog"]');

      if (!isEditing && !createModalOpen) {
        closeModal();
      }
    } else if ((event.key === 'f' || event.key === 'F') && !event.ctrlKey && !event.metaKey) {
      // Don't trigger if user is typing in an input field
      const activeElement = document.activeElement;
      const isInputField = activeElement && (
        activeElement.tagName === 'INPUT' ||
        activeElement.tagName === 'TEXTAREA' ||
        activeElement.contentEditable === 'true' ||
        activeElement.classList.contains('ProseMirror')
      );

      if (!isInputField) {
        event.preventDefault();
        if (isModal) {
          // In modal mode: open full details
          openFullDetails();
        } else {
          // In full details mode: focus status field
          editingCustomFields = {};
          editingStatus = true;
        }
      }
    } else if ((event.key === 'w' || event.key === 'W') && !event.ctrlKey && !event.metaKey) {
      // Don't trigger if user is typing in an input field
      const activeElement = document.activeElement;
      const isInputField = activeElement && (
        activeElement.tagName === 'INPUT' || 
        activeElement.tagName === 'TEXTAREA' || 
        activeElement.contentEditable === 'true' ||
        activeElement.classList.contains('ProseMirror')
      );
      
      if (!isInputField) {
        event.preventDefault();
        // Check if Create Child Work Item is available
        if (availableSubIssueTypes && availableSubIssueTypes.length > 0) {
          handleCreateSubIssue();
        }
      }
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
    if (saving) return;
    
    try {
      saving = true;
      let updateData = {};
      
      if (field === 'title') {
        const newTitle = directValue || editTitle.trim();
        if (newTitle === item.title) {
          cancelEdit('title');
          return;
        }
        updateData.title = newTitle;
      } else if (field === 'description') {
        const newDescription = directValue !== null ? directValue : editDescription;
        if (newDescription === (item.description || '')) {
          cancelEdit('description');
          return;
        }
        updateData.description = newDescription;
      } else if (field === 'status') {
        const newStatus = directValue || editStatus;
        if (newStatus === item.status) {
          cancelEdit('status');
          return;
        }
        updateData.status = newStatus;
      } else if (field === 'status_id') {
        const newStatusId = directValue !== null ? directValue : null;
        if (newStatusId === item.status_id) {
          cancelEdit('status_id');
          return;
        }
        updateData.status_id = newStatusId;

        // Optimistic update - update UI immediately
        item = {
          ...item,
          status_id: newStatusId
        };
      } else if (field === 'priority') {
        const newPriority = directValue || editPriority;
        if (newPriority === item.priority) {
          cancelEdit('priority');
          return;
        }
        updateData.priority = newPriority;
      } else if (field === 'priority_id') {
        const newPriorityId = directValue !== null ? directValue : null;
        if (newPriorityId === item.priority_id) {
          cancelEdit('priority_id');
          return;
        }
        updateData.priority_id = newPriorityId;

        // Optimistic update - update UI immediately
        item = {
          ...item,
          priority_id: newPriorityId
        };
      } else if (field === 'due_date') {
        const newDueDate = directValue !== null ? directValue : null;
        if (newDueDate === item.due_date) {
          cancelEdit('due_date');
          return;
        }
        updateData.due_date = newDueDate;

        // Optimistic update - update UI immediately
        item = {
          ...item,
          due_date: newDueDate
        };
      } else if (field === 'milestone') {
        const newMilestone = directValue !== null ? directValue : editMilestone;
        if (newMilestone === item.milestone_id) {
          cancelEdit('milestone');
          return;
        }
        updateData.milestone_id = newMilestone;

        // Optimistic update - update UI immediately
        item = {
          ...item,
          milestone_id: newMilestone
        };
      } else if (field === 'iteration') {
        const newIteration = directValue !== null ? directValue : null;
        if (newIteration === item.iteration_id) {
          return;
        }
        updateData.iteration_id = newIteration;

        // Optimistic update - update UI immediately
        item = {
          ...item,
          iteration_id: newIteration,
          iteration_name: iterationName !== undefined ? iterationName : item.iteration_name
        };
      } else if (field === 'project') {
        const newProject = directValue !== null ? directValue : editProject;
        // Handle object value with project_id and inherit_project
        if (typeof newProject === 'object' && newProject !== null) {
          updateData.project_id = newProject.project_id;
          updateData.inherit_project = newProject.inherit_project;

          // Optimistic update - update UI immediately
          item = {
            ...item,
            project_id: newProject.project_id,
            inherit_project: newProject.inherit_project
          };
        } else {
          // Fallback for old format
          if (newProject === item.project_id) {
            cancelEdit('project');
            return;
          }
          updateData.project_id = newProject;

          // Optimistic update - update UI immediately
          item = {
            ...item,
            project_id: newProject
          };
        }
      } else if (field === 'assignee') {
        const newAssignee = directValue !== undefined ? directValue : editAssignee;
        if (newAssignee === item.assignee_id) {
          cancelEdit('assignee');
          return;
        }
        updateData.assignee_id = newAssignee;

        // Optimistic update - update UI immediately
        item = {
          ...item,
          assignee_id: newAssignee,
          assignee_name: assigneeName !== undefined ? assigneeName : item.assignee_name
        };
      } else if (field.startsWith('custom_field_')) {
        const fieldId = field.replace('custom_field_', '');
        let newValue = directValue !== null ? directValue : editCustomFieldValues[fieldId];
        const currentValue = item.custom_field_values?.[fieldId] || '';
        
        // Convert number fields to actual numbers
        const fieldDef = customFieldDefinitions.find(field => field.id === parseInt(fieldId));
        if (fieldDef && fieldDef.field_type === 'number' && newValue !== null && newValue !== undefined && newValue !== '') {
          newValue = parseFloat(newValue);
          // If parsing failed, keep as string or handle as needed
          if (isNaN(newValue)) {
            newValue = directValue !== null ? directValue : editCustomFieldValues[fieldId];
          }
        }
        
        if (newValue === currentValue) {
          cancelEdit(field);
          return;
        }
        
        // Update custom field values
        updateData.custom_field_values = {
          ...(item.custom_field_values || {}),
          [fieldId]: newValue
        };
      }

      // Update item via API
      const updatedItem = await api.items.update(item.id, updateData);
      
      // Update local item data
      item = { ...item, ...updatedItem };
      
      // For assignee field, also update the assignee name if provided
      if (field === 'assignee' && assigneeName !== null) {
        item = { ...item, assignee_name: assigneeName };
      }

      // For iteration field, also update the iteration name if provided
      if (field === 'iteration' && iterationName !== undefined) {
        item = { ...item, iteration_name: iterationName };
      }

      // Mark that changes were made
      hasChanges = true;
      
      // Exit editing mode
      cancelEdit(field);
      
    } catch (err) {
      console.error('Failed to update item:', err);
      showError('Failed to update item', err.message || String(err));
    } finally {
      saving = false;
    }
  }

  function cancelEdit(field) {
    
    if (field === 'title') {
      editingTitle = false;
      editTitle = item?.title || ''; // Reset to original item title
    } else if (field === 'description') {
      editingDescription = false;
      // Force reactivity by creating a new reference
      editDescription = String(item?.description || '');
    } else if (field === 'status' || field === 'status_id') {
      editingStatus = false;
      editStatus = '';
    } else if (field === 'priority' || field === 'priority_id') {
      editingPriority = false;
      editPriority = '';
    } else if (field === 'due_date') {
      editingDueDate = false;
    } else if (field === 'milestone') {
      editingMilestone = false;
      editMilestone = null;
    } else if (field === 'iteration') {
      editingIteration = false;
      editIteration = null;
    } else if (field === 'project') {
      editingProject = false;
      editProject = null;
    } else if (field === 'assignee') {
      editingAssignee = false;
      editAssignee = null;
    } else if (field.startsWith('custom_field_')) {
      const fieldId = field.replace('custom_field_', '');
      delete editingCustomFields[fieldId];
      delete editCustomFieldValues[fieldId];
      editingCustomFields = { ...editingCustomFields }; // Trigger reactivity
    }
  }
  
  function handleStartEditingCustomField(event) {
    const fieldId = event.detail.fieldId;
    // Cancel assignee editing when starting to edit a custom field
    editingAssignee = false;
    editingCustomFields[fieldId] = true;
    // Use nullish coalescing to preserve 0 values for number fields
    const currentValue = item.custom_field_values?.[fieldId];
    editCustomFieldValues[fieldId] = currentValue !== null && currentValue !== undefined ? currentValue : '';
    editingCustomFields = { ...editingCustomFields }; // Trigger reactivity
  }
  
  function handleSwitchTab(event) {
    tab = event.detail.tab;
    const url = `/workspaces/${workspaceId}/items/${itemId}${tab !== 'comments' ? `?tab=${tab}` : ''}`;
    navigate(url);
  }
  
  function handleCreateSubIssue() {
    startCreateSubIssue();
  }

  function handleCancelAddLink() {
    showAddLinkForm = false;
    addLinkData = { link_type_id: null, target_id: null, target_title: '', target_type: 'item' };
  }

  function handleSelectItem(event) {
    const { selectedItem } = event.detail;
    addLinkData.target_id = selectedItem.id;
    addLinkData.target_title = selectedItem.title;
    addLinkData.target_type = selectedItem.type || (Number(addLinkData.link_type_id) === TEST_LINK_TYPE_ID ? 'test_case' : 'item');
  }

  function handleViewTestCase(event) {
    const { testCaseId } = event.detail || {};
    if (!testCaseId) return;
    const normalizedId = Number(testCaseId);
    if (!Number.isFinite(normalizedId)) {
      console.warn('Received invalid test case ID from link:', testCaseId);
      return;
    }
    selectedTestCaseId = normalizedId;
    showTestCaseModal = true;
  }

  function handleCloseTestCaseModal() {
    showTestCaseModal = false;
    selectedTestCaseId = null;
  }

  async function handleAddLink() {
    if (!addLinkData.link_type_id || !addLinkData.target_id) {
      console.error('Missing required data:', { link_type_id: addLinkData.link_type_id, target_id: addLinkData.target_id });
      return;
    }

    try {
      const result = await api.links.create({
        source_type: "item",
        source_id: parseInt(itemId),
        target_type: addLinkData.target_type || (Number(addLinkData.link_type_id) === TEST_LINK_TYPE_ID ? "test_case" : "item"),
        target_id: parseInt(addLinkData.target_id),
        link_type_id: parseInt(addLinkData.link_type_id)
      });
      
      
      // Reset form and close
      showAddLinkForm = false;
      addLinkData = { link_type_id: null, target_id: null, target_title: '', target_type: 'item' };
      
      // Reload links
      await loadData();
    } catch (error) {
      console.error('Error creating link:', error);
      console.error('Error details:', error.message);
      showError('Failed to create link', error.message || 'Unknown error');
    }
  }

  async function handleRemoveLink(event) {
    const { linkId } = event.detail;
    
    try {
      await api.links.delete(linkId);
      await loadData(); // Reload to refresh links
    } catch (error) {
      console.error('Error removing link:', error);
    }
  }

  function handleStartEditingAssignee() {
    // Cancel all custom field editing when starting to edit assignee
    editingCustomFields = {};
    editingAssignee = true;
  }

  function handleStartEditingMilestone() {
    // Cancel all custom field editing when starting to edit milestone
    editingCustomFields = {};
    editingMilestone = true;
  }

  function handleStartEditingIteration() {
    // Cancel all custom field editing when starting to edit iteration
    editingCustomFields = {};
    editingIteration = true;
  }

  function handleStartEditingPriority() {
    // Cancel all custom field editing when starting to edit priority
    editingCustomFields = {};
    editingPriority = true;
  }

  function handleStartEditingDueDate() {
    // Cancel all custom field editing when starting to edit due date
    editingCustomFields = {};
    editingDueDate = true;
  }

  function handleStartEditingStatus() {
    // Cancel all custom field editing when starting to edit status
    editingCustomFields = {};
    editingStatus = true;
  }

  function handleStartEditingProject() {
    // Cancel all custom field editing when starting to edit project
    editingCustomFields = {};
    editingProject = true;
  }

  async function handleStartTimer() {
    // Guard: Check if we're already starting a timer
    if (isStartingTimer) {
      console.log('Timer start already in progress, skipping duplicate request');
      return;
    }

    // Guard: Use reactive store values
    if (!$canStartTimer) {
      if (timer.syncing) {
        showError(t('items.timerBusy'), t('items.timerSyncingMessage'));
      } else if ($activeTimer) {
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

      await timer.start(timerData);
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
    showTimeLogModal = true;
  }

  async function handleModalSave(event) {
    try {
      const data = event.detail;
      await api.time.worklogs.create(data);

      // Reload worklogs
      const worklogsData = await api.time.worklogs.getByItem(itemId);
      timeWorklogs = worklogsData || [];

      showTimeLogModal = false;
    } catch (error) {
      console.error('Failed to save worklog:', error);
      showError(t('items.failedToSaveTimeEntry'), error.message || t('errors.UNKNOWN'));
    }
  }

  function handleModalCancel() {
    showTimeLogModal = false;
  }

  // Get default project for time logging
  function getDefaultProjectForTimeLogging() {
    if (item?.time_project_id) {
      return item.time_project_id;
    }
    if (item?.effective_project_id) {
      return item.effective_project_id;
    }
    if (workspace?.time_project_id) {
      return workspace.time_project_id;
    }
    return null;
  }

  async function handleCopyItem() {
    try {
      const copiedItem = await api.items.copy(item.id);

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

  async function handleDeleteItem() {
    const confirmed = await confirm({
      title: t('items.deleteWorkItem'),
      message: t('items.confirmDeleteItem', { title: item.title }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
      icon: Trash2
    });

    if (!confirmed) {
      return;
    }

    try {
      await api.items.delete(item.id);

      // Navigate back to list
      navigate(`/workspaces/${workspaceId}/collections/default/list`);


    } catch (err) {
      console.error('Failed to delete item:', err);
      showError(t('items.failedToDelete'), err.message || String(err));
    }
  }

  async function loadWatchStatus() {
    if (!item?.id || loadingWatchStatus) return;

    try {
      loadingWatchStatus = true;
      const result = await api.items.getWatchStatus(item.id);
      isWatching = result.watching || false;
    } catch (err) {
      console.error('Failed to load watch status:', err);
      isWatching = false;
    } finally {
      loadingWatchStatus = false;
    }
  }

  async function toggleWatch() {
    if (!item?.id) return;

    try {
      if (isWatching) {
        await api.items.removeWatch(item.id);
        isWatching = false;
      } else {
        await api.items.addWatch(item.id);
        isWatching = true;
      }

      // Trigger update for homepage watched items
      hasChanges = true;

    } catch (err) {
      console.error('Failed to toggle watch:', err);
      showError(t('items.failedToUpdateWatchStatus'), err.message || String(err));
    }
  }

  function populateDropdownItems() {
    if (!item) return;

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
        icon: isWatching ? BookmarkCheck : Bookmark,
        title: isWatching ? t('items.unwatchWorkItem') : t('items.watchWorkItem'),
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

    dropdownItems = items;
  }

  // Reactive statement to handle itemId changes for navigation between items
  $effect(() => {
    if (itemId !== previousItemId && !loading) {
      previousItemId = itemId;
      transitioning = true;


      // Small delay to allow fade out animation
      setTimeout(() => {
        loading = true;

        // Clear all state before loading new data to prevent stale data during navigation
        item = null;
        parentHierarchy = [];
        childItems = [];
        availableStatusTransitions = [];
        customFieldDefinitions = [];
        workspaceScreenFields = [];
        itemLinks = [];

        loadData().then(() => {
          populateDropdownItems();
          loading = false;
          transitioning = false;
        }).catch((error) => {
          console.error('Failed to load item data after navigation:', error);
          loading = false;
          transitioning = false;
        });
      }, 150);
    }
  });

  // Load diagrams for the item
  async function loadDiagrams() {
    if (!item?.id) return;

    try {
      loadingDiagrams = true;
      diagrams = await api.getDiagrams(item.id) || [];
    } catch (err) {
      console.error('Failed to load diagrams:', err);
      diagrams = [];
    } finally {
      loadingDiagrams = false;
    }
  }

  // Handle diagram saved event - reload diagrams
  async function handleDiagramSaved() {
    await loadDiagrams();
  }

  onMount(async () => {
    // Initialize timer state from server
    await timer.initialize();

    await loadData();
    document.addEventListener('keydown', handleKeydown);

    // Register context-sensitive commands for this item
    // Pass current timer status to avoid creating reactive dependency
    registerItemContextCommands(!!timer.getCurrentTimer());
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
        showAddLinkForm = true;
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
      registerItemContextCommands(!!timer.getCurrentTimer());
      populateDropdownItems();
    }
  });

  // Re-register commands when active timer status changes
  // This allows us to show/hide the "Start Timer" command based on timer status
  let previousTimerStatus; // Plain variable, not reactive - prevents self-invalidation
  $effect(() => {
    const currentTimerStatus = !!$activeTimer;
    if (item && previousTimerStatus !== undefined && previousTimerStatus !== currentTimerStatus) {
      // Timer status changed, re-register commands with new status
      registerItemContextCommands(currentTimerStatus);
    }
    previousTimerStatus = currentTimerStatus;
  });

  // Rebuild dropdown when watch status changes
  $effect(() => {
    isWatching;
    item && populateDropdownItems();
  });

  // Reload worklogs when timer stops (activeTimer becomes null from non-null)
  let previousActiveTimer; // Plain variable, not reactive - prevents self-invalidation
  $effect(() => {
    const currentTimer = $activeTimer;

    // If we had an active timer and now it's null, and it was for this item, reload worklogs
    if (previousActiveTimer && !currentTimer && previousActiveTimer.item_id === parseInt(itemId)) {
      // Timer was stopped, reload worklogs for this item
      api.time.worklogs.getByItem(itemId).then(worklogs => {
        timeWorklogs = worklogs || [];
      }).catch(err => {
        console.error('Failed to reload worklogs after timer stop:', err);
      });
    }

    // Update previous timer - using plain variable doesn't trigger effect re-run
    previousActiveTimer = currentTimer;
  });

  onDestroy(() => {
    document.removeEventListener('keydown', handleKeydown);
    // Unregister context commands when component is destroyed
    unregisterContextCommands('item-detail');
    // Cleanup timer intervals
    timer.cleanup();
  });

  async function loadData() {
    try {
      loadingLinks = true;
      const [itemData, workspaceData, linkTypesData, linksData, customFieldsData, milestonesData, iterationsData, projectsData, worklogsData, customersData, workItemsData, workspacesData] = await Promise.all([
        api.items.get(itemId),
        api.workspaces.get(workspaceId),
        api.linkTypes.getAll(),
        api.links.getForItem('items', itemId),
        api.customFields.getAll(),
        api.milestones.getAll(),
        api.iterations.getAll({ workspace_id: workspaceId, include_global: true }),
        api.time.projects.getByWorkspace(workspaceId),
        api.time.worklogs.getByItem(itemId),
        api.time.customers.getAll(),
        api.items.getAll({ limit: 100 }),
        api.workspaces.getAll()
      ]);

      item = itemData;
      // Ensure assignee_id is never undefined to prevent binding errors
      if (item.assignee_id === undefined) {
        item.assignee_id = null;
      }
      workspace = workspaceData;
      customFieldDefinitions = customFieldsData || [];

      // Filter milestones by workspace milestone category restrictions
      let allMilestones = milestonesData || [];
      if (workspace && workspace.milestone_categories && workspace.milestone_categories.length > 0) {
        const allowedCategoryIds = workspace.milestone_categories;
        milestones = allMilestones.filter(m => allowedCategoryIds.includes(m.category_id));
      } else {
        milestones = allMilestones;
      }

      iterations = iterationsData || [];
      timeProjects = projectsData || [];
      timeWorklogs = worklogsData || [];
      customers = customersData || [];
      workItems = workItemsData?.items || workItemsData || [];
      workspaces = workspacesData || [];

      // Load priorities based on workspace configuration
      await loadPriorities();

      // Load status transitions and watch status for this item
      loadAvailableStatusTransitions();
      loadWatchStatus();
      linkTypes = linkTypesData;
      
      
      // Process links data - combine incoming and outgoing links
      const allLinks = [];
      if (linksData.outgoing) {
        allLinks.push(...linksData.outgoing);
      }
      if (linksData.incoming) {
        allLinks.push(...linksData.incoming);
      }
      itemLinks = allLinks;
      
      editTitle = item.title;
      editDescription = item.description || '';
      
      // Load parent hierarchy if item has parents, otherwise clear it
      if (item.parent_id) {
        await loadParentHierarchy();
      } else {
        parentHierarchy = [];
      }
      
      // Load child items and hierarchy data
      await loadChildItems();
      await loadItemTypeData();
      
      // Load attachment settings and attachments
      await attachmentManager.loadSettings();
      if (attachmentManager.isEnabled()) {
        await attachmentManager.load();
      }

      // Load diagrams (always load, not dependent on attachment settings)
      await loadDiagrams();

      // Load workspace screen configuration
      await loadWorkspaceScreenFields();
      
      loading = false;
      loadingLinks = false;
    } catch (err) {
      console.error('Failed to load item or workspace:', err);
      error = err.message || 'Failed to load data';
      loading = false;
      loadingLinks = false;
    }
  }

  // Reactive search for work items when searchQuery changes
  let searchTimeout;
  $effect(() => {
    const trimmedQuery = (searchQuery || '').trim();
    const searchType = Number(addLinkData.link_type_id) === TEST_LINK_TYPE_ID ? 'test_case' : 'item';

    if (trimmedQuery.length >= 2) {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(async () => {
        try {
          searching = true;
          const results = await api.links.search(trimmedQuery, searchType, 10);
          const items = Array.isArray(results) ? results : [];
          searchResults = searchType === 'item'
            ? items.filter(item => item.id !== parseInt(itemId))
            : items;
        } catch (error) {
          console.error('Search failed:', error);
          searchResults = [];
        } finally {
          searching = false;
        }
      }, 300);
    } else {
      clearTimeout(searchTimeout);
      searchResults = [];
      searching = false;
    }
  });

  // Reset incompatible selections if link type changes
  $effect(() => {
    const isTestLink = Number(addLinkData.link_type_id) === TEST_LINK_TYPE_ID;
    if (!isTestLink && addLinkData.target_type === 'test_case') {
      addLinkData.target_id = null;
      addLinkData.target_title = '';
      addLinkData.target_type = 'item';
    }
  });

  // Parent hierarchy function (from original)
  async function loadParentHierarchy() {
    try {
      
      // Get all items in workspace
      const response = await api.items.getAll({ workspace_id: workspaceId });
      
      // Handle different response formats
      let allItems = [];
      if (Array.isArray(response)) {
        allItems = response;
      } else if (response && Array.isArray(response.items)) {
        allItems = response.items;
      } else if (response && response.data && Array.isArray(response.data)) {
        allItems = response.data;
      } else {
        console.warn('Unexpected response format from api.items.getAll:', response);
        parentHierarchy = [];
        return;
      }
      
      
      // Build parent hierarchy using the new ancestor API
      try {
        const ancestors = await api.items.getAncestors(item.id);
        
        // Load item types for parent items to show icons
        try {
          const itemTypesData = await api.itemTypes.getAll();
          parentHierarchy = ancestors.map(ancestor => {
            if (ancestor.item_type_id) {
              const itemType = itemTypesData.find(type => type.id === ancestor.item_type_id);
              return { ...ancestor, itemType };
            }
            return ancestor;
          });
        } catch (err) {
          console.warn('Failed to load item types for parent hierarchy:', err);
          // Use ancestors without item type information
          parentHierarchy = ancestors;
        }
        
      } catch (error) {
        console.error('Failed to load ancestors:', error);
        parentHierarchy = [];
      }
      
    } catch (err) {
      console.error('Failed to load parent hierarchy:', err);
      parentHierarchy = [];
    }
  }

  // Load workspace screen fields configuration
  async function loadWorkspaceScreenFields() {
    try {
      let screenId = null;

      // Try to get screen from configuration set if assigned
      if (workspace?.configuration_set_id) {
        const configSet = await api.configurationSets.get(workspace.configuration_set_id);
        screenId = configSet?.edit_screen_id || configSet?.create_screen_id || configSet?.view_screen_id;
      }

      // Fallback to default screen (ID 1) if no configuration set or no screens assigned
      if (!screenId) {
        screenId = 1;
      }

      // Get the full screen object (includes both custom and system fields)
      const screen = await api.screens.get(screenId);
      const screenFields = screen?.fields || [];

      // Separate custom fields so we can render them in configured order
      workspaceScreenFields = screenFields.filter(field => field.field_type === 'custom');

      // Determine which system fields should be shown.
      // Screens API currently stores system selections in the main fields payload,
      // so derive them here and fall back to legacy system_fields if present.
      const configuredSystemFields = screenFields
        .filter(field => field.field_type === 'system')
        .map(field => field.field_identifier);

      if (configuredSystemFields.length > 0) {
        workspaceScreenSystemFields = configuredSystemFields;
      } else {
        workspaceScreenSystemFields = screen?.system_fields || [];
      }

    } catch (err) {
      console.error('Failed to load workspace screen fields:', err);
      workspaceScreenFields = [];
      workspaceScreenSystemFields = [];
    }
  }

  // Child items and hierarchy functions (from original)
  async function loadChildItems() {
    try {
      loadingChildItems = true;
      const response = await api.items.getChildren(itemId);
      
      // Handle different response formats
      if (Array.isArray(response)) {
        childItems = response;
      } else if (response && Array.isArray(response.items)) {
        childItems = response.items;
      } else if (response && response.data && Array.isArray(response.data)) {
        childItems = response.data;
      } else {
        childItems = [];
      }
    } catch (err) {
      console.error('[loadChildItems] Failed to load child items:', err);
      childItems = [];
    } finally {
      loadingChildItems = false;
    }
  }

  async function loadItemTypeData() {
    try {
      // Load current item's type and hierarchy levels
      const [itemTypesData, hierarchyLevels] = await Promise.all([
        api.itemTypes.getAll(),
        api.hierarchyLevels.getAll()
      ]);
      
      itemTypes = itemTypesData || [];
      
      if (item.item_type_id) {
        currentItemType = itemTypes.find(type => type.id === item.item_type_id);
        if (currentItemType) {
          currentHierarchyLevel = hierarchyLevels.find(level => level.level === currentItemType.hierarchy_level);
        }
      }
      
      // Find available sub-issue types (next level down)
      if (currentItemType && currentHierarchyLevel) {
        const nextLevel = currentHierarchyLevel.level + 1;
        availableSubIssueTypes = itemTypes.filter(type => type.hierarchy_level === nextLevel);
      } else {
        availableSubIssueTypes = [];
      }
      
    } catch (err) {
      console.error('Failed to load item type data:', err);
      currentItemType = null;
      currentHierarchyLevel = null;
      availableSubIssueTypes = [];
    }
  }

  // Sub-issue creation function (from original)
  function startCreateSubIssue() {

    if (availableSubIssueTypes.length === 0) {
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
          parentId: item.id, 
          parentTitle: item.title,
          availableItemTypes: availableSubIssueTypes
        } 
      }));
      
      // Open the modal (this will load workspaces)
      window.dispatchEvent(new CustomEvent('open-create-modal'));
      
      // After modal is open and workspaces are loaded, set the workspace
      setTimeout(() => {
        window.dispatchEvent(new CustomEvent('set-create-workspace', {
          detail: {
            workspaceId: workspaceId,
            workspaceName: workspace?.name
          }
        }));
      }, 200);
    }, 150);
  }
</script>

{#snippet contentSnippet()}
  <ItemDetailContent
    {loading}
    {error}
    {item}
    {workspace}
    {isModal}
    {parentHierarchy}
    {currentItemType}
    {currentHierarchyLevel}
    {iconMap}
    {workspaceId}
    bind:editingTitle
    bind:editTitle
    {saving}
    {dropdownItems}
    statusOptions={statusOptions}
    bind:editingDescription
    bind:editDescription
    {itemLinks}
    {loadingLinks}
    {availableSubIssueTypes}
    {childItems}
    {loadingChildItems}
    bind:showAddLinkForm
    bind:addLinkData
    linkTypes={filteredLinkTypes}
    bind:searchResults
    bind:searchQuery
    {searching}
    {itemTypes}
    {tab}
    {moduleSettings}
    {timeWorklogs}
    {timeProjects}
    activeTimer={$activeTimer}
    {editingStatus}
    {editingPriority}
    {editingDueDate}
    {editingProject}
    {editingAssignee}
    {editingMilestone}
    {editingIteration}
    {editingCustomFields}
    {editCustomFieldValues}
    {workspaceScreenFields}
    {workspaceScreenSystemFields}
    {customFieldDefinitions}
    {milestones}
    {iterations}
    {priorities}
    attachments={attachmentManager.attachments}
    attachmentPagination={attachmentManager.pagination}
    attachmentSettings={attachmentManager.settings}
    {diagrams}
    {loadingDiagrams}
    on:navigate={handleNavigate}
    on:go-back={handleGoBack}
    on:copy-key={handleCopyKey}
    on:save-field={handleSaveField}
    on:cancel-edit={handleCancelEdit}
    on:switch-tab={handleSwitchTab}
    on:create-sub-issue={handleCreateSubIssue}
    on:cancel-add-link={handleCancelAddLink}
    on:add-link={handleAddLink}
    on:select-item={handleSelectItem}
    on:remove-link={handleRemoveLink}
    on:view-test-case={handleViewTestCase}
    on:start-editing-assignee={handleStartEditingAssignee}
    on:start-editing-milestone={handleStartEditingMilestone}
    on:start-editing-iteration={handleStartEditingIteration}
    on:start-editing-priority={handleStartEditingPriority}
    on:start-editing-due-date={handleStartEditingDueDate}
    on:start-editing-status={handleStartEditingStatus}
    on:start-editing-project={handleStartEditingProject}
    on:start-editing-custom-field={handleStartEditingCustomField}
    on:start-timer={handleStartTimer}
    on:log-time={handleLogTime}
    on:parent-changed={handleParentChanged}
    on:attachment-upload={attachmentManager.handleUpload}
    on:attachment-upload-files={attachmentManager.uploadFiles}
    on:attachment-delete={attachmentManager.handleDelete}
    on:attachment-page-change={attachmentManager.handlePageChange}
    on:attachment-page-size-change={attachmentManager.handlePageSizeChange}
    on:diagram-saved={handleDiagramSaved}
    on:close={closeModal}
  />
{/snippet}

{#if isModal}
  <Modal
    isOpen={true}
    maxWidth={isFullscreen ? 'max-w-[95vw]' : 'max-w-[80vw]'}
    onclose={closeModal}
  >
    <div
      bind:this={modalElement}
      class="flex flex-col relative w-full {isFullscreen ? 'h-[95vh]' : 'max-h-[90vh]'}"
    >
      {#if showTestCaseModal}
        <TestCaseViewModal
          embedded={true}
          bind:isOpen={showTestCaseModal}
          testCaseId={selectedTestCaseId}
          on:close={handleCloseTestCaseModal}
        />
      {:else}
        {#if item && workspace}
          <!-- Modal Header -->
          <div class="flex items-center justify-between p-4 border-b" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
            <div class="flex items-center gap-3">
              <h1 class="text-lg font-semibold" style="color: var(--ds-text);">{t('items.workItemDetails')}</h1>
              <span class="px-2 py-1 text-sm font-mono rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {workspace.key}-{item.workspace_item_number}
              </span>
            </div>
            <div class="flex items-center gap-2">
              <button
                onclick={openFullDetails}
                class="inline-flex items-center gap-2 px-3 py-1.5 bg-[var(--ds-interactive)] text-white rounded hover:bg-[var(--ds-interactive-hovered)] transition-colors text-sm font-medium"
                title="Open full details (F)"
              >
                <ExternalLink class="w-4 h-4" />
                {t('items.fullDetails')}
                <span class="ml-1 px-1.5 py-0.5 bg-[var(--ds-interactive-hovered)] bg-opacity-50 rounded text-xs font-mono">F</span>
              </button>
              <button
                onclick={toggleFullscreen}
                class="p-2 rounded transition-colors"
                style="color: var(--ds-text-subtle);"
                onmouseenter={(e) => { e.currentTarget.style.color = 'var(--ds-text)'; e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; }}
                onmouseleave={(e) => { e.currentTarget.style.color = 'var(--ds-text-subtle)'; e.currentTarget.style.backgroundColor = ''; }}
                title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
              >
                {#if isFullscreen}
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
            class:opacity-30={transitioning}
            class:opacity-100={!transitioning}
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
      class:opacity-30={transitioning}
      class:opacity-100={!transitioning}
    >
      {@render contentSnippet()}
    </div>
  {/key}
</div>

{#if !isModal}
  <TestCaseViewModal
    bind:isOpen={showTestCaseModal}
    testCaseId={selectedTestCaseId}
    on:close={handleCloseTestCaseModal}
  />
{/if}

<!-- Time Log Modal -->
{#if showTimeLogModal}
  <TimeLogModal
    defaultProjectId={getDefaultProjectForTimeLogging()}
    defaultItemId={parseInt(itemId)}
    projects={timeProjects}
    {customers}
    {workItems}
    {workspaces}
    showProjectField={true}
    showWorkItemField={false}
    on:save={handleModalSave}
    on:cancel={handleModalCancel}
  />
{/if}
{/if}


