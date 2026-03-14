<script>
  import { Filter } from 'lucide-svelte';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null, config = {} } = $props();

  let filteredItems = [];
</script>

<WidgetState
  loading={false}
  error={null}
  isEmpty={filteredItems.length === 0}
  emptyIcon={Filter}
  emptyTitle={t('widgets.itemFilter.emptyTitle')}
  emptySubtitle={t('widgets.itemFilter.emptySubtitle')}
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
