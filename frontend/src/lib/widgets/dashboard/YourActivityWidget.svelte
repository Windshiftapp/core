<script>
  import { Clock, Edit, MessageSquare, Eye } from '@lucide/svelte';
  import { homepageStore } from '../../stores';
  import { navigate } from '../../router.js';

  let activeTab = $derived(homepageStore.activeTab);
  let loading = $derived(homepageStore.loading);

  let items = $derived.by(() => {
    if (activeTab === 'edited') return homepageStore.recentlyEdited;
    if (activeTab === 'commented') return homepageStore.recentlyCommented;
    return homepageStore.recentlyViewed;
  });

  const tabs = [
    { id: 'viewed', label: 'Viewed', icon: Eye },
    { id: 'edited', label: 'Edited', icon: Edit },
    { id: 'commented', label: 'Commented', icon: MessageSquare },
  ];

  function open(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.item_id}`);
  }
</script>

<div class="flex flex-col gap-3">
  <div class="flex gap-1 p-0.5 rounded" style="background-color: var(--ds-background-neutral);">
    {#each tabs as tab}
      {@const isActive = activeTab === tab.id}
      <button
        class="flex-1 flex items-center justify-center gap-1.5 px-2 py-1.5 rounded text-xs font-medium transition-colors"
        style={isActive
          ? 'background-color: var(--ds-surface-raised); color: var(--ds-text); box-shadow: var(--shadow-sm);'
          : 'color: var(--ds-text-subtle);'}
        onclick={() => homepageStore.setActiveTab(tab.id)}
      >
        <svelte:component this={tab.icon} class="w-3.5 h-3.5" />
        {tab.label}
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
      <p class="text-sm">No recent activity</p>
    </div>
  {:else}
    <ul class="flex flex-col">
      {#each items.slice(0, 6) as item (item.item_id)}
        <li>
          <button
            class="w-full text-left px-2 py-1.5 rounded flex items-start gap-2 transition-colors"
            onmouseenter={(e) =>
              (e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
            onmouseleave={(e) => (e.currentTarget.style.backgroundColor = '')}
            onclick={() => open(item)}
          >
            <div class="min-w-0 flex-1">
              <p class="text-sm truncate" style="color: var(--ds-text);">{item.title}</p>
              <p class="text-[0.7rem] mt-0.5" style="color: var(--ds-text-subtle);">
                {item.workspace_key}-{item.workspace_item_number}
                {#if item.last_activity}
                  · {homepageStore.formatRelativeTime(item.last_activity)}
                {/if}
              </p>
            </div>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
