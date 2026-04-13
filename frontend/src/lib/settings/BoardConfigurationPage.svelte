<script>
  import { onMount, onDestroy } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { navigate } from '../router.js';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { CARD_SELECTABLE_FIELDS, getSystemFieldName } from '../stores/fieldConfig.js';
  import { confirm } from '../composables/useConfirm.js';
  import { getCollection } from '../features/collections/collectionService.js';
  import { Plus, GripVertical, Trash2, ChevronDown, X, Grip } from 'lucide-svelte';
  import { useGradientStyles, loadWorkspaceGradient } from '../stores/workspaceGradient.svelte.js';
  import ViewHeader from '../layout/ViewHeader.svelte';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import DropIndicator from '../layout/DropIndicator.svelte';
  import CollectionViewSwitcher from '../features/collections/CollectionViewSwitcher.svelte';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let currentCollectionName = $state('Default');
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
  let customFieldDefinitions = $state([]);

  // DnD state
  let statusDragState = $state(new Map());
  let columnDragState = $state(new Map());
  let statusSearchQuery = $state('');
  let expandedColumns = $state(new Set());
  let setupCleanups = [];
  let setupTimeout;

  const styles = useGradientStyles();

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
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
    }
    await loadData();
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
    statusSearchQuery;
    expandedColumns;
    if (!loading && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => setupDragAndDrop(), 50);
    }
  });

  async function loadData() {
    try {
      if (workspaceId) {
        workspace = await api.workspaces.get(workspaceId);
      }

      if (collectionId) {
        const collection = await getCollection(collectionId);
        if (collection) {
          currentCollectionName = collection.name;
          publicSlug = (collection.is_public && collection.public_slug) ? collection.public_slug : null;
        }
      } else {
        currentCollectionName = 'Default';
      }

      const statusMap = new Map();

      if (collectionId) {
        const collection = await getCollection(collectionId);

        if (collection && collection.ql_query) {
          try {
            const items = await api.items.getAll({ ql: collection.ql_query });
            const workspaceIds = [...new Set(items.map(item => item.workspace_id).filter(id => id))];

            for (const wsId of workspaceIds) {
              const wsStatuses = await api.workspaces.getStatuses(wsId);
              wsStatuses.forEach(status => statusMap.set(status.id, status));
            }

            if (workspaceIds.length === 0) {
              const allStatuses = workspaceId
                ? await api.workspaces.getStatuses(workspaceId)
                : await api.statuses.getAll();
              allStatuses.forEach(status => statusMap.set(status.id, status));
            }
          } catch (error) {
            console.error('Failed to load items for collection:', error);
            const allStatuses = workspaceId
              ? await api.workspaces.getStatuses(workspaceId)
              : await api.statuses.getAll();
            allStatuses.forEach(status => statusMap.set(status.id, status));
          }
        } else {
          const allStatuses = workspaceId
            ? await api.workspaces.getStatuses(workspaceId)
            : await api.statuses.getAll();
          allStatuses.forEach(status => statusMap.set(status.id, status));
        }
      } else if (workspaceId) {
        const wsStatuses = await api.workspaces.getStatuses(workspaceId);
        wsStatuses.forEach(status => statusMap.set(status.id, status));
      } else {
        const allStatuses = await api.statuses.getAll();
        allStatuses.forEach(status => statusMap.set(status.id, status));
      }

      statuses = Array.from(statusMap.values());

      try {
        boardConfig = await api.collections.getBoardConfiguration(collectionId, workspaceId);
        columns = (boardConfig.columns || []).map(col => ({
          ...col,
          status_ids: col.status_ids || []
        }));
        backlogStatusIDs = boardConfig.backlog_status_ids || [];
        cardFields = boardConfig.card_fields || [];
      } catch (error) {
        if (error.status !== 404) {
          console.error('Failed to load board configuration:', error);
        }
        columns = [];
        backlogStatusIDs = [];
        cardFields = [];
      }

      try {
        const cfData = await api.customFields.getAll(workspaceId ? { workspace_id: workspaceId } : {});
        customFieldDefinitions = (cfData?.data || cfData || []);
      } catch (e) {
        customFieldDefinitions = [];
      }

      // Expand all columns by default
      expandedColumns = new Set(columns.map((_, i) => i));
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
  }

  function setupDragAndDrop() {
    cleanupDragAndDrop();

    // --- Available statuses (left panel) ---
    document.querySelectorAll('[data-available-status]').forEach(element => {
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
    document.querySelectorAll('[data-column-status]').forEach(element => {
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
    document.querySelectorAll('[data-column-drop-zone]').forEach(element => {
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
    document.querySelectorAll('[data-board-column]').forEach(element => {
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
          return attachClosestEdge({}, { input, element, allowedEdges: ['top', 'bottom'] });
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
    const insertIndex = closestEdge === 'bottom' ? toIndex + 1 : toIndex;
    const adjustedIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;

    const newColumns = [...columns];
    const [moved] = newColumns.splice(fromIndex, 1);
    newColumns.splice(adjustedIndex, 0, moved);
    columns = newColumns.map((col, i) => ({ ...col, display_order: i }));

    // Update expanded set indices
    const newExpanded = new Set();
    for (const idx of expandedColumns) {
      if (idx === fromIndex) {
        newExpanded.add(adjustedIndex);
      } else if (fromIndex < idx && idx <= adjustedIndex) {
        newExpanded.add(idx - 1);
      } else if (adjustedIndex <= idx && idx < fromIndex) {
        newExpanded.add(idx + 1);
      } else {
        newExpanded.add(idx);
      }
    }
    expandedColumns = newExpanded;

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
    expandedColumns = new Set([...expandedColumns, columns.length - 1]);
    hasChanges = true;
  }

  function removeColumn(index) {
    columns = columns.filter((_, i) => i !== index);
    columns = columns.map((col, i) => ({ ...col, display_order: i }));
    // Rebuild expanded set
    const newExpanded = new Set();
    for (const idx of expandedColumns) {
      if (idx < index) newExpanded.add(idx);
      else if (idx > index) newExpanded.add(idx - 1);
    }
    expandedColumns = newExpanded;
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

  function toggleColumnExpanded(index) {
    const next = new Set(expandedColumns);
    if (next.has(index)) {
      next.delete(index);
    } else {
      next.add(index);
    }
    expandedColumns = next;
  }

  function getStatusName(statusId) {
    const s = statuses.find(s => s.id === statusId);
    return s ? s.name : statusId;
  }

  function getStatusColor(statusId) {
    const s = statuses.find(s => s.id === statusId);
    return s?.color || '#6b7280';
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

  // --- Card Fields ---

  const systemFieldOptions = CARD_SELECTABLE_FIELDS.map(f => ({
    identifier: f.identifier,
    label: f.name
  }));

  let selectedCardFieldIds = $derived(new Set(cardFields.map(f => f.field_identifier)));

  let availableSystemFields = $derived(
    systemFieldOptions.filter(f => !selectedCardFieldIds.has(f.identifier))
  );

  let availableCustomFields = $derived(
    (customFieldDefinitions || []).filter(f => !selectedCardFieldIds.has(`custom_field_${f.id}`))
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

  function reorderCardFields(fromIndex, toIndex) {
    if (fromIndex === toIndex) return;
    const newFields = [...cardFields];
    const [moved] = newFields.splice(fromIndex, 1);
    newFields.splice(toIndex, 0, moved);
    cardFields = newFields.map((f, i) => ({ ...f, display_order: i }));
    hasChanges = true;
  }

  function getCardFieldLabel(field) {
    if (field.field_type === 'system') {
      return getSystemFieldName(field.field_identifier);
    }
    // Custom field
    const cfId = field.field_identifier.replace('custom_field_', '');
    const cf = customFieldDefinitions.find(d => String(d.id) === cfId);
    return cf?.name || field.field_identifier;
  }

  // --- Save / Reset / Cancel ---

  async function saveConfiguration() {
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
        card_fields: cardFields.map((f, i) => ({
          field_identifier: f.field_identifier,
          field_type: f.field_type,
          display_order: i,
          width: 0
        }))
      };

      if (boardConfig && boardConfig.id) {
        await api.collections.updateBoardConfiguration(collectionId, boardConfig.id, payload);
      } else {
        const newConfig = await api.collections.createBoardConfiguration(collectionId, workspaceId, payload);
        boardConfig = newConfig;
      }

      hasChanges = false;
      goToBoard();
    } catch (error) {
      console.error('Failed to save board configuration:', error);
      alert(t('dialogs.alerts.failedToSave', { error: error.message }));
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
        hasChanges = false;
        goToBoard();
      } catch (error) {
        console.error('Failed to delete board configuration:', error);
        alert(t('dialogs.alerts.failedToResetConfig', { error: error.message }));
      }
    } else {
      columns = [];
      backlogStatusIDs = [];
      cardFields = [];
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
  <div class="min-h-screen" style="{styles.backgroundStyle} {styles.contextVars}">
    <div class="p-6">
      <div class="space-y-6">
        <!-- Header with view tabs -->
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName}
          viewName="Configure Board"
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
        <Card rounded="xl" shadow padding="spacious" class="w-full" style="border-color: {styles.hasCustomBackground ? 'transparent' : 'var(--ds-border)'};">
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
        <div class="grid grid-cols-1 lg:grid-cols-5 gap-6 mt-6 mb-6">
          <!-- Left Panel: Available Statuses -->
          <div class="lg:col-span-2 rounded-xl p-4 border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
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
                  data-available-status={JSON.stringify({ id: status.id, name: status.name, color: status.color })}
                  class="group flex items-center gap-3 px-3 py-2 rounded border transition-all duration-200 cursor-grab hover:border-blue-300 active:cursor-grabbing"
                  style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none; -webkit-user-select: none;"
                  onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
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
                  <span class="w-2.5 h-2.5 rounded-full flex-shrink-0" style="background-color: {status.color || '#6b7280'};"></span>
                  <!-- Status name -->
                  <span class="text-sm truncate" style="color: var(--ds-text);">{status.name}</span>
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
          <div class="lg:col-span-3 rounded-xl p-4 border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
            <div class="flex items-center justify-between mb-4">
              <h4 class="text-sm font-semibold" style="color: var(--ds-text);">
                {t('settings.boardConfig.boardColumns')}
              </h4>
              <Button variant="default" size="small" onclick={addColumn}>
                <Plus class="w-4 h-4 mr-1" />
                {t('settings.boardConfig.addColumn')}
              </Button>
            </div>

            <div class="space-y-3 min-h-48 max-h-[60vh] overflow-y-auto" style="overscroll-behavior: contain;">
              {#each columns as column, colIndex (colIndex)}
                <!-- Column section -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  data-board-column={colIndex}
                  class="relative rounded-lg border transition-all"
                  style="border-color: {column.status_ids.length === 0 ? 'var(--ds-border-warning, #ca8a04)' : 'var(--ds-border)'}; border-style: {column.status_ids.length === 0 ? 'dashed' : 'solid'}; background-color: var(--ds-surface-raised);"
                >
                  <!-- Column reorder DropIndicator -->
                  {#if columnDragState.get(colIndex)?.closestEdge}
                    <DropIndicator edge={columnDragState.get(colIndex)?.closestEdge} gap={12} />
                  {/if}

                  <!-- Column Header -->
                  <div class="flex items-center gap-2 px-3 py-2 border-b" style="border-color: var(--ds-border);">
                    <!-- Drag handle for column reorder -->
                    <!-- svelte-ignore a11y_no_static_element_interactions -->
                    <div
                      data-column-drag-handle
                      class="cursor-grab active:cursor-grabbing flex-shrink-0"
                      style="color: var(--ds-text-subtlest); touch-action: none;"
                    >
                      <GripVertical class="w-4 h-4" />
                    </div>

                    <!-- Expand/Collapse toggle -->
                    <button
                      class="flex-shrink-0 p-0.5 rounded transition-transform"
                      style="color: var(--ds-text-subtle);"
                      onclick={() => toggleColumnExpanded(colIndex)}
                    >
                      <ChevronDown class="w-4 h-4 transition-transform {expandedColumns.has(colIndex) ? '' : '-rotate-90'}" />
                    </button>

                    <!-- Column name input -->
                    <input
                      type="text"
                      value={column.name}
                      oninput={(e) => updateColumnName(colIndex, e.target.value)}
                      class="flex-1 px-2 py-1 border rounded text-sm font-semibold min-w-0"
                      style="border-color: var(--ds-border); color: var(--ds-text); background-color: var(--ds-surface);"
                      placeholder={t('placeholders.columnName')}
                    />

                    <!-- WIP limit input -->
                    <div class="flex items-center gap-1 flex-shrink-0">
                      <span class="text-xs whitespace-nowrap" style="color: var(--ds-text-subtle);">{t('settings.boardConfig.wipLimit')}:</span>
                      <input
                        type="number"
                        value={column.wip_limit || ''}
                        oninput={(e) => updateWIPLimit(colIndex, e.target.value)}
                        class="w-14 px-1.5 py-1 border rounded text-sm text-center"
                        style="border-color: var(--ds-border); color: var(--ds-text); background-color: var(--ds-surface);"
                        placeholder="--"
                        min="1"
                      />
                    </div>

                    <!-- Delete column -->
                    <button
                      onclick={() => removeColumn(colIndex)}
                      class="p-1 rounded transition-colors flex-shrink-0"
                      style="color: var(--ds-text-danger);"
                      onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-danger-subtle)'}
                      onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
                      title={t('common.delete')}
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>

                  <!-- Column Body (collapsible) -->
                  {#if expandedColumns.has(colIndex)}
                    <div class="p-2 space-y-1">
                      {#each column.status_ids as statusId, statusIndex (statusId)}
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div
                          data-column-status
                          data-status-id={statusId}
                          data-col-index={colIndex}
                          data-status-index={statusIndex}
                          class="relative group flex items-center gap-2 px-3 py-1.5 rounded border transition-all duration-200"
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

                      <!-- Drop zone for appending -->
                      <div
                        data-column-drop-zone={colIndex}
                        class="border-2 border-dashed rounded px-3 py-3 text-center transition-colors"
                        style="border-color: var(--ds-border); color: var(--ds-text-subtlest);"
                      >
                        <span class="text-xs">{t('settings.boardConfig.dropStatusesHere')}</span>
                      </div>
                    </div>
                  {/if}
                </div>
              {/each}

              {#if columns.length === 0}
                <div class="text-center py-8">
                  <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">{t('settings.boardConfig.noStatusesMapped')}</p>
                  <Button variant="default" size="small" onclick={addColumn}>
                    <Plus class="w-4 h-4 mr-1" />
                    {t('settings.boardConfig.addColumn')}
                  </Button>
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
                  <span class="flex-1">{status.name}</span>
                </button>
              {/each}
            </div>

            {#if backlogStatusIDs.length === 0}
              <p class="text-sm mt-4" style="color: var(--ds-text-warning, #ca8a04);">
                {t('settings.boardConfig.noStatusesSelected')}
              </p>
            {:else}
              <p class="text-sm mt-4" style="color: var(--ds-text-subtle);">
                {backlogStatusIDs.length} {backlogStatusIDs.length === 1 ? 'status' : 'statuses'} selected for backlog
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
              <div class="space-y-1 mb-6">
                {#each cardFields as field, index (field.field_identifier)}
                  <div class="flex items-center gap-2 px-3 py-2 rounded border transition-all"
                       style="background: var(--ds-background-input); border-color: var(--ds-border);">
                    <!-- Drag handles for reorder -->
                    <button
                      class="flex-shrink-0 cursor-pointer"
                      style="color: var(--ds-text-subtlest);"
                      disabled={index === 0}
                      onclick={() => reorderCardFields(index, index - 1)}
                      title="Move up"
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
              <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">{t('settings.boardConfig.addField')}</h4>

              <!-- System fields -->
              {#if availableSystemFields.length > 0}
                <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">{t('settings.boardConfig.systemFields')}</p>
                <div class="flex flex-wrap gap-2 mb-4">
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
              {/if}

              <!-- Custom fields -->
              {#if availableCustomFields.length > 0}
                <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">{t('settings.boardConfig.customFields')}</p>
                <div class="flex flex-wrap gap-2">
                  {#each availableCustomFields as cf}
                    <button
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
            disabled={!boardConfig && columns.length === 0}
          >
            {t('settings.boardConfig.resetToDefault')}
          </button>

          <div class="flex gap-3">
            <Button variant="default" onclick={cancelChanges} disabled={saving}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              onclick={saveConfiguration}
              disabled={saving || (activeTab === 'columns' && columns.length === 0)}
              loading={saving}
            >
              {saving ? t('common.saving') : t('common.saveChanges')}
            </Button>
          </div>
        </div>
        </Card>
      </div>
    </div>
  </div>
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      {t('common.notFound')}
    </div>
  </div>
{/if}

<style>
  .remove-status-btn:hover {
    background: color-mix(in srgb, currentColor 10%, transparent);
  }
</style>
