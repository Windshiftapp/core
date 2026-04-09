<script>
  import { CalendarDays, RefreshCw } from 'lucide-svelte';
  import { api } from '../api.js';
  import { formatDateShort } from '../utils/dateFormatter.js';
  import WidgetState from './WidgetState.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  const MAX_ITERATIONS = 5;

  let iterations = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let currentWorkspaceId = $state(null);
  let refreshInFlight = $state(false);
  let activeFetchId = $state(0);

  function isActiveIteration(iteration) {
    return iteration.status === 'active' || iteration.status === 'in_progress';
  }

  function isRelevantIteration(iteration) {
    // Keep active iterations regardless of dates
    if (isActiveIteration(iteration)) return true;
    // Exclude completed iterations with end_date in the past
    if (iteration.status === 'completed' || iteration.status === 'done') {
      const endDate = iteration.end_date ? new Date(iteration.end_date) : null;
      if (endDate && endDate.getTime() < Date.now()) return false;
    }
    return true;
  }

  async function loadIterations() {
    if (!workspaceId) {
      iterations = [];
      return;
    }

    const fetchId = ++activeFetchId;
    loading = true;
    error = null;
    refreshInFlight = true;

    try {
      const allIterations = await api.iterations.getAll({ workspace_id: workspaceId });
      const iterationList = Array.isArray(allIterations) ? allIterations : (allIterations?.iterations ?? []);

      const relevant = iterationList
        .filter(isRelevantIteration)
        .sort((a, b) => {
          // Active first, then by start_date ascending
          const aActive = isActiveIteration(a) ? 0 : 1;
          const bActive = isActiveIteration(b) ? 0 : 1;
          if (aActive !== bActive) return aActive - bActive;
          const dateA = a.start_date ? new Date(a.start_date).getTime() : Infinity;
          const dateB = b.start_date ? new Date(b.start_date).getTime() : Infinity;
          return dateA - dateB;
        })
        .slice(0, MAX_ITERATIONS);

      // Fetch progress for each iteration
      const withProgress = await Promise.all(
        relevant.map(async (iteration) => {
          try {
            const progress = await api.iterations.getProgress(iteration.id);
            return { ...iteration, progress };
          } catch {
            return { ...iteration, progress: null };
          }
        })
      );

      if (fetchId === activeFetchId) {
        iterations = withProgress;
        error = null;
      }
    } catch (err) {
      console.error('Failed to load iterations:', err);
      if (fetchId === activeFetchId) {
        iterations = [];
        error = t('widgets.iterationTimeline.loadError');
      }
    } finally {
      if (fetchId === activeFetchId) {
        loading = false;
        refreshInFlight = false;
      }
    }
  }

  function handleRefresh() {
    if (!refreshInFlight) {
      loadIterations();
    }
  }

  function getStatusLabel(iteration) {
    if (isActiveIteration(iteration)) return 'Active';
    if (iteration.status === 'planned' || iteration.status === 'pending') return 'Planned';
    return iteration.status || 'Planned';
  }

  function getPercentComplete(progress) {
    if (!progress) return 0;
    return progress.percent_complete ?? 0;
  }

  function getItemCounts(progress) {
    if (!progress) return { completed: 0, total: 0 };
    const completed = progress.completed_items ?? 0;
    const total = progress.total_items ?? 0;
    return { completed, total };
  }

  $effect(() => {
    if (workspaceId !== currentWorkspaceId) {
      currentWorkspaceId = workspaceId;
      if (workspaceId) {
        loadIterations();
      } else {
        iterations = [];
      }
    }
  });
</script>

<div class="iteration-timeline-widget">
  <div class="flex items-center justify-between mb-4 text-xs" style="color: var(--ds-text-subtle);">
    <span>{loading ? t('widgets.iterationTimeline.loadingStatus') : t('widgets.iterationTimeline.iterationCount', { count: iterations.length })}</span>
    <button
      class="flex items-center gap-1 transition-colors disabled:opacity-50 refresh-btn"
      onclick={handleRefresh}
      disabled={loading || !workspaceId}
      aria-label={t('widgets.iterationTimeline.refreshAriaLabel')}
    >
      <RefreshCw class="h-3.5 w-3.5" />
      {t('common.refresh')}
    </button>
  </div>

  <WidgetState
    {loading}
    {error}
    isEmpty={iterations.length === 0}
    loadingText={t('widgets.iterationTimeline.loadingText')}
    emptyIcon={CalendarDays}
    emptyTitle={t('widgets.iterationTimeline.emptyTitle')}
    emptySubtitle={t('widgets.iterationTimeline.emptySubtitle')}
    onRetry={handleRefresh}
  >
    {#snippet children()}
      <div class="space-y-3">
        {#each iterations as iteration}
          {@const percent = getPercentComplete(iteration.progress)}
          {@const counts = getItemCounts(iteration.progress)}
          {@const active = isActiveIteration(iteration)}
          <div
            class="rounded border px-4 py-3"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          >
            <div class="flex items-center justify-between gap-2 mb-1">
              <div class="flex items-center gap-2 min-w-0">
                <p class="text-sm font-medium truncate" style="color: var(--ds-text);">{iteration.name}</p>
                {#if iteration.type_color}
                  <span class="inline-block w-2 h-2 rounded-full flex-shrink-0" style="background-color: {iteration.type_color};"></span>
                {/if}
              </div>
              <span
                class="text-xs font-medium px-2 py-0.5 rounded whitespace-nowrap"
                class:bg-green-100={active}
                class:text-green-700={active}
                class:bg-gray-100={!active}
                class:text-gray-600={!active}
              >
                {getStatusLabel(iteration)}
              </span>
            </div>

            {#if iteration.start_date || iteration.end_date}
              <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">
                {iteration.start_date ? formatDateShort(iteration.start_date) : '?'}
                {' \u2014 '}
                {iteration.end_date ? formatDateShort(iteration.end_date) : '?'}
              </p>
            {/if}

            <!-- Progress bar -->
            <div class="w-full rounded-full h-2" style="background-color: var(--ds-background-neutral, #e5e7eb);">
              <div
                class="h-2 rounded-full transition-all"
                style="width: {percent}%; background-color: var(--ds-interactive, #3b82f6);"
              ></div>
            </div>
            <div class="flex items-center justify-between mt-1">
              <span class="text-xs" style="color: var(--ds-text-subtle);">
                {counts.completed}/{counts.total} done
              </span>
              <span class="text-xs font-medium" style="color: var(--ds-text-subtle);">
                {percent}%
              </span>
            </div>

            {#if iteration.progress?.status_breakdown}
              {@const breakdown = iteration.progress.status_breakdown}
              <div class="flex items-center gap-3 mt-1 text-xs" style="color: var(--ds-text-subtle);">
                {#if breakdown.done != null}
                  <span class="text-green-600">{breakdown.done} done</span>
                {/if}
                {#if breakdown.in_progress != null}
                  <span class="text-blue-600">{breakdown.in_progress} in progress</span>
                {/if}
                {#if breakdown.todo != null}
                  <span>{breakdown.todo} to do</span>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/snippet}
  </WidgetState>
</div>

<style>
  .iteration-timeline-widget button:disabled svg {
    opacity: 0.6;
  }

  .refresh-btn {
    color: var(--ds-text-subtle);
  }

  .refresh-btn:hover {
    color: var(--ds-interactive);
  }
</style>
