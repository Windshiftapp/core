<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { Plus, Check, X, Trash2, ChevronDown, MoreHorizontal, Calendar, Edit, ClipboardList } from 'lucide-svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import DeleteItemDialog from '../../dialogs/DeleteItemDialog.svelte';
  import { navigate } from '../../router.js';
  import ItemDetail from '../items/ItemDetail.svelte';
  import PersonalTaskDetail from '../personal/PersonalTaskDetail.svelte';
  import { getStatusStyleFromStatuses, getTextColorForBackground } from '../../utils/statusColors.js';
  import { authStore } from '../../stores';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';

  export let workspaceId;

  let personalTodos = [];
  let assignedWork = [];
  let statuses = [];
  let statusCategories = [];
  let statusesByWorkspace = new Map();
  let loading = true;
  let newTodoTitle = '';
  let isAddingTodo = false;
  let showItemModal = false;
  let selectedItemId = null;

  // Delete confirmation dialog state
  let showDeleteConfirm = false;
  let itemToDelete = null;
  let isPersonalDelete = true;

  // Status configuration for ItemPicker
  const statusConfig = {
    icon: {
      type: 'color-dot',
      source: (item) => item.categoryColor || '#9CA3AF',
      size: 'w-2 h-2'
    },
    primary: {
      text: (item) => item.label
    },
    searchFields: ['label', 'value'],
    getValue: (item) => item.id,
    getLabel: (item) => item.label
  };

  // Transform statuses for ItemPicker
  $: statusOptions = statuses.map(status => ({
    id: status.id,
    label: status.name,
    value: status.name,
    categoryColor: status.category_color
  }));

  onMount(async () => {
    await Promise.all([loadStatuses(), loadStatusCategories(), loadPersonalTodos(), loadAssignedWork()]);
    loading = false;
  });

  async function loadStatuses() {
    try {
      statuses = await api.workspaces.getStatuses(workspaceId);
    } catch (error) {
      console.error('Failed to load statuses:', error);
      statuses = [];
    }
  }

  async function loadStatusCategories() {
    try {
      statusCategories = await api.statusCategories.getAll();
    } catch (error) {
      console.error('Failed to load status categories:', error);
      statusCategories = [];
    }
  }

  async function loadPersonalTodos() {
    try {
      const filters = { 
        workspace_id: workspaceId,
        limit: 100
      };
      const response = await api.items.getAll(filters);
      personalTodos = response?.items || response || [];
    } catch (error) {
      console.error('Failed to load personal todos:', error);
      personalTodos = [];
    }
  }

  async function loadAssignedWork() {
    try {
      // Get the current authenticated user's ID
      const user = authStore.currentUser;
      if (!user || !user.id) {
        console.warn('No authenticated user found for loading assigned work');
        assignedWork = [];
        return;
      }

      const filters = {
        assignee_id: user.id,
        limit: 100
      };
      const response = await api.items.getAll(filters);
      let allAssigned = response?.items || response || [];

      // Filter out items from personal workspace to avoid duplicates
      assignedWork = allAssigned.filter(item => item.workspace_id !== parseInt(workspaceId));
    } catch (error) {
      console.error('Failed to load assigned work:', error);
      assignedWork = [];
    }
  }

  function startAddingTodo() {
    isAddingTodo = true;
    newTodoTitle = '';
    // Focus the input after DOM update
    setTimeout(() => {
      document.getElementById('new-todo-input')?.focus();
    }, 10);
  }

  function cancelAddingTodo() {
    isAddingTodo = false;
    newTodoTitle = '';
  }

  async function saveTodo() {
    if (!newTodoTitle.trim()) return;
    
    try {
      // Find default status (should be "Open")
      const defaultStatus = statuses.find(s => s.is_default) || statuses.find(s => s.name.toLowerCase() === 'open') || statuses[0];
      
      const todoData = {
        title: newTodoTitle.trim(),
        description: '',
        workspace_id: parseInt(workspaceId),
        status_id: defaultStatus?.id || 1
      };
      
      await api.items.create(todoData);
      await loadPersonalTodos();
      cancelAddingTodo();
    } catch (error) {
      console.error('Failed to create todo:', error);
      alert(t('todo.failedToCreate') + ': ' + (error.message || error));
    }
  }

  async function changeItemStatus(item, newStatusId, isPersonal = true) {
    try {
      await api.items.update(item.id, { status_id: newStatusId });
      
      if (isPersonal) {
        await loadPersonalTodos();
      } else {
        await loadAssignedWork();
      }
    } catch (error) {
      console.error('Failed to update item status:', error);
    }
  }

  function getStatusesForWorkspace(workspaceId) {
    // For now, return all statuses since we don't have workspace-specific status configuration
    // In a real system, this would filter based on workspace configuration
    return statuses;
  }

  function getStatusName(statusId) {
    const statusObj = statuses.find(s => s.id === statusId);
    return statusObj?.name || 'Open';
  }

  function getStatusById(statusId) {
    return statuses.find(s => s.id === statusId);
  }

  function getStatusCategory(statusName) {
    const status = statuses.find(s => s.name.toLowerCase() === statusName?.toLowerCase());
    return statusCategories.find(c => c.id === status?.category_id);
  }


  // Column definitions for assigned work DataTable
  $: assignedWorkColumns = [
    {
      key: 'title',
      label: t('common.title'),
      width: '40%',
      slot: 'name'
    },
    {
      key: 'workspace_name',
      label: t('workspaces.workspace'),
      width: '20%'
    },
    {
      key: 'status',
      label: t('common.status'),
      width: '15%',
      slot: 'status'
    },
    {
      key: 'created_at',
      label: t('common.created'),
      width: '15%',
      slot: 'date'
    },
    {
      key: 'actions',
      label: t('common.actions'),
      width: '10%'
    }
  ];

  function buildItemActions(item) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        onClick: () => openItem(item.id)
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50 hover:text-red-700',
        onClick: () => deleteTodo(item, false)
      }
    ];
  }

  // Personal task helpers
  function isPersonalTaskCompleted(todo) {
    const status = statuses.find(s => s.id === todo.status_id);
    return status?.category_name === 'Done' || status?.name.toLowerCase().includes('complete') || status?.name.toLowerCase().includes('done');
  }

  async function togglePersonalTask(todo) {
    try {
      let targetStatusId;
      
      if (isPersonalTaskCompleted(todo)) {
        // If completed, move to "Open" or first non-done status
        const openStatus = statuses.find(s => s.name.toLowerCase() === 'open') || 
                          statuses.find(s => s.category_name !== 'Done') || 
                          statuses[0];
        targetStatusId = openStatus.id;
      } else {
        // If not completed, move to "Done" or first done status
        const doneStatus = statuses.find(s => s.category_name === 'Done') ||
                          statuses.find(s => s.name.toLowerCase().includes('done')) ||
                          statuses.find(s => s.name.toLowerCase().includes('complete'));
        targetStatusId = doneStatus?.id;
      }
      
      if (targetStatusId) {
        await changeItemStatus(todo, targetStatusId, true);
      }
    } catch (error) {
      console.error('Failed to toggle personal task:', error);
    }
  }

  function openItem(itemId) {
    selectedItemId = itemId;
    showItemModal = true;
  }

  function closeItemModal() {
    showItemModal = false;
    selectedItemId = null;
  }

  function isPersonalWorkspaceItem(itemId) {
    return personalTodos.some(todo => todo.id === itemId);
  }

  function handleItemUpdate() {
    loadPersonalTodos();
    loadAssignedWork();
  }

  function getWorkspaceIdForItem(itemId) {
    // Check if it's a personal todo
    const personalTodo = personalTodos.find(todo => todo.id === itemId);
    if (personalTodo) {
      // Use the workspace_id from the item itself, not the current workspaceId
      return personalTodo.workspace_id || parseInt(workspaceId);
    }
    
    // Check if it's an assigned work item
    const assignedItem = assignedWork.find(item => item.id === itemId);
    if (assignedItem) {
      return assignedItem.workspace_id; // Use the item's workspace ID
    }
    
    // Fallback to current workspace (ensure it's a number)
    return parseInt(workspaceId);
  }


  function deleteTodo(todo, isPersonal = true) {
    itemToDelete = todo;
    isPersonalDelete = isPersonal;
    showDeleteConfirm = true;
  }

  async function handleDeleteComplete(result) {
    // Reload the appropriate list after deletion
    if (isPersonalDelete) {
      await loadPersonalTodos();
    } else {
      await loadAssignedWork();
    }
    itemToDelete = null;
  }

  function handleDeleteError(error) {
    console.error('Failed to delete todo:', error);
  }

  function handleKeydown(event) {
    if (event.key === 'Enter') {
      saveTodo();
    } else if (event.key === 'Escape') {
      cancelAddingTodo();
    }
  }
</script>

<div style="background-color: var(--ds-surface);">
  <div class="p-6">
    {#if loading}
      <div class="rounded-xl border shadow-sm p-8 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('todo.loadingTasks')}</div>
      </div>
    {:else}
      <div class="flex flex-col gap-6 max-w-4xl">
        <!-- Personal Tasks -->
        <div class="rounded border shadow-sm overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <!-- Header -->
          <div class="px-5 py-4 border-b flex items-center" style="border-color: var(--ds-border);">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('todo.myPersonalTasks')}</h2>
          </div>

          <!-- Content -->
          <div class="p-5">
            <!-- Add Todo Section -->
            <div class="mb-4">
              {#if isAddingTodo}
                <div class="flex items-center gap-3 p-3 border rounded" style="border-color: var(--ds-interactive); background-color: var(--ds-background-selected);">
                  <input
                    id="new-todo-input"
                    type="text"
                    bind:value={newTodoTitle}
                    onkeydown={handleKeydown}
                    placeholder={t('todo.whatNeedsToBeDone')}
                    class="flex-1 px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                    style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border);"
                  />
                  <button
                    onclick={saveTodo}
                    disabled={!newTodoTitle.trim()}
                    class="p-2 text-green-600 hover:text-green-700 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed add-btn"
                  >
                    <Check class="w-5 h-5" />
                  </button>
                  <button
                    onclick={cancelAddingTodo}
                    class="p-2 rounded transition-colors cancel-btn"
                    style="color: var(--ds-text-subtle);"
                  >
                    <X class="w-5 h-5" />
                  </button>
                </div>
              {:else}
                <button
                  onclick={startAddingTodo}
                  class="w-full flex items-center gap-3 p-3 border-2 border-dashed rounded transition-colors add-task-btn"
                  style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
                >
                  <Plus class="w-5 h-5" />
                  {t('todo.addPersonalTask')}
                </button>
              {/if}
            </div>

            <!-- Personal Todo List -->
            {#if personalTodos.length === 0}
              <div class="text-center py-8" style="color: var(--ds-text-subtle);">
                <div class="text-sm font-medium mb-1">{t('todo.noPersonalTasks')}</div>
                <div class="text-xs">{t('todo.addFirstTask')}</div>
              </div>
            {:else}
              <div class="space-y-2">
                {#each personalTodos as todo (todo.id)}
                  <div class="group flex items-center gap-3 p-3 border rounded transition-colors todo-row" style="border-color: var(--ds-border);">
                    <!-- Simple Checkbox -->
                    <Checkbox
                      checked={isPersonalTaskCompleted(todo)}
                      onchange={() => togglePersonalTask(todo)}
                      size="small"
                    />

                    <!-- Todo Content with Key -->
                    <div class="flex-1 min-w-0 cursor-pointer flex items-center gap-2" onclick={() => openItem(todo.id)}>
                      <button
                        onclick={() => openItem(todo.id)}
                        class="text-xs font-mono px-1.5 py-0.5 rounded whitespace-nowrap flex-shrink-0 transition-colors item-key"
                        style="color: var(--ds-text-subtle); background-color: var(--ds-surface);"
                        title={t('items.clickToViewDetails')}
                      >
                        {todo.workspace_key || 'WORK'}-{todo.id}
                      </button>
                      <div class="flex-1 min-w-0 font-medium" style="color: {isPersonalTaskCompleted(todo) ? 'var(--ds-text-subtle)' : 'var(--ds-text)'}; {isPersonalTaskCompleted(todo) ? 'text-decoration: line-through;' : ''}">
                        {todo.title}
                      </div>
                    </div>

                    <!-- Actions -->
                    <div class="flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onclick={() => deleteTodo(todo, true)}
                        class="p-1 text-red-500 hover:text-red-700 rounded transition-colors delete-btn"
                      >
                        <Trash2 class="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>

              <!-- Personal Tasks Summary -->
              <div class="mt-4 pt-3 border-t text-xs text-center" style="border-color: var(--ds-border); color: var(--ds-text-subtle);">
                {t('todo.ofPersonalTasksRemaining', {
                  count: personalTodos.filter(t => {
                    const status = statuses.find(s => s.id === t.status_id);
                    return status?.category_name !== 'Done';
                  }).length,
                  total: personalTodos.length
                })}
              </div>
            {/if}
          </div>
        </div>

        <!-- Assigned Work Items -->
        <div>
          <!-- Header -->
          <div class="px-5 py-4 border border-b-0 rounded-t flex items-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('todo.assignedToMe')}</h2>
          </div>

          <!-- DataTable -->
          <DataTable
            columns={assignedWorkColumns}
            data={assignedWork}
            keyField="id"
            emptyIcon={ClipboardList}
            emptyMessage={t('todo.noAssignedWork')}
            emptyDescription={t('todo.assignedItemsWillAppear')}
            actionItems={buildItemActions}
            onRowClick={(item) => openItem(item.id)}
            class="rounded-t-none border border-t-0 shadow-sm overflow-hidden"
          >
            {#snippet name({ item })}
              <div class="flex items-center gap-2 min-w-0">
                <button
                  onclick={(e) => { e.stopPropagation(); openItem(item.id); }}
                  class="text-xs font-mono px-1.5 py-0.5 rounded whitespace-nowrap flex-shrink-0 transition-colors cursor-pointer item-key"
                  style="color: var(--ds-text-subtle); background-color: var(--ds-surface);"
                  title={t('items.clickToViewDetails')}
                >
                  {item.workspace_key || 'WORK'}-{item.id}
                </button>
                <span class="font-medium truncate" style="color: var(--ds-text);">
                  {item.title}
                </span>
              </div>
            {/snippet}

            {#snippet status({ item })}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div onclick={(e) => e.stopPropagation()}>
              {#each [getStatusCategory(getStatusName(item.status_id))] as statusCategory}
                <ItemPicker
                  value={item.status_id ?? null}
                  items={statusOptions}
                  config={statusConfig}
                  placeholder={t('common.select')}
                  showUnassigned={false}
                  on:select={async (e) => {
                    const selectedStatus = e.detail;
                    if (selectedStatus?.id && selectedStatus.id !== item.status_id) {
                      await changeItemStatus(item, selectedStatus.id, false);
                    }
                  }}
                >
                  {#snippet children()}
                    <span
                      class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium cursor-pointer transition-colors max-w-full truncate"
                      style={statusCategory && statusCategory.color ? `background-color: ${statusCategory.color}; color: ${getTextColorForBackground(statusCategory.color)};` : getStatusStyleFromStatuses(getStatusName(item.status_id), statuses)}
                    >
                      {getStatusName(item.status_id)}
                    </span>
                  {/snippet}
                </ItemPicker>
              {/each}
              </div>
            {/snippet}

            {#snippet date({ item })}
              <div class="flex items-center gap-1 text-sm whitespace-nowrap" style="color: var(--ds-text-subtle);">
                <Calendar class="w-4 h-4" />
                {formatDate(item.created_at) || '-'}
              </div>
            {/snippet}
          </DataTable>
        </div>
      </div>
    {/if}
  </div>
</div>

<!-- Item Detail Modal -->
{#if showItemModal && selectedItemId}
  {#if isPersonalWorkspaceItem(selectedItemId)}
    <PersonalTaskDetail
      itemId={selectedItemId}
      workspaceId={getWorkspaceIdForItem(selectedItemId)}
      {statuses}
      on:close={closeItemModal}
      on:update={handleItemUpdate}
    />
  {:else}
    <ItemDetail
      workspaceId={getWorkspaceIdForItem(selectedItemId)}
      itemId={selectedItemId}
      isModal={true}
      onclose={closeItemModal}
    />
  {/if}
{/if}

<!-- Delete Confirmation Dialog -->
<DeleteItemDialog
  bind:show={showDeleteConfirm}
  item={itemToDelete}
  ondeleted={handleDeleteComplete}
  onerror={handleDeleteError}
/>

<style>
  .add-btn:hover {
    background-color: rgba(22, 163, 74, 0.1);
  }

  .cancel-btn:hover {
    background-color: var(--ds-surface);
  }

  .add-task-btn:hover {
    border-color: var(--ds-interactive) !important;
    color: var(--ds-interactive) !important;
  }

  .todo-row:hover {
    background-color: var(--ds-surface);
  }

  .item-key:hover {
    background-color: var(--ds-surface) !important;
    color: var(--ds-text) !important;
  }

  .delete-btn:hover {
    background-color: rgba(239, 68, 68, 0.1);
  }
</style>

