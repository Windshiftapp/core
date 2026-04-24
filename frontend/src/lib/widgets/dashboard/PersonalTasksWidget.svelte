<script>
  import { ListChecks } from 'lucide-svelte';
  import { authStore } from '../../stores';
  import { api } from '../../api.js';
  import { formatDueDate, getDueBadgeClass } from '../../utils/dateFormatter.js';
  import { navigate } from '../../router.js';

  let tasks = $state([]);
  let loading = $state(false);
  let errored = $state(false);
  let personalWorkspaceId = $state(null);
  let fetchVersion = 0;
  let lastUserId = null;

  const currentUserId = $derived($authStore?.currentUser?.id ?? null);

  $effect(() => {
    if (currentUserId && currentUserId !== lastUserId) {
      lastUserId = currentUserId;
      personalWorkspaceId = null;
      load();
    } else if (!currentUserId && lastUserId !== null) {
      lastUserId = null;
      tasks = [];
      personalWorkspaceId = null;
    }
  });

  async function load() {
    const version = ++fetchVersion;
    loading = true;
    errored = false;
    try {
      if (!personalWorkspaceId) {
        const ws = await api.workspaces.getOrCreatePersonal();
        if (version !== fetchVersion) return;
        personalWorkspaceId = ws?.id ?? null;
      }
      if (!personalWorkspaceId) {
        tasks = [];
        return;
      }
      const response = await api.items.getAll({
        ql: `workspace_id = ${personalWorkspaceId}`,
        limit: 30,
        order_by: 'updated_at',
      });
      if (version !== fetchVersion) return;
      const raw = Array.isArray(response) ? response : (response?.items ?? []);
      const active = raw
        .filter((i) => i && i.id && !i.completed_at)
        .map((i) => ({
          ...i,
          dueDate: i.due_date ? new Date(i.due_date) : null,
        }));
      active.sort((a, b) => {
        if (a.dueDate && b.dueDate) return a.dueDate - b.dueDate;
        if (a.dueDate) return -1;
        if (b.dueDate) return 1;
        return 0;
      });
      tasks = active.slice(0, 6);
    } catch (err) {
      if (version !== fetchVersion) return;
      console.error('Failed to load personal tasks:', err);
      errored = true;
      tasks = [];
    } finally {
      if (version === fetchVersion) loading = false;
    }
  }

  function open(task) {
    navigate(`/workspaces/${task.workspace_id}/items/${task.id}`);
  }
</script>

{#if loading && tasks.length === 0}
  <div class="space-y-2 animate-pulse">
    {#each Array(3) as _}
      <div class="h-11 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if errored}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <ListChecks class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">Couldn't load your personal tasks</p>
  </div>
{:else if tasks.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <ListChecks class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">Your personal todo list is empty</p>
  </div>
{:else}
  <ul class="flex flex-col gap-1.5">
    {#each tasks as task (task.id)}
      <li>
        <button
          class="w-full flex items-center justify-between gap-3 p-2 rounded border text-left transition-colors"
          style="border-color: var(--ds-border); background-color: var(--ds-surface);"
          onmouseenter={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
          onmouseleave={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-surface)')}
          onclick={() => open(task)}
        >
          <div class="min-w-0 flex-1">
            <p class="text-sm truncate" style="color: var(--ds-text);">{task.title}</p>
            <p class="text-[0.7rem] mt-0.5" style="color: var(--ds-text-subtle);">
              {task.workspace_key}-{task.workspace_item_number}
              {#if task.status_name}· {task.status_name}{/if}
            </p>
          </div>
          {#if task.dueDate}
            <span
              class={`inline-flex items-center rounded-full px-2 py-0.5 text-[0.65rem] font-semibold ${getDueBadgeClass(task.dueDate)}`}
            >
              {formatDueDate(task.dueDate)}
            </span>
          {/if}
        </button>
      </li>
    {/each}
  </ul>
{/if}
