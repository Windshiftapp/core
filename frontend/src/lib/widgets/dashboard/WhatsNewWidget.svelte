<script>
  import Inbox from '@lucide/svelte/icons/inbox';
  import { homepageStore } from '../../stores/homepageStore.svelte.js';
  import { workspacesStore } from '../../stores/workspaces.svelte.js';
  import { notifications, notificationActions } from '../../stores/notifications.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import { itemIdFromActionUrl } from '../../utils/actionUrl.js';
  import { navigate } from '../../router.js';
  import DashboardItemRow from './DashboardItemRow.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  const MAX_ENTRIES_PER_WORKSPACE = 5;
  const MAX_WORKSPACES = 5;
  // What's New is a feed of recent issue updates, so only surface entries
  // from the last two weeks regardless of read state.
  const RECENT_WINDOW_MS = 14 * 24 * 60 * 60 * 1000;

  function isRecent(timestamp) {
    const ts = new Date(timestamp).getTime();
    return Number.isFinite(ts) && ts >= Date.now() - RECENT_WINDOW_MS;
  }

  let recentNotifications = $derived($notifications.filter((n) => isRecent(n.timestamp)));
  let watchedItems = $derived(homepageStore.watchedItems);
  let loading = $derived(homepageStore.loading);

  let workspaceMap = $derived(
    new Map(($workspacesStore.allWorkspaces || []).map((w) => [w.id, w]))
  );

  let groups = $derived.by(() => {
    const entries = [];

    for (const n of recentNotifications) {
      const actionUrl = n.actionUrl || n.action_url;
      const m = actionUrl?.match(/^\/workspaces\/(\d+)\//);
      if (!m) continue;
      entries.push({
        id: `n-${n.id}`,
        kind: 'notification',
        notificationId: n.id,
        itemId: itemIdFromActionUrl(actionUrl),
        workspaceId: parseInt(m[1], 10),
        timestamp: n.timestamp,
        title: n.message || n.title || t('notifications.notification'),
        read: n.read,
        link: actionUrl,
      });
    }

    for (const w of watchedItems) {
      if (!w.last_active_at || !isRecent(w.last_active_at)) continue;
      entries.push({
        id: `w-${w.item_id}`,
        kind: 'item',
        itemId: w.item_id,
        workspaceId: w.workspace_id,
        timestamp: w.last_active_at,
        item: w,
        link: `/workspaces/${w.workspace_id}/items/${w.item_id}`,
      });
    }

    const buckets = new Map();
    for (const e of entries) {
      const ws = workspaceMap.get(e.workspaceId);
      if (!ws) continue;
      let bucket = buckets.get(e.workspaceId);
      if (!bucket) {
        bucket = { workspaceId: e.workspaceId, workspaceName: ws.name, entries: [] };
        buckets.set(e.workspaceId, bucket);
      }
      bucket.entries.push(e);
    }

    const ts = (v) => (v ? new Date(v).getTime() : 0);

    const result = Array.from(buckets.values()).map((b) => {
      b.entries.sort((a, c) => ts(c.timestamp) - ts(a.timestamp));
      b.entries = b.entries.slice(0, MAX_ENTRIES_PER_WORKSPACE);
      b.newest = b.entries[0]?.timestamp;
      return b;
    });

    result.sort((a, b) => ts(b.newest) - ts(a.newest));
    return result.slice(0, MAX_WORKSPACES);
  });

  function open(entry) {
    if (entry?.kind === 'notification' && !entry.read) {
      notificationActions.markAsRead(entry.notificationId);
    }
    if (entry?.itemId != null) {
      itemModalStore.open(Number(entry.itemId), entry.workspaceId);
      return;
    }
    if (entry?.link) navigate(entry.link);
  }

  function itemKey(item) {
    if (item.workspace_key == null || item.workspace_item_number == null) return null;
    return `${item.workspace_key}-${item.workspace_item_number}`;
  }
</script>

{#if loading && groups.length === 0}
  <div class="space-y-2 animate-pulse">
    {#each Array(3) as _}
      <div class="h-10 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if groups.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Inbox class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">{t('dashboard.allCaughtUp')}</p>
  </div>
{:else}
  <div class="flex flex-col gap-3">
    {#each groups as g (g.workspaceId)}
      <section class="flex flex-col">
        <h3
          class="text-[0.65rem] uppercase tracking-wide font-medium mb-1 px-1"
          style="color: var(--ds-text-subtle);"
        >
          {g.workspaceName}
        </h3>
        <ul class="flex flex-col gap-1.5">
          {#each g.entries as e (e.id)}
            <li>
              {#if e.kind === 'item'}
                <DashboardItemRow
                  title={e.item.title}
                  itemKey={itemKey(e.item)}
                  href={`/workspaces/${e.item.workspace_id}/items/${e.item.item_id}`}
                  statusName={e.item.status}
                  statusBuiltinKey={e.item.status_builtin_key}
                  statusColor={e.item.status_color}
                  priorityName={e.item.priority_name}
                  priorityBuiltinKey={e.item.priority_builtin_key}
                  priorityColor={e.item.priority_color}
                  timestamp={homepageStore.formatRelativeTime(e.timestamp)}
                  dataTestid="whats-new-entry"
                  onclick={() => open(e)}
                />
              {:else}
                <DashboardItemRow
                  title={e.title}
                  href={e.link}
                  timestamp={homepageStore.formatRelativeTime(e.timestamp)}
                  unread={!e.read}
                  dataTestid="whats-new-entry"
                  onclick={() => open(e)}
                />
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
{/if}
