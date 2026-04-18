<script>
  import { onMount } from 'svelte';
  import { slide } from 'svelte/transition';
  import { api } from '../../api.js';
  import { Plus, Check, X, Trash2, ChevronDown, ChevronRight } from 'lucide-svelte';
  import WorkItemRow from './WorkItemRow.svelte';
  import DeleteItemDialog from '../../dialogs/DeleteItemDialog.svelte';
  import ItemDetail from '../items/ItemDetail.svelte';
  import PersonalTaskDetail from '../personal/PersonalTaskDetail.svelte';
  import { authStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';
  import EmptyState from '../../components/EmptyState.svelte';

  let { workspaceId } = $props();

  let personalTodos = $state([]);
  let assignedWork = $state([]);
  let statuses = $state([]);
  let statusCategories = $state([]);
  let loading = $state(true);
  let newTodoTitle = $state('');
  let isAddingTodo = $state(false);
  let showItemModal = $state(false);
  let selectedItemId = $state(null);

  // Delete confirmation dialog state
  let showDeleteConfirm = $state(false);
  let itemToDelete = $state(null);
  let isPersonalDelete = $state(true);

  // Collapsible section state with localStorage persistence
  const COLLAPSED_KEY = `todo-collapsed-${workspaceId}`;
  let personalCollapsed = $state(false);
  let assignedCollapsed = $state(false);

  function loadCollapsedState() {
    try {
      const saved = localStorage.getItem(COLLAPSED_KEY);
      if (saved) {
        const parsed = JSON.parse(saved);
        personalCollapsed = parsed.personal ?? false;
        assignedCollapsed = parsed.assigned ?? false;
      }
    } catch { /* ignore */ }
  }

  function persistCollapsedState() {
    try {
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify({
        personal: personalCollapsed,
        assigned: assignedCollapsed
      }));
    } catch { /* ignore */ }
  }

  function togglePersonalCollapsed() {
    personalCollapsed = !personalCollapsed;
    persistCollapsedState();
  }

  function toggleAssignedCollapsed() {
    assignedCollapsed = !assignedCollapsed;
    persistCollapsedState();
  }

  onMount(() => {
    loadCollapsedState();
    Promise.all([loadStatuses(), loadStatusCategories(), loadPersonalTodos(), loadAssignedWork()]).then(() => {
      loading = false;
    });
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
      const user = authStore.currentUser;
      if (!user || !user.id) {
        assignedWork = [];
        return;
      }

      const filters = {
        assignee_id: user.id,
        limit: 100
      };
      const response = await api.items.getAll(filters);
      let allAssigned = response?.items || response || [];
      assignedWork = allAssigned.filter(item => item.workspace_id !== parseInt(workspaceId));
    } catch (error) {
      console.error('Failed to load assigned work:', error);
      assignedWork = [];
    }
  }

  function startAddingTodo() {
    isAddingTodo = true;
    newTodoTitle = '';
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

  function isPersonalTaskCompleted(todo) {
    const status = statuses.find(s => s.id === todo.status_id);
    return status?.category_name === 'Done' || status?.name.toLowerCase().includes('complete') || status?.name.toLowerCase().includes('done');
  }

  async function togglePersonalTask(todo) {
    try {
      let targetStatusId;

      if (isPersonalTaskCompleted(todo)) {
        const openStatus = statuses.find(s => s.name.toLowerCase() === 'open') ||
                          statuses.find(s => s.category_name !== 'Done') ||
                          statuses[0];
        targetStatusId = openStatus.id;
      } else {
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
    const personalTodo = personalTodos.find(todo => todo.id === itemId);
    if (personalTodo) {
      return personalTodo.workspace_id || parseInt(workspaceId);
    }

    const assignedItem = assignedWork.find(item => item.id === itemId);
    if (assignedItem) {
      return assignedItem.workspace_id;
    }

    return parseInt(workspaceId);
  }

  function deleteTodo(todo, isPersonal = true) {
    itemToDelete = todo;
    isPersonalDelete = isPersonal;
    showDeleteConfirm = true;
  }

  async function handleDeleteComplete(result) {
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

  let personalRemaining = $derived(personalTodos.filter(todo => {
    const status = statuses.find(s => s.id === todo.status_id);
    return status?.category_name !== 'Done';
  }).length);
</script>

<div style="background-color: var(--ds-surface);">
  <div class="p-6">
    {#if loading}
      <div class="text-center py-12 animate-pulse" style="color: var(--ds-text-subtle);">{t('todo.loadingTasks')}</div>
    {:else}
      <div class="flex flex-col gap-4">
        <!-- Personal Tasks Section -->
        <div>
          <button
            class="w-full flex items-center gap-2 px-3 py-2 rounded-lg transition-colors select-none section-header"
            onclick={togglePersonalCollapsed}
          >
            <span class="flex-shrink-0" style="color: var(--ds-text-subtle);">
              {#if personalCollapsed}
                <ChevronRight class="w-4 h-4" />
              {:else}
                <ChevronDown class="w-4 h-4" />
              {/if}
            </span>
            <span class="font-semibold text-sm" style="color: var(--ds-text);">{t('todo.myPersonalTasks')}</span>
            <span class="ml-auto text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);">
              {personalTodos.length} {personalTodos.length === 1 ? 'item' : 'items'}
            </span>
          </button>

          {#if !personalCollapsed}
            <div transition:slide={{ duration: 200 }} class="mt-1">
              <!-- Add Todo -->
              <div class="mb-2 px-1">
                {#if isAddingTodo}
                  <div class="flex items-center gap-3 p-3 border rounded-lg" style="border-color: var(--ds-interactive); background-color: var(--ds-background-selected);">
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
                    class="w-full flex items-center gap-3 p-3 border-2 border-dashed rounded-lg transition-colors add-task-btn"
                    style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
                  >
                    <Plus class="w-5 h-5" />
                    {t('todo.addPersonalTask')}
                  </button>
                {/if}
              </div>

              <!-- Personal Todo List -->
              {#if personalTodos.length === 0}
                <EmptyState
                  title={t('todo.noPersonalTasks')}
                  description={t('todo.addFirstTask')}
                />
              {:else}
                <div class="flex flex-col gap-1 px-1">
                  {#each personalTodos as todo (todo.id)}
                    <WorkItemRow
                      item={todo}
                      {statuses}
                      {statusCategories}
                      showIcon={false}
                      showStatus={false}
                      onclick={() => openItem(todo.id)}
                    >
                      {#snippet leading()}
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div onclick={(e) => e.stopPropagation()}>
                          <Checkbox
                            checked={isPersonalTaskCompleted(todo)}
                            onchange={() => togglePersonalTask(todo)}
                            size="small"
                          />
                        </div>
                      {/snippet}
                      {#snippet trailing()}
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div class="flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity" onclick={(e) => e.stopPropagation()}>
                          <button
                            onclick={() => deleteTodo(todo, true)}
                            class="p-1 text-red-500 hover:text-red-700 rounded transition-colors delete-btn"
                          >
                            <Trash2 class="w-4 h-4" />
                          </button>
                        </div>
                      {/snippet}
                    </WorkItemRow>
                  {/each}
                </div>

                <!-- Summary -->
                <div class="mt-3 pt-2 border-t text-xs text-center mx-1" style="border-color: var(--ds-border); color: var(--ds-text-subtle);">
                  {t('todo.ofPersonalTasksRemaining', {
                    count: personalRemaining,
                    total: personalTodos.length
                  })}
                </div>
              {/if}
            </div>
          {/if}
        </div>

        <!-- Assigned to Me Section -->
        <div>
          <button
            class="w-full flex items-center gap-2 px-3 py-2 rounded-lg transition-colors select-none section-header"
            onclick={toggleAssignedCollapsed}
          >
            <span class="flex-shrink-0" style="color: var(--ds-text-subtle);">
              {#if assignedCollapsed}
                <ChevronRight class="w-4 h-4" />
              {:else}
                <ChevronDown class="w-4 h-4" />
              {/if}
            </span>
            <span class="font-semibold text-sm" style="color: var(--ds-text);">{t('todo.assignedToMe')}</span>
            <span class="ml-auto text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);">
              {assignedWork.length} {assignedWork.length === 1 ? 'item' : 'items'}
            </span>
          </button>

          {#if !assignedCollapsed}
            <div transition:slide={{ duration: 200 }} class="mt-1">
              {#if assignedWork.length === 0}
                <EmptyState
                  title={t('todo.noAssignedWork')}
                  description={t('todo.assignedItemsWillAppear')}
                />
              {:else}
                <div class="flex flex-col gap-1 px-1">
                  {#each assignedWork as item (item.id)}
                    <WorkItemRow
                      {item}
                      {statuses}
                      {statusCategories}
                      showWorkspace={true}
                      showStatus={true}
                      onclick={() => openItem(item.id)}
                    >
                      {#snippet trailing()}
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div class="flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity" onclick={(e) => e.stopPropagation()}>
                          <button
                            onclick={() => deleteTodo(item, false)}
                            class="p-1 text-red-500 hover:text-red-700 rounded transition-colors delete-btn"
                          >
                            <Trash2 class="w-4 h-4" />
                          </button>
                        </div>
                      {/snippet}
                    </WorkItemRow>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
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
      onclose={closeItemModal}
      onupdate={handleItemUpdate}
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
  .section-header:hover {
    background-color: rgba(0, 0, 0, 0.05);
  }

  :global(.dark) .section-header:hover {
    background-color: rgba(255, 255, 255, 0.05);
  }

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

  .delete-btn:hover {
    background-color: rgba(239, 68, 68, 0.1);
  }
</style>
