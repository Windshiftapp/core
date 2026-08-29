<script>
  import DashboardItemRow from './DashboardItemRow.svelte';

  // Shared skeleton / error / empty / list rendering for the task widgets
  // (assigned-to-me and personal tasks). Only the icon and messages differ.
  let {
    loading = false,
    errored = false,
    tasks = [],
    icon: Icon,
    errorMessage = '',
    emptyMessage = '',
    density,
    openTask,
  } = $props();
</script>

{#if loading && tasks.length === 0}
  <div class="space-y-2 animate-pulse">
    {#each Array(3) as _}
      <div class="h-11 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if errored}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Icon class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">{errorMessage}</p>
  </div>
{:else if tasks.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Icon class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">{emptyMessage}</p>
  </div>
{:else}
  <ul class="flex flex-col gap-1.5">
    {#each tasks as task (task.id)}
      <li>
        <DashboardItemRow
          title={task.title}
          itemKey={`${task.workspace_key}-${task.workspace_item_number}`}
          statusName={task.status_name}
          statusBuiltinKey={task.status_builtin_key}
          statusColor={task.status_color}
          priorityName={task.priority_name}
          priorityBuiltinKey={task.priority_builtin_key}
          priorityColor={task.priority_color}
          dueDate={task.dueDate}
          {density}
          onclick={() => openTask(task)}
        />
      </li>
    {/each}
  </ul>
{/if}
