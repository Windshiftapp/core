<script>
  import { Search } from 'lucide-svelte';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null, config = {} } = $props();

  let searchResults = [];
</script>

<WidgetState
  loading={false}
  error={null}
  isEmpty={searchResults.length === 0}
  emptyIcon={Search}
  emptyTitle={t('widgets.savedSearch.emptyTitle')}
  emptySubtitle={t('widgets.savedSearch.emptySubtitle')}
>
  {#snippet children()}
    <div class="space-y-2">
      {#each searchResults as result}
        <div class="p-2 hover:bg-gray-50 rounded border border-gray-100">
          <p class="text-sm font-medium text-gray-900">{result.title}</p>
        </div>
      {/each}
    </div>
  {/snippet}
</WidgetState>
