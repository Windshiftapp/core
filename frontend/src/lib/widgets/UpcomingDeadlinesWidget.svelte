<script>
  import { Calendar, CalendarDays, Flag, RefreshCw } from '@lucide/svelte';
  import { api } from '../api.js';
  import DueMark from './dashboard/DueMark.svelte';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null, collectionFilter = null } = $props();

  const MAX_ENTRIES = 10;
  const DAYS_AHEAD = 14;

  let upcomingEntries = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let currentWorkspaceId = $state(null);
  let currentCollectionFilter = $state(null);
  let activeFetchId = $state(0);

  function normalizeDate(dateString) {
    if (!dateString) return null;
    const date = new Date(dateString);
    return Number.isNaN(date.getTime()) ? null : date;
  }

  function isWithinWindow(dateString, now, cutoff) {
    const d = normalizeDate(dateString);
    return d && d.getTime() >= now && d.getTime() <= cutoff;
  }

  async function loadUpcomingEntries() {
    if (!workspaceId) {
      upcomingEntries = [];
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
      parts.push('due_date >= now()');
      parts.push('status_completed = false');
      const vql = parts.join(' AND ');

      const [itemsResponse, allMilestones, allIterations] = await Promise.all([
        api.items.getAll({ ql: vql, limit: 50 }),
        api.milestones.getAll({ workspace_id: workspaceId }),
        api.iterations.getAll({ workspace_id: workspaceId }),
      ]);

      const items = itemsResponse?.data ?? [];
      const milestonesArr = Array.isArray(allMilestones) ? allMilestones : [];
      const iterationsArr = Array.isArray(allIterations) ? allIterations : [];

      const now = Date.now();
      const cutoff = now + DAYS_AHEAD * 24 * 60 * 60 * 1000;
      const excludedStatuses = ['completed', 'cancelled'];

      const normalizedItems = items
        .filter(item => isWithinWindow(item.due_date, now, cutoff))
        .map(item => ({
          id: item.id,
          title: item.title,
          deadline: item.due_date,
          type: 'item',
          key: item.workspace_key && item.workspace_item_number
            ? `${item.workspace_key}-${item.workspace_item_number}`
            : `#${item.id}`,
          subtitle: item.status_name || '',
          color: null,
        }));

      const normalizedMilestones = milestonesArr
        .filter(m => !excludedStatuses.includes(m.status?.toLowerCase()) && isWithinWindow(m.target_date, now, cutoff))
        .map(m => ({
          id: `milestone-${m.id}`,
          title: m.name,
          deadline: m.target_date,
          type: 'milestone',
          key: null,
          subtitle: m.status || '',
          color: m.category_color || null,
        }));

      const normalizedIterations = iterationsArr
        .filter(it => !excludedStatuses.includes(it.status?.toLowerCase()) && isWithinWindow(it.end_date, now, cutoff))
        .map(it => ({
          id: `iteration-${it.id}`,
          title: it.name,
          deadline: it.end_date,
          type: 'iteration',
          key: null,
          subtitle: it.status || '',
          color: it.type_color || null,
        }));

      const merged = [...normalizedItems, ...normalizedMilestones, ...normalizedIterations]
        .sort((a, b) => new Date(a.deadline).getTime() - new Date(b.deadline).getTime())
        .slice(0, MAX_ENTRIES);

      if (fetchId === activeFetchId) {
        upcomingEntries = merged;
        error = null;
      }
    } catch (err) {
      console.error('Failed to load upcoming deadlines:', err);
      if (fetchId === activeFetchId) {
        upcomingEntries = [];
        error = t('widgets.upcomingDeadlines.loadError');
      }
    } finally {
      if (fetchId === activeFetchId) {
        loading = false;
      }
    }
  }

  function handleRefresh() {
    if (!loading) {
      loadUpcomingEntries();
    }
  }

  $effect(() => {
    if (workspaceId !== currentWorkspaceId || collectionFilter !== currentCollectionFilter) {
      currentWorkspaceId = workspaceId;
      currentCollectionFilter = collectionFilter;
      if (workspaceId) {
        loadUpcomingEntries();
      } else {
        upcomingEntries = [];
      }
    }
  });
</script>

<div class="upcoming-deadlines-widget">
  <div class="flex items-center justify-between mb-4 text-xs" style="color: var(--ds-text-subtle);">
    <span>{loading ? t('widgets.upcomingDeadlines.loadingStatus') : t('widgets.upcomingDeadlines.itemCount', { count: upcomingEntries.length })}</span>
    <button
      class="flex items-center gap-1 transition-colors disabled:opacity-50 refresh-btn"
      onclick={handleRefresh}
      disabled={loading || !workspaceId}
      aria-label={t('widgets.upcomingDeadlines.refreshAriaLabel')}
    >
      <RefreshCw class="h-3.5 w-3.5" />
      {t('common.refresh')}
    </button>
  </div>

  <WidgetState
    {loading}
    {error}
    isEmpty={upcomingEntries.length === 0}
    loadingText={t('widgets.upcomingDeadlines.loadingText')}
    emptyIcon={Calendar}
    emptyTitle={t('widgets.upcomingDeadlines.emptyTitle')}
    emptySubtitle={t('widgets.upcomingDeadlines.emptySubtitle')}
    onRetry={handleRefresh}
  >
    {#snippet children()}
      <div class="space-y-1">
        {#each upcomingEntries as entry}
          <div
            class="flex items-center justify-between gap-4 rounded border border-[var(--ds-border,#e5e7eb)] bg-[var(--ds-surface-raised,#fff)] px-4 py-3 shadow-sm transition-shadow hover:shadow-md"
          >
            <div class="flex items-center gap-3 flex-1 min-w-0">
              {#if entry.type === 'milestone'}
                <Flag class="h-4 w-4 shrink-0" style="color: {entry.color || 'var(--ds-text-subtle)'};" />
              {:else if entry.type === 'iteration'}
                <CalendarDays class="h-4 w-4 shrink-0" style="color: {entry.color || 'var(--ds-text-subtle)'};" />
              {/if}
              <div class="min-w-0">
                <p class="text-sm truncate" style="color: var(--ds-text);">{entry.title}</p>
                <div class="flex flex-wrap items-center gap-3 text-xs mt-1" style="color: var(--ds-text-subtle);">
                  {#if entry.type === 'item'}
                    <span class="font-mono">{entry.key}</span>
                  {:else}
                    <span class="capitalize">{entry.type}</span>
                    {#if entry.subtitle}
                      <span>&middot; {entry.subtitle}</span>
                    {/if}
                  {/if}
                </div>
              </div>
            </div>
            <DueMark dueDate={entry.deadline} />
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
    color: var(--ds-interactive);
  }
</style>
