<script>
  import { Eye } from 'lucide-svelte';
  import { homepageStore } from '../../stores';
  import { navigate } from '../../router.js';

  let items = $derived(homepageStore.watchedItems);
  let loading = $derived(homepageStore.loading);

  function open(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.item_id}`);
  }
</script>

{#if loading && items.length === 0}
  <div class="space-y-2 animate-pulse">
    {#each Array(3) as _}
      <div class="h-9 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if items.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Eye class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">You aren't watching any items</p>
  </div>
{:else}
  <ul class="flex flex-col">
    {#each items.slice(0, 5) as item (item.item_id)}
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
            <p class="text-[0.7rem] mt-0.5 flex items-center gap-1" style="color: var(--ds-text-subtle);">
              <span>{item.workspace_key}-{item.workspace_item_number}</span>
              {#if item.status}
                <span aria-hidden="true">·</span>
                <span>{item.status}</span>
              {/if}
            </p>
          </div>
        </button>
      </li>
    {/each}
  </ul>
{/if}
