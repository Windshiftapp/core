<script>
  import { Clock } from 'lucide-svelte';
  import { api } from '../api.js';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import WidgetState from './WidgetState.svelte';

  export let workspaceId = null;
  export let maxItems = 10;

  let items = [];
  let loading = false;
  let error = null;
  let fetchVersion = 0;

  $: if (workspaceId !== undefined) {
    loadRecentActivity();
  }

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
      error = 'Unable to load recent items';
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
  loadingText="Loading recent items..."
  emptyIcon={Clock}
  emptyTitle="No recent activity"
  emptySubtitle="Items you update will appear here."
  onRetry={loadRecentActivity}
>
  {#snippet children()}
    <div class="flex flex-col divide-y divide-gray-100 rounded-xl border border-gray-200">
      {#each items as item (item.item_id)}
        <a
          class="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 transition"
          href={`/workspaces/${item.workspace_id}/items/${item.item_id}`}
        >
          <div class="flex-shrink-0">
            <div class="rounded-full bg-blue-50 text-blue-600 p-2">
              <Clock class="h-4 w-4" />
            </div>
          </div>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-medium text-slate-900">{item.title}</p>
            <p class="text-xs text-slate-500">
              {item.workspace_key}-{item.workspace_item_number}
            </p>
          </div>
          <div class="text-xs text-slate-400">
            {formatRelativeCompact(item.lastActivityDate)}
          </div>
        </a>
      {/each}
    </div>
  {/snippet}
</WidgetState>
