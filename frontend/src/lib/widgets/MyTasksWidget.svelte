<script>
  import { CheckSquare } from 'lucide-svelte';
  import { authStore } from '../stores';
  import { api } from '../api.js';
  import { formatDueDate, getDueBadgeClass } from '../utils/dateFormatter.js';
  import WidgetState from './WidgetState.svelte';

  export let workspaceId = null;
  export let maxItems = 8;

  let tasks = [];
  let loading = false;
  let error = null;
  let fetchVersion = 0;
  let lastFetchKey = null;

  $: currentUserId = $authStore?.currentUser?.id ?? null;
  $: fetchKey = currentUserId ? `${currentUserId}` : null;

  $: if (fetchKey && fetchKey !== lastFetchKey) {
    lastFetchKey = fetchKey;
    loadAssignedTasks(currentUserId);
  } else if (!fetchKey && lastFetchKey !== null) {
    lastFetchKey = null;
    tasks = [];
    loading = false;
    error = null;
  }

  async function loadAssignedTasks(userId) {
    const currentVersion = ++fetchVersion;
    loading = true;
    error = null;

    try {
      const response = await api.items.getAll({
        assignee_id: userId,
        limit: maxItems * 3,
        order_by: 'created_at'
      });

      if (currentVersion !== fetchVersion) return;

      const rawItems = Array.isArray(response)
        ? response
        : (response?.items ?? []);

      const normalized = rawItems
        .filter(item => item && item.id)
        .map(item => ({
          ...item,
          dueDate: item.due_date ? new Date(item.due_date) : null,
          updatedDate: item.updated_at ? new Date(item.updated_at) : null
        }));

      const active = normalized.filter(item => !item.completed_at);

      active.sort((a, b) => {
        if (a.dueDate && b.dueDate) return a.dueDate - b.dueDate;
        if (a.dueDate) return -1;
        if (b.dueDate) return 1;
        if (a.updatedDate && b.updatedDate) return b.updatedDate - a.updatedDate;
        return 0;
      });

      tasks = active.slice(0, maxItems);
    } catch (err) {
      if (currentVersion !== fetchVersion) return;
      console.error('Failed to load My Tasks widget:', err);
      error = 'Unable to load tasks';
      tasks = [];
    } finally {
      if (currentVersion === fetchVersion) {
        loading = false;
      }
    }
  }
</script>

<WidgetState
  {loading}
  {error}
  isEmpty={tasks.length === 0}
  loadingText="Loading your tasks..."
  emptyIcon={CheckSquare}
  emptyTitle="No open tasks assigned to you"
  emptySubtitle="You're all caught up!"
  onRetry={() => fetchKey && loadAssignedTasks(currentUserId)}
>
  {#snippet children()}
    <div class="flex flex-col gap-2">
      {#each tasks as task}
        <a
          class="flex items-center justify-between gap-4 rounded-xl border border-gray-200 px-4 py-3 transition hover:-translate-y-px hover:border-blue-200 hover:shadow-sm"
          href={`/workspaces/${task.workspace_id}/items/${task.id}`}
        >
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-semibold text-slate-900">{task.title}</p>
            <p class="mt-0.5 flex items-center gap-1 text-xs text-slate-500">
              <span>{task.workspace_key}-{task.workspace_item_number}</span>
              {#if task.status_name}
                <span aria-hidden="true">•</span>
                <span>{task.status_name}</span>
              {/if}
            </p>
          </div>
          <div class="flex flex-col items-end gap-1 text-xs">
            <span class={`inline-flex items-center rounded-full px-2 py-0.5 font-semibold ${getDueBadgeClass(task.dueDate)}`}>
              {formatDueDate(task.dueDate)}
            </span>
            {#if task.priority_name}
              <span class="uppercase tracking-wide text-[0.65rem] text-gray-500">
                {task.priority_name}
              </span>
            {/if}
          </div>
        </a>
      {/each}
    </div>
  {/snippet}
</WidgetState>
