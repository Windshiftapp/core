<script>
  import { CalendarDays } from 'lucide-svelte';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  let sprints = [];
</script>

<WidgetState
  loading={false}
  error={null}
  isEmpty={sprints.length === 0}
  emptyIcon={CalendarDays}
  emptyTitle={t('widgets.sprintTimeline.emptyTitle')}
  emptySubtitle={t('widgets.sprintTimeline.emptySubtitle')}
>
  {#snippet children()}
    <div class="space-y-3">
      {#each sprints as sprint}
        <div class="p-3 rounded border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <p class="text-sm font-medium" style="color: var(--ds-text);">{sprint.name}</p>
          <p class="text-xs" style="color: var(--ds-text-subtle);">{sprint.start_date} - {sprint.end_date}</p>
          <div class="mt-2 w-full rounded-full h-2" style="background-color: var(--ds-background-neutral);">
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
