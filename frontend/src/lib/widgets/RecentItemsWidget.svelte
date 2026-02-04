<script>
  import { Clock } from 'lucide-svelte';
  import { api } from '../api.js';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null, maxItems = 10 } = $props();

  let items = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let fetchVersion = $state(0);
  let lastWorkspaceId = $state(undefined);

  $effect(() => {
    if (workspaceId !== lastWorkspaceId) {
      lastWorkspaceId = workspaceId;
      loadRecentActivity();
    }
  });

  async function loadRecentActivity() {
    const currentVersion = ++fetchVersion;
    loading = true;
    error = null;

    try {
      const data = await api.homepage.get();
      if (currentVersion !== fetchVersion) return;

      const sources = [
        ...(data?.recently_viewed ?? []),
        ...(data?.recently_edited ?? []),
        ...(data?.recently_commented ?? [])
      ];

      const deduped = [];
      const seen = new Set();

      sources.forEach(activity => {
        if (!activity?.item_id) return;
        if (workspaceId && activity.workspace_id !== Number(workspaceId)) return;

        const key = activity.item_id;
        if (seen.has(key)) return;
        seen.add(key);

        deduped.push({
          ...activity,
          lastActivityDate: activity.last_activity ? new Date(activity.last_activity) : null
        });
      });

      deduped.sort((a, b) => {
        const left = a.lastActivityDate?.getTime() ?? 0;
        const right = b.lastActivityDate?.getTime() ?? 0;
        return right - left;
      });

      items = deduped.slice(0, maxItems);
    } catch (err) {
      if (currentVersion !== fetchVersion) return;
      console.error('Failed to load recent items widget:', err);
      error = t('widgets.recentItems.loadError');
      items = [];
    } finally {
      if (currentVersion === fetchVersion) {
        loading = false;
      }
    }
  }
</script>

<WidgetState
  {loading}
  {error}
  isEmpty={items.length === 0}
  loadingText={t('widgets.recentItems.loadingText')}
  emptyIcon={Clock}
  emptyTitle={t('widgets.recentItems.emptyTitle')}
  emptySubtitle={t('widgets.recentItems.emptySubtitle')}
  onRetry={loadRecentActivity}
>
  {#snippet children()}
    <div class="flex flex-col rounded-xl border recent-items-list" style="border-color: var(--ds-border);">
      {#each items as item (item.item_id)}
        <a
          class="flex items-center gap-3 px-4 py-3 transition recent-item-row"
          href={`/workspaces/${item.workspace_id}/items/${item.item_id}`}
        >
          <div class="flex-shrink-0">
            <div class="rounded-full bg-blue-50 text-blue-600 p-2">
              <Clock class="h-4 w-4" />
            </div>
          </div>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-medium" style="color: var(--ds-text);">{item.title}</p>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              {item.workspace_key}-{item.workspace_item_number}
            </p>
          </div>
          <div class="text-xs" style="color: var(--ds-text-subtlest);">
            {formatRelativeCompact(item.lastActivityDate)}
          </div>
        </a>
      {/each}
    </div>
  {/snippet}
</WidgetState>

<style>
  .recent-items-list > :not(:last-child) {
    border-bottom: 1px solid var(--ds-border);
  }
  .recent-item-row:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
