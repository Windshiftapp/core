<script>
  import Eye from '@lucide/svelte/icons/eye';
  import { homepageStore } from '../../stores/homepageStore.svelte.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import DashboardItemRow from './DashboardItemRow.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let items = $derived(homepageStore.watchedItems);
  let loading = $derived(homepageStore.loading);

  function open(item) {
    itemModalStore.open(item.item_id, item.workspace_id);
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
    <p class="text-sm">{t('dashboard.states.watchedItemsEmpty')}</p>
  </div>
{:else}
  <ul class="flex flex-col gap-1.5">
    {#each items.slice(0, 6) as item (item.item_id)}
      <li>
        <DashboardItemRow
          title={item.title}
          itemKey={`${item.workspace_key}-${item.workspace_item_number}`}
          itemTypeName={item.item_type_name}
          itemTypeIcon={item.item_type_icon}
          itemTypeColor={item.item_type_color}
          href={`/workspaces/${item.workspace_id}/items/${item.item_id}`}
          statusName={item.status}
          statusBuiltinKey={item.status_builtin_key}
          statusColor={item.status_color}
          priorityName={item.priority_name}
          priorityBuiltinKey={item.priority_builtin_key}
          priorityColor={item.priority_color}
          timestamp={item.last_active_at
            ? homepageStore.formatRelativeTime(item.last_active_at)
            : null}
          onclick={() => open(item)}
        />
      </li>
    {/each}
  </ul>
{/if}
