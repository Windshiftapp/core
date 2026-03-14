<script>
  import { onMount } from 'svelte';
  import { navigate } from '../router.js';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { getCollection } from '../features/collections/collectionService.js';
  import { Plus, GripVertical, Trash2, X } from 'lucide-svelte';
  import { useGradientStyles, loadWorkspaceGradient } from '../stores/workspaceGradient.svelte.js';
  import ViewHeader from '../layout/ViewHeader.svelte';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import CollectionViewSwitcher from '../features/collections/CollectionViewSwitcher.svelte';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let currentCollectionName = $state('Default');
  let loading = $state(true);
  let saving = $state(false);
  let boardConfig = $state(null);
  let columns = $state([]);
  let statuses = $state([]);
  let hasChanges = $state(false);
  let draggedColumnIndex = $state(null);
  let activeTab = $state('columns'); // 'columns' or 'backlog'
  let backlogStatusIDs = $state([]); // Status IDs to show in backlog

  const styles = useGradientStyles();

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
    }
    await loadData();
    loading = false;
  });

  async function loadData() {
    try {
      // Load workspace
      workspace = await api.workspaces.get(workspaceId);

      // Load collection name if this is a specific collection
      if (collectionId) {
        const collection = await getCollection(collectionId);
        if (collection) {
          currentCollectionName = collection.name;
        }
      } else {
        // No collection - board configuration only works with collections
        // Show message to user
        currentCollectionName = 'Default';
      }

      // Load statuses based on workspaces represented in the collection
      const statusMap = new Map(); // Map<status_id, status> to deduplicate

      if (collectionId) {
        // Get collection to access its QL query
        const collection = await getCollection(collectionId);

        if (collection && collection.ql_query) {
          try {
            // Query items in the collection using the QL query
            const items = await api.items.getAll({ ql: collection.ql_query });

            // Extract unique workspace IDs from items
            const workspaceIds = [...new Set(items.map(item => item.workspace_id).filter(id => id))];

            // For each workspace, load statuses (uses workflow from config set or default workflow)
            for (const wsId of workspaceIds) {
              const wsStatuses = await api.workspaces.getStatuses(wsId);
              wsStatuses.forEach(status => statusMap.set(status.id, status));
            }

            // If no items found or no workspaces, use current workspace's statuses
            if (workspaceIds.length === 0) {
              const wsStatuses = await api.workspaces.getStatuses(workspaceId);
              wsStatuses.forEach(status => statusMap.set(status.id, status));
            }
          } catch (error) {
            console.error('Failed to load items for collection:', error);
            // Fall back to workspace statuses on error
            const wsStatuses = await api.workspaces.getStatuses(workspaceId);
            wsStatuses.forEach(status => statusMap.set(status.id, status));
          }
        } else {
          // No QL query - use workspace's statuses
          const wsStatuses = await api.workspaces.getStatuses(workspaceId);
          wsStatuses.forEach(status => statusMap.set(status.id, status));
        }
      } else {
        // No collection specified - use workspace's statuses
        const wsStatuses = await api.workspaces.getStatuses(workspaceId);
        wsStatuses.forEach(status => statusMap.set(status.id, status));
      }

      // Convert map to array
      statuses = Array.from(statusMap.values());

      // Try to load existing board configuration
      try {
        boardConfig = await api.collections.getBoardConfiguration(collectionId, workspaceId);
        columns = (boardConfig.columns || []).map(col => ({
          ...col,
          status_ids: col.status_ids || []
        }));
        backlogStatusIDs = boardConfig.backlog_status_ids || [];
      } catch (error) {
        if (error.status !== 404) {
          console.error('Failed to load board configuration:', error);
        }
        // No configuration exists, start with empty columns and backlog config
        columns = [];
        backlogStatusIDs = [];
      }
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  function addColumn() {
    const newColumn = {
      name: `${t('settings.boardConfig.columns')} ${columns.length + 1}`,
      display_order: columns.length,
      wip_limit: null,
      color: '#f3f4f6', // Default gray
      status_ids: []
    };
    columns = [...columns, newColumn];
    hasChanges = true;
  }

  function removeColumn(index) {
    columns = columns.filter((_, i) => i !== index);
    // Update display orders
    columns = columns.map((col, i) => ({ ...col, display_order: i }));
    hasChanges = true;
  }

  function toggleStatus(columnIndex, statusId) {
    const column = columns[columnIndex];
    const statusIndex = column.status_ids.indexOf(statusId);

    if (statusIndex >= 0) {
      column.status_ids = column.status_ids.filter(id => id !== statusId);
    } else {
      column.status_ids = [...column.status_ids, statusId];
    }

    columns = [...columns]; // Trigger reactivity
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

  function handleDragStart(index) {
    draggedColumnIndex = index;
  }

  function handleDragOver(event, targetIndex) {
    event.preventDefault();
    if (draggedColumnIndex === null || draggedColumnIndex === targetIndex) return;

    // Reorder columns
    const newColumns = [...columns];
    const draggedColumn = newColumns[draggedColumnIndex];
    newColumns.splice(draggedColumnIndex, 1);
    newColumns.splice(targetIndex, 0, draggedColumn);

    // Update display orders
    columns = newColumns.map((col, i) => ({ ...col, display_order: i }));
    draggedColumnIndex = targetIndex;
    hasChanges = true;
  }

  function handleDragEnd() {
    draggedColumnIndex = null;
  }

  function toggleBacklogStatus(statusId) {
    const index = backlogStatusIDs.indexOf(statusId);
    if (index >= 0) {
      backlogStatusIDs = backlogStatusIDs.filter(id => id !== statusId);
    } else {
      backlogStatusIDs = [...backlogStatusIDs, statusId];
    }
    hasChanges = true;
  }

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
        backlog_status_ids: backlogStatusIDs
      };

      if (boardConfig && boardConfig.id) {
        // Update existing configuration
        await api.collections.updateBoardConfiguration(collectionId, boardConfig.id, payload);
      } else {
        // Create new configuration
        const newConfig = await api.collections.createBoardConfiguration(collectionId, workspaceId, payload);
        boardConfig = newConfig;
      }

      hasChanges = false;
      // Navigate back to board
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
        hasChanges = false;
        // Navigate back to board
        goToBoard();
      } catch (error) {
        console.error('Failed to delete board configuration:', error);
        alert(t('dialogs.alerts.failedToResetConfig', { error: error.message }));
      }
    } else {
      columns = [];
      backlogStatusIDs = [];
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
    const url = collectionId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/board`
      : `/workspaces/${workspaceId}/board`;
    navigate(url);
  }

</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('common.loading')}</div>
  </div>
{:else if workspace}
  <div class="min-h-screen" style="{styles.backgroundStyle}">
    <div class="p-6">
      <div class="space-y-6">
        <!-- Header with view tabs -->
        <ViewHeader
          workspaceName={workspace.name}
          collection={currentCollectionName}
          viewName="Configure Board"
          itemCount={columns.length}
          hasGradient={styles.hasCustomBackground}
          textStyle={styles.textStyle}
          subtleTextStyle={styles.subtleTextStyle}
        >
          {#snippet actions()}
            <CollectionViewSwitcher
              {workspaceId}
              {collectionId}
              activeView="configure"
              hasGradient={styles.hasCustomBackground}
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
              class:border-blue-600={activeTab === 'columns'}
              class:border-transparent={activeTab !== 'columns'}
              style:color={activeTab === 'columns' ? '#2563eb' : 'var(--ds-text-subtle)'}
              onclick={() => activeTab = 'columns'}
            >
              {t('settings.boardConfig.columns')}
            </button>
            <button
              class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
              class:border-blue-600={activeTab === 'backlog'}
              class:border-transparent={activeTab !== 'backlog'}
              style:color={activeTab === 'backlog' ? '#2563eb' : 'var(--ds-text-subtle)'}
              onclick={() => activeTab = 'backlog'}
            >
              {t('settings.boardConfig.backlog')}
            </button>
          </div>
        </div>

        <!-- Columns Tab -->
        {#if activeTab === 'columns'}
        <div class="flex gap-4 mt-6 mb-6 overflow-x-auto pb-4">
          {#each columns as column, index (index)}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="rounded border shadow-sm flex-shrink-0 transition-opacity"
              class:opacity-50={draggedColumnIndex === index}
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); width: 280px;"
              draggable="true"
              ondragstart={() => handleDragStart(index)}
              ondragover={(e) => handleDragOver(e, index)}
              ondragend={handleDragEnd}
            >
              <!-- Column header -->
              <div class="p-3 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
                <div class="flex items-center gap-2 flex-1 min-w-0">
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
                  <div
                    class="cursor-grab active:cursor-grabbing"
                    style="color: var(--ds-text-subtlest);"
                    onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                    onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtlest)'}
                    title={t('settings.boardConfig.dragToReorder')}
                  >
                    <GripVertical class="w-4 h-4" />
                  </div>
                  <input
                    type="text"
                    value={column.name}
                    oninput={(e) => updateColumnName(index, e.target.value)}
                    class="flex-1 px-2 py-1 border rounded text-sm font-semibold min-w-0"
                    style="border-color: var(--ds-border); color: var(--ds-text); background-color: var(--ds-surface);"
                    placeholder={t('placeholders.columnName')}
                  />
                </div>
                <button
                  onclick={() => removeColumn(index)}
                  class="p-1 rounded transition-colors flex-shrink-0"
                  style="color: #dc2626;"
                  onmouseenter={(e) => e.currentTarget.style.backgroundColor = '#dc26261A'}
                  onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
                  title={t('common.delete')}
                >
                  <Trash2 class="w-4 h-4" />
                </button>
              </div>

              <!-- Column content -->
              <div class="p-3 space-y-3">
                <!-- WIP Limit -->
                <div>
                  <label for="wip-limit-{index}" class="text-xs font-medium block mb-1" style="color: var(--ds-text);">
                    {t('settings.boardConfig.wipLimit')}
                  </label>
                  <input
                    type="number"
                    id="wip-limit-{index}"
                    value={column.wip_limit || ''}
                    oninput={(e) => updateWIPLimit(index, e.target.value)}
                    class="w-full px-2 py-1 border rounded text-sm"
                    style="border-color: var(--ds-border); color: var(--ds-text); background-color: var(--ds-surface);"
                    placeholder={t('common.none')}
                    min="1"
                  />
                </div>

                <!-- Status mapping -->
                <div>
                  <span class="text-xs font-medium block mb-2" style="color: var(--ds-text);">
                    {t('settings.boardConfig.mappedStatuses')}
                  </span>
                  <div class="space-y-1">
                    {#each statuses as status}
                      {@const isSelected = column.status_ids.includes(status.id)}
                      <button
                        onclick={() => toggleStatus(index, status.id)}
                        class="w-full px-2 py-1.5 text-xs rounded transition-colors border text-left"
                        style={isSelected
                          ? 'background-color: #3b82f61A; border-color: #3b82f6; color: #2563eb;'
                          : 'background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text-subtle);'}
                      >
                        {#if isSelected}
                          <span class="mr-1">✓</span>
                        {/if}
                        {status.name}
                      </button>
                    {/each}
                  </div>
                  {#if column.status_ids.length === 0}
                    <p class="text-xs mt-2" style="color: #ca8a04;">
                      {t('settings.boardConfig.noStatusesMapped')}
                    </p>
                  {/if}
                </div>
              </div>
            </div>
          {/each}

          <!-- Add column button -->
          <button
            onclick={addColumn}
            class="flex-shrink-0 border-2 border-dashed rounded transition-colors flex items-center justify-center"
            style="border-color: var(--ds-border); color: var(--ds-text-subtle); width: 280px; min-height: 200px;"
            onmouseenter={(e) => { e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; e.currentTarget.style.borderColor = '#60a5fa'; }}
            onmouseleave={(e) => { e.currentTarget.style.backgroundColor = ''; e.currentTarget.style.borderColor = 'var(--ds-border)'; }}
          >
            <div class="flex flex-col items-center gap-2">
              <Plus class="w-8 h-8" />
              <span class="font-medium">{t('settings.boardConfig.addColumn')}</span>
            </div>
          </button>
        </div>

        {:else if activeTab === 'backlog'}
        <!-- Backlog Tab -->
        <div class="mt-6 mb-6">
          <div class="bg-white rounded border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
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
                    ? 'background-color: #3b82f61A; border-color: #3b82f6; color: #2563eb;'
                    : 'background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text-subtle);'}
                >
                  <span class="flex-shrink-0 w-5">
                    {#if isSelected}
                      <span style="color: #2563eb;">✓</span>
                    {/if}
                  </span>
                  <span class="flex-1">{status.name}</span>
                </button>
              {/each}
            </div>

            {#if backlogStatusIDs.length === 0}
              <p class="text-sm mt-4" style="color: #ca8a04;">
                {t('settings.boardConfig.noStatusesSelected')}
              </p>
            {:else}
              <p class="text-sm mt-4" style="color: var(--ds-text-subtle);">
                {backlogStatusIDs.length} {backlogStatusIDs.length === 1 ? 'status' : 'statuses'} selected for backlog
              </p>
            {/if}
          </div>
        </div>
        {/if}

        <!-- Action buttons -->
        <div class="flex items-center justify-between border-t pt-6" style="border-color: var(--ds-border);">
          <button
            onclick={resetToDefault}
            class="px-4 py-2 text-sm rounded transition-colors"
            style="color: #dc2626;"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = '#dc26261A'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            disabled={!boardConfig && columns.length === 0}
          >
            {t('settings.boardConfig.resetToDefault')}
          </button>

          <div class="flex gap-3">
            <button
              onclick={cancelChanges}
              class="px-4 py-2 text-sm border rounded transition-colors"
              style="border-color: var(--ds-border); color: var(--ds-text);"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
              disabled={saving}
            >
              {t('common.cancel')}
            </button>
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
