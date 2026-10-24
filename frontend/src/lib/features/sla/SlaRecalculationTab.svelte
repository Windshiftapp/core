<script>
  import { sla } from '../../api/sla.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Select from '../../components/Select.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Play from '@lucide/svelte/icons/play';
  import { t } from '../../stores/i18n.svelte.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';

  let { workspaceId = null } = $props();

  let jobs = $state([]);
  let metrics = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let triggering = $state(false);
  let targetMetricId = $state(0);

  const metricOptions = $derived([
    { value: 0, label: t('workspaceSettings.serviceLevels.allMetrics') },
    ...metrics.map((metric) => ({ value: metric.id, label: metric.name })),
  ]);

  function metricName(metricId) {
    return metrics.find((metric) => metric.id === metricId)?.name || `#${metricId}`;
  }

  function jobKindLabel(kind) {
    const key = {
      recalc_item: 'recalcItem',
      recalc_metric: 'recalcMetric',
      recalc_calendar: 'recalcCalendar',
    }[kind];
    return key ? t(`workspaceSettings.serviceLevels.jobKinds.${key}`) : kind;
  }

  async function loadJobs() {
    try {
      loading = true;
      error = null;
      jobs = (await sla.getRecalculations(workspaceId)) ?? [];
    } catch (err) {
      error = err?.message || t('workspaceSettings.serviceLevels.loadRecalculationsFailed');
    } finally {
      loading = false;
    }
  }

  async function loadMetrics() {
    metrics = (await sla.getMetrics(workspaceId).catch(() => [])) ?? [];
  }

  let loadedWorkspaceId = null;
  $effect(() => {
    if (!workspaceId || workspaceId === loadedWorkspaceId) return;
    loadedWorkspaceId = workspaceId;
    void Promise.all([loadJobs(), loadMetrics()]);
  });

  async function trigger() {
    triggering = true;
    try {
      const result = await sla.startRecalculation(workspaceId, targetMetricId);
      successToast(
        t('workspaceSettings.serviceLevels.recalculationStarted', {
          count: result?.enqueued ?? 0,
        })
      );
      await loadJobs();
    } catch (err) {
      errorToast(err?.message || t('workspaceSettings.serviceLevels.recalculationFailed'));
    } finally {
      triggering = false;
    }
  }

  const columns = $derived([
    { key: 'kind', label: t('workspaceSettings.serviceLevels.jobKind'), width: '140px', slot: 'kind' },
    { key: 'target', label: t('workspaceSettings.serviceLevels.jobTarget'), slot: 'target' },
    { key: 'attempts', label: t('workspaceSettings.serviceLevels.attempts'), width: '90px' },
    { key: 'state', label: t('workspaceSettings.serviceLevels.jobState'), width: '120px', slot: 'state' },
    { key: 'due_at', label: t('workspaceSettings.serviceLevels.dueAt'), width: '200px', slot: 'due_at' },
  ]);
</script>

<PageHeader
  icon={RefreshCw}
  title={t('workspaceSettings.serviceLevels.recalculationsTitle')}
  subtitle={t('workspaceSettings.serviceLevels.recalculationsSubtitle')}
>
  {#snippet actions()}
    <Button
      variant="default"
      icon={RefreshCw}
      onclick={loadJobs}
      disabled={loading}
      dataTestid="sla-recalc-refresh"
    >
      {t('common.refresh')}
    </Button>
    <Button
      variant="primary"
      icon={Play}
      onclick={trigger}
      disabled={triggering}
      dataTestid="sla-recalc-trigger"
    >
      {t('workspaceSettings.serviceLevels.startRecalculation')}
    </Button>
  {/snippet}
</PageHeader>

<div class="mb-4 max-w-sm">
  <label class="block text-sm mb-1" for="sla-recalc-metric">
    {t('workspaceSettings.serviceLevels.recalculationScope')}
  </label>
  <Select id="sla-recalc-metric" bind:value={targetMetricId} options={metricOptions} />
</div>

{#if error}
  <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-recalc-error">
    {error}
  </div>
{/if}

<DataTable
  columns={columns}
  data={jobs}
  keyField="id"
  {loading}
  emptyMessage={t('workspaceSettings.serviceLevels.noRecalculations')}
  emptyIcon={RefreshCw}
  rowAttrs={(job) => ({ 'data-testid': `sla-recalc-row-${job.id}` })}
>
  {#snippet kind(job)}
    <span class="text-xs" style="color: var(--ds-text-subtle)">{jobKindLabel(job.kind)}</span>
  {/snippet}
  {#snippet target(job)}
    {#if job.metric_id}
      <span>{metricName(job.metric_id)}</span>
    {:else if job.item_id}
      <span>#{job.item_id}</span>
    {/if}
  {/snippet}
  {#snippet state(job)}
    {#if job.state === 'failed'}
      <Lozenge color="red" text={t('workspaceSettings.serviceLevels.jobFailed')} />
    {:else}
      <Lozenge color="blue" text={t('workspaceSettings.serviceLevels.jobPending')} />
    {/if}
  {/snippet}
  {#snippet due_at(job)}
    <span class="text-xs" style="color: var(--ds-text-subtle)">{job.due_at}</span>
  {/snippet}
</DataTable>
