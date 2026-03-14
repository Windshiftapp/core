<script>
  import { Calendar } from 'lucide-svelte';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  let upcomingItems = [];
</script>

<WidgetState
  loading={false}
  error={null}
  isEmpty={upcomingItems.length === 0}
  emptyIcon={Calendar}
  emptyTitle={t('widgets.upcomingDeadlines.emptyTitle')}
  emptySubtitle={t('widgets.upcomingDeadlines.emptySubtitle')}
>
  {#snippet children()}
    <div class="space-y-2">
      {#each upcomingItems as item}
        <div class="p-2 hover:bg-gray-50 rounded border border-gray-100">
          <p class="text-sm font-medium text-gray-900">{item.title}</p>
          <p class="text-xs text-gray-500">Due: {item.due_date}</p>
        </div>
      {/each}
    </div>
  {/snippet}
</WidgetState>
