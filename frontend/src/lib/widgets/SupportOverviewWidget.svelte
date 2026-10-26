<script>
  import Download from '@lucide/svelte/icons/download';
  import LifeBuoy from '@lucide/svelte/icons/life-buoy';
  import { support } from '../api/support.js';
  import { t } from '../stores/i18n.svelte.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import { downloadSupportMetricsCSV } from '../features/support/metricColumns.js';
  import Chart from './Chart.svelte';
  import SupportStatTile from './SupportStatTile.svelte';
  import {
    supportChartData,
    supportDateRange,
    supportHasActivity,
    supportSlaLabel,
    supportWindowDays,
    toISOStringDate,
  } from './supportMetricsShared.js';

  let { config = {}, onconfigchange = null } = $props();

  const configuredIds = $derived(
    Array.isArray(config?.workspace_ids) ? config.workspace_ids.map(Number) : []
  );
  const windowDays = $derived(supportWindowDays(config));

  let availableWorkspaces = $state([]);
  let editingWorkspaces = $state(false);
  let doc = $state(/** @type {any} */ (null));
  let loading = $state(false);
  let errored = $state(false);
  let unavailableCount = $state(0);
  let version = 0;
  /** @type {string | null} */
  let lastLoadKey = null;

  const loadKey = $derived(
    configuredIds.length > 0 ? `${configuredIds.join(',')}:${windowDays}` : null
  );

  const hasActivity = $derived(supportHasActivity(doc));

  const workspaceNameById = $derived(
    new Map(availableWorkspaces.map((workspace) => [Number(workspace.id), workspace.name]))
  );

  const chartData = $derived(supportChartData(doc, t));

  const slaLabel = $derived(supportSlaLabel(doc?.summary, t));

  $effect(() => {
    loadWorkspaces();
  });

  $effect(() => {
    if (loadKey && loadKey !== lastLoadKey) {
      lastLoadKey = loadKey;
      load();
    }
  });

  async function loadWorkspaces() {
    try {
      const list = await workspacesStore.load();
      if (Array.isArray(list)) {
        availableWorkspaces = list;
      }
    } catch (error) {
      console.error('Failed to load workspaces for support overview:', error);
    }
  }

  async function load() {
    const current = ++version;
    loading = true;
    errored = false;
    unavailableCount = 0;
    try {
      const { from, to } = supportDateRange(windowDays);
      const response = await support.getMetricsAggregate({
        workspace_ids: configuredIds,
        from,
        to,
        bucket: 'day',
      });
      if (current !== version) return;
      doc = response;
    } catch (error) {
      if (current !== version) return;
      console.error('Failed to load support overview:', error);
      errored = true;
      doc = null;
    } finally {
      if (current === version) loading = false;
    }
  }

  // The overview spans several workspaces; there is no single SSE topic for
  // it, so it refreshes on tab focus instead of live invalidations.
  $effect(() => {
    const onFocus = () => {
      if (configuredIds.length > 0) load();
    };
    window.addEventListener('focus', onFocus);
    return () => window.removeEventListener('focus', onFocus);
  });

  function toggleWorkspace(id) {
    const next = configuredIds.includes(id)
      ? configuredIds.filter((value) => value !== id)
      : [...configuredIds, id].sort((a, b) => a - b);
    if (next.length > 10) return;
    onconfigchange?.({ workspace_ids: next });
  }

  function exportCSV() {
    if (!doc) return;
    downloadSupportMetricsCSV(doc, `support-overview-${toISOStringDate(new Date())}.csv`);
  }
</script>

<div class="px-3 py-2" data-testid="support-overview-widget">
  {#snippet workspaceChips()}
    <div class="flex flex-wrap gap-2" data-testid="support-overview-workspace-picker">
      {#each availableWorkspaces as workspace (workspace.id)}
        {@const id = Number(workspace.id)}
        {@const selected = configuredIds.includes(id)}
        <button
          type="button"
          class="rounded-full border px-3 py-1 text-xs"
          style="border-color: {selected ? 'var(--ds-accent, #6366f1)' : 'var(--ds-border)'}; background: {selected
            ? 'var(--ds-surface-sunken)'
            : 'transparent'}; color: var(--ds-text);"
          aria-pressed={selected}
          data-testid={`support-overview-workspace-${id}`}
          onclick={() => toggleWorkspace(id)}
        >
          {workspace.name || workspace.key}
        </button>
      {/each}
    </div>
  {/snippet}
  {#if configuredIds.length === 0}
    <div
      class="flex flex-col items-center gap-3 rounded-xl border border-dashed px-4 py-6 text-center"
      style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
      data-testid="support-overview-setup"
    >
      <LifeBuoy class="h-7 w-7 opacity-60" />
      <div>
        <p class="text-sm font-medium" style="color: var(--ds-text);">
          {t('dashboard.states.supportOverviewSetupTitle')}
        </p>
        <p class="mt-1 text-xs">{t('dashboard.states.supportOverviewSetupSubtitle')}</p>
      </div>
      {#if availableWorkspaces.length > 0}
        <div class="max-h-40 overflow-y-auto">
          {@render workspaceChips()}
        </div>
      {/if}
    </div>
  {:else if loading && !doc}
    <p class="py-4 text-center text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
  {:else if errored}
    <p class="py-4 text-center text-sm" style="color: var(--ds-status-danger-text);">
      {t('dashboard.states.supportMetricsLoadError')}
    </p>
  {:else if doc}
    {#if editingWorkspaces}
      <div class="mb-3">
        {@render workspaceChips()}
      </div>
    {/if}
    {#if unavailableCount > 0}
      <p class="mb-2 text-xs" style="color: var(--ds-status-warning-text);">
        {t('dashboard.states.supportSourcesUnavailable', { count: unavailableCount })}
      </p>
    {/if}
    <div class="mb-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
      <SupportStatTile label={t('dashboard.states.supportBacklog')} value={doc.summary.backlog} />
      <SupportStatTile label={t('dashboard.states.supportUnassigned')} value={doc.summary.unassigned} />
      <SupportStatTile label={t('dashboard.states.supportCreated')} value={doc.summary.created} />
      <SupportStatTile label={t('dashboard.states.supportSlaCompliance')} value={slaLabel} />
    </div>

    {#if chartData.categories.length > 0}
      <Chart
        type="bar"
        series={chartData.series}
        categories={chartData.categories}
        minHeight={150}
        showYAxis={false}
        emptyMessage={t('dashboard.states.supportMetricsEmpty')}
      />
    {/if}

    <div class="mt-2 flex items-center justify-between gap-2 text-xs" style="color: var(--ds-text-subtle);">
      <span class="truncate">{configuredIds.map((id) => workspaceNameById.get(id) ?? `#${id}`).join(', ')}</span>
      <div class="flex shrink-0 items-center gap-2">
        <button
          type="button"
          class="rounded px-1.5 py-1 text-xs"
          style="color: var(--ds-text-subtle);"
          onclick={() => (editingWorkspaces = !editingWorkspaces)}
          data-testid="support-overview-edit-workspaces"
        >
          {editingWorkspaces ? t('common.done') || 'Done' : t('common.edit') || 'Edit'}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded px-1.5 py-1 text-xs"
          style="color: var(--ds-text-subtle);"
          onclick={exportCSV}
          data-testid="support-overview-export"
        >
          <Download class="h-3.5 w-3.5" />
          {t('dashboard.states.supportExportCSV')}
        </button>
      </div>
    </div>
  {/if}
</div>
