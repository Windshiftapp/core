<script>
  import { Filter } from 'lucide-svelte';
  import WidgetState from './WidgetState.svelte';

  export let workspaceId = null;
  export let config = {};

  let filteredItems = [];
</script>

<WidgetState
  loading={false}
  error={null}
  isEmpty={filteredItems.length === 0}
  emptyIcon={Filter}
  emptyTitle="No items match filter"
  emptySubtitle="Configure filter to see items"
>
  {#snippet children()}
    <div class="space-y-2">
      {#each filteredItems as item}
        <div class="p-2 hover:bg-gray-50 rounded border border-gray-100">
          <p class="text-sm font-medium text-gray-900">{item.title}</p>
        </div>
      {/each}
    </div>
  {/snippet}
</WidgetState>
