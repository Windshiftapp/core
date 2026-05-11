<script>
  import { Eye } from '@lucide/svelte';
  import { homepageStore } from '../../stores';
  import { navigate } from '../../router.js';
  import DashboardItemRow from './DashboardItemRow.svelte';

  let items = $derived(homepageStore.watchedItems);
  let loading = $derived(homepageStore.loading);

  function open(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.item_id}`);
  }
</script>

{#if loading && items.length === 0}
  <div class="space-y-2 animate-pulse">
    {#each Array(3) as _}
      <div class="h-11 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if items.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Eye class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">You aren't watching any items</p>
  </div>
{:else}
  <ul class="flex flex-col gap-1.5">
    {#each items.slice(0, 6) as item (item.item_id)}
      <li>
        <DashboardItemRow
          title={item.title}
          itemKey={`${item.workspace_key}-${item.workspace_item_number}`}
          statusName={item.status}
          statusColor={item.status_color}
          priorityName={item.priority_name}
          priorityColor={item.priority_color}
          timestamp={item.last_activity ? homepageStore.formatRelativeTime(item.last_activity) : null}
          onclick={() => open(item)}
        />
      </li>
    {/each}
  </ul>
{/if}
