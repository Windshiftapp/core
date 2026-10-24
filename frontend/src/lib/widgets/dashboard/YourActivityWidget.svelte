<script>
  import Clock from '@lucide/svelte/icons/clock';
  import Edit from '@lucide/svelte/icons/edit';
  import MessageSquare from '@lucide/svelte/icons/message-square';
  import Eye from '@lucide/svelte/icons/eye';
  import { homepageStore } from '../../stores/homepageStore.svelte.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import DashboardItemRow from './DashboardItemRow.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let activeTab = $derived(homepageStore.activeTab);
  let loading = $derived(homepageStore.loading);

  let items = $derived.by(() => {
    if (activeTab === 'edited') return homepageStore.recentlyEdited;
    if (activeTab === 'commented') return homepageStore.recentlyCommented;
    return homepageStore.recentlyViewed;
  });

  const tabs = [
    { id: 'viewed', labelKey: 'dashboard.viewed', icon: Eye },
    { id: 'edited', labelKey: 'dashboard.edited', icon: Edit },
    { id: 'commented', labelKey: 'dashboard.commented', icon: MessageSquare },
  ];

  function open(item) {
    itemModalStore.open(item.item_id, item.workspace_id);
  }

  function itemKey(item) {
    if (item.workspace_key == null || item.workspace_item_number == null) return null;
    return `${item.workspace_key}-${item.workspace_item_number}`;
  }
</script>

<div class="flex flex-col gap-3">
  <div class="flex gap-1 p-0.5 rounded" style="background-color: var(--ds-background-neutral);">
    {#each tabs as tab}
      {@const isActive = activeTab === tab.id}
      {@const TabIcon = tab.icon}
      <button
        class="flex-1 flex items-center justify-center gap-1.5 px-2 py-1.5 rounded text-xs font-medium transition-colors"
        style={isActive
          ? 'background-color: var(--ds-surface-raised); color: var(--ds-text); box-shadow: var(--shadow-sm);'
          : 'color: var(--ds-text-subtle);'}
        onclick={() => homepageStore.setActiveTab(tab.id)}
      >
        <TabIcon class="w-3.5 h-3.5" />
        {t(tab.labelKey)}
      </button>
    {/each}
  </div>

  {#if loading && items.length === 0}
    <div class="space-y-2 animate-pulse">
      {#each Array(3) as _}
        <div class="h-9 rounded" style="background-color: var(--ds-background-neutral);"></div>
      {/each}
    </div>
  {:else if items.length === 0}
    <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
      <Clock class="w-6 h-6 mb-2 opacity-60" />
      <p class="text-sm">{t('dashboard.noRecentActivity')}</p>
    </div>
  {:else}
    <ul class="flex flex-col gap-1.5">
      {#each items.slice(0, 6) as item (item.item_id)}
        <li>
          <DashboardItemRow
            title={item.title}
            itemKey={itemKey(item)}
            href={`/workspaces/${item.workspace_id}/items/${item.item_id}`}
            statusName={item.status}
            statusBuiltinKey={item.status_builtin_key}
            statusColor={item.status_color}
            priorityName={item.priority_name}
            priorityBuiltinKey={item.priority_builtin_key}
            priorityColor={item.priority_color}
            timestamp={item.last_activity
              ? homepageStore.formatRelativeTime(item.last_activity)
              : null}
            dataTestid="your-activity-entry"
            onclick={() => open(item)}
          />
        </li>
      {/each}
    </ul>
  {/if}
</div>
