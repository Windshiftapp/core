<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { ChevronDown, ChevronRight, Plus, Check, X, ExternalLink, ListTodo, CheckCircle2 } from 'lucide-svelte';
  import { slide } from 'svelte/transition';
  import { authStore, workspacesStore } from '../../stores';
  import Tooltip from '../../components/Tooltip.svelte';
  import Text from '../../components/Text.svelte';

  let {
    itemId,
    workspaceId
  } = $props();

  // Status ID constants (matching backend constants/statuses.go)
  const STATUS_ID_OPEN = 1;    // Default status for new tasks
  const STATUS_ID_CLOSED = 6;  // Status for completed tasks

  let personalTasks = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let expanded = $state(false);
  let showAddForm = $state(false);
  let newTaskTitle = $state('');
  let adding = $state(false);
  let addTaskInput = $state(null);

  // Focus input when add form is shown
  $effect(() => {
    if (showAddForm && addTaskInput) {
      addTaskInput.focus();
    }
  });
  let personalWorkspace = $derived($workspacesStore.personalWorkspace);
  let hasInitiallyLoaded = $state(false);
  let showCompleted = $state(false);

  // Storage keys
  const STORAGE_KEY_EXPANDED = 'personalTasksExpanded';
  const STORAGE_KEY_SHOW_COMPLETED = 'personalTasksShowCompleted';

  // Load expanded state from localStorage
  function loadExpandedState() {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_EXPANDED);
      return saved ? JSON.parse(saved) : false;
    } catch (err) {
      console.error('Failed to load personal tasks expanded state:', err);
      return false;
    }
  }

  // Save expanded state to localStorage
  function saveExpandedState(isExpanded) {
    try {
      localStorage.setItem(STORAGE_KEY_EXPANDED, JSON.stringify(isExpanded));
    } catch (err) {
      console.error('Failed to save personal tasks expanded state:', err);
    }
  }

  // Load show completed state from localStorage
  function loadShowCompletedState() {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_SHOW_COMPLETED);
      return saved ? JSON.parse(saved) : false;
    } catch (err) {
      console.error('Failed to load personal tasks show completed state:', err);
      return false;
    }
  }

  // Save show completed state to localStorage
  function saveShowCompletedState(show) {
    try {
      localStorage.setItem(STORAGE_KEY_SHOW_COMPLETED, JSON.stringify(show));
    } catch (err) {
      console.error('Failed to save personal tasks show completed state:', err);
    }
  }

  // Toggle expanded state and save preference
  function toggleExpanded() {
    expanded = !expanded;
    saveExpandedState(expanded);
  }

  // Toggle show completed state and save preference
  function toggleShowCompleted(e) {
    e.stopPropagation(); // Prevent expanding/collapsing the section
    showCompleted = !showCompleted;
    saveShowCompletedState(showCompleted);
  }

  // Helper function to check if task is completed
  function isTaskCompleted(task) {
    // Check status_id first (current approach)
    if (task.status_id === STATUS_ID_CLOSED) {
      return true;
    }
    // Fallback to deprecated status fields for backward compatibility
    const status = (task.status || '').toLowerCase();
    const statusName = (task.status_name || '').toLowerCase();
    return status === 'completed' || status === 'done' || status === 'closed' ||
           statusName === 'completed' || statusName === 'done' || statusName === 'closed';
  }

  // Derived state: filter tasks by completion status
  let openTasks = $derived(personalTasks.filter(task => !isTaskCompleted(task)));
  let completedTasks = $derived(personalTasks.filter(task => isTaskCompleted(task)));
  let displayedTasks = $derived(showCompleted ? personalTasks : openTasks);

  // Load personal tasks for this work item
  async function loadPersonalTasks() {
    if (!itemId) return;

    try {
      loading = true;
      error = null;
      const tasks = await api.items.getPersonalTasks(itemId);
      personalTasks = tasks || [];

      // On first load, auto-expand if there are tasks, otherwise use saved preference
      if (!hasInitiallyLoaded) {
        hasInitiallyLoaded = true;
        if (personalTasks.length > 0) {
          expanded = true;
        } else {
          expanded = loadExpandedState();
        }
      }
    } catch (err) {
      console.error('Failed to load personal tasks:', err);
      error = err.message;
      personalTasks = [];
    } finally {
      loading = false;
    }
  }

  // Create a new personal task linked to this work item
  async function handleAddTask() {
    if (!newTaskTitle.trim() || !personalWorkspace) return;

    try {
      adding = true;
      error = null;

      const newTask = await api.items.create({
        workspace_id: personalWorkspace.id,
        title: newTaskTitle.trim(),
        related_work_item_id: itemId,
        status_id: STATUS_ID_OPEN
      });

      // Add to list
      personalTasks = [...personalTasks, newTask];

      // Reset form
      newTaskTitle = '';
      showAddForm = false;
    } catch (err) {
      console.error('Failed to create personal task:', err);
      error = err.message;
    } finally {
      adding = false;
    }
  }

  // Mark task as complete
  async function handleToggleComplete(task) {
    try {
      const isDone = isTaskCompleted(task);
      const newStatusId = isDone ? STATUS_ID_OPEN : STATUS_ID_CLOSED;

      await api.items.update(task.id, { status_id: newStatusId });

      // Update in list with new status_id
      personalTasks = personalTasks.map(t =>
        t.id === task.id ? { ...t, status_id: newStatusId } : t
      );
    } catch (err) {
      console.error('Failed to update task:', err);
      error = err.message;
    }
  }

  // Unlink task from work item
  async function handleUnlink(task) {
    try {
      await api.items.unlinkPersonalTask(task.id);

      // Remove from list
      personalTasks = personalTasks.filter(t => t.id !== task.id);
    } catch (err) {
      console.error('Failed to unlink task:', err);
      error = err.message;
    }
  }

  // Navigate to task detail
  function handleNavigateToTask(task) {
    if (personalWorkspace) {
      navigate(`/personal/items/${task.id}`);
    }
  }

  onMount(() => {
    workspacesStore.loadPersonalWorkspace();
    loadPersonalTasks();
    // Load showCompleted preference
    showCompleted = loadShowCompletedState();
  });

  // Reload when itemId changes
  $effect(() => {
    if (itemId) {
      // Reset the hasInitiallyLoaded flag when itemId changes
      hasInitiallyLoaded = false;
      loadPersonalTasks();
    }
  });
</script>

<!-- Personal Tasks Section -->
<div class="mb-4">
  <!-- Divider -->
  <div class="border-t my-4" style="border-color: var(--ds-border);"></div>

  <!-- Section Header -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="w-full flex items-center justify-between mb-3 group cursor-pointer"
    onclick={toggleExpanded}
  >
    <div class="flex items-center gap-2">
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">Personal Tasks</Text>
      {#if openTasks.length > 0}
        <span class="px-1.5 py-0.5 text-xs font-medium rounded-full" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
          {openTasks.length}
        </span>
      {/if}
      {#if completedTasks.length > 0}
        <button
          class="px-1.5 py-0.5 text-xs font-medium rounded-full transition-colors"
          style="background-color: {showCompleted ? 'var(--ds-background-success-subtle)' : 'var(--ds-surface)'}; color: {showCompleted ? 'var(--ds-text-success)' : 'var(--ds-text-subtle)'};"
          onclick={toggleShowCompleted}
          title="{showCompleted ? 'Hide' : 'Show'} {completedTasks.length} completed task{completedTasks.length === 1 ? '' : 's'}"
        >
          <CheckCircle2 size={12} class="inline" />
          {completedTasks.length}
        </button>
      {/if}
    </div>
    <div class="flex items-center gap-1">
      <button
        class="p-1 rounded transition-colors opacity-0 group-hover:opacity-100"
        class:invisible={showAddForm || !expanded || !personalWorkspace}
        onclick={(e) => { e.stopPropagation(); showAddForm = true; }}
        title="Add personal task"
      >
        <Plus size={14} style="color: var(--ds-text-subtle);" />
      </button>
      {#if expanded}
        <ChevronDown size={16} style="color: var(--ds-text-subtle);" />
      {:else}
        <ChevronRight size={16} style="color: var(--ds-text-subtle);" />
      {/if}
    </div>
  </div>

  <!-- Expandable Content -->
  {#if expanded}
    <div transition:slide={{ duration: 200 }} class="mt-1">
      {#if loading}
        <div class="py-2 text-xs" style="color: var(--ds-text-subtle);">
          Loading...
        </div>
      {:else if error}
        <div class="py-2 text-xs text-red-600">
          {error}
        </div>
      {:else if !personalWorkspace}
        <div class="py-2 text-xs" style="color: var(--ds-text-subtle);">
          No personal workspace found
        </div>
      {:else}
        <!-- Add task form -->
        {#if showAddForm}
          <div class="py-2 rounded-md mb-2" style="background-color: var(--ds-surface);">
            <form onsubmit={(e) => { e.preventDefault(); handleAddTask(); }} class="flex flex-col gap-2">
              <input
                type="text"
                bind:this={addTaskInput}
                bind:value={newTaskTitle}
                placeholder="Task title..."
                class="px-2 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                disabled={adding}
              />
              <div class="flex gap-2 justify-end">
                <button
                  type="button"
                  class="px-2 py-1 text-xs rounded"
                  style="color: var(--ds-text);"
                  onclick={() => { showAddForm = false; newTaskTitle = ''; }}
                  disabled={adding}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  class="px-2 py-1 text-xs rounded disabled:opacity-50 disabled:cursor-not-allowed"
                  style="background-color: var(--ds-interactive); color: var(--ds-text-inverse);"
                  disabled={adding || !newTaskTitle.trim()}
                >
                  Add
                </button>
              </div>
            </form>
          </div>
        {/if}

        <!-- Tasks list -->
        {#if displayedTasks.length === 0 && !showAddForm}
          <div class="py-2 text-xs" style="color: var(--ds-text-subtle);">
            {#if personalTasks.length === 0}
              No tasks yet
            {:else}
              No {showCompleted ? '' : 'open '}tasks
            {/if}
          </div>
        {:else if displayedTasks.length > 0}
          <div class="space-y-0.5">
            {#each displayedTasks as task (task.id)}
              <div class="py-2 rounded-md group flex items-start gap-2 task-row">
                <!-- Complete checkbox -->
                <button
                  class="mt-0.5 flex-shrink-0"
                  onclick={() => handleToggleComplete(task)}
                  title={isTaskCompleted(task) ? 'Mark incomplete' : 'Mark complete'}
                >
                  {#if isTaskCompleted(task)}
                    <div class="w-4 h-4 bg-green-500 rounded flex items-center justify-center">
                      <Check size={12} class="text-white" strokeWidth={2.5} />
                    </div>
                  {:else}
                    <div class="w-4 h-4 border-2 rounded checkbox-unchecked" style="border-color: var(--ds-border);"></div>
                  {/if}
                </button>

                <!-- Task title -->
                <button
                  class="flex-1 text-left text-xs"
                  style="color: {isTaskCompleted(task) ? 'var(--ds-text-subtle)' : 'var(--ds-text)'}; {isTaskCompleted(task) ? 'text-decoration: line-through;' : ''}"
                  onclick={() => handleNavigateToTask(task)}
                  title="Open task"
                >
                  {task.title}
                </button>

                <!-- Actions -->
                <div class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button
                    class="p-0.5 rounded action-btn"
                    onclick={() => handleNavigateToTask(task)}
                    title="Open task"
                  >
                    <ExternalLink size={12} style="color: var(--ds-text-subtle);" />
                  </button>
                  <button
                    class="p-0.5 rounded action-btn-danger"
                    onclick={() => handleUnlink(task)}
                    title="Unlink task"
                  >
                    <X size={12} class="text-red-600" />
                  </button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .task-row:hover {
    background-color: var(--ds-surface);
  }

  .action-btn:hover {
    background-color: var(--ds-surface);
  }

  .action-btn-danger:hover {
    background-color: rgba(239, 68, 68, 0.1);
  }

  .checkbox-unchecked:hover {
    border-color: var(--ds-text-subtle) !important;
  }
</style>
