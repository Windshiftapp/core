<script>
  import { Bell, Eye, Inbox } from '@lucide/svelte';
  import { homepageStore, workspacesStore } from '../../stores';
  import { notifications, notificationActions } from '../../stores/notifications.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';

  const MAX_ENTRIES_PER_WORKSPACE = 5;
  const MAX_WORKSPACES = 5;

  let recentNotifications = $derived.by(() => {
    const dayAgo = Date.now() - 24 * 60 * 60 * 1000;
    return $notifications.filter((notification) => {
      if (!notification.read) return true;
      return new Date(notification.timestamp).getTime() >= dayAgo;
    });
  });
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
        notificationId: n.id,
        workspaceId: parseInt(m[1], 10),
        timestamp: n.timestamp,
        source: 'notification',
        title: n.message || n.title || t('notifications.notification'),
        subtitle: null,
        link: actionUrl,
        read: n.read,
      });
    }

    for (const w of watchedItems) {
      if (!w.last_activity) continue;
      entries.push({
        id: `w-${w.item_id}`,
        workspaceId: w.workspace_id,
        timestamp: w.last_activity,
        source: 'watched',
        title: w.title,
        subtitle: w.status,
        link: `/workspaces/${w.workspace_id}/items/${w.item_id}`,
        read: true,
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
    if (entry?.source === 'notification' && !entry.read) {
      notificationActions.markAsRead(entry.notificationId);
    }
    if (entry?.link) navigate(entry.link);
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
              <button
                data-testid="whats-new-entry"
                data-entry-id={e.id}
                data-read={e.read}
                class="w-full text-left p-2 flex items-start gap-2 rounded border transition-colors"
                style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                onmouseenter={(e2) =>
                  (e2.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
                onmouseleave={(e2) => (e2.currentTarget.style.backgroundColor = 'var(--ds-surface)')}
                onclick={() => open(e)}
              >
                {#if e.source === 'watched'}
                  <Eye
                    class="w-4 h-4 mt-0.5 flex-shrink-0"
                    style="color: var(--ds-text-subtlest);"
                  />
                {:else}
                  <Bell
                    class="w-4 h-4 mt-0.5 flex-shrink-0"
                    style={e.read
                      ? 'color: var(--ds-text-subtlest);'
                      : 'color: var(--ds-icon-accent-blue);'}
                  />
                {/if}
                <div class="min-w-0 flex-1">
                  <p
                    class="text-sm truncate"
                    style={e.read ? 'color: var(--ds-text-subtle);' : 'color: var(--ds-text); font-weight: 500;'}
                  >
                    {e.title}
                  </p>
                  <p
                    class="text-[0.7rem] mt-0.5 flex items-center gap-1.5"
                    style="color: var(--ds-text-subtle);"
                  >
                    {#if e.subtitle}<span class="truncate">{e.subtitle}</span><span>·</span>{/if}
                    <span>{homepageStore.formatRelativeTime(e.timestamp)}</span>
                  </p>
                </div>
              </button>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
{/if}
