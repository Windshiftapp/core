<script>
  import { onMount } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import Text from '../../components/Text.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import Select from '../../components/Select.svelte';
  import Card from '../../components/Card.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Chart from '../../widgets/Chart.svelte';
  import ForecastPanel from './ForecastPanel.svelte';

  const fallbackColors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16'];
  const fmtD = (s) => { const d = new Date(s); return `${String(d.getMonth()+1).padStart(2,'0')}/${String(d.getDate()).padStart(2,'0')}`; };
  function formatHours(h) {
    if (h < 1) return `${Math.round(h * 60)}m`;
    if (h < 24) return `${h.toFixed(1)}h`;
    return `${(h / 24).toFixed(1)}d`;
  }

  let { workspaceId = null } = $props();

  let loading = $state(true);
  let analyticsData = $state(null);
  let collections = $state([]);
  let selectedCollection = $state('');

  // Date range (default: last 30 days)
  const today = new Date();
  const thirtyDaysAgo = new Date(today);
  thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);
  let endDate = $state(today.toISOString().split('T')[0]);
  let startDate = $state(thirtyDaysAgo.toISOString().split('T')[0]);

  const collectionOptions = $derived([
    { value: '', label: t('analytics.allItems') },
    ...collections.map(c => ({ value: String(c.id), label: c.name })),
  ]);

  async function fetchCollections() {
    if (!workspaceId) return;
    try {
      const data = await api.collections.getAll({ workspace_id: workspaceId });
      collections = data || [];
    } catch {
      collections = [];
    }
  }

  async function fetchAnalytics() {
    if (!workspaceId) return;
    loading = true;
    try {
      const params = { start_date: startDate, end_date: endDate };
      if (selectedCollection) {
        params.collection_id = selectedCollection;
      }
      analyticsData = await api.analytics.getAnalytics(workspaceId, params);
    } catch (err) {
      console.error('Failed to load analytics:', err);
      analyticsData = null;
    } finally {
      loading = false;
    }
  }

  onMount(async () => {
    await fetchCollections();
    await fetchAnalytics();
  });

  async function onFilterChange() {
    await fetchAnalytics();
  }

  // Derived data sections
  const dataset = $derived(analyticsData?.dataset || null);
  const velocityData = $derived(analyticsData?.velocity || null);
  const cfdData = $derived(analyticsData?.cumulative_flow || null);
  const cycleTimeData = $derived(analyticsData?.cycle_time || null);
  const forecastData = $derived(analyticsData?.forecast || null);

  // Data basis text
  const dataBasisText = $derived.by(() => {
    if (!dataset) return '';
    if (dataset.iteration_count > 0) {
      return t('analytics.datasetBasis', {
        count: dataset.iteration_count,
        items: dataset.total_items,
        iterations: dataset.iteration_count,
      });
    }
    return t('analytics.datasetBasisNoIterations', { items: dataset.total_items });
  });

  const iterationNames = $derived(
    (dataset?.iterations || []).map(i => i.name).join(' → ')
  );

  // Velocity chart transforms
  const velIters = $derived(velocityData?.iterations || []);
  const velCategories = $derived(velIters.map(i => i.name));
  const velSeries = $derived([{
    key: 'completed', label: t('analytics.velocity.completed'), color: '#3b82f6',
    values: velIters.map(i => i.completed_count)
  }]);
  const velRefLines = $derived(
    velocityData?.averages?.avg_count > 0
      ? [{ value: velocityData.averages.avg_count, color: '#f59e0b', label: velocityData.averages.avg_count.toFixed(1), dashed: true }]
      : []
  );

  // CFD chart transforms
  const cfdCats = $derived(cfdData?.categories || []);
  const cfdPoints = $derived(cfdData?.data_points || []);
  const cfdCategories = $derived(cfdPoints.map(d => fmtD(d.date)));
  const cfdSeries = $derived(cfdCats.map((cat, i) => ({
    key: cat.name, label: cat.name,
    color: cat.color || fallbackColors[i % fallbackColors.length],
    values: cfdPoints.map(dp => (dp.counts || {})[cat.name] || 0)
  })));

  // Cycle time chart transforms
  const ctStages = $derived(cycleTimeData?.stages || []);
  const ctTotal = $derived(cycleTimeData?.total_cycle_time || {});
  const ctAnalyzed = $derived(cycleTimeData?.total_items_analyzed || 0);
  const ctCategories = $derived(ctStages.map(s => s.name));
  const ctSeries = $derived([
    { key: 'p85', label: 'P85', color: '#06b6d4', values: ctStages.map(s => s.p85_hours || 0), opacity: 0.3 },
    { key: 'avg', label: t('analytics.cycleTime.avgTotal'), color: '#3b82f6', values: ctStages.map(s => s.avg_hours || 0) }
  ]);
</script>

<div class="analytics-page min-h-screen" style="background-color: var(--ds-surface);">
  <div class="p-6">
    <PageHeader title={t('analytics.title')}>
      {#snippet actions()}
        <div class="flex items-center gap-3 flex-wrap">
          <Label size="xs" color="subtle">{t('analytics.collection')}</Label>
          <Select
            options={collectionOptions}
            value={selectedCollection}
            onchange={(v) => { selectedCollection = v; onFilterChange(); }}
            size="small"
            class="w-48"
          />
          <div style="width: 1px; height: 24px; background: var(--ds-border);" class="mx-1"></div>
          <Label size="xs" color="subtle">{t('analytics.dateRange')}</Label>
          <div class="w-40">
            <Input type="date" size="small" bind:value={startDate} onchange={onFilterChange} />
          </div>
          <span style="color: var(--ds-text-subtle);">—</span>
          <div class="w-40">
            <Input type="date" size="small" bind:value={endDate} onchange={onFilterChange} />
          </div>
        </div>
      {/snippet}
    </PageHeader>

    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Text variant="subtle" size="sm">{t('analytics.loading')}</Text>
      </div>
    {:else}
      <!-- Data basis banner -->
      {#if dataset}
        <div class="mb-6 p-4 rounded-lg" style="background: var(--ds-surface-sunken); border: 1px solid var(--ds-border);">
          <div class="flex items-center gap-2 mb-1">
            <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{dataBasisText}</Text>
          </div>
          {#if iterationNames}
            <Text variant="subtle" size="xs">{iterationNames}</Text>
          {/if}
        </div>
      {/if}

      <!-- Charts grid -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card variant="raised" padding="default" style="border-color: var(--ds-border);">
          {#snippet header()}
            <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('analytics.velocity.title')}</h3>
          {/snippet}
          {#if velocityData && velIters.length > 0}
            <div class="flex items-center gap-4 text-xs mb-3" style="color: var(--ds-text-subtle);">
              <div class="flex items-center gap-1.5">
                <span class="inline-block w-3 h-3 rounded-sm" style="background: #3b82f6;"></span>
                {t('analytics.velocity.completed')}
              </div>
              <div class="flex items-center gap-1.5">
                <span class="inline-block w-3 h-0.5" style="background: #f59e0b;"></span>
                {t('analytics.velocity.average')}
              </div>
            </div>
            <Chart type="bar" series={velSeries} categories={velCategories} referenceLines={velRefLines} maxXLabels={8}>
              {#snippet tooltipContent({ index, category })}
                {@const iter = velIters[index]}
                <div style="font-weight:600;">{category}</div>
                <div style="color:var(--ds-text-subtle);font-size:0.7rem;">{iter.completed_count} items &middot; {iter.completed_points ?? 0} pts</div>
                <div style="color:var(--ds-text-subtle);font-size:0.7rem;">{iter.total_count} total</div>
              {/snippet}
            </Chart>
          {:else}
            <div class="flex flex-col items-center justify-center py-10 gap-2" style="color: var(--ds-text-subtle);">
              {#if velocityData?.data_quality?.reason}
                <Text variant="subtle" size="sm">{t('analytics.insufficientData.' + velocityData.data_quality.reason)}</Text>
              {:else}
                <Text variant="subtle" size="sm">{t('analytics.noData')}</Text>
              {/if}
            </div>
          {/if}
        </Card>

        <Card variant="raised" padding="default" style="border-color: var(--ds-border);">
          {#snippet header()}
            <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('analytics.cumulativeFlow.title')}</h3>
          {/snippet}
          {#if cfdData && cfdPoints.length > 0}
            <Chart type="stacked-area" series={cfdSeries} categories={cfdCategories} />
          {:else}
            <div class="flex flex-col items-center justify-center py-10 gap-2" style="color: var(--ds-text-subtle);">
              {#if cfdData?.data_quality?.reason}
                <Text variant="subtle" size="sm">{t('analytics.insufficientData.' + cfdData.data_quality.reason)}</Text>
              {:else}
                <Text variant="subtle" size="sm">{t('analytics.noData')}</Text>
              {/if}
            </div>
          {/if}
        </Card>

        <Card variant="raised" padding="default" style="border-color: var(--ds-border);">
          {#snippet header()}
            <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('analytics.cycleTime.title')}</h3>
          {/snippet}
          {#if cycleTimeData && ctStages.length > 0}
            <Text variant="subtle" size="xs" class="mb-3">{ctAnalyzed} {t('analytics.cycleTime.itemsAnalyzed')}</Text>
            <!-- Summary card -->
            <div class="flex gap-4 mb-4 p-3 rounded-lg" style="background: var(--ds-surface-sunken);">
              <div class="text-center">
                <Text variant="subtle" size="xs">{t('analytics.cycleTime.avgTotal')}</Text>
                <div class="text-lg font-semibold" style="color: var(--ds-text);">{formatHours(ctTotal.avg_hours || 0)}</div>
              </div>
              <div class="text-center">
                <Text variant="subtle" size="xs">{t('analytics.cycleTime.median')}</Text>
                <div class="text-lg font-semibold" style="color: var(--ds-text);">{formatHours(ctTotal.median_hours || 0)}</div>
              </div>
              <div class="text-center">
                <Text variant="subtle" size="xs">{t('analytics.cycleTime.p85')}</Text>
                <div class="text-lg font-semibold" style="color: var(--ds-text);">{formatHours(ctTotal.p85_hours || 0)}</div>
              </div>
            </div>
            <Chart type="horizontal-bar" series={ctSeries} categories={ctCategories} valueFormat={formatHours}>
              {#snippet tooltipContent({ index, category })}
                {@const stage = ctStages[index]}
                <div style="font-weight:600;">{category}</div>
                <div style="color:var(--ds-text-subtle);font-size:0.7rem;">avg {formatHours(stage.avg_hours)} &middot; med {formatHours(stage.median_hours)}</div>
                <div style="color:var(--ds-text-subtle);font-size:0.7rem;">p85 {formatHours(stage.p85_hours)}</div>
              {/snippet}
            </Chart>
          {:else}
            <div class="flex flex-col items-center justify-center py-10 gap-2" style="color: var(--ds-text-subtle);">
              {#if cycleTimeData?.data_quality?.reason}
                <Text variant="subtle" size="sm">{t('analytics.insufficientData.' + cycleTimeData.data_quality.reason)}</Text>
              {:else}
                <Text variant="subtle" size="sm">{t('analytics.noData')}</Text>
              {/if}
            </div>
          {/if}
        </Card>

        <Card variant="raised" padding="default" style="border-color: var(--ds-border);">
          {#snippet header()}
            <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('analytics.forecast.title')}</h3>
          {/snippet}
          <ForecastPanel data={forecastData} />
        </Card>
      </div>
    {/if}
  </div>
</div>
