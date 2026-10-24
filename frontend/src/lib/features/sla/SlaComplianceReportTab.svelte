<script>
  import { sla } from '../../api/sla.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Input from '../../components/Input.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import BarChart3 from '@lucide/svelte/icons/bar-chart-3';
  import Download from '@lucide/svelte/icons/download';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDate } from '../../utils/dateFormatter.js';

  let { workspaceId = null } = $props();

  let report = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let from = $state('');
  let to = $state('');

  function formatDuration(ms) {
    if (ms == null) return '—';
    let remaining = Math.abs(ms);
    const days = Math.floor(remaining / 86400000);
    remaining -= days * 86400000;
    const hours = Math.floor(remaining / 3600000);
    remaining -= hours * 3600000;
    const minutes = Math.round(remaining / 60000);
    const parts = [];
    if (days) parts.push(`${days}d`);
    if (hours) parts.push(`${hours}h`);
    if (!days && minutes) parts.push(`${minutes}m`);
    return parts.join(' ') || '0m';
  }

  async function loadReport() {
    try {
      loading = true;
      error = null;
      report = await sla.getReport(workspaceId, { from, to });
    } catch (err) {
      error = err?.message || t('workspaceSettings.serviceLevels.reportLoadFailed');
    } finally {
      loading = false;
    }
  }

  let loadedWorkspaceId = null;
  $effect(() => {
    if (!workspaceId || workspaceId === loadedWorkspaceId) return;
    loadedWorkspaceId = workspaceId;
    void loadReport();
  });

  function exportCSV() {
    if (!report) return;
    const rows = [
      [
        t('workspaceSettings.serviceLevels.metric'),
        t('workspaceSettings.serviceLevels.ongoing'),
        t('workspaceSettings.serviceLevels.currentlyBreached'),
        t('workspaceSettings.serviceLevels.completed'),
        t('workspaceSettings.serviceLevels.breached'),
        t('workspaceSettings.serviceLevels.avgElapsed'),
      ],
    ];
    for (const metric of report.metrics ?? []) {
      rows.push([
        metric.metric_name,
        metric.ongoing,
        metric.currently_breached,
        metric.completed,
        metric.breached,
        metric.avg_elapsed_ms ?? '',
      ]);
    }
    const csv = rows
      .map((row) => row.map((value) => `"${String(value).replace(/"/g, '""')}"`).join(','))
      .join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `sla-report-${workspaceId}.csv`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  const metricColumns = $derived([
    { key: 'metric_name', label: t('workspaceSettings.serviceLevels.metric') },
    { key: 'ongoing', label: t('workspaceSettings.serviceLevels.ongoing'), width: '100px' },
    {
      key: 'currently_breached',
      label: t('workspaceSettings.serviceLevels.currentlyBreached'),
      width: '150px',
      slot: 'currently_breached',
    },
    { key: 'completed', label: t('workspaceSettings.serviceLevels.completed'), width: '110px' },
    { key: 'breached', label: t('workspaceSettings.serviceLevels.breached'), width: '110px', slot: 'breached' },
    { key: 'avg', label: t('workspaceSettings.serviceLevels.avgElapsed'), width: '130px', slot: 'avg' },
  ]);

  const breachColumns = $derived([
    { key: 'item_key', label: t('items.itemKey'), width: '120px' },
    { key: 'title', label: t('items.itemTitle') },
    { key: 'metric_name', label: t('workspaceSettings.serviceLevels.metric'), width: '160px' },
    { key: 'stopped_at', label: t('workspaceSettings.serviceLevels.stoppedAt'), width: '160px', slot: 'stopped_at' },
    { key: 'elapsed', label: t('workspaceSettings.serviceLevels.elapsed'), width: '110px', slot: 'elapsed' },
  ]);
</script>

<PageHeader
  icon={BarChart3}
  title={t('workspaceSettings.serviceLevels.reportTitle')}
  subtitle={t('workspaceSettings.serviceLevels.reportSubtitle')}
>
  {#snippet actions()}
    <Button
      variant="default"
      icon={Download}
      onclick={exportCSV}
      disabled={!report}
      dataTestid="sla-report-export"
    >
      {t('workspaceSettings.serviceLevels.exportCsv')}
    </Button>
    <Button
      variant="default"
      icon={RefreshCw}
      onclick={loadReport}
      disabled={loading}
      dataTestid="sla-report-refresh"
    >
      {t('common.refresh')}
    </Button>
  {/snippet}
</PageHeader>

<div class="flex flex-wrap items-end gap-3 mb-4">
  <div>
    <label class="block text-sm mb-1" for="sla-report-from">
      {t('workspaceSettings.serviceLevels.from')}
    </label>
    <Input id="sla-report-from" type="date" bind:value={from} dataTestid="sla-report-from" />
  </div>
  <div>
    <label class="block text-sm mb-1" for="sla-report-to">
      {t('workspaceSettings.serviceLevels.to')}
    </label>
    <Input id="sla-report-to" type="date" bind:value={to} dataTestid="sla-report-to" />
  </div>
  <Button variant="primary" onclick={loadReport} disabled={loading} dataTestid="sla-report-apply">
    {t('workspaceSettings.serviceLevels.apply')}
  </Button>
</div>

<p class="mb-3 text-xs" style="color: var(--ds-text-subtle)">
  {t('workspaceSettings.serviceLevels.currentStateNote')}
</p>

{#if error}
  <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-report-error">
    {error}
  </div>
{/if}

<DataTable
  columns={metricColumns}
  data={report?.metrics ?? []}
  keyField="metric_id"
  {loading}
  emptyMessage={t('workspaceSettings.serviceLevels.noReportData')}
  emptyIcon={BarChart3}
  rowAttrs={(metric) => ({ 'data-testid': `sla-report-metric-${metric.metric_id}` })}
>
  {#snippet currently_breached(metric)}
    {#if metric.currently_breached > 0}
      <Lozenge color="red" text={String(metric.currently_breached)} />
    {:else}
      <span>0</span>
    {/if}
  {/snippet}
  {#snippet breached(metric)}
    {#if metric.breached > 0}
      <Lozenge color="red" text={String(metric.breached)} />
    {:else}
      <span>0</span>
    {/if}
  {/snippet}
  {#snippet avg(metric)}
    <span>{formatDuration(metric.avg_elapsed_ms)} / {formatDuration(metric.avg_goal_ms)}</span>
  {/snippet}
</DataTable>

{#if report?.coverage && report.coverage.reference !== 'none'}
  <h4 class="mt-6 mb-2 text-sm font-semibold" style="color: var(--ds-text)">
    {t('workspaceSettings.serviceLevels.reportCoverage')}
  </h4>
  <div
    class="rounded border p-3 text-sm grid grid-cols-2 gap-2 md:grid-cols-4"
    style="border-color: var(--ds-border)"
    data-testid="sla-report-coverage"
  >
    <div>
      <div class="text-xs" style="color: var(--ds-text-subtle)">
        {t('workspaceSettings.serviceLevels.coverageReference')}
      </div>
      <div>{report.coverage.reference}</div>
    </div>
    <div>
      <div class="text-xs" style="color: var(--ds-text-subtle)">
        {t('workspaceSettings.serviceLevels.slaCounted')}
      </div>
      <div>{formatDuration(report.coverage.sla_counted_ms)}</div>
    </div>
    <div>
      <div class="text-xs" style="color: var(--ds-text-subtle)">
        {t('workspaceSettings.serviceLevels.teamService')}
      </div>
      <div>{formatDuration(report.coverage.team_service_ms)}</div>
    </div>
    <div>
      <div class="text-xs" style="color: var(--ds-text-subtle)">
        {t('workspaceSettings.serviceLevels.uncoveredTime')}
      </div>
      <div>{formatDuration(report.coverage.uncovered_ms)}</div>
    </div>
  </div>
  {#if report.coverage.current_state_reference}
    <p class="mt-1 text-xs" style="color: var(--ds-text-subtle)">
      {t('workspaceSettings.serviceLevels.currentStateReference')}
    </p>
  {/if}
{/if}

{#if (report?.breached_items ?? []).length > 0}
  <h4 class="mt-6 mb-2 text-sm font-semibold" style="color: var(--ds-text)">
    {t('workspaceSettings.serviceLevels.breachedItems')}
  </h4>
  <DataTable
    columns={breachColumns}
    data={report.breached_items}
    keyField="cycle_id"
    rowAttrs={(breach) => ({ 'data-testid': `sla-report-breach-${breach.cycle_id}` })}
  >
    {#snippet stopped_at(breach)}
      <span class="text-xs" style="color: var(--ds-text-subtle)">
        {breach.stopped_at ? formatDate(breach.stopped_at) : '—'}
      </span>
    {/snippet}
    {#snippet elapsed(breach)}
      <span>{formatDuration(breach.elapsed_ms)} / {formatDuration(breach.goal_duration_ms)}</span>
    {/snippet}
  </DataTable>
{/if}
