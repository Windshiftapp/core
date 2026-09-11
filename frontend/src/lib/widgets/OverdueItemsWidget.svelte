<script>
  import { AlertCircle, RefreshCw } from '@lucide/svelte';
  import { api } from '../api.js';
  import { dateOnlyKey, formatDate, formatDueDate, getDaysOverdue } from '../utils/dateFormatter.js';
  import { serverNow } from '../utils/serverClock.js';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null, collectionFilter = null } = $props();

  const MAX_ITEMS = 8;

  let overdueItems = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let currentWorkspaceId = $state(null);
  let activeFetchId = $state(0);
  let currentCollectionFilter = $state(null);

  async function loadOverdueItems() {
    if (!workspaceId) {
      overdueItems = [];
      return;
    }

    const fetchId = ++activeFetchId;
    loading = true;
    error = null;

    try {
      const trimmedFilter = (collectionFilter || '').trim();
      const parts = [];
      if (trimmedFilter) {
        parts.push(`(${trimmedFilter})`);
      }
      parts.push(`workspace_id = ${workspaceId}`);
      parts.push('due_date < now()');
      parts.push('status_completed = false');
      const vql = parts.join(' AND ');
      const response = await api.items.getAll({
        ql: vql,
        limit: 50 // fetch more than needed, filter client-side
      });
      const items = response?.data ?? [];
      const parsedItems = items
        .map(item => ({
          id: item.id,
          title: item.title,
          due_date: item.due_date,
          status_name: item.status_name || '',
          workspace_key: item.workspace_key,
          workspace_item_number: item.workspace_item_number
        }))
        .filter(item => {
          const dueDate = dateOnlyKey(item.due_date);
          return dueDate && dueDate < formatDate(serverNow());
        })
        .sort((a, b) => {
          const dateA = new Date(a.due_date);
          const dateB = new Date(b.due_date);
          return dateA.getTime() - dateB.getTime();
        })
        .slice(0, MAX_ITEMS);

      if (fetchId === activeFetchId) {
        overdueItems = parsedItems;
        error = null;
      }
    } catch (err) {
      console.error('Failed to load overdue items:', err);
      if (fetchId === activeFetchId) {
        overdueItems = [];
        error = t('widgets.overdueItems.loadError');
      }
    } finally {
      if (fetchId === activeFetchId) {
        loading = false;
      }
    }
  }

  function getItemKey(item) {
    if (item.workspace_key && item.workspace_item_number) {
      return `${item.workspace_key}-${item.workspace_item_number}`;
    }
    return `#${item.id}`;
  }

  function handleRefresh() {
    if (!loading) {
      loadOverdueItems();
    }
  }

  $effect(() => {
    if (workspaceId !== currentWorkspaceId || collectionFilter !== currentCollectionFilter) {
      currentWorkspaceId = workspaceId;
      currentCollectionFilter = collectionFilter;
      if (workspaceId) {
        loadOverdueItems();
      } else {
        overdueItems = [];
      }
    }
  });
</script>

<div class="overdue-items-widget">
  <div class="flex items-center justify-between mb-4 text-xs" style="color: var(--ds-text-subtle);">
    <span>{loading ? t('widgets.overdueItems.loadingStatus') : t('widgets.overdueItems.itemCount', { count: overdueItems.length })}</span>
    <button
      class="flex items-center gap-1 transition-colors disabled:opacity-50 refresh-btn"
      onclick={handleRefresh}
      disabled={loading || !workspaceId}
      aria-label={t('widgets.overdueItems.refreshAriaLabel')}
    >
      <RefreshCw class="h-3.5 w-3.5" />
      {t('common.refresh')}
    </button>
  </div>

  <WidgetState
    {loading}
    {error}
    isEmpty={overdueItems.length === 0}
    loadingText={t('widgets.overdueItems.loadingText')}
    emptyIcon={AlertCircle}
    emptyTitle={t('widgets.overdueItems.emptyTitle')}
    emptySubtitle={t('widgets.overdueItems.emptySubtitle')}
    onRetry={handleRefresh}
  >
    {#snippet children()}
      <div class="space-y-1">
        {#each overdueItems as item}
          {@const overdueDays = getDaysOverdue(item.due_date)}
          <div
            class="flex items-center justify-between gap-4 rounded border border-[var(--ds-border,#e5e7eb)] bg-[var(--ds-surface-raised,#fff)] px-4 py-3 shadow-sm transition-shadow hover:shadow-md"
          >
            <div class="flex items-center gap-3 flex-1 min-w-0">
              <div class="min-w-0">
                <p class="text-sm truncate" style="color: var(--ds-text);">{item.title}</p>
                <div class="flex flex-wrap items-center gap-3 text-xs mt-1" style="color: var(--ds-text-subtle);">
                  <span class="font-mono">{getItemKey(item)}</span>
                  <span class="font-medium" style="color: var(--ds-text-danger);">{formatDueDate(item.due_date)}</span>
                </div>
              </div>
            </div>
            {#if overdueDays > 0}
              <span class="text-xs font-semibold whitespace-nowrap" style="color: var(--ds-text-danger);">{t('widgets.overdueItems.daysOverdue', { days: overdueDays })}</span>
            {/if}
          </div>
        {/each}
      </div>
    {/snippet}
  </WidgetState>
</div>

<style>
  .refresh-btn {
    color: var(--ds-text-subtle);
  }

  .refresh-btn:hover {
    color: var(--ds-text-danger);
  }
</style>
