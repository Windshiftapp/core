<script>
  import { CalendarDays } from 'lucide-svelte';
  import WidgetState from './WidgetState.svelte';

  export let workspaceId = null;

  let sprints = [];
</script>

<WidgetState
  loading={false}
  error={null}
  isEmpty={sprints.length === 0}
  emptyIcon={CalendarDays}
  emptyTitle="No active sprints"
  emptySubtitle="Sprint timelines will appear here"
>
  {#snippet children()}
    <div class="space-y-3">
      {#each sprints as sprint}
        <div class="p-3 bg-gray-50 rounded border border-gray-200">
          <p class="text-sm font-medium text-gray-900">{sprint.name}</p>
          <p class="text-xs text-gray-500">{sprint.start_date} - {sprint.end_date}</p>
          <div class="mt-2 w-full bg-gray-200 rounded-full h-2">
            <div
              class="h-2 rounded-full bg-blue-600"
              style="width: {sprint.progress}%"
            ></div>
          </div>
        </div>
      {/each}
    </div>
  {/snippet}
</WidgetState>
