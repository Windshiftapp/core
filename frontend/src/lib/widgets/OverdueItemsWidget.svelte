<script>
  import { AlertCircle, RefreshCw } from 'lucide-svelte';
  import { api } from '../api.js';
  import { formatDueDate, getDaysOverdue } from '../utils/dateFormatter.js';
  import WidgetState from './WidgetState.svelte';

  export let workspaceId = null;
  export let collectionFilter = null;

  const MAX_ITEMS = 8;

  let overdueItems = [];
  let loading = false;
  let error = null;
  let currentWorkspaceId = null;
  let refreshInFlight = false;
  let statusesPromise;
  let activeFetchId = 0;
  let currentCollectionFilter = null;

  function normalizeDate(dateString) {
    if (!dateString) return null;
    const date = new Date(dateString);
    return Number.isNaN(date.getTime()) ? null : date;
  }

  async function getDoneStatusIds() {
    try {
      if (!statusesPromise) {
        statusesPromise = api.statuses.getAll();
      }
      const statuses = await statusesPromise;
      if (!Array.isArray(statuses)) return [];
      return statuses
        .filter(status => status?.category_name?.toLowerCase().trim() === 'done')
        .map(status => status.id)
        .filter(Boolean);
    } catch (statusError) {
      console.warn('Failed to load statuses for overdue widget:', statusError);
      return [];
    }
  }

  async function loadOverdueItems() {
    if (!workspaceId) {
      overdueItems = [];
      return;
    }

    const fetchId = ++activeFetchId;
    loading = true;
    error = null;
    refreshInFlight = true;

    try {
      const doneStatusIds = await getDoneStatusIds();
      const trimmedFilter = (collectionFilter || '').trim();
      const parts = [];
      if (trimmedFilter) {
        parts.push(`(${trimmedFilter})`);
      }
      parts.push(`workspace_id = ${workspaceId}`);
      parts.push('due_date < now()');
      let vql = parts.join(' AND ');
      if (doneStatusIds.length > 0) {
        vql += ` AND status_id NOT IN (${doneStatusIds.join(',')})`;
      }
      const response = await api.items.getAll({
        vql,
        limit: 50 // fetch more than needed, filter client-side
      });
      const items = Array.isArray(response) ? response : (response?.items ?? []);
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
          const dueDate = normalizeDate(item.due_date);
          return dueDate && dueDate.getTime() < Date.now();
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
        error = 'Unable to load overdue items';
      }
    } finally {
      if (fetchId === activeFetchId) {
        loading = false;
        refreshInFlight = false;
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
    if (!refreshInFlight) {
      loadOverdueItems();
    }
  }

  $: if (workspaceId !== currentWorkspaceId || collectionFilter !== currentCollectionFilter) {
    currentWorkspaceId = workspaceId;
    currentCollectionFilter = collectionFilter;
    if (workspaceId) {
      loadOverdueItems();
    } else {
      overdueItems = [];
    }
  }
</script>

<div class="overdue-items-widget">
  <div class="flex items-center justify-between mb-4 text-xs text-gray-500">
    <span>{loading ? 'Loading overdue items…' : `${overdueItems.length} overdue item${overdueItems.length === 1 ? '' : 's'}`}</span>
    <button
      class="flex items-center gap-1 text-gray-500 hover:text-red-600 transition-colors disabled:opacity-50"
      onclick={handleRefresh}
      disabled={loading || !workspaceId}
      aria-label="Refresh overdue items"
    >
      <RefreshCw class="h-3.5 w-3.5" />
      Refresh
    </button>
  </div>

  <WidgetState
    {loading}
    {error}
    isEmpty={overdueItems.length === 0}
    loadingText="Loading overdue items..."
    emptyIcon={AlertCircle}
    emptyTitle="No overdue items"
    emptySubtitle="All caught up!"
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
                <p class="text-sm font-semibold text-gray-900 truncate">{item.title}</p>
                <div class="flex flex-wrap items-center gap-3 text-xs mt-1 text-gray-500">
                  <span class="font-mono">{getItemKey(item)}</span>
                  <span class="text-red-600 font-medium">{formatDueDate(item.due_date)}</span>
                </div>
              </div>
            </div>
            {#if overdueDays > 0}
              <span class="text-xs font-semibold text-red-600 whitespace-nowrap">{overdueDays}d overdue</span>
            {/if}
          </div>
        {/each}
      </div>
    {/snippet}
  </WidgetState>
</div>

<style>
  .overdue-items-widget button:disabled svg {
    opacity: 0.6;
  }

  .error-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 1.5rem 0;
    text-align: center;
  }
</style>
