<script>
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId } = $props();

  let rows = $state([]);
  let loading = $state(false);
  let errored = $state(false);
  let version = 0;

  const maxPoints = $derived(rows.reduce((max, row) => Math.max(max, row.total_points), 0));

  $effect(() => {
    load();
  });

  async function load() {
    const v = ++version;
    loading = true;
    errored = false;
    try {
      const response = await api.items.getStoryPointsByAssignee({ workspace_id: workspaceId });
      if (v !== version) return;
      rows = response || [];
    } catch (err) {
      if (v !== version) return;
      console.error('Failed to load story points summary:', err);
      errored = true;
      rows = [];
    } finally {
      if (v === version) loading = false;
    }
  }
</script>

<div class="px-3 py-2">
  {#if loading}
    <p class="text-sm py-4 text-center" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
  {:else if errored}
    <p class="text-sm py-4 text-center" style="color: var(--ds-text-accent-red);">{t('dashboard.states.storyPointsLoadError')}</p>
  {:else if rows.length === 0}
    <p class="text-sm py-4 text-center" style="color: var(--ds-text-subtle);">{t('dashboard.states.storyPointsEmpty')}</p>
  {:else}
    <ul class="space-y-2 m-0 p-0 list-none" data-testid="story-points-by-assignee">
      {#each rows as row (row.assignee_id)}
        <li class="flex items-center gap-3 text-sm">
          <span class="w-32 min-w-0 truncate" style="color: var(--ds-text);" title={row.display_name}>
            {row.display_name || t('common.none')}
          </span>
          <span class="flex-1 h-2 rounded overflow-hidden" style="background: var(--ds-surface-sunken);">
            <span
              class="block h-full rounded"
              style="width: {maxPoints > 0 ? Math.max(4, (row.total_points / maxPoints) * 100) : 0}%; background: var(--ds-accent, var(--ds-text));"
            ></span>
          </span>
          <span class="text-right whitespace-nowrap" style="color: var(--ds-text);">
            {row.total_points}
            <span class="text-xs" style="color: var(--ds-text-subtle);">({row.item_count})</span>
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</div>
