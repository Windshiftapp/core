<script>
  import { Calendar } from 'lucide-svelte';
  import WidgetState from './WidgetState.svelte';

  export let workspaceId = null;

  let upcomingItems = [];
</script>

<WidgetState
  loading={false}
  error={null}
  isEmpty={upcomingItems.length === 0}
  emptyIcon={Calendar}
  emptyTitle="No upcoming deadlines"
  emptySubtitle="Items with due dates will appear here"
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
