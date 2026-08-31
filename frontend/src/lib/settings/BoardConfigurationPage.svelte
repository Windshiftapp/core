<script>
  import { onMount, onDestroy } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { navigate } from '../router.js';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { CARD_SELECTABLE_FIELDS } from '../stores/fieldConfig.js';
  import { confirm } from '../composables/useConfirm.js';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import { collectionStore } from '../stores/collectionContext.svelte.js';
  import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
  import { loadBoardConfigurationPageData } from './boardConfigurationData.js';
  import { Plus, GripVertical, X, Grip, Settings } from '@lucide/svelte';
  import { useGradientStyles, loadWorkspaceGradient } from '../stores/workspaceGradient.svelte.js';
  import ViewHeader from '../layout/ViewHeader.svelte';
  import StaticViewBackground from '../layout/StaticViewBackground.svelte';
  import Button from '../components/Button.svelte';
  import Panel from '../components/Panel.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import DropIndicator from '../layout/DropIndicator.svelte';
  import CollectionViewSwitcher from '../features/collections/CollectionViewSwitcher.svelte';
  import { objectDisplayName } from '../utils/systemLabels.js';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let currentCollectionName = $state('Default');
  let isPublicCollection = $state(false);
  let publicSlug = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let boardConfig = $state(null);
  let columns = $state([]);
  let statuses = $state([]);
  let hasChanges = $state(false);
  let activeTab = $state('columns');
  let backlogStatusIDs = $state([]);
  let cardFields = $state([]);
  let showRightmostColumnLast50 = $state(false);
  let trimCompletedItemsByAge = $state(false);
  let completedItemRetentionDays = $state(30);
  let completedItemRetentionError = $state('');
  let customFieldDefinitions = $state([]);

  const PUBLIC_BOARD_CARD_FIELDS = new Set([
    'key',
    'title',
    'status',
    'priority',
    'assignee',
    'item_type',
    'story_points',
    'due_date',
    'labels',
  ]);

  // DnD state
  let statusDragState = $state(new Map());
  let columnDragState = $state(new Map());
  let cardFieldDragState = $state(new Map());
  let statusSearchQuery = $state('');
  let setupCleanups = [];
  let setupTimeout;

  const styles = useGradientStyles();

  // The workspace-default board config (collectionId == null) writes a row
  // that applies to every viewer of the workspace, so the backend requires
  // `workspace.admin` on those mutations. Collection-specific configs are
  // still gated by collection ownership on the backend — the frontend trusts
  // the backend on that path.
  let canConfigure = $derived(
    collectionId != null || workspacePermissions.canAdminWorkspace(workspaceId)
  );

  // Custom-field administration is global and system-administrator-only. A
  // workspace administrator who is not a system administrator must not be
  // shown an unusable privileged action.
  let isSystemAdmin = $derived(
    /** @type {any} */ (workspacePermissions).isSystemAdmin === true
  );
  let isNonSystemWorkspaceAdmin = $derived(
    !isSystemAdmin && workspacePermissions.canAdminWorkspace(workspaceId)
  );
  let hasCustomFields = $derived(customFieldDefinitions.length > 0);
  let boardConfigReturnTo = $derived(
    collectionId != null && workspaceId != null
      ? `/workspaces/${workspaceId}/collections/${collectionId}/board/configure`
      : collectionId != null
        ? `/collections/${collectionId}/board/configure`
        : workspaceId != null
          ? `/workspaces/${workspaceId}/board/configure`
          : '/'
  );

  async function goCustomFields(create) {
    if (hasChanges) {
      const confirmed = await confirm({
        title: t('common.discardChanges'),
        message: t('dialogs.confirmations.discardChanges'),
        confirmText: t('common.discard'),
        cancelText: t('common.cancel'),
      });
      if (!confirmed) return;
    }
    const returnTo = encodeURIComponent(boardConfigReturnTo);
    navigate(`/admin/custom-fields?returnTo=${returnTo}${create ? '&action=create' : ''}`);
  }

  // Derived: set of all assigned status IDs
  let assignedStatusIds = $derived(new Set(columns.flatMap(c => c.status_ids)));

  // Derived: available (unassigned) statuses, filtered by search
  let availableStatuses = $derived.by(() => {
    return statuses.filter(s => {
      if (assignedStatusIds.has(s.id)) return false;
      if (!statusSearchQuery.trim()) return true;
      return s.name.toLowerCase().includes(statusSearchQuery.toLowerCase());
    });
  });

  onMount(async () => {
    await Promise.all([
      workspaceId ? loadWorkspaceGradient(workspaceId) : Promise.resolve(),
      loadData()
    ]);
    loading = false;
  });

  onDestroy(() => {
    cleanupDragAndDrop();
  });

  // Re-setup DnD when columns or statuses change
  $effect(() => {
    // Track dependencies
    columns;
    statuses;
    cardFields;
    activeTab;
    canConfigure;
    statusSearchQuery;
    if (!loading && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => setupDragAndDrop(), 50);
    }
  });

  async function loadData() {
    try {
      const data = await loadBoardConfigurationPageData(
        api,
        workspaceDataStore,
        workspaceId,
        collectionId
      );
      workspace = data.workspace;
      statuses = data.statuses;
      customFieldDefinitions = data.customFieldDefinitions;
      currentCollectionName = data.collection?.name || 'Default';
      isPublicCollection = data.collection?.is_public === true;
      publicSlug = isPublicCollection && data.collection?.public_slug
        ? data.collection.public_slug
        : null;
      boardConfig = data.boardConfiguration;

      if (boardConfig) {
        columns = (boardConfig.columns || []).map(col => ({
          ...col,
          status_ids: col.status_ids || []
        }));
        backlogStatusIDs = boardConfig.backlog_status_ids?.length > 0
          ? boardConfig.backlog_status_ids
          : statuses.filter(s => !s.is_default && !s.is_completed).map(s => s.id);
        cardFields = (boardConfig.card_fields || []).filter(
          field => !isPublicCollection ||
            (field.field_type === 'system' && PUBLIC_BOARD_CARD_FIELDS.has(field.field_identifier))
        );
        showRightmostColumnLast50 = Boolean(boardConfig.show_rightmost_column_last_50);
        const savedRetentionDays = Number(boardConfig.completed_item_retention_days);
        trimCompletedItemsByAge = !showRightmostColumnLast50 && Number.isInteger(savedRetentionDays) && savedRetentionDays > 0;
        completedItemRetentionDays = trimCompletedItemsByAge ? savedRetentionDays : 30;
        completedItemRetentionError = '';
      } else {
        columns = [];
        backlogStatusIDs = statuses.filter(s => !s.is_default && !s.is_completed).map(s => s.id);
        cardFields = [];
        showRightmostColumnLast50 = false;
        trimCompletedItemsByAge = false;
        completedItemRetentionDays = 30;
        completedItemRetentionError = '';
      }
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  // --- DnD Setup/Cleanup ---

  function cleanupDragAndDrop() {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];
    statusDragState = new Map();
    columnDragState = new Map();
    cardFieldDragState = new Map();
  }

  function setupDragAndDrop() {
    cleanupDragAndDrop();

    // --- Available statuses (left panel) ---
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-available-status]')).forEach(element => {
      const statusData = JSON.parse(element.dataset.availableStatus);

      const cleanup = draggable({
        element,
        getInitialData: () => ({ status: statusData, type: 'available-status' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => { element.style.opacity = ''; }
      });

      setupCleanups.push(cleanup);
    });

    // --- Status items inside columns ---
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-column-status]')).forEach(element => {
      const colIndex = parseInt(element.dataset.colIndex);
      const statusIndex = parseInt(element.dataset.statusIndex);
      const statusId = columns[colIndex]?.status_ids?.[statusIndex];
      if (statusId == null) return;

      statusDragState.set(`${colIndex}-${statusId}`, { closestEdge: null });

      const dragHandle = element.querySelector('.cursor-grab');
      const draggableCleanup = draggable({
        element,
        dragHandle: dragHandle || element,
        getInitialData: () => ({ statusId, colIndex, statusIndex, type: 'column-status' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => {
          element.style.opacity = '';
          statusDragState.forEach((_, key) => {
            statusDragState.set(key, { closestEdge: null });
          });
          statusDragState = new Map(statusDragState);
        }
      });

      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'column-status' && data.colIndex === colIndex && data.statusIndex === statusIndex) return false;
          return data.type === 'available-status' || data.type === 'column-status';
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, { input, element, allowedEdges: ['top', 'bottom'] });
        },
        onDragEnter: ({ self }) => {
          const closestEdge = extractClosestEdge(self.data);
          statusDragState.set(`${colIndex}-${statusId}`, { closestEdge });
          statusDragState = new Map(statusDragState);
        },
        onDragLeave: () => {
          statusDragState.set(`${colIndex}-${statusId}`, { closestEdge: null });
          statusDragState = new Map(statusDragState);
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          const data = source.data;

          if (data.type === 'available-status') {
            addStatusToColumnAtPosition(data.status, colIndex, statusIndex, closestEdge);
          } else if (data.type === 'column-status') {
            moveStatusBetweenColumns(data.statusId, data.colIndex, colIndex, statusIndex, closestEdge);
          }

          statusDragState.set(`${colIndex}-${statusId}`, { closestEdge: null });
          statusDragState = new Map(statusDragState);
        }
      });

      setupCleanups.push(() => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });

    // --- Column drop zones (empty area at bottom of each column) ---
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-column-drop-zone]')).forEach(element => {
      const colIndex = parseInt(element.dataset.columnDropZone);

      const cleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          return data.type === 'available-status' || data.type === 'column-status';
        },
        onDragEnter: () => { element.style.borderColor = 'var(--ds-interactive)'; element.style.background = 'var(--ds-surface-hovered)'; },
        onDragLeave: () => { element.style.borderColor = 'var(--ds-border)'; element.style.background = ''; },
        onDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'available-status') {
            addStatusToColumn(data.status, colIndex);
          } else if (data.type === 'column-status') {
            moveStatusBetweenColumns(data.statusId, data.colIndex, colIndex, columns[colIndex].status_ids.length, 'bottom');
          }
          element.style.borderColor = 'var(--ds-border)';
          element.style.background = '';
        }
      });

      setupCleanups.push(cleanup);
    });

    // --- Column headers (for column reordering) ---
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-board-column]')).forEach(element => {
      const colIndex = parseInt(element.dataset.boardColumn);

      columnDragState.set(colIndex, { closestEdge: null });

      const dragHandle = element.querySelector('[data-column-drag-handle]');
      const draggableCleanup = draggable({
        element,
        dragHandle: dragHandle || element,
        getInitialData: () => ({ colIndex, type: 'board-column' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => {
          element.style.opacity = '';
          columnDragState.forEach((_, key) => {
            columnDragState.set(key, { closestEdge: null });
          });
          columnDragState = new Map(columnDragState);
        }
      });

      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'board-column' && data.colIndex === colIndex) return false;
          return data.type === 'board-column';
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, { input, element, allowedEdges: ['left', 'right'] });
        },
        onDragEnter: ({ self }) => {
          const closestEdge = extractClosestEdge(self.data);
          columnDragState.set(colIndex, { closestEdge });
          columnDragState = new Map(columnDragState);
        },
        onDragLeave: () => {
          columnDragState.set(colIndex, { closestEdge: null });
          columnDragState = new Map(columnDragState);
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          if (source.data.type === 'board-column') {
            reorderColumn(source.data.colIndex, colIndex, closestEdge);
          }
          columnDragState.set(colIndex, { closestEdge: null });
          columnDragState = new Map(columnDragState);
        }
      });

      setupCleanups.push(() => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });

    // --- Card fields (for card-field reordering) ---
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-card-field]')).forEach(element => {
      if (!canConfigure) return;

      const fieldIndex = parseInt(element.dataset.cardFieldIndex);
      const fieldIdentifier = element.dataset.cardFieldIdentifier;
      if (!fieldIdentifier || Number.isNaN(fieldIndex)) return;

      cardFieldDragState.set(fieldIdentifier, { closestEdge: null });

      const dragHandle = element.querySelector('[data-card-field-drag-handle]');
      const draggableCleanup = draggable({
        element,
        dragHandle: dragHandle || element,
        getInitialData: () => ({ fieldIndex, fieldIdentifier, type: 'card-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => {
          element.style.opacity = '';
          cardFieldDragState.forEach((_, key) => {
            cardFieldDragState.set(key, { closestEdge: null });
          });
          cardFieldDragState = new Map(cardFieldDragState);
        }
      });

      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'card-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'card-field';
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, { input, element, allowedEdges: ['top', 'bottom'] });
        },
        onDragEnter: ({ self }) => {
          cardFieldDragState.set(fieldIdentifier, { closestEdge: extractClosestEdge(self.data) });
          cardFieldDragState = new Map(cardFieldDragState);
        },
        onDragLeave: () => {
          cardFieldDragState.set(fieldIdentifier, { closestEdge: null });
          cardFieldDragState = new Map(cardFieldDragState);
        },
        onDrop: ({ self, source }) => {
          if (source.data.type === 'card-field') {
            reorderCardFields(source.data.fieldIndex, fieldIndex, extractClosestEdge(self.data));
          }
          cardFieldDragState.set(fieldIdentifier, { closestEdge: null });
          cardFieldDragState = new Map(cardFieldDragState);
        }
      });

      setupCleanups.push(() => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });
  }

  // --- Status manipulation functions ---

  function addStatusToColumn(status, columnIndex) {
    const col = columns[columnIndex];
    if (col.status_ids.includes(status.id)) return;
    col.status_ids = [...col.status_ids, status.id];
    columns = [...columns];
    hasChanges = true;
  }

  function addStatusToColumnAtPosition(status, targetColumnIndex, targetStatusIndex, closestEdge) {
    const col = columns[targetColumnIndex];
    if (col.status_ids.includes(status.id)) return;
    const insertIndex = closestEdge === 'bottom' ? targetStatusIndex + 1 : targetStatusIndex;
    const newIds = [...col.status_ids];
    newIds.splice(insertIndex, 0, status.id);
    col.status_ids = newIds;
    columns = [...columns];
    hasChanges = true;
  }

  function moveStatusBetweenColumns(statusId, fromColumnIndex, toColumnIndex, targetStatusIndex, closestEdge) {
    if (fromColumnIndex === toColumnIndex) {
      // Reorder within same column
      const col = columns[fromColumnIndex];
      const fromIndex = col.status_ids.indexOf(statusId);
      if (fromIndex === -1) return;

      const insertIndex = closestEdge === 'bottom' ? targetStatusIndex + 1 : targetStatusIndex;
      const adjustedIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;

      const newIds = [...col.status_ids];
      newIds.splice(fromIndex, 1);
      newIds.splice(adjustedIndex, 0, statusId);
      col.status_ids = newIds;
    } else {
      // Move between columns
      const fromCol = columns[fromColumnIndex];
      const toCol = columns[toColumnIndex];
      fromCol.status_ids = fromCol.status_ids.filter(id => id !== statusId);

      const insertIndex = closestEdge === 'bottom' ? targetStatusIndex + 1 : targetStatusIndex;
      const clampedIndex = Math.min(insertIndex, toCol.status_ids.length);
      const newIds = [...toCol.status_ids];
      newIds.splice(clampedIndex, 0, statusId);
      toCol.status_ids = newIds;
    }
    columns = [...columns];
    hasChanges = true;
  }

  function removeStatusFromColumn(columnIndex, statusId) {
    columns[columnIndex].status_ids = columns[columnIndex].status_ids.filter(id => id !== statusId);
    columns = [...columns];
    hasChanges = true;
  }

  function reorderColumn(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;
    const insertIndex = closestEdge === 'right' ? toIndex + 1 : toIndex;
    const adjustedIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;

    const newColumns = [...columns];
    const [moved] = newColumns.splice(fromIndex, 1);
    newColumns.splice(adjustedIndex, 0, moved);
    columns = newColumns.map((col, i) => ({ ...col, display_order: i }));

    hasChanges = true;
  }

  // --- Column CRUD ---

  function addColumn() {
    const newColumn = {
      name: `${t('settings.boardConfig.columns')} ${columns.length + 1}`,
      display_order: columns.length,
      wip_limit: null,
      color: '#f3f4f6',
      status_ids: []
    };
    columns = [...columns, newColumn];
    hasChanges = true;
  }

  function removeColumn(index) {
    columns = columns.filter((_, i) => i !== index);
    columns = columns.map((col, i) => ({ ...col, display_order: i }));
    hasChanges = true;
  }

  function updateColumnName(index, name) {
    columns[index].name = name;
    columns = [...columns];
    hasChanges = true;
  }

  function updateWIPLimit(index, limit) {
    columns[index].wip_limit = limit === '' || limit === null ? null : parseInt(limit);
    columns = [...columns];
    hasChanges = true;
  }

  function getStatusName(statusId) {
    const s = statuses.find(s => s.id === statusId);
    return s ? objectDisplayName(s, 'status') : statusId;
  }

  function getStatusColor(statusId) {
    const s = statuses.find(s => s.id === statusId);
    return s?.category_color || '#6b7280';
  }

  // --- Backlog ---

  function toggleBacklogStatus(statusId) {
    const index = backlogStatusIDs.indexOf(statusId);
    if (index >= 0) {
      backlogStatusIDs = backlogStatusIDs.filter(id => id !== statusId);
    } else {
      backlogStatusIDs = [...backlogStatusIDs, statusId];
    }
    hasChanges = true;
  }

  // --- Board display options ---

  function setShowRightmostColumnLast50(value) {
    showRightmostColumnLast50 = value;
    if (value) {
      trimCompletedItemsByAge = false;
      completedItemRetentionError = '';
    }
    hasChanges = true;
  }

  function setTrimCompletedItemsByAge(value) {
    trimCompletedItemsByAge = value;
    if (value) showRightmostColumnLast50 = false;
    completedItemRetentionError = '';
    hasChanges = true;
  }

  function updateCompletedItemRetentionDays(value) {
    completedItemRetentionDays = value;
    completedItemRetentionError = '';
    hasChanges = true;
  }

  // --- Card Fields ---

  let systemFieldOptions = $derived(
    CARD_SELECTABLE_FIELDS
      .filter(f => !isPublicCollection || PUBLIC_BOARD_CARD_FIELDS.has(f.identifier))
      .map(f => ({
        identifier: f.identifier,
        label: t(`settings.boardConfig.systemFieldNames.${f.identifier}`)
      }))
  );

  let selectedCardFieldIds = $derived(new Set(cardFields.map(f => f.field_identifier)));

  let availableSystemFields = $derived(
    systemFieldOptions.filter(f => !selectedCardFieldIds.has(f.identifier))
  );

  let availableCustomFields = $derived(
    isPublicCollection
      ? []
      : (customFieldDefinitions || []).filter(f => !selectedCardFieldIds.has(`custom_field_${f.id}`))
  );

  function addCardField(identifier, fieldType) {
    cardFields = [...cardFields, {
      field_identifier: identifier,
      field_type: fieldType,
      display_order: cardFields.length,
      width: 0
    }];
    hasChanges = true;
  }

  function removeCardField(identifier) {
    cardFields = cardFields.filter(f => f.field_identifier !== identifier);
    hasChanges = true;
  }

  function reorderCardFields(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;
    const insertIndex = closestEdge === 'bottom' ? toIndex + 1 : toIndex;
    const adjustedIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;
    if (fromIndex === adjustedIndex) return;

    const newFields = [...cardFields];
    const [moved] = newFields.splice(fromIndex, 1);
    newFields.splice(adjustedIndex, 0, moved);
    cardFields = newFields.map((f, i) => ({ ...f, display_order: i }));
    hasChanges = true;
  }

  function handleCardFieldReorderKeydown(event, index) {
    if (event.key === 'ArrowUp' && index > 0) {
      event.preventDefault();
      reorderCardFields(index, index - 1, 'top');
    } else if (event.key === 'ArrowDown' && index < cardFields.length - 1) {
      event.preventDefault();
      reorderCardFields(index, index + 1, 'bottom');
    }
  }

  function getCardFieldLabel(field) {
    if (field.field_type === 'system') {
      return t(`settings.boardConfig.systemFieldNames.${field.field_identifier}`);
    }
    // Custom field
    const cfId = field.field_identifier.replace('custom_field_', '');
    const cf = customFieldDefinitions.find(d => String(d.id) === cfId);
    return cf?.name || field.field_identifier;
  }

  // --- Save / Reset / Cancel ---

  async function saveConfiguration() {
    const retentionDays = Number(completedItemRetentionDays);
    if (
      trimCompletedItemsByAge &&
      (!Number.isInteger(retentionDays) || retentionDays < 1 || retentionDays > 3650)
    ) {
      completedItemRetentionError = t('settings.boardConfig.retentionRangeError');
      return;
    }

    saving = true;
    try {
      const payload = {
        columns: columns.map((col, index) => ({
          id: col.id || null,
          name: col.name,
          display_order: index,
          wip_limit: col.wip_limit,
          color: col.color,
          status_ids: col.status_ids
        })),
        backlog_status_ids: backlogStatusIDs,
        list_columns: boardConfig?.list_columns || [],
        roadmap_config: boardConfig?.roadmap_config || null,
        card_fields: cardFields.map((f, i) => ({
          field_identifier: f.field_identifier,
          field_type: f.field_type,
          display_order: i,
          width: 0
        })),
        show_rightmost_column_last_50: showRightmostColumnLast50,
        completed_item_retention_days: trimCompletedItemsByAge ? retentionDays : null
      };

      if (boardConfig && boardConfig.id) {
        await api.collections.updateBoardConfiguration(collectionId, boardConfig.id, payload);
      } else {
        const newConfig = await api.collections.createBoardConfiguration(collectionId, workspaceId, payload);
        boardConfig = newConfig;
      }
      collectionStore.invalidateBoardConfiguration(workspaceId, collectionId);

      hasChanges = false;
      goToBoard();
    } catch (error) {
      console.error('Failed to save board configuration:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message }));
    } finally {
      saving = false;
    }
  }

  async function resetToDefault() {
    const confirmed = await confirm({
      title: t('common.reset'),
      message: t('dialogs.confirmations.resetBoardConfig'),
      confirmText: t('common.reset'),
      cancelText: t('common.cancel'),
      variant: 'warning'
    });
    if (!confirmed) return;

    if (boardConfig) {
      try {
        await api.collections.deleteBoardConfiguration(collectionId, boardConfig.id);
        boardConfig = null;
        columns = [];
        backlogStatusIDs = [];
        cardFields = [];
        showRightmostColumnLast50 = false;
        trimCompletedItemsByAge = false;
        completedItemRetentionDays = 30;
        completedItemRetentionError = '';
        hasChanges = false;
        goToBoard();
      } catch (error) {
        console.error('Failed to delete board configuration:', error);
        errorToast(t('dialogs.alerts.failedToResetConfig', { error: error.message }));
      }
    } else {
      columns = [];
      backlogStatusIDs = [];
      cardFields = [];
      showRightmostColumnLast50 = false;
      trimCompletedItemsByAge = false;
      completedItemRetentionDays = 30;
      completedItemRetentionError = '';
      hasChanges = false;
    }
  }

  async function cancelChanges() {
    if (hasChanges) {
      const confirmed = await confirm({
        title: t('common.discardChanges'),
        message: t('dialogs.confirmations.discardChanges'),
        confirmText: t('common.discard'),
        cancelText: t('common.cancel'),
        variant: 'warning'
      });
      if (!confirmed) return;
    }
    goToBoard();
  }

  function goToBoard() {
    const url = workspaceId
      ? (collectionId ? `/workspaces/${workspaceId}/collections/${collectionId}/board` : `/workspaces/${workspaceId}/board`)
      : `/collections/${collectionId}/board`;
    navigate(url);
  }

</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('common.loading')}</div>
  </div>
{:else if workspace || !workspaceId}
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
  >
      <div class="space-y-6">
        <!-- Header with view tabs -->
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName === 'Default' ? t('common.default') : currentCollectionName}
          viewName={t('settings.boardConfig.configureBoard')}
          itemCount={columns.length}
        >
          {#snippet actions()}
            <CollectionViewSwitcher
              {workspaceId}
              {collectionId}
              activeView="configure"
              {publicSlug}
            />
          {/snippet}
        </ViewHeader>

        <!-- Configuration content in raised box -->
        <Panel padding="spacious" class="w-full" style="border-color: {styles.hasCustomBackground ? 'transparent' : 'var(--ds-border)'};">
        <!-- Tab Navigation -->
        <div class="border-b" style="border-color: var(--ds-border);">
          <div class="flex gap-4">
            <button
              class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
              class:border-transparent={activeTab !== 'columns'}
              style:color={activeTab === 'columns' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}
              style:border-color={activeTab === 'columns' ? 'var(--ds-interactive)' : 'transparent'}
              onclick={() => activeTab = 'columns'}
            >
              {t('settings.boardConfig.columns')}
            </button>
            <button
              class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
              class:border-transparent={activeTab !== 'backlog'}
              style:color={activeTab === 'backlog' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}
              style:border-color={activeTab === 'backlog' ? 'var(--ds-interactive)' : 'transparent'}
              onclick={() => activeTab = 'backlog'}
            >
              {t('settings.boardConfig.backlog')}
            </button>
            <button
              data-testid="board-config-card-fields-tab"
              class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
              class:border-transparent={activeTab !== 'cardFields'}
              style:color={activeTab === 'cardFields' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}
              style:border-color={activeTab === 'cardFields' ? 'var(--ds-interactive)' : 'transparent'}
              onclick={() => activeTab = 'cardFields'}
            >
              {t('settings.boardConfig.cardFields')}
            </button>
          </div>
        </div>

        <!-- Columns Tab -->
        {#if activeTab === 'columns'}
        <div class="mt-4 flex flex-col gap-4 rounded border p-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <Checkbox
            bind:checked={showRightmostColumnLast50}
            onchange={setShowRightmostColumnLast50}
            disabled={!canConfigure}
            dataTestid="board-rightmost-limit-enabled"
            label={t('settings.boardConfig.latestFiftyRightmost')}
            hint={t('settings.boardConfig.latestFiftyRightmostHint')}
            size="small"
          />
          {#if !showRightmostColumnLast50}
            <div class="border-t pt-4" style="border-color: var(--ds-border);">
              <Checkbox
                bind:checked={trimCompletedItemsByAge}
                onchange={setTrimCompletedItemsByAge}
                disabled={!canConfigure}
                dataTestid="board-completed-age-enabled"
                label={t('settings.boardConfig.hideCompletedByAge')}
                hint={t('settings.boardConfig.hideCompletedByAgeHint')}
                size="small"
              />
              {#if trimCompletedItemsByAge}
                <div class="ml-6 mt-3 max-w-xs">
                  <label
                    for="board-completed-retention-days"
                    class="mb-1.5 block text-xs font-medium"
                    style="color: var(--ds-text-subtle);"
                  >
                    {t('settings.boardConfig.recentActivityWindow')}
                  </label>
                  <div class="flex items-center gap-2">
                    <Input
                      id="board-completed-retention-days"
                      type="number"
                      value={completedItemRetentionDays}
                      oninput={(event) => updateCompletedItemRetentionDays(event.currentTarget.value)}
                      min={1}
                      max={3650}
                      step={1}
                      size="small"
                      dataTestid="board-completed-retention-days"
                      ariaDescribedby="board-completed-retention-help"
                      class="w-24"
                    />
                    <span class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.boardConfig.days')}</span>
                  </div>
                  <p
                    id="board-completed-retention-help"
                    class="mt-1.5 text-xs"
                    style="color: {completedItemRetentionError ? 'var(--ds-text-danger)' : 'var(--ds-text-subtlest)'};"
                  >
                    {completedItemRetentionError || t('settings.boardConfig.retentionWindowHint')}
                  </p>
                </div>
              {/if}
            </div>
          {/if}
        </div>

        <div class="grid grid-cols-1 lg:grid-cols-6 gap-3 mt-4 mb-6">
          <!-- Left Panel: Available Statuses -->
          <div class="lg:col-span-2 rounded-xl p-3 border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
            <h4 class="text-sm font-semibold mb-1" style="color: var(--ds-text);">
              {t('settings.boardConfig.availableStatuses')} ({availableStatuses.length})
            </h4>
            <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
              {t('settings.boardConfig.dragStatusesToColumns')}
            </p>

            <SearchInput
              bind:value={statusSearchQuery}
              placeholder={t('settings.boardConfig.searchStatuses')}
              size="small"
              className="mb-3"
            />

            <div class="space-y-1 min-h-48 max-h-[60vh] overflow-y-auto" style="overscroll-behavior: contain;">
              {#each availableStatuses as status (status.id)}
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  data-available-status={JSON.stringify({ id: status.id, name: status.name, color: status.category_color })}
                  class="group flex items-center gap-2 px-2 py-1.5 rounded border transition-all duration-200 cursor-grab hover:border-blue-300 active:cursor-grabbing"
                  style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none; -webkit-user-select: none;"
                  onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
                  onmouseleave={(e) => e.currentTarget.style.background = 'var(--ds-background-input)'}
                >
                  <!-- 6-dot drag handle -->
                  <div class="flex-shrink-0">
                    <svg class="w-4 h-4 group-hover:text-blue-500" style="color: var(--ds-text-subtlest);" fill="currentColor" viewBox="0 0 24 24">
                      <circle cx="9" cy="6" r="1.5"/>
                      <circle cx="15" cy="6" r="1.5"/>
                      <circle cx="9" cy="12" r="1.5"/>
                      <circle cx="15" cy="12" r="1.5"/>
                      <circle cx="9" cy="18" r="1.5"/>
                      <circle cx="15" cy="18" r="1.5"/>
                    </svg>
                  </div>
                  <!-- Color dot -->
                  <span class="w-2.5 h-2.5 rounded-full flex-shrink-0" style="background-color: {status.category_color || '#6b7280'};"></span>
                  <!-- Status name -->
                  <span class="text-sm truncate" style="color: var(--ds-text);">{objectDisplayName(status, 'status')}</span>
                </div>
              {/each}

              {#if availableStatuses.length === 0}
                <div class="text-center py-6">
                  <p class="text-sm" style="color: var(--ds-text-subtle);">
                    {#if statusSearchQuery.trim()}
                      {t('settings.boardConfig.noStatusesMatchSearch')}
                    {:else}
                      {t('settings.boardConfig.allStatusesAssigned')}
                    {/if}
                  </p>
                </div>
              {/if}
            </div>
          </div>

          <!-- Right Panel: Board Columns -->
          <div class="lg:col-span-4 rounded-xl p-3 border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
            <div class="flex items-center justify-between mb-3">
              <h4 class="text-sm font-semibold" style="color: var(--ds-text);">
                {t('settings.boardConfig.boardColumns')}
              </h4>
              <Button variant="default" size="small" onclick={addColumn}>
                <Plus class="w-4 h-4 mr-1" />
                {t('settings.boardConfig.addColumn')}
              </Button>
            </div>

            <div class="flex gap-2 min-h-48 max-h-[70vh] overflow-x-auto overflow-y-hidden pb-1" style="overscroll-behavior: contain;">
              {#each columns as column, colIndex (colIndex)}
                <!-- Column section -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  data-board-column={colIndex}
                  class="relative rounded-lg border transition-all w-60 flex-shrink-0 flex flex-col"
                  style="border-color: {column.status_ids.length === 0 ? 'var(--ds-border-warning, #ca8a04)' : 'var(--ds-border)'}; border-style: {column.status_ids.length === 0 ? 'dashed' : 'solid'}; background-color: var(--ds-surface-raised);"
                >
                  <!-- Column reorder DropIndicator -->
                  {#if columnDragState.get(colIndex)?.closestEdge}
                    <DropIndicator edge={columnDragState.get(colIndex)?.closestEdge} gap={8} />
                  {/if}

                  <!-- Column name and actions -->
                  <div class="flex items-center gap-1.5 px-2 py-1.5">
                    <!-- svelte-ignore a11y_no_static_element_interactions -->
                    <div
                      data-column-drag-handle
                      class="cursor-grab active:cursor-grabbing flex-shrink-0"
                      style="color: var(--ds-text-subtlest); touch-action: none;"
                    >
                      <GripVertical class="w-4 h-4" />
                    </div>

                    <Input
                      type="text"
                      value={column.name}
                      oninput={(e) => updateColumnName(colIndex, e.currentTarget.value)}
                      dataTestid={`board-column-name-${colIndex}`}
                      class="flex-1 font-semibold min-w-0"
                      placeholder={t('placeholders.columnName')}
                      size="small"
                    />

                    <button
                      onclick={() => removeColumn(colIndex)}
                      class="p-1 rounded transition-colors flex-shrink-0"
                      style="color: var(--ds-text-danger);"
                      onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-danger-subtle)'}
                      onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
                      title={t('common.delete')}
                    >
                      <X class="w-4 h-4" />
                    </button>
                  </div>

                  <!-- WIP limit -->
                  <div class="flex items-center justify-between gap-3 px-2 pb-2 border-b" style="border-color: var(--ds-border);">
                    <label
                      for={`board-column-wip-${colIndex}`}
                      class="text-xs font-medium"
                      style="color: var(--ds-text-subtle);"
                    >
                      {t('settings.boardConfig.wipLimit')}
                    </label>
                    <Input
                      id={`board-column-wip-${colIndex}`}
                      type="number"
                      value={column.wip_limit || ''}
                      oninput={(e) => updateWIPLimit(colIndex, e.currentTarget.value)}
                      dataTestid={`board-column-wip-${colIndex}`}
                      class="text-center flex-shrink-0"
                      style="width: 3.75rem;"
                      placeholder="—"
                      title={t('settings.boardConfig.wipLimit')}
                      min={1}
                      size="small"
                    />
                  </div>

                  <!-- Column Body -->
                  <div class="p-1.5 flex flex-col gap-1 flex-1 overflow-y-auto">
                    {#each column.status_ids as statusId, statusIndex (statusId)}
                      <!-- svelte-ignore a11y_no_static_element_interactions -->
                      <div
                        data-column-status
                        data-status-id={statusId}
                        data-col-index={colIndex}
                        data-status-index={statusIndex}
                        class="relative group flex items-center gap-2 px-2 py-1.5 rounded border transition-all duration-200"
                        style="background: var(--ds-background-input); border-color: var(--ds-border); user-select: none;"
                      >
                        <!-- DropIndicator for status insertion -->
                        {#if statusDragState.get(`${colIndex}-${statusId}`)?.closestEdge}
                          <DropIndicator edge={statusDragState.get(`${colIndex}-${statusId}`)?.closestEdge} gap={4} />
                        {/if}

                        <!-- Drag handle -->
                        <div class="cursor-grab active:cursor-grabbing flex-shrink-0" style="touch-action: none;">
                          <svg class="w-3.5 h-3.5 group-hover:text-blue-500" style="color: var(--ds-text-subtlest);" fill="currentColor" viewBox="0 0 24 24">
                            <circle cx="9" cy="6" r="1.5"/>
                            <circle cx="15" cy="6" r="1.5"/>
                            <circle cx="9" cy="12" r="1.5"/>
                            <circle cx="15" cy="12" r="1.5"/>
                            <circle cx="9" cy="18" r="1.5"/>
                            <circle cx="15" cy="18" r="1.5"/>
                          </svg>
                        </div>
                        <!-- Color dot -->
                        <span class="w-2 h-2 rounded-full flex-shrink-0" style="background-color: {getStatusColor(statusId)};"></span>
                        <!-- Name -->
                        <span class="text-sm flex-1 truncate" style="color: var(--ds-text);">{getStatusName(statusId)}</span>
                        <!-- Remove button -->
                        <button
                          onclick={() => removeStatusFromColumn(colIndex, statusId)}
                          class="opacity-0 group-hover:opacity-100 p-0.5 rounded transition-all flex-shrink-0"
                          style="color: var(--ds-text-subtle);"
                          onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text-danger)'}
                          onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                          title={t('common.remove')}
                        >
                          <X class="w-3.5 h-3.5" />
                        </button>
                      </div>
                    {/each}

                    <!-- Drop zone fills remaining column height -->
                    <div
                      data-column-drop-zone={colIndex}
                      class="border-2 border-dashed rounded flex items-center justify-center flex-1 min-h-[3rem] px-2 transition-colors"
                      style="border-color: var(--ds-border); color: var(--ds-text-subtlest);"
                    >
                      <span class="text-xs text-center">{t('settings.boardConfig.dropStatusesHere')}</span>
                    </div>
                  </div>
                </div>
              {/each}

              {#if columns.length === 0}
                <div class="flex-1">
                  <EmptyState title={t('settings.boardConfig.noStatusesMapped')}>
                    {#snippet action()}
                      <Button variant="default" size="small" icon={Plus} onclick={addColumn}>
                        {t('settings.boardConfig.addColumn')}
                      </Button>
                    {/snippet}
                  </EmptyState>
                </div>
              {/if}
            </div>
          </div>
        </div>

        {:else if activeTab === 'backlog'}
        <!-- Backlog Tab -->
        <div class="mt-6 mb-6">
          <div class="rounded border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            <h3 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">{t('settings.boardConfig.backlogStatuses')}</h3>
            <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
              {t('settings.boardConfig.backlogStatusesHelp')}
            </p>

            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
              {#each statuses as status}
                {@const isSelected = backlogStatusIDs.includes(status.id)}
                <button
                  onclick={() => toggleBacklogStatus(status.id)}
                  class="px-4 py-3 text-sm rounded transition-colors border text-left flex items-center gap-2"
                  style={isSelected
                    ? 'background-color: var(--ds-interactive-subtle, #3b82f61A); border-color: var(--ds-interactive); color: var(--ds-interactive);'
                    : 'background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text-subtle);'}
                >
                  <span class="flex-shrink-0 w-5">
                    {#if isSelected}
                      <span style="color: var(--ds-interactive);">✓</span>
                    {/if}
                  </span>
                  <span class="flex-1">{objectDisplayName(status, 'status')}</span>
                </button>
              {/each}
            </div>

            {#if backlogStatusIDs.length === 0}
              <p class="text-sm mt-4" style="color: var(--ds-text-warning, #ca8a04);">
                {t('settings.boardConfig.noStatusesSelected')}
              </p>
            {:else}
              <p class="text-sm mt-4" style="color: var(--ds-text-subtle);">
                {t('settings.boardConfig.backlogSelected', { count: backlogStatusIDs.length })}
              </p>
            {/if}
          </div>
        </div>

        {:else if activeTab === 'cardFields'}
        <!-- Card Fields Tab -->
        <div class="mt-6 mb-6">
          <div class="rounded border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            <h3 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">{t('settings.boardConfig.cardFieldsTitle')}</h3>
            <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
              {t('settings.boardConfig.cardFieldsDescription')}
            </p>

            <!-- Selected card fields -->
            {#if cardFields.length > 0}
              <div class="space-y-1 mb-6" data-testid="board-card-fields-list">
                {#each cardFields as field, index (field.field_identifier)}
                  <div
                       data-testid={`board-card-field-row-${field.field_identifier}`}
                       data-field-identifier={field.field_identifier}
                       data-card-field
                       data-card-field-index={index}
                       data-card-field-identifier={field.field_identifier}
                       class="relative flex items-center gap-2 px-3 py-2 rounded border transition-all"
                       style="background: var(--ds-background-input); border-color: var(--ds-border); user-select: none;">
                    {#if cardFieldDragState.get(field.field_identifier)?.closestEdge}
                      <DropIndicator edge={cardFieldDragState.get(field.field_identifier)?.closestEdge} gap={4} />
                    {/if}
                    <!-- Drag handles for reorder -->
                    <button
                      type="button"
                      data-card-field-drag-handle
                      data-testid="board-card-field-drag-handle"
                      class="flex-shrink-0 cursor-grab active:cursor-grabbing p-0.5 rounded"
                      style="color: var(--ds-text-subtlest); touch-action: none;"
                      disabled={!canConfigure}
                      onkeydown={(event) => handleCardFieldReorderKeydown(event, index)}
                      aria-label={`${t('settings.boardConfig.dragToReorder')}: ${getCardFieldLabel(field)}`}
                      aria-keyshortcuts="ArrowUp ArrowDown"
                      title={t('settings.boardConfig.dragToReorder')}
                    >
                      <GripVertical class="w-4 h-4" />
                    </button>
                    <span class="text-sm flex-1" style="color: var(--ds-text);">{getCardFieldLabel(field)}</span>
                    <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-surface); color: var(--ds-text-subtle);">{field.field_type}</span>
                    <button
                      onclick={() => removeCardField(field.field_identifier)}
                      class="p-0.5 rounded transition-colors flex-shrink-0"
                      style="color: var(--ds-text-subtle);"
                      onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text-danger)'}
                      onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                      title={t('common.remove')}
                    >
                      <X class="w-4 h-4" />
                    </button>
                  </div>
                {/each}
              </div>
            {:else}
              <p class="text-sm mb-6" style="color: var(--ds-text-warning, #ca8a04);">
                {t('settings.boardConfig.noCardFields')}
              </p>
            {/if}

            <!-- Add field section -->
            <div>
              <h4 class="text-base font-semibold mb-4" style="color: var(--ds-text);">{t('settings.boardConfig.addField')}</h4>

              <!-- System fields -->
              {#if availableSystemFields.length > 0}
                <section class="mb-5" aria-labelledby="board-card-fields-system-heading">
                  <h5
                    id="board-card-fields-system-heading"
                    class="text-sm font-medium mb-2"
                    style="color: var(--ds-text-subtle);"
                  >
                    {t('settings.boardConfig.systemFields')}
                  </h5>
                  <div class="flex flex-wrap gap-2">
                    {#each availableSystemFields as sf}
                      <button
                        onclick={() => addCardField(sf.identifier, 'system')}
                        class="px-3 py-1.5 text-xs rounded-full border transition-colors"
                        style="border-color: var(--ds-border); color: var(--ds-text); background: var(--ds-surface);"
                        onmouseenter={(e) => { e.currentTarget.style.borderColor = 'var(--ds-interactive)'; e.currentTarget.style.color = 'var(--ds-interactive)'; }}
                        onmouseleave={(e) => { e.currentTarget.style.borderColor = 'var(--ds-border)'; e.currentTarget.style.color = 'var(--ds-text)'; }}
                      >
                        <Plus class="w-3 h-3 inline mr-1" />{sf.label}
                      </button>
                    {/each}
                  </div>
                </section>
              {/if}

              <!-- Custom fields -->
              {#if availableCustomFields.length > 0 || isSystemAdmin || isNonSystemWorkspaceAdmin}
                <section
                  data-testid="board-card-fields-custom-section"
                  aria-labelledby="board-card-fields-custom-heading"
                  class={availableSystemFields.length > 0 ? 'pt-5 border-t' : ''}
                  style={availableSystemFields.length > 0 ? 'border-color: var(--ds-border);' : undefined}
                >
                  <div class="flex flex-wrap items-center gap-3 mb-3">
                    <h5
                      id="board-card-fields-custom-heading"
                      data-testid="board-card-fields-custom-heading"
                      class="text-sm font-medium"
                      style="color: var(--ds-text-subtle);"
                    >
                      {t('settings.boardConfig.customFields')}
                    </h5>

                    {#if isSystemAdmin}
                      <button
                        data-testid="board-custom-fields-action"
                        type="button"
                        onclick={() => goCustomFields(!hasCustomFields)}
                        class="inline-flex items-center px-3 py-1.5 text-xs rounded-full border transition-colors"
                        style="border-color: var(--ds-border); color: var(--ds-interactive); background: var(--ds-surface);"
                      >
                        {#if hasCustomFields}
                          <Settings class="w-3 h-3 mr-1" />
                        {:else}
                          <Plus class="w-3 h-3 mr-1" />
                        {/if}
                        {hasCustomFields ? t('settings.boardConfig.manageCustomFields') : t('settings.boardConfig.createCustomField')}
                      </button>
                    {/if}
                  </div>

                  {#if isNonSystemWorkspaceAdmin}
                    <p
                      data-testid="board-custom-fields-admin-note"
                      class="text-xs mb-3 p-2 rounded"
                      style="color: var(--ds-text-subtle); background: var(--ds-surface); border: 1px solid var(--ds-border);"
                    >
                      {t('settings.boardConfig.customFieldsGlobalNote')}
                    </p>
                  {/if}

                  {#if availableCustomFields.length > 0}
                    <div class="flex flex-wrap gap-2">
                      {#each availableCustomFields as cf}
                        <button
                          data-testid={`board-card-field-add-custom-${cf.id}`}
                          onclick={() => addCardField(`custom_field_${cf.id}`, 'custom')}
                          class="px-3 py-1.5 text-xs rounded-full border transition-colors"
                          style="border-color: var(--ds-border); color: var(--ds-text); background: var(--ds-surface);"
                          onmouseenter={(e) => { e.currentTarget.style.borderColor = 'var(--ds-interactive)'; e.currentTarget.style.color = 'var(--ds-interactive)'; }}
                          onmouseleave={(e) => { e.currentTarget.style.borderColor = 'var(--ds-border)'; e.currentTarget.style.color = 'var(--ds-text)'; }}
                        >
                          <Plus class="w-3 h-3 inline mr-1" />{cf.name}
                        </button>
                      {/each}
                    </div>
                  {/if}
                </section>
              {/if}
            </div>
          </div>
        </div>
        {/if}

        <!-- Action buttons -->
        <div class="flex items-center justify-between border-t pt-6" style="border-color: var(--ds-border);">
          <button
            onclick={resetToDefault}
            class="px-4 py-2 text-sm rounded transition-colors"
            style="color: var(--ds-text-danger);"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-danger-subtle)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            disabled={!canConfigure || (!boardConfig && columns.length === 0)}
            title={!canConfigure ? t('workspaceSettings.accessDeniedDescription') : ''}
          >
            {t('settings.boardConfig.resetToDefault')}
          </button>

          <div class="flex gap-3">
            <Button variant="default" onclick={cancelChanges} disabled={saving}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              dataTestid="board-config-save"
              onclick={saveConfiguration}
              disabled={!canConfigure || saving}
              loading={saving}
              title={!canConfigure ? t('workspaceSettings.accessDeniedDescription') : ''}
            >
              {saving ? t('common.saving') : t('common.saveChanges')}
            </Button>
          </div>
        </div>
        </Panel>
      </div>
  </StaticViewBackground>
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      {t('common.notFound')}
    </div>
  </div>
{/if}
