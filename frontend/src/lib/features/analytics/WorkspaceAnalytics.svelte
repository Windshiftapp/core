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
  import VelocityChart from './VelocityChart.svelte';
  import CumulativeFlowChart from './CumulativeFlowChart.svelte';
  import CycleTimeChart from './CycleTimeChart.svelte';
  import ForecastPanel from './ForecastPanel.svelte';

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
      return t('analytics.datasetBasis')
        .replace('{items}', dataset.total_items)
        .replace('{iterations}', dataset.iteration_count);
    }
    return t('analytics.datasetBasisNoIterations').replace('{items}', dataset.total_items);
  });

  const iterationNames = $derived(
    (dataset?.iterations || []).map(i => i.name).join(' → ')
  );
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
          <VelocityChart data={velocityData} />
        </Card>

        <Card variant="raised" padding="default" style="border-color: var(--ds-border);">
          {#snippet header()}
            <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('analytics.cumulativeFlow.title')}</h3>
          {/snippet}
          <CumulativeFlowChart data={cfdData} />
        </Card>

        <Card variant="raised" padding="default" style="border-color: var(--ds-border);">
          {#snippet header()}
            <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('analytics.cycleTime.title')}</h3>
          {/snippet}
          <CycleTimeChart data={cycleTimeData} />
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
