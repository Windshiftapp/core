<script>
  import Download from '@lucide/svelte/icons/download';
  import { support } from '../api/support.js';
  import { useCollectionEventStream } from '../composables/useCollectionEventStream.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
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

  let { workspaceId, collectionId = null, config = {} } = $props();

  const windowDays = $derived(supportWindowDays(config));

  let doc = $state(/** @type {any} */ (null));
  let loading = $state(false);
  let errored = $state(false);
  let version = 0;
  let lastLoadKey = null;

  const loadKey = $derived(
    workspaceId ? `${workspaceId}:${collectionId ?? 'ws'}:${windowDays}` : null
  );

  const hasActivity = $derived(supportHasActivity(doc));

  const chartData = $derived(supportChartData(doc, t));

  const slaLabel = $derived(supportSlaLabel(doc?.summary, t));

  $effect(() => {
    if (loadKey && loadKey !== lastLoadKey) {
      lastLoadKey = loadKey;
      load();
    }
  });

  async function load() {
    const current = ++version;
    loading = true;
    errored = false;
    try {
      const { from, to } = supportDateRange(windowDays);
      const params = {
        from,
        to,
        bucket: 'day',
      };
      if (collectionId) {
        params.collection_id = collectionId;
      } else {
        params.workspace_id = workspaceId;
      }
      const response = await support.getMetricsAggregate(params);
      if (current !== version) return;
      doc = response;
    } catch (error) {
      if (current !== version) return;
      console.error('Failed to load support metrics:', error);
      errored = true;
      doc = null;
    } finally {
      if (current === version) loading = false;
    }
  }

  // Live refresh: the workspace/collection SSE stream fires coarse item
  // invalidations; debounce them so bursts collapse into one refetch.
  let refreshTimer = null;
  const stream = useCollectionEventStream(
    () => {
      if (collectionId) return { kind: 'collection', id: collectionId };
      return { kind: 'workspace', id: workspaceId };
    },
    {
      onInvalidate: () => {
        if (refreshTimer) return;
        refreshTimer = setTimeout(() => {
          refreshTimer = null;
          load();
        }, 5000);
      },
    }
  );

  $effect(() => {
    const onFocus = () => {
      if (loadKey) load();
    };
    window.addEventListener('focus', onFocus);
    return () => {
      window.removeEventListener('focus', onFocus);
      if (refreshTimer) {
        clearTimeout(refreshTimer);
        refreshTimer = null;
      }
    };
  });

  function exportCSV() {
    if (!doc) return;
    downloadSupportMetricsCSV(doc, `support-metrics-${toISOStringDate(new Date())}.csv`);
  }
</script>

<div class="px-3 py-2" data-testid="support-metrics-widget">
  {#if loading && !doc}
    <p class="py-4 text-center text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
  {:else if errored}
    <p class="py-4 text-center text-sm" style="color: var(--ds-status-danger-text);">
      {t('dashboard.states.supportMetricsLoadError')}
    </p>
  {:else if doc && !hasActivity}
    <p class="py-4 text-center text-sm" style="color: var(--ds-text-subtle);">
      {t('dashboard.states.supportMetricsEmpty')}
    </p>
  {:else if doc}
    <div class="mb-3 grid grid-cols-2 gap-2 sm:grid-cols-5" data-testid="support-metrics-summary">
      <SupportStatTile
        label={t('dashboard.states.supportBacklog')}
        value={doc.summary.backlog}
        dataTestid="support-metrics-backlog"
      />
      <SupportStatTile label={t('dashboard.states.supportUnassigned')} value={doc.summary.unassigned} />
      <SupportStatTile label={t('dashboard.states.supportCreated')} value={doc.summary.created} />
      <SupportStatTile label={t('dashboard.states.supportResolved')} value={doc.summary.resolved} />
      <SupportStatTile label={t('dashboard.states.supportSlaCompliance')} value={slaLabel} />
    </div>

    {#if chartData.categories.length > 0}
      <Chart
        type="bar"
        series={chartData.series}
        categories={chartData.categories}
        minHeight={140}
        showYAxis={false}
        emptyMessage={t('dashboard.states.supportMetricsEmpty')}
      />
    {/if}

    <div class="mt-2 flex items-center justify-between text-xs" style="color: var(--ds-text-subtle);">
      <span>
        {#if !stream.connected}{t('dashboard.states.supportLiveDelayed')} · {/if}
        {doc.summary.first_response ? `${t('dashboard.states.supportFirstResponseP50')}: ${Math.round(doc.summary.first_response.p50_ms)}ms` : ''}
      </span>
      <button
        type="button"
        class="inline-flex items-center gap-1 rounded px-1.5 py-1 text-xs"
        style="color: var(--ds-text-subtle);"
        onclick={exportCSV}
        data-testid="support-metrics-export"
      >
        <Download class="h-3.5 w-3.5" />
        {t('dashboard.states.supportExportCSV')}
      </button>
    </div>
  {/if}
</div>
